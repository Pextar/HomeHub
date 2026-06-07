package store

import "testing"

func newSceneTestStore() *Store {
	return &Store{
		Sockets: map[string]*Socket{
			"lamp": {ID: "lamp", Protocol: "nexa", Code: "1:0"},
		},
		Rooms: map[string]*Room{
			"r1": {ID: "r1", Name: "Kitchen"},
		},
		RF: noopRF{},
	}
}

func TestValidateSceneIconAndColor(t *testing.T) {
	s := newSceneTestStore()

	// Valid icon + accent are accepted; colour is normalised to lowercase.
	sc := &Scene{Name: "Evening", Icon: "moon", Color: "AMBER"}
	if err := s.ValidateScene(sc); err != nil {
		t.Fatalf("valid scene rejected: %v", err)
	}
	if sc.Color != "amber" {
		t.Errorf("color not normalised: got %q want amber", sc.Color)
	}

	// Unknown accent key is rejected.
	if err := s.ValidateScene(&Scene{Name: "X", Color: "chartreuse"}); err == nil {
		t.Error("expected unknown color to be rejected")
	}

	// Junk icon (non-alphanumeric) is rejected.
	if err := s.ValidateScene(&Scene{Name: "X", Icon: "moon!"}); err == nil {
		t.Error("expected junk icon to be rejected")
	}

	// Empty icon/colour stay optional.
	if err := s.ValidateScene(&Scene{Name: "Plain"}); err != nil {
		t.Errorf("plain scene rejected: %v", err)
	}
}

func TestExecuteActionRoomTarget(t *testing.T) {
	s := &Store{
		Sockets: map[string]*Socket{
			"a": {ID: "a", Protocol: "nexa", Code: "1:0", Room: "Kitchen"},
			"b": {ID: "b", Protocol: "nexa", Code: "1:1", Room: "Kitchen"},
			"c": {ID: "c", Protocol: "nexa", Code: "1:2", Room: "Bedroom"},
		},
		Rooms: map[string]*Room{"r1": {ID: "r1", Name: "Kitchen"}},
		RF:    noopRF{},
	}

	if err := s.ExecuteAction("room", "r1", "on"); err != nil {
		t.Fatalf("room action: %v", err)
	}
	if !s.Sockets["a"].State || !s.Sockets["b"].State {
		t.Error("kitchen sockets should be on")
	}
	if s.Sockets["c"].State {
		t.Error("bedroom socket should be untouched by a kitchen room action")
	}
}
