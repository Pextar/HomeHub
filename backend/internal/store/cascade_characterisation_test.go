package store

import "testing"

// Characterisation tests for the cascades.
//
// CascadeDeleteSocket is the widest of them: it has to know about every
// field anywhere in the store that can hold a socket id. The project's own
// notes flag it as something that must be kept in step by hand whenever a
// new reference is added, which is exactly the kind of rule that rots.
// These tests pin every reference it currently clears, so a reference index
// that replaces the hand-written list can be shown to cover the same ground.

func cascadeStore(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir(), noopRF{})
	s.Sockets["sk"] = &Socket{ID: "sk", Name: "Doomed", Code: "1:1", Protocol: "nexa"}
	s.Sockets["keep"] = &Socket{ID: "keep", Name: "Keeper", Code: "1:2", Protocol: "nexa"}
	return s
}

func TestCascadeDeleteSocketClearsEveryReference(t *testing.T) {
	s := cascadeStore(t)

	s.Schedules["hit"] = &Schedule{ID: "hit", TargetType: "socket", TargetID: "sk", Action: "on"}
	s.Schedules["miss"] = &Schedule{ID: "miss", TargetType: "socket", TargetID: "keep", Action: "on"}
	// Same id, different target type — the match must key off both.
	s.Schedules["othertype"] = &Schedule{ID: "othertype", TargetType: "group", TargetID: "sk", Action: "on"}

	s.Timers["hit"] = &Timer{ID: "hit", TargetType: "socket", TargetID: "sk", Action: "off"}
	s.Timers["miss"] = &Timer{ID: "miss", TargetType: "socket", TargetID: "keep", Action: "off"}

	s.Groups["g"] = &Group{ID: "g", Name: "G", SocketIDs: []string{"sk", "keep"}}
	s.Users["u"] = &User{ID: "u", Username: "kid", SocketIDs: []string{"sk", "keep"}}
	s.Scenes["c"] = &Scene{ID: "c", Name: "C", Steps: []SceneStep{{
		DelayMinutes: 0,
		Actions:      []SceneAction{{SocketID: "sk", Action: "on"}, {SocketID: "keep", Action: "on"}},
	}}}

	delete(s.Sockets, "sk")
	s.CascadeDeleteSocket("sk")

	if _, ok := s.Schedules["hit"]; ok {
		t.Error("a schedule targeting the socket survived")
	}
	if _, ok := s.Schedules["miss"]; !ok {
		t.Error("a schedule targeting another socket was deleted")
	}
	if _, ok := s.Schedules["othertype"]; !ok {
		t.Error("a group schedule sharing the id was deleted")
	}
	if _, ok := s.Timers["hit"]; ok {
		t.Error("a timer targeting the socket survived")
	}
	if _, ok := s.Timers["miss"]; !ok {
		t.Error("a timer targeting another socket was deleted")
	}
	if got := s.Groups["g"].SocketIDs; len(got) != 1 || got[0] != "keep" {
		t.Errorf("group members = %v, want just keep", got)
	}
	if got := s.Users["u"].SocketIDs; len(got) != 1 || got[0] != "keep" {
		t.Errorf("user sockets = %v, want just keep", got)
	}
	if got := s.Scenes["c"].Steps[0].Actions; len(got) != 1 || got[0].SocketID != "keep" {
		t.Errorf("scene actions = %+v, want just keep", got)
	}
}

func TestCascadeDeleteSocketPrunesAutomations(t *testing.T) {
	t.Run("a rule triggered by the socket is dropped", func(t *testing.T) {
		s := cascadeStore(t)
		s.Automations["a"] = &Automation{ID: "a", Name: "A", Rules: []AutomationRule{
			{
				Trigger: AutomationTrigger{Type: "device", SocketID: "sk"},
				Actions: []AutomationAction{{TargetType: "socket", TargetID: "keep", Action: "on"}},
			},
			{
				Trigger: AutomationTrigger{Type: "device", SocketID: "keep"},
				Actions: []AutomationAction{{TargetType: "socket", TargetID: "keep", Action: "on"}},
			},
		}}
		s.CascadeDeleteSocket("sk")

		// The rule can never fire again, so it goes even though its action
		// pointed somewhere still alive.
		if got := s.Automations["a"].Rules; len(got) != 1 {
			t.Fatalf("rules = %d, want 1", len(got))
		}
		if s.Automations["a"].Rules[0].Trigger.SocketID != "keep" {
			t.Error("the wrong rule was kept")
		}
	})

	t.Run("a condition on the socket is dropped but the rule survives", func(t *testing.T) {
		s := cascadeStore(t)
		s.Automations["a"] = &Automation{ID: "a", Name: "A", Rules: []AutomationRule{{
			Trigger: AutomationTrigger{Type: "time", Time: "07:00"},
			Conditions: []AutomationCondition{
				{Type: "device", SocketID: "sk"},
				{Type: "device", SocketID: "keep"},
			},
			Actions: []AutomationAction{{TargetType: "socket", TargetID: "keep", Action: "on"}},
		}}}
		s.CascadeDeleteSocket("sk")

		rules := s.Automations["a"].Rules
		if len(rules) != 1 {
			t.Fatalf("rules = %d, want the rule kept", len(rules))
		}
		if got := rules[0].Conditions; len(got) != 1 || got[0].SocketID != "keep" {
			t.Errorf("conditions = %+v, want only the surviving socket", got)
		}
	})

	// A rule with nothing left to do is removed, and an automation with no
	// rules left goes with it.
	t.Run("an automation left with no actions is deleted outright", func(t *testing.T) {
		s := cascadeStore(t)
		s.Automations["a"] = &Automation{ID: "a", Name: "A", Rules: []AutomationRule{{
			Trigger: AutomationTrigger{Type: "time", Time: "07:00"},
			Actions: []AutomationAction{{TargetType: "socket", TargetID: "sk", Action: "on"}},
		}}}
		s.CascadeDeleteSocket("sk")

		if _, ok := s.Automations["a"]; ok {
			t.Error("an automation with no remaining actions survived")
		}
	})
}

