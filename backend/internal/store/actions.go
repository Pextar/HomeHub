package store

import (
	"fmt"
	"time"
)

// ApplyState changes a single socket's state and fires the RF command.
// Caller must hold Mu (write lock). On RF failure the previous state is
// restored. Save is intentionally NOT called here — callers batch.
func (s *Store) ApplyState(socket *Socket, target *bool) error {
	previous := socket.State
	if target == nil {
		socket.State = !socket.State
	} else {
		socket.State = *target
	}
	if err := s.RF.Send(socket.Code, socket.Protocol, socket.State); err != nil {
		socket.State = previous
		return err
	}
	if socket.State != previous {
		if s.OnChange != nil {
			s.OnChange()
		}
		if s.OnStateChange != nil && !s.SuppressStateChange {
			s.OnStateChange(*socket, socket.State)
		}
	}
	return nil
}

// ExecuteAction runs the given action against the given target. Caller
// must hold Mu (write lock). Per-target failures are returned but Save
// is NOT called — callers batch.
func (s *Store) ExecuteAction(targetType, targetID, action string) error {
	switch targetType {
	case "socket":
		socket, ok := s.Sockets[targetID]
		if !ok {
			return fmt.Errorf("socket %q no longer exists", targetID)
		}
		var target *bool
		switch action {
		case "on":
			t := true
			target = &t
		case "off":
			t := false
			target = &t
		case "toggle":
			target = nil
		default:
			return fmt.Errorf("unsupported socket action %q", action)
		}
		return s.ApplyState(socket, target)

	case "group":
		group, ok := s.Groups[targetID]
		if !ok {
			return fmt.Errorf("group %q no longer exists", targetID)
		}
		var firstErr error
		for _, sid := range group.SocketIDs {
			if err := s.ExecuteAction("socket", sid, action); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr

	case "scene":
		scene, ok := s.Scenes[targetID]
		if !ok {
			return fmt.Errorf("scene %q no longer exists", targetID)
		}
		// Inline migration: scenes that were never run through Load/ValidateScene
		// may still carry the legacy flat Actions slice (e.g. in tests).
		if len(scene.Steps) == 0 && len(scene.Actions) > 0 {
			scene.Steps = []SceneStep{{DelayMinutes: 0, Actions: scene.Actions}}
			scene.Actions = nil
		}
		if len(scene.Steps) == 0 {
			return nil
		}
		// Execute the first step immediately (caller holds Mu).
		firstErr := s.execStepLocked(scene.Steps[0])
		// Schedule any remaining steps to fire after their delay.
		for _, step := range scene.Steps[1:] {
			s.ScheduleStep(step)
		}
		return firstErr

	default:
		return fmt.Errorf("unsupported target type %q", targetType)
	}
}

// QueueLight buffers a smart-light brightness/colour command to be applied by
// FlushLights once the lock is released. Caller must hold Mu (write lock).
func (s *Store) QueueLight(socket Socket, level *int, color string) {
	s.pendingLights = append(s.pendingLights, lightCmd{socket: socket, level: level, color: color})
}

// FlushLights drains queued smart-light commands and sends them to the bridge.
// It briefly takes Mu to swap the buffer, then makes the (network) bridge calls
// WITHOUT the lock held. Caller must NOT hold Mu. Safe to call when empty.
func (s *Store) FlushLights() {
	s.Mu.Lock()
	cmds := s.pendingLights
	s.pendingLights = nil
	light := s.Light
	s.Mu.Unlock()
	if light == nil {
		return
	}
	for _, c := range cmds {
		_ = light.SetLight(c.socket, c.level, c.color)
	}
}

// execStepLocked executes all actions in a single SceneStep.
// Caller must hold Mu (write lock). Smart-light brightness/colour
// commands are queued via QueueLight; drain them with FlushLights
// after releasing the lock.
func (s *Store) execStepLocked(step SceneStep) error {
	var firstErr error
	for _, a := range step.Actions {
		if err := s.ExecuteAction("socket", a.SocketID, a.Action); err != nil && firstErr == nil {
			firstErr = err
		}
		// After switching a smart light on, queue its brightness/colour.
		// The actual bridge call happens in FlushLights, off-lock.
		if a.Action == "on" && (a.Level != nil || a.Color != "") {
			if sock, ok := s.Sockets[a.SocketID]; ok {
				s.QueueLight(*sock, a.Level, a.Color)
			}
		}
	}
	return firstErr
}

// ScheduleStep launches a goroutine that waits for step.DelayMinutes
// and then acquires the lock, executes the step, saves, and flushes
// smart-light commands. Fire-and-forget: errors are silently ignored
// (matching scheduler/automation behaviour for scene steps).
// Caller may or may not hold Mu — the goroutine acquires it when it wakes up.
func (s *Store) ScheduleStep(step SceneStep) {
	delay := time.Duration(step.DelayMinutes) * time.Minute
	time.AfterFunc(delay, func() {
		s.Mu.Lock()
		_ = s.execStepLocked(step)
		_ = s.Save()
		s.Mu.Unlock()
		s.FlushLights()
	})
}

// CascadeDeleteSocket removes a socket from every group/scene and
// deletes schedules and timers that target it directly. Caller must
// hold Mu.
func (s *Store) CascadeDeleteSocket(socketID string) {
	for sid, sch := range s.Schedules {
		if sch.TargetType == "socket" && sch.TargetID == socketID {
			delete(s.Schedules, sid)
		}
	}
	for tid, t := range s.Timers {
		if t.TargetType == "socket" && t.TargetID == socketID {
			delete(s.Timers, tid)
		}
	}
	for _, g := range s.Groups {
		g.SocketIDs = filterStrings(g.SocketIDs, socketID)
	}
	for _, sc := range s.Scenes {
		for i := range sc.Steps {
			out := sc.Steps[i].Actions[:0]
			for _, a := range sc.Steps[i].Actions {
				if a.SocketID != socketID {
					out = append(out, a)
				}
			}
			sc.Steps[i].Actions = out
		}
	}
	for _, u := range s.Users {
		u.SocketIDs = filterStrings(u.SocketIDs, socketID)
	}
	s.pruneAutomationsForSocket(socketID)
}

// pruneAutomationsForSocket cleans automations that reference a deleted
// socket. An automation triggered by the socket's state can never fire again,
// so it is removed; otherwise device conditions and socket actions pointing at
// it are dropped, and the automation is removed if no actions remain. Caller
// must hold Mu.
func (s *Store) pruneAutomationsForSocket(socketID string) {
	for id, a := range s.Automations {
		if a.Trigger.Type == "device" && a.Trigger.SocketID == socketID {
			delete(s.Automations, id)
			continue
		}
		conds := a.Conditions[:0]
		for _, c := range a.Conditions {
			if c.Type == "device" && c.SocketID == socketID {
				continue
			}
			conds = append(conds, c)
		}
		a.Conditions = conds
		a.Actions = filterActions(a.Actions, "socket", socketID)
		if len(a.Actions) == 0 {
			delete(s.Automations, id)
		}
	}
}

// PruneAutomationsForSensor removes automations whose trigger watches a
// deleted sensor (sensors are only referenced by triggers). Caller holds Mu.
func (s *Store) PruneAutomationsForSensor(sensorID string) {
	for id, a := range s.Automations {
		if a.Trigger.Type == "sensor" && a.Trigger.SensorID == sensorID {
			delete(s.Automations, id)
		}
	}
}

// PruneAutomationsForTarget drops actions that target a deleted group or
// scene, removing the automation if it is left with no actions. Caller holds Mu.
func (s *Store) PruneAutomationsForTarget(targetType, targetID string) {
	for id, a := range s.Automations {
		a.Actions = filterActions(a.Actions, targetType, targetID)
		if len(a.Actions) == 0 {
			delete(s.Automations, id)
		}
	}
}

func filterActions(in []AutomationAction, targetType, targetID string) []AutomationAction {
	out := in[:0]
	for _, act := range in {
		if act.TargetType == targetType && act.TargetID == targetID {
			continue
		}
		out = append(out, act)
	}
	return out
}

func filterStrings(in []string, drop string) []string {
	out := in[:0]
	for _, v := range in {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}
