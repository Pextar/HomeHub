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
	return &Server{Store: st, SessionSecret: []byte("test-secret-32-bytes-long-padxxx")}
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