func TestCascadeDeleteRoom(t *testing.T) {
	s := cascadeStore(t)
	s.Rooms["rm"] = &Room{ID: "rm", Name: "Lounge"}
	s.Schedules["hit"] = &Schedule{ID: "hit", TargetType: "room", TargetID: "rm", Action: "on"}
	s.Schedules["miss"] = &Schedule{ID: "miss", TargetType: "socket", TargetID: "rm", Action: "on"}
	s.Timers["hit"] = &Timer{ID: "hit", TargetType: "room", TargetID: "rm", Action: "off"}
	s.Automations["a"] = &Automation{ID: "a", Name: "A", Rules: []AutomationRule{{
		Trigger: AutomationTrigger{Type: "time", Time: "07:00"},
		Actions: []AutomationAction{{TargetType: "room", TargetID: "rm", Action: "on"}},
	}}}

	s.CascadeDeleteRoom("rm")

	if _, ok := s.Schedules["hit"]; ok {
		t.Error("a room schedule survived")
	}
	if _, ok := s.Schedules["miss"]; !ok {
		t.Error("a socket schedule sharing the id was deleted")
	}
	if _, ok := s.Timers["hit"]; ok {
		t.Error("a room timer survived")
	}
	if _, ok := s.Automations["a"]; ok {
		t.Error("an automation left with no room actions survived")
	}
}

func TestPruneAutomationsForSensor(t *testing.T) {
	s := cascadeStore(t)
	s.Automations["a"] = &Automation{ID: "a", Name: "A", Rules: []AutomationRule{
		{
			Trigger: AutomationTrigger{Type: "sensor", SensorID: "sn"},
			Actions: []AutomationAction{{TargetType: "socket", TargetID: "keep", Action: "on"}},
		},
		{
			Trigger: AutomationTrigger{Type: "time", Time: "07:00"},
			Actions: []AutomationAction{{TargetType: "socket", TargetID: "keep", Action: "on"}},
		},
	}}
	s.PruneAutomationsForSensor("sn")

	if got := s.Automations["a"].Rules; len(got) != 1 || got[0].Trigger.Type != "time" {
		t.Errorf("rules = %+v, want only the time rule", got)
	}
}

// A scene created by the scene wizard owns its automations, so deleting the
// scene deletes them — distinct from merely pruning references to it.
func TestDeleteAutomationsOwnedByScene(t *testing.T) {
	s := cascadeStore(t)
	s.Automations["owned"] = &Automation{ID: "owned", Name: "From wizard", SceneID: "ce"}
	s.Automations["other"] = &Automation{ID: "other", Name: "Other", SceneID: "different"}
	s.Automations["free"] = &Automation{ID: "free", Name: "Standalone"}

	s.DeleteAutomationsOwnedByScene("ce")

	if _, ok := s.Automations["owned"]; ok {
		t.Error("the scene's own automation survived")
	}
	if _, ok := s.Automations["other"]; !ok {
		t.Error("another scene's automation was deleted")
	}
	if _, ok := s.Automations["free"]; !ok {
		t.Error("a standalone automation was deleted")
	}
}

func TestCascadeDeleteSpeakerLeavesOtherZoneMembers(t *testing.T) {
	s := cascadeStore(t)
	s.Sonos["a"] = &SonosSpeaker{ID: "a", Name: "A", IP: "192.168.1.10"}
	s.KEF["b"] = &KEFSpeaker{ID: "b", Name: "B", IP: "192.168.1.20"}
	s.Zones["z"] = &Zone{ID: "z", Name: "Both", Members: []string{
		QualifySonos("a"), QualifyKEF("b"),
	}}

	s.CascadeDeleteSpeaker(QualifySonos("a"))

	got := s.Zones["z"].Members
	if len(got) != 1 || got[0] != QualifyKEF("b") {
		t.Errorf("zone members = %v, want only the KEF", got)
	}
}
