package store

import "testing"

type noopRF struct{}

func (noopRF) Send(code, protocol string, state bool) error { return nil }

type recLight struct {
	socketID string
	level    *int
	color    string
	calls    int
}

func (r *recLight) SetLight(socket Socket, level *int, color string) error {
	r.socketID = socket.ID
	r.level = level
	r.color = color
	r.calls++
	return nil
}

func TestSceneAppliesLevelAndColorToSmartLight(t *testing.T) {
	lvl := 30
	light := &recLight{}
	s := &Store{
		Sockets: map[string]*Socket{
			"lamp": {ID: "lamp", Protocol: "tasmota", Code: "1.2.3.4"},
			"plug": {ID: "plug", Protocol: "nexa", Code: "1:0"},
		},
		Scenes: map[string]*Scene{
			"sc": {ID: "sc", Actions: []SceneAction{
				{SocketID: "lamp", Action: "on", Level: &lvl, Color: "f5bd6e"},
				{SocketID: "plug", Action: "on"}, // RF, no level/colour
			}},
		},
		RF:    noopRF{},
		Light: light,
	}

	if err := s.ExecuteAction("scene", "sc", "activate"); err != nil {
		t.Fatalf("activate scene: %v", err)
	}
	if light.calls != 0 {
		t.Fatalf("SetLight should be deferred to FlushLights, but ran %d times during execution", light.calls)
	}
	s.FlushLights()

	if light.calls != 1 {
		t.Fatalf("expected SetLight once (smart light only), got %d", light.calls)
	}
	if light.socketID != "lamp" {
		t.Errorf("SetLight target = %q, want lamp", light.socketID)
	}
	if light.level == nil || *light.level != 30 {
		t.Errorf("SetLight level = %v, want 30", light.level)
	}
	if light.color != "f5bd6e" {
		t.Errorf("SetLight color = %q, want f5bd6e", light.color)
	}
	if !s.Sockets["plug"].State || !s.Sockets["lamp"].State {
		t.Error("both sockets should be switched on by the scene")
	}
}
