package api

import (
	"strings"
	"testing"

	"rf-socket-controller/internal/store"
)

// noopRF is a transmitter that always succeeds, so tool executors run the full
// staged/apply path without real hardware.
type noopRF struct{}

func (noopRF) Send(code, protocol string, state bool) error { return nil }

func assistantTestServer(t *testing.T) *Server {
	t.Helper()
	st := store.New(t.TempDir(), noopRF{})
	st.Sockets["lamp"] = &store.Socket{ID: "lamp", Name: "Kitchen Lamp", Code: "1:0", Protocol: "nexa", Room: "Kitchen", State: false}
	st.Sockets["fan"] = &store.Socket{ID: "fan", Name: "Bedroom Fan", Code: "2:0", Protocol: "nexa", Room: "Bedroom", State: true}
	st.Rooms["r1"] = &store.Room{ID: "r1", Name: "Kitchen"}
	st.Rooms["r2"] = &store.Room{ID: "r2", Name: "Bedroom"}
	st.Groups["g1"] = &store.Group{ID: "g1", Name: "Lights", SocketIDs: []string{"lamp", "fan"}}
	st.Sensors["temp"] = &store.Sensor{ID: "temp", Name: "Outdoor Temp", Kind: "temperature", Unit: "C"}
	return &Server{Store: st, SessionSecret: []byte("test-secret-32-bytes-long-padxxx")}
}

func TestCreateAutomationTool(t *testing.T) {
	s := assistantTestServer(t)
	tools := s.assistantTools()

	if !tools["create_automation"].NeedsConfirm {
		t.Fatalf("create_automation must require confirmation")
	}

	// A sunset rule, gated by a device condition, acting on a room — names
	// throughout, resolved server-side to ids.
	args := map[string]any{
		"name": "Evening lights",
		"rules": []any{
			map[string]any{
				"trigger": map[string]any{"type": "time", "time_mode": "sunset", "solar_offset_minutes": -15},
				"conditions": []any{
					map[string]any{"type": "device", "device": "kitchen lamp", "state": "off"},
				},
				"actions": []any{
					map[string]any{"target_type": "room", "target": "bedroom", "action": "on"},
				},
			},
		},
	}
	res := tools["create_automation"].Execute(nil, args)
	if !strings.Contains(res, "\"created\":\"Evening lights\"") || !strings.Contains(res, "\"enabled\":true") {
		t.Fatalf("create result = %q", res)
	}
	if len(s.Store.Automations) != 1 {
		t.Fatalf("expected exactly one automation, got %d", len(s.Store.Automations))
	}
	var got *store.Automation
	for _, a := range s.Store.Automations {
		got = a
	}
	if !got.Enabled {
		t.Fatalf("new automation should be enabled")
	}
	act := got.Rules[0].Actions[0]
	if act.TargetType != "room" || act.TargetID != "r2" {
		t.Fatalf("room action should resolve to room id r2, got %+v", act)
	}
	if got.Rules[0].Conditions[0].SocketID != "lamp" {
		t.Fatalf("condition device should resolve to socket id lamp, got %+v", got.Rules[0].Conditions[0])
	}
	if got.Rules[0].Trigger.TimeMode != "sunset" || got.Rules[0].Trigger.SolarOffsetMinutes != -15 {
		t.Fatalf("trigger not preserved: %+v", got.Rules[0].Trigger)
	}
}

func TestCreateAutomationUnknownTarget(t *testing.T) {
	s := assistantTestServer(t)
	args := map[string]any{
		"name": "Bad",
		"rules": []any{
			map[string]any{
				"trigger": map[string]any{"type": "time", "time": "07:00"},
				"actions": []any{map[string]any{"target_type": "device", "target": "ghost lamp", "action": "on"}},
			},
		},
	}
	res := s.assistantTools()["create_automation"].Execute(nil, args)
	if !strings.Contains(res, "no device named") {
		t.Fatalf("unknown target result = %q, want a resolution reason", res)
	}
	if len(s.Store.Automations) != 0 {
		t.Fatalf("nothing should be saved on a resolution failure")
	}
}

