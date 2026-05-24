package store

import "fmt"

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
		var firstErr error
		for _, a := range scene.Actions {
			if err := s.ExecuteAction("socket", a.SocketID, a.Action); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr

	default:
		return fmt.Errorf("unsupported target type %q", targetType)
	}
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
		out := sc.Actions[:0]
		for _, a := range sc.Actions {
			if a.SocketID != socketID {
				out = append(out, a)
			}
		}
		sc.Actions = out
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
