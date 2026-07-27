package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"homehub/internal/store"
)

// Further characterisation of the create/update paths that the first file
// left uncovered: sockets and scenes. Both have quirks that a shared CRUD
// helper would have to preserve or deliberately change.

// ---------------------------------------------------------------- sockets

func TestCreateSocketCharacterisation(t *testing.T) {
	t.Run("a valid nexa socket is created", func(t *testing.T) {
		srv := testServer(t)
		rec := do(t, srv, http.MethodPost, "/api/sockets",
			`{"name":"Lamp","code":"12345:1","protocol":"nexa","room":"Lounge"}`)
		mustStatus(t, rec, http.StatusCreated)

		var got store.Socket
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !strings.HasPrefix(got.ID, "socket_") {
			t.Errorf("generated id = %q, want a socket_ prefix", got.ID)
		}
	})

	// Unlike every other create handler, createSocket validates *before*
	// taking the store lock. The observable effect is the same, but the
	// ordering matters to anything that moves validation inside a
	// transaction helper.
	t.Run("an invalid nexa code is rejected", func(t *testing.T) {
		srv := testServer(t)
		rec := do(t, srv, http.MethodPost, "/api/sockets",
			`{"name":"Lamp","code":"not-a-code","protocol":"nexa"}`)
		mustStatus(t, rec, http.StatusBadRequest)
		if len(srv.Store.Sockets) != 0 {
			t.Error("an invalid socket was stored")
		}
	})

	t.Run("name and code are required", func(t *testing.T) {
		srv := testServer(t)
		mustStatus(t, do(t, srv, http.MethodPost, "/api/sockets",
			`{"code":"12345:1","protocol":"nexa"}`), http.StatusBadRequest)
		mustStatus(t, do(t, srv, http.MethodPost, "/api/sockets",
			`{"name":"Lamp","protocol":"nexa"}`), http.StatusBadRequest)
	})

	t.Run("a client-supplied duplicate id is a conflict", func(t *testing.T) {
		srv := testServer(t)
		srv.Store.Sockets["socket_1"] = &store.Socket{ID: "socket_1", Name: "Existing", Code: "1:1"}
		rec := do(t, srv, http.MethodPost, "/api/sockets",
			`{"id":"socket_1","name":"Lamp","code":"12345:1","protocol":"nexa"}`)
		mustStatus(t, rec, http.StatusConflict)
		if srv.Store.Sockets["socket_1"].Name != "Existing" {
			t.Error("the existing socket was overwritten")
		}
	})

	// A matter node id must be numeric so it can't smuggle path segments
	// into the bridge URL.
	t.Run("a matter node id must be numeric", func(t *testing.T) {
		srv := testServer(t)
		mustStatus(t, do(t, srv, http.MethodPost, "/api/sockets",
			`{"name":"Bulb","code":"../evil","protocol":"matter"}`), http.StatusBadRequest)
		mustStatus(t, do(t, srv, http.MethodPost, "/api/sockets",
			`{"name":"Bulb","code":"42","protocol":"matter"}`), http.StatusCreated)
	})
}

// ----------------------------------------------------------------- scenes

func sceneBody(t *testing.T) string {
	t.Helper()
	return `{"name":"Movie","room":"Lounge","icon":"film","color":"amber",
	         "steps":[{"delay_minutes":0,"actions":[{"socket_id":"s1","action":"on"}]}]}`
}

func TestCreateSceneCharacterisation(t *testing.T) {
	t.Run("a scene with steps is created", func(t *testing.T) {
		srv, _ := actionServer(t)
		rec := do(t, srv, http.MethodPost, "/api/scenes", sceneBody(t))
		mustStatus(t, rec, http.StatusCreated)

		var got store.Scene
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !strings.HasPrefix(got.ID, "scene_") {
			t.Errorf("generated id = %q", got.ID)
		}
		if len(got.Steps) != 1 || len(got.Steps[0].Actions) != 1 {
			t.Errorf("steps = %+v", got.Steps)
		}
	})

	t.Run("a scene referencing an unknown socket is rejected", func(t *testing.T) {
		srv, _ := actionServer(t)
		rec := do(t, srv, http.MethodPost, "/api/scenes",
			`{"name":"Bad","steps":[{"delay_minutes":0,"actions":[{"socket_id":"nope","action":"on"}]}]}`)
		mustStatus(t, rec, http.StatusBadRequest)
	})

	t.Run("a client-supplied duplicate id is a conflict", func(t *testing.T) {
		srv, _ := actionServer(t)
		srv.Store.Scenes["scene_1"] = &store.Scene{ID: "scene_1", Name: "Existing"}
		rec := do(t, srv, http.MethodPost, "/api/scenes",
			`{"id":"scene_1","name":"Movie","steps":[{"delay_minutes":0,"actions":[{"socket_id":"s1","action":"on"}]}]}`)
		mustStatus(t, rec, http.StatusConflict)
	})
}

func TestUpdateSceneCharacterisation(t *testing.T) {
	newSrv := func(t *testing.T) *Server {
		srv, _ := actionServer(t)
		srv.Store.Scenes["scene_1"] = &store.Scene{
			ID: "scene_1", Name: "Movie", Room: "Lounge", Icon: "film", Color: "amber",
			Steps: []store.SceneStep{{
				DelayMinutes: 0,
				Actions:      []store.SceneAction{{SocketID: "s1", Action: "on"}},
			}},
		}
		return srv
	}

	t.Run("an empty name is ignored but room/icon/colour are overwritten", func(t *testing.T) {
		// Asymmetry worth pinning: name uses "empty means leave alone", while
		// room, icon and colour are assigned unconditionally, so omitting
		// them clears them.
		srv := newSrv(t)
		mustStatus(t, do(t, srv, http.MethodPut, "/api/scenes/scene_1", `{"name":"  "}`), http.StatusOK)

		got := srv.Store.Scenes["scene_1"]
		if got.Name != "Movie" {
			t.Errorf("name = %q, want it preserved", got.Name)
		}
		if got.Room != "" || got.Icon != "" || got.Color != "" {
			t.Errorf("room/icon/color = %q/%q/%q, want all cleared by omission",
				got.Room, got.Icon, got.Color)
		}
	})

	t.Run("omitted steps are preserved", func(t *testing.T) {
		srv := newSrv(t)
		mustStatus(t, do(t, srv, http.MethodPut, "/api/scenes/scene_1", `{"name":"Renamed"}`), http.StatusOK)
		if got := srv.Store.Scenes["scene_1"].Steps; len(got) != 1 {
			t.Errorf("steps = %+v, want them preserved when omitted", got)
		}
	})

	t.Run("supplying steps clears the legacy actions field", func(t *testing.T) {
		srv := newSrv(t)
		srv.Store.Scenes["scene_1"].Actions = []store.SceneAction{{SocketID: "s1", Action: "on"}}
		mustStatus(t, do(t, srv, http.MethodPut, "/api/scenes/scene_1",
			`{"steps":[{"delay_minutes":5,"actions":[{"socket_id":"s2","action":"off"}]}]}`), http.StatusOK)

		got := srv.Store.Scenes["scene_1"]
		if got.Actions != nil {
			t.Errorf("legacy actions = %+v, want them cleared", got.Actions)
		}
		if len(got.Steps) != 1 || got.Steps[0].DelayMinutes != 5 {
			t.Errorf("steps = %+v", got.Steps)
		}
	})

	t.Run("an unknown id is a not-found", func(t *testing.T) {
		srv := newSrv(t)
		mustStatus(t, do(t, srv, http.MethodPut, "/api/scenes/nope", `{"name":"X"}`), http.StatusNotFound)
	})
}
