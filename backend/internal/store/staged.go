package store

import (
	"fmt"
	"strings"
	"sync"
)

// Staged sends split a multi-socket operation into three phases so device I/O
// never runs while Mu is held:
//
//	st.Mu.Lock()
//	staged, err := st.StageAction(tt, tid, action)   // resolve, no I/O
//	st.Mu.Unlock()
//	st.SendStaged(staged)                            // device I/O, off-lock
//	st.Mu.Lock()
//	firstErr := st.ApplyStaged(staged)               // fold results back in
//	... activity entry, Save() ...
//	st.Mu.Unlock()
//	st.FlushLights()
//
// Single-socket operations keep using ApplyState, which transmits
// synchronously and reports the device error in the same HTTP response.

// StagedSend is one resolved on/off transmission. Entries whose target socket
// was missing at stage time carry a pre-set Err and are skipped by SendStaged,
// so per-socket failures flow through the same reporting as send failures.
type StagedSend struct {
	SocketID string
	Name     string
	Code     string
	Protocol string
	State    bool // desired state
	Err      error
}

// isNetworkProtocol mirrors the routing in sender.Multi: these protocols go
// out over the network and tolerate concurrent requests; everything else is a
// 433 MHz transmission that must not overlap on air. Keep in sync with
// sender.Multi.Send.
func isNetworkProtocol(p string) bool {
	switch {
	case strings.EqualFold(p, "tasmota"),
		strings.EqualFold(p, "matter"),
		strings.EqualFold(p, "matter-thread"),
		strings.EqualFold(p, "mqtt"):
		return true
	}
	return false
}

// Transmit sends one on/off command to a device. 433 MHz transmissions are
// serialized via txMu so concurrent sends (e.g. a manual toggle racing a
// scheduled group action) cannot overlap on air; network protocols pass
// straight through.
func (s *Store) Transmit(code, protocol string, state bool) error {
	if !isNetworkProtocol(protocol) {
		s.txMu.Lock()
		defer s.txMu.Unlock()
	}
	return s.RF.Send(code, protocol, state)
}

// StageAction resolves an action against a socket/group/room/scene target into
// the flat list of device sends it implies, performing no I/O. For scenes this
// also queues smart-light brightness/colour (QueueLight) and schedules delayed
// steps, mirroring ExecuteAction's scene branch. Caller must hold Mu (write
// lock).
func (s *Store) StageAction(targetType, targetID, action string) ([]StagedSend, error) {
	switch targetType {
	case "socket":
		ss := s.stageSocket(targetID, action)
		if ss.Err != nil {
			return nil, ss.Err
		}
		return []StagedSend{ss}, nil

	case "group":
		group, ok := s.Groups[targetID]
		if !ok {
			return nil, fmt.Errorf("group %q no longer exists", targetID)
		}
		out := make([]StagedSend, 0, len(group.SocketIDs))
		for _, sid := range group.SocketIDs {
			out = append(out, s.stageSocket(sid, action))
		}
		return out, nil

	case "room":
		room, ok := s.Rooms[targetID]
		if !ok {
			return nil, fmt.Errorf("room %q no longer exists", targetID)
		}
		var out []StagedSend
		for _, sock := range s.Sockets {
			if sock.Room == room.Name {
				out = append(out, s.stageSocket(sock.ID, action))
			}
		}
		return out, nil

	case "scene":
		scene, ok := s.Scenes[targetID]
		if !ok {
			return nil, fmt.Errorf("scene %q no longer exists", targetID)
		}
		// Inline migration, matching ExecuteAction: scenes never run through
		// Load/ValidateScene may still carry the legacy flat Actions slice.
		if len(scene.Steps) == 0 && len(scene.Actions) > 0 {
			scene.Steps = []SceneStep{{DelayMinutes: 0, Actions: scene.Actions}}
			scene.Actions = nil
		}
		if len(scene.Steps) == 0 {
			return nil, nil
		}
		out := s.stageStep(scene.Steps[0])
		for _, step := range scene.Steps[1:] {
			s.ScheduleStep(step)
		}
		return out, nil

	default:
		return nil, fmt.Errorf("unsupported target type %q", targetType)
	}
}

// StageSocketSend resolves a single socket action into a staged send for
// callers that build their own fan-out list (bulk/room handlers). Failures
// (missing socket, bad action) are reported via the entry's Err. Caller must
// hold Mu.
func (s *Store) StageSocketSend(socketID, action string) StagedSend {
	return s.stageSocket(socketID, action)
}

