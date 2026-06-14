package store

import "testing"

func newAutoStore(autos map[string]*Automation) *Store {
	return &Store{Automations: autos}
}

// rule wraps a single trigger/conditions/actions into a one-element rule list.
func rule(t AutomationTrigger, conds []AutomationCondition, acts []AutomationAction) []AutomationRule {
	return []AutomationRule{{Trigger: t, Conditions: conds, Actions: acts}}
}

func TestPruneAutomationsForSocket(t *testing.T) {
	s := newAutoStore(map[string]*Automation{
		// Triggered by the socket's state — must be removed entirely.
		"trig": {ID: "trig", Rules: rule(
			AutomationTrigger{Type: "device", SocketID: "sock"}, nil,
			[]AutomationAction{{TargetType: "group", TargetID: "g1", Action: "on"}})},
		// Uses the socket in a condition + one of two actions — kept, pruned.
		"mixed": {ID: "mixed", Rules: rule(
			AutomationTrigger{Type: "time", Time: "07:00"},
			[]AutomationCondition{{Type: "device", SocketID: "sock", State: "on"}},
			[]AutomationAction{
				{TargetType: "socket", TargetID: "sock", Action: "off"},
				{TargetType: "scene", TargetID: "sc1", Action: "activate"},
			})},
		// Only action targets the socket — left with none, must be removed.
		"only": {ID: "only", Rules: rule(
			AutomationTrigger{Type: "time", Time: "08:00"}, nil,
			[]AutomationAction{{TargetType: "socket", TargetID: "sock", Action: "on"}})},
		// Unrelated — must survive untouched.
		"keep": {ID: "keep", Rules: rule(
			AutomationTrigger{Type: "time", Time: "09:00"}, nil,
			[]AutomationAction{{TargetType: "group", TargetID: "g2", Action: "on"}})},
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
	if len(mixed.Rules) != 1 {
		t.Fatalf("mixed should keep its single rule, got %d", len(mixed.Rules))
	}
	if len(mixed.Rules[0].Conditions) != 0 {
		t.Errorf("device condition on deleted socket should be dropped, got %d", len(mixed.Rules[0].Conditions))
	}
	if len(mixed.Rules[0].Actions) != 1 || mixed.Rules[0].Actions[0].TargetType != "scene" {
		t.Errorf("socket action should be dropped, leaving the scene action; got %+v", mixed.Rules[0].Actions)
	}
	if _, ok := s.Automations["keep"]; !ok {
		t.Error("unrelated automation should survive")
	}
}

func TestPruneAutomationsForSensorAndTarget(t *testing.T) {
	s := newAutoStore(map[string]*Automation{
		"sensor": {ID: "sensor", Rules: rule(
			AutomationTrigger{Type: "sensor", SensorID: "temp", Op: "below", Value: 18}, nil,
			[]AutomationAction{{TargetType: "socket", TargetID: "s1", Action: "on"}})},
		"group": {ID: "group", Rules: rule(
			AutomationTrigger{Type: "time", Time: "07:00"}, nil,
			[]AutomationAction{{TargetType: "group", TargetID: "g1", Action: "off"}})},
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