func TestCreateAutomationSensorTrigger(t *testing.T) {
	s := assistantTestServer(t)
	args := map[string]any{
		"name": "Cool down",
		"rules": []any{
			map[string]any{
				"trigger": map[string]any{"type": "sensor", "sensor": "outdoor temp", "op": "above", "value": 25.0},
				"actions": []any{map[string]any{"target_type": "group", "target": "Lights", "action": "off"}},
			},
		},
	}
	res := s.assistantTools()["create_automation"].Execute(nil, args)
	if !strings.Contains(res, "\"created\":\"Cool down\"") {
		t.Fatalf("create result = %q", res)
	}
	var got *store.Automation
	for _, a := range s.Store.Automations {
		got = a
	}
	tr := got.Rules[0].Trigger
	if tr.Type != "sensor" || tr.SensorID != "temp" || tr.Op != "above" || tr.Value != 25 {
		t.Fatalf("sensor trigger not resolved: %+v", tr)
	}
}

func TestSummarizeAutomation(t *testing.T) {
	args := map[string]any{
		"name": "Evening lights",
		"rules": []any{
			map[string]any{
				"trigger": map[string]any{"type": "time", "time_mode": "sunset"},
				"actions": []any{map[string]any{"target_type": "room", "target": "Bedroom", "action": "on"}},
			},
		},
	}
	summary, affected := summarizeAutomation(args)
	if !strings.Contains(summary, "Evening lights") || !strings.Contains(summary, "at sunset") || !strings.Contains(summary, "turn on Bedroom") {
		t.Fatalf("summary = %q", summary)
	}
	if len(affected) != 1 || affected[0] != "Bedroom" {
		t.Fatalf("affected = %v", affected)
	}
}

func TestControlDeviceTool(t *testing.T) {
	s := assistantTestServer(t)
	tools := s.assistantTools()

	// Resolve by name (case-insensitive) and turn on.
	res := tools["control_device"].Execute(nil, map[string]any{"device": "kitchen lamp", "action": "on"})
	if !strings.Contains(res, "\"state\":\"on\"") {
		t.Fatalf("control_device result = %q, want state on", res)
	}
	if !s.Store.Sockets["lamp"].State {
		t.Fatalf("lamp should be on after tool call")
	}

	// Unknown device returns a reason, not a panic, and mutates nothing.
	res = tools["control_device"].Execute(nil, map[string]any{"device": "ghost", "action": "off"})
	if !strings.Contains(res, "no device named") {
		t.Fatalf("unknown device result = %q, want a 'no device named' reason", res)
	}
}

func TestControlDeviceUnsupportedAction(t *testing.T) {
	s := assistantTestServer(t)
	res := s.assistantTools()["control_device"].Execute(nil, map[string]any{"device": "kitchen lamp", "action": "explode"})
	if !strings.Contains(res, "unsupported action") {
		t.Fatalf("result = %q, want unsupported action", res)
	}
	if s.Store.Sockets["lamp"].State {
		t.Fatalf("lamp must stay off on an invalid action")
	}
}

func TestBulkToolNeedsConfirm(t *testing.T) {
	s := assistantTestServer(t)
	tools := s.assistantTools()
	for _, name := range []string{"all_devices", "control_room", "control_group"} {
		if !tools[name].NeedsConfirm {
			t.Errorf("%s should require confirmation", name)
		}
	}
	for _, name := range []string{"control_device", "activate_scene", "get_state", "get_sensor_readings"} {
		if tools[name].NeedsConfirm {
			t.Errorf("%s should not require confirmation", name)
		}
	}
}

func TestConfirmationTokenRoundTrip(t *testing.T) {
	s := assistantTestServer(t)
	user := &store.User{ID: "u1", Admin: true}

	token, err := s.signConfirmation(pendingAction{Tool: "all_devices", Args: map[string]any{"action": "off"}, UserID: "u1"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := s.verifyConfirmation(token, user)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Tool != "all_devices" || got.Args["action"] != "off" {
		t.Fatalf("decoded action = %+v", got)
	}

	// A different user can't redeem it.
	if _, err := s.verifyConfirmation(token, &store.User{ID: "u2", Admin: true}); err == nil {
		t.Fatalf("expected cross-user rejection")
	}

	// Tampering breaks the signature.
	if _, err := s.verifyConfirmation(token+"x", user); err == nil {
		t.Fatalf("expected tampered token rejection")
	}
}

func TestRoomControlAppliesToRoom(t *testing.T) {
	s := assistantTestServer(t)
	// Turn the Bedroom off (fan starts on); Kitchen lamp must be untouched.
	res := s.assistantTools()["control_room"].Execute(nil, map[string]any{"room": "bedroom", "action": "off"})
	if !strings.Contains(res, "\"room\":\"Bedroom\"") {
		t.Fatalf("control_room result = %q", res)
	}
	if s.Store.Sockets["fan"].State {
		t.Fatalf("bedroom fan should be off")
	}
}