// stageSocket resolves a single socket action. A missing socket or unsupported
// action is reported via the entry's Err rather than dropped, so group/scene
// fan-outs keep their per-socket failure reporting. Caller must hold Mu.
func (s *Store) stageSocket(socketID, action string) StagedSend {
	sock, ok := s.Sockets[socketID]
	if !ok {
		return StagedSend{SocketID: socketID, Err: fmt.Errorf("socket %q no longer exists", socketID)}
	}
	var desired bool
	switch action {
	case "on":
		desired = true
	case "off":
		desired = false
	case "toggle":
		desired = !sock.State
	default:
		return StagedSend{SocketID: socketID, Name: sock.Name,
			Err: fmt.Errorf("unsupported socket action %q", action)}
	}
	return StagedSend{
		SocketID: sock.ID,
		Name:     sock.Name,
		Code:     sock.Code,
		Protocol: sock.Protocol,
		State:    desired,
	}
}

// stageStep stages all actions in a single SceneStep and queues smart-light
// brightness/colour for lights being switched on (drained later by
// FlushLights). Mirrors execStepLocked — keep the two in sync. Caller must
// hold Mu.
func (s *Store) stageStep(step SceneStep) []StagedSend {
	out := make([]StagedSend, 0, len(step.Actions))
	for _, a := range step.Actions {
		out = append(out, s.stageSocket(a.SocketID, a.Action))
		if a.Action == "on" && (a.Level != nil || a.Color != "") {
			if sock, ok := s.Sockets[a.SocketID]; ok {
				s.QueueLight(*sock, a.Level, a.Color)
			}
		}
	}
	return out
}

// StageAutomationActions stages every action of an automation, including the
// smart-light brightness/colour fan-out for socket/group/room targets being
// switched on. A missing target becomes a single errored entry so callers
// still surface "no longer exists" in their failure reporting. Caller must
// hold Mu (write lock).
func (s *Store) StageAutomationActions(actions []AutomationAction) []StagedSend {
	var out []StagedSend
	for _, act := range actions {
		staged, err := s.StageAction(act.TargetType, act.TargetID, act.Action)
		if err != nil {
			out = append(out, StagedSend{Err: err})
			continue
		}
		out = append(out, staged...)
		if act.Action == "on" && (act.Level != nil || act.Color != "") {
			switch act.TargetType {
			case "socket":
				if sock, ok := s.Sockets[act.TargetID]; ok {
					s.QueueLight(*sock, act.Level, act.Color)
				}
			case "group":
				if grp, ok := s.Groups[act.TargetID]; ok {
					for _, sid := range grp.SocketIDs {
						if sock, ok := s.Sockets[sid]; ok {
							s.QueueLight(*sock, act.Level, act.Color)
						}
					}
				}
			case "room":
				if rm, ok := s.Rooms[act.TargetID]; ok {
					for _, sock := range s.Sockets {
						if sock.Room == rm.Name {
							s.QueueLight(*sock, act.Level, act.Color)
						}
					}
				}
			}
		}
	}
	return out
}

// SendStaged transmits every staged command, recording per-entry errors.
// Network protocols go out in parallel (one slow Tasmota/Matter device no
// longer delays the rest); 433 MHz sends run sequentially since they share
// the transmitter and the air. Entries with a pre-set Err are skipped.
// Caller must NOT hold Mu.
func (s *Store) SendStaged(staged []StagedSend) {
	var wg sync.WaitGroup
	for i := range staged {
		c := &staged[i]
		if c.Err != nil || !isNetworkProtocol(c.Protocol) {
			continue
		}
		wg.Add(1)
		go func(c *StagedSend) {
			defer wg.Done()
			c.Err = s.Transmit(c.Code, c.Protocol, c.State)
		}(c)
	}
	for i := range staged {
		c := &staged[i]
		// Protocol check first: network entries are owned by their goroutine
		// above until wg.Wait(), so their Err must not be read here.
		if isNetworkProtocol(c.Protocol) || c.Err != nil {
			continue
		}
		c.Err = s.Transmit(c.Code, c.Protocol, c.State)
	}
	wg.Wait()
}

// ApplyStaged folds successful sends back into socket state, firing OnChange
// and (unless SuppressStateChange) OnStateChange exactly as ApplyState would.
// Sockets deleted while the sends were in flight are skipped. Returns the
// first error among the staged entries. Save is intentionally NOT called —
// callers batch, like with ApplyState. Caller must hold Mu (write lock).
func (s *Store) ApplyStaged(staged []StagedSend) error {
	var firstErr error
	changed := false
	for i := range staged {
		c := &staged[i]
		if c.Err != nil {
			if firstErr == nil {
				firstErr = c.Err
			}
			continue
		}
		sock, ok := s.Sockets[c.SocketID]
		if !ok {
			continue
		}
		if sock.State != c.State {
			sock.State = c.State
			changed = true
			if s.OnStateChange != nil && !s.SuppressStateChange {
				s.OnStateChange(*sock, sock.State)
			}
		}
	}
	if changed && s.OnChange != nil {
		s.OnChange()
	}
	return firstErr
}
