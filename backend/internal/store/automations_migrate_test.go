package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAutomationLegacyJSONMigratesToSingleRule(t *testing.T) {
	legacy := `{
		"id":"a1","name":"old","enabled":true,
		"trigger":{"type":"time","time":"07:00"},
		"conditions":[{"type":"device","socket_id":"s1","state":"on"}],
		"actions":[{"target_type":"socket","target_id":"s1","action":"on"}]
	}`

	var a Automation
	if err := json.Unmarshal([]byte(legacy), &a); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if len(a.Rules) != 1 {
		t.Fatalf("expected legacy automation to fold into 1 rule, got %d", len(a.Rules))
	}
	r := a.Rules[0]
	if r.Trigger.Type != "time" || r.Trigger.Time != "07:00" {
		t.Errorf("trigger not migrated: %+v", r.Trigger)
	}
	if len(r.Conditions) != 1 || r.Conditions[0].SocketID != "s1" {
		t.Errorf("conditions not migrated: %+v", r.Conditions)
	}
	if len(r.Actions) != 1 || r.Actions[0].TargetID != "s1" {
		t.Errorf("actions not migrated: %+v", r.Actions)
	}

	// Re-marshalling must use the new shape only (no top-level trigger/actions).
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"trigger"`) && !strings.Contains(string(b), `"rules"`) {
		t.Errorf("re-marshalled automation should be in rules shape: %s", b)
	}
	var a2 Automation
	if err := json.Unmarshal(b, &a2); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if len(a2.Rules) != 1 || a2.Rules[0].Trigger.Time != "07:00" {
		t.Errorf("round-trip lost rule data: %+v", a2.Rules)
	}
}

func TestAutomationMultiRuleJSONRoundTrips(t *testing.T) {
	src := `{"id":"a2","name":"dusk","enabled":true,"rules":[
		{"trigger":{"type":"time","time_mode":"sunset"},"actions":[{"target_type":"socket","target_id":"s1","action":"on"}]},
		{"trigger":{"type":"time","time":"23:00"},"actions":[{"target_type":"socket","target_id":"s1","action":"off"}]}
	]}`
	var a Automation
	if err := json.Unmarshal([]byte(src), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(a.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(a.Rules))
	}
	if a.Rules[0].Actions[0].Action != "on" || a.Rules[1].Actions[0].Action != "off" {
		t.Errorf("rule actions wrong: %+v", a.Rules)
	}
}
