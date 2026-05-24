package store

import "testing"

func newAutoStore(autos map[string]*Automation) *Store {
	return &Store{Automations: autos}
}

func TestPruneAutomationsForSocket(t *testing.T) {
	s := newAutoStore(map[string]*Automation{
		// Triggered by the socket's state — must be removed entirely.
		"trig": {ID: "trig", Trigger: AutomationTrigger{Type: "device", SocketID: "sock"},
			Actions: []AutomationAction{{TargetType: "group", TargetID: "g1", Action: "on"}}},
		// Uses the socket in a condition + one of two actions — kept, pruned.
		"mixed": {ID: "mixed", Trigger: AutomationTrigger{Type: "time", Time: "07:00"},
			Conditions: []AutomationCondition{{Type: "device", SocketID: "sock", State: "on"}},
			Actions: []AutomationAction{
				{TargetType: "socket", TargetID: "sock", Action: "off"},
				{TargetType: "scene", TargetID: "sc1", Action: "activate"},
			}},
		// Only action targets the socket — left with none, must be removed.
		"only": {ID: "only", Trigger: AutomationTrigger{Type: "time", Time: "08:00"},
			Actions: []AutomationAction{{TargetType: "socket", TargetID: "sock", Action: "on"}}},
		// Unrelated — must survive untouched.
		"keep": {ID: "keep", Trigger: AutomationTrigger{Type: "time", Time: "09:00"},
			Actions: []AutomationAction{{TargetType: "group", TargetID: "g2", Action: "on"}}},
	})

	s.pruneAutomationsForSocket("sock")

	if _, ok := s.Automations["trig"]; ok {
		t.Error("automation triggered by deleted socket should be removed")
	}
	if _, ok := s.Automations["only"]; ok {
		t.Error("automation left with no actions should be removed")
	}
	mixed, ok := s.Automations["mixed"]
	if !ok {
		t.Fatal("mixed automation should survive")
	}
	if len(mixed.Conditions) != 0 {
		t.Errorf("device condition on deleted socket should be dropped, got %d", len(mixed.Conditions))
	}
	if len(mixed.Actions) != 1 || mixed.Actions[0].TargetType != "scene" {
		t.Errorf("socket action should be dropped, leaving the scene action; got %+v", mixed.Actions)
	}
	if _, ok := s.Automations["keep"]; !ok {
		t.Error("unrelated automation should survive")
	}
}

func TestPruneAutomationsForSensorAndTarget(t *testing.T) {
	s := newAutoStore(map[string]*Automation{
		"sensor": {ID: "sensor", Trigger: AutomationTrigger{Type: "sensor", SensorID: "temp", Op: "below", Value: 18},
			Actions: []AutomationAction{{TargetType: "socket", TargetID: "s1", Action: "on"}}},
		"group": {ID: "group", Trigger: AutomationTrigger{Type: "time", Time: "07:00"},
			Actions: []AutomationAction{{TargetType: "group", TargetID: "g1", Action: "off"}}},
	})

	s.PruneAutomationsForSensor("temp")
	if _, ok := s.Automations["sensor"]; ok {
		t.Error("automation triggered by deleted sensor should be removed")
	}

	s.PruneAutomationsForTarget("group", "g1")
	if _, ok := s.Automations["group"]; ok {
		t.Error("automation left with no actions after group delete should be removed")
	}
}
