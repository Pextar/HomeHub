package store

import "testing"

// (recRF, with its calls counter, is defined in staged_test.go.)

// A "set" action on a smart socket applies brightness/colour via the light
// bridge without transmitting an on/off command or changing tracked state.
func TestSetActionAppliesLightWithoutToggling(t *testing.T) {
	lvl := 40
	rf := &recRF{}
	light := &recLight{}
	s := &Store{
		Sockets: map[string]*Socket{
			"lamp": {ID: "lamp", Protocol: "tasmota", Code: "1.2.3.4", State: true},
		},
		RF:    rf,
		Light: light,
	}

	actions := []AutomationAction{
		{TargetType: "socket", TargetID: "lamp", Action: "set", Level: &lvl, Color: "c4a4e0"},
	}

	s.Mu.Lock()
	staged := s.StageAutomationActions(actions)
	s.Mu.Unlock()
	s.SendStaged(staged)
	s.Mu.Lock()
	if err := s.ApplyStaged(staged); err != nil {
		s.Mu.Unlock()
		t.Fatalf("apply staged: %v", err)
	}
	s.Mu.Unlock()

	if rf.calls != 0 {
		t.Fatalf("a set action must not transmit on/off, got %d sends", rf.calls)
	}
	if light.calls != 0 {
		t.Fatalf("SetLight should be deferred to FlushLights, ran %d times early", light.calls)
	}
	s.FlushLights()

	if light.calls != 1 {
		t.Fatalf("expected SetLight once, got %d", light.calls)
	}
	if light.level == nil || *light.level != 40 {
		t.Errorf("SetLight level = %v, want 40", light.level)
	}
	if light.color != "c4a4e0" {
		t.Errorf("SetLight color = %q, want c4a4e0", light.color)
	}
	if !s.Sockets["lamp"].State {
		t.Error("set action must leave the socket's tracked state unchanged (still on)")
	}
}

// A "set" targeting a group fans the brightness/colour out to smart members
// only, still without any on/off transmission.
func TestSetActionOnGroupFansOutLightOnly(t *testing.T) {
	lvl := 60
	rf := &recRF{}
	light := &recLight{}
	s := &Store{
		Sockets: map[string]*Socket{
			"lamp": {ID: "lamp", Protocol: "tasmota", Code: "1.2.3.4", State: false},
			"plug": {ID: "plug", Protocol: "nexa", Code: "1:0", State: false},
		},
		Groups: map[string]*Group{
			"g": {ID: "g", SocketIDs: []string{"lamp", "plug"}},
		},
		RF:    rf,
		Light: light,
	}

	actions := []AutomationAction{
		{TargetType: "group", TargetID: "g", Action: "set", Level: &lvl},
	}

	s.Mu.Lock()
	staged := s.StageAutomationActions(actions)
	s.Mu.Unlock()
	s.SendStaged(staged)
	s.Mu.Lock()
	_ = s.ApplyStaged(staged)
	s.Mu.Unlock()
	s.FlushLights()

	if rf.calls != 0 {
		t.Fatalf("set on a group must not transmit on/off, got %d sends", rf.calls)
	}
	// QueueLight fans the brightness/colour out to the members; the real
	// bridge no-ops non-smart protocols, but a command is queued regardless.
	if light.calls == 0 {
		t.Error("expected the group's light command to be queued for its members")
	}
	if s.Sockets["lamp"].State || s.Sockets["plug"].State {
		t.Error("set must not switch any member on")
	}
}
