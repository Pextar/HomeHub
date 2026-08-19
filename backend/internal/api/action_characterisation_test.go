package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"

	"homehub/internal/store"
)

// Characterisation tests for the multi-device action endpoints — group
// on/off, room on/off, scene activate and the bulk all-on/all-off.
//
// These all share the staged flow (StageAction under Mu → SendStaged off
// the lock → ApplyStaged under Mu → Save), which is the part of the API
// most likely to be reshaped and the part where getting it wrong means
// either a deadlock or device I/O under the store lock. What is pinned
// here is the observable contract: the response shape, which sockets end
// up switched, how a device failure is reported, and the single summary
// entry left in the activity log.

// recordRF is an RFSender that records what was transmitted and can be told
// to fail for a specific code, so partial-failure handling is testable
// without a radio.
type recordRF struct {
	mu       sync.Mutex
	sent     []string
	failCode string
}

func (r *recordRF) Send(code, protocol string, state bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if code == r.failCode {
		return errors.New("transmit failed")
	}
	r.sent = append(r.sent, code)
	return nil
}

func (r *recordRF) sentCodes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}

// actionServer builds a server with three RF sockets, a group holding two of
// them and a room containing the same two.
func actionServer(t *testing.T) (*Server, *recordRF) {
	t.Helper()
	rf := &recordRF{}
	st := store.New(t.TempDir(), rf)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	st.Sockets["s1"] = &store.Socket{ID: "s1", Name: "Lamp", Code: "1001", Protocol: "nexa", Room: "Lounge"}
	st.Sockets["s2"] = &store.Socket{ID: "s2", Name: "Fan", Code: "1002", Protocol: "nexa", Room: "Lounge"}
	st.Sockets["s3"] = &store.Socket{ID: "s3", Name: "Other", Code: "1003", Protocol: "nexa", Room: "Study"}
	st.Groups["group_1"] = &store.Group{ID: "group_1", Name: "Downstairs", SocketIDs: []string{"s1", "s2"}}
	return &Server{Store: st, SPADir: t.TempDir(), Audio: testAudio(st), Announce: testAnnouncer()}, rf
}

type bulkResponse struct {
	Group    string              `json:"group"`
	Room     string              `json:"room"`
	Scene    string              `json:"scene"`
	Updated  int                 `json:"updated"`
	Failures []map[string]string `json:"failures"`
}

func decodeBulk(t *testing.T, body []byte) bulkResponse {
	t.Helper()
	var out bulkResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return out
}

func TestGroupActionCharacterisation(t *testing.T) {
	t.Run("switches every member and reports the count", func(t *testing.T) {
		srv, rf := actionServer(t)
		rec := do(t, srv, http.MethodPost, "/api/groups/group_1/on", "")
		mustStatus(t, rec, http.StatusOK)

		got := decodeBulk(t, rec.Body.Bytes())
		if got.Group != "Downstairs" || got.Updated != 2 || len(got.Failures) != 0 {
			t.Errorf("response = %+v, want Downstairs/2/no failures", got)
		}
		if !srv.Store.Sockets["s1"].State || !srv.Store.Sockets["s2"].State {
			t.Error("group members were not switched on")
		}
		if srv.Store.Sockets["s3"].State {
			t.Error("a non-member was switched")
		}
		if len(rf.sentCodes()) != 2 {
			t.Errorf("transmitted %v, want two sends", rf.sentCodes())
		}
	})

	t.Run("a device failure is reported without failing the request", func(t *testing.T) {
		srv, rf := actionServer(t)
		rf.failCode = "1002"

		rec := do(t, srv, http.MethodPost, "/api/groups/group_1/on", "")
		mustStatus(t, rec, http.StatusOK)

		got := decodeBulk(t, rec.Body.Bytes())
		if got.Updated != 1 || len(got.Failures) != 1 {
			t.Fatalf("response = %+v, want 1 updated and 1 failure", got)
		}
		// The reachable socket still switches; the failed one keeps its state.
		if !srv.Store.Sockets["s1"].State {
			t.Error("the reachable socket was not switched")
		}
		if srv.Store.Sockets["s2"].State {
			t.Error("a socket that failed to transmit was recorded as on")
		}
	})

	t.Run("toggle flips each member independently", func(t *testing.T) {
		srv, _ := actionServer(t)
		srv.Store.Sockets["s1"].State = true

		mustStatus(t, do(t, srv, http.MethodPost, "/api/groups/group_1/toggle", ""), http.StatusOK)
		if srv.Store.Sockets["s1"].State {
			t.Error("s1 should have toggled off")
		}
		if !srv.Store.Sockets["s2"].State {
			t.Error("s2 should have toggled on")
		}
	})

	t.Run("an unknown group is a not-found", func(t *testing.T) {
		srv, _ := actionServer(t)
		rec := do(t, srv, http.MethodPost, "/api/groups/nope/on", "")
		mustStatus(t, rec, http.StatusNotFound)
	})

	// One summary entry per bulk action, not one per socket.
	t.Run("leaves a single activity entry", func(t *testing.T) {
		srv, _ := actionServer(t)
		before := len(srv.Store.Activity.Recent(50))
		mustStatus(t, do(t, srv, http.MethodPost, "/api/groups/group_1/on", ""), http.StatusOK)

		entries := srv.Store.Activity.Recent(50)
		if len(entries) != before+1 {
			t.Fatalf("activity grew by %d, want 1", len(entries)-before)
		}
		e := entries[0]
		if e.Kind != "group" || e.Action != "on" || e.Label != "Downstairs" || e.Source != "manual" {
			t.Errorf("entry = %+v", e)
		}
		if e.Status == "error" {
			t.Errorf("entry marked as error: %+v", e)
		}
	})

	t.Run("a partial failure marks the activity entry as an error", func(t *testing.T) {
		srv, rf := actionServer(t)
		rf.failCode = "1002"
		mustStatus(t, do(t, srv, http.MethodPost, "/api/groups/group_1/on", ""), http.StatusOK)

		e := srv.Store.Activity.Recent(50)[0]
		if e.Status != "error" || e.Error == "" {
			t.Errorf("entry = %+v, want an error status with a summary", e)
		}
	})
}

func TestRoomActionCharacterisation(t *testing.T) {
	t.Run("switches every socket in the room by name", func(t *testing.T) {
		srv, _ := actionServer(t)
		rec := do(t, srv, http.MethodPost, "/api/rooms/Lounge/on", "")
		mustStatus(t, rec, http.StatusOK)

		got := decodeBulk(t, rec.Body.Bytes())
		if got.Room != "Lounge" || got.Updated != 2 {
			t.Errorf("response = %+v", got)
		}
		if srv.Store.Sockets["s3"].State {
			t.Error("a socket in another room was switched")
		}
	})

	// Room matching is case-insensitive, matching the rename cascade.
	t.Run("matches the room name case-insensitively", func(t *testing.T) {
		srv, _ := actionServer(t)
		rec := do(t, srv, http.MethodPost, "/api/rooms/lounge/on", "")
		mustStatus(t, rec, http.StatusOK)
		if decodeBulk(t, rec.Body.Bytes()).Updated != 2 {
			t.Error("lowercase room name did not match")
		}
	})

	t.Run("a room with no sockets is a not-found", func(t *testing.T) {
		srv, _ := actionServer(t)
		rec := do(t, srv, http.MethodPost, "/api/rooms/Nowhere/on", "")
		mustStatus(t, rec, http.StatusNotFound)
	})
}

func TestBulkAllCharacterisation(t *testing.T) {
	srv, _ := actionServer(t)

	rec := do(t, srv, http.MethodPost, "/api/sockets/all/on", "")
	mustStatus(t, rec, http.StatusOK)
	for id, sock := range srv.Store.Sockets {
		if !sock.State {
			t.Errorf("socket %s was not switched on", id)
		}
	}

	mustStatus(t, do(t, srv, http.MethodPost, "/api/sockets/all/off", ""), http.StatusOK)
	for id, sock := range srv.Store.Sockets {
		if sock.State {
			t.Errorf("socket %s was not switched off", id)
		}
	}
}

func TestSceneActivateCharacterisation(t *testing.T) {
	newSrv := func(t *testing.T) (*Server, *recordRF) {
		srv, rf := actionServer(t)
		srv.Store.Scenes["scene_1"] = &store.Scene{
			ID: "scene_1", Name: "Movie",
			Steps: []store.SceneStep{{
				DelayMinutes: 0,
				Actions: []store.SceneAction{
					{SocketID: "s1", Action: "on"},
					{SocketID: "s2", Action: "off"},
				},
			}},
		}
		return srv, rf
	}

	t.Run("applies the scene's immediate step", func(t *testing.T) {
		srv, _ := newSrv(t)
		srv.Store.Sockets["s2"].State = true

		rec := do(t, srv, http.MethodPost, "/api/scenes/scene_1/activate", "")
		mustStatus(t, rec, http.StatusOK)

		if !srv.Store.Sockets["s1"].State {
			t.Error("s1 should be on")
		}
		if srv.Store.Sockets["s2"].State {
			t.Error("s2 should be off")
		}
	})

	t.Run("an unknown scene is a not-found", func(t *testing.T) {
		srv, _ := newSrv(t)
		mustStatus(t, do(t, srv, http.MethodPost, "/api/scenes/nope/activate", ""), http.StatusNotFound)
	})
}

// Deleting a scene must also drop the automations the scene wizard created
// for it, not just prune references to it.
func TestDeleteSceneCascades(t *testing.T) {
	srv, _ := actionServer(t)
	srv.Store.Scenes["scene_1"] = &store.Scene{ID: "scene_1", Name: "Movie"}
	srv.Store.Schedules["sch"] = &store.Schedule{
		ID: "sch", TargetType: "scene", TargetID: "scene_1", Action: "activate", Time: "20:00",
	}
	srv.Store.Timers["tmr"] = &store.Timer{
		ID: "tmr", TargetType: "scene", TargetID: "scene_1", Action: "activate",
	}
	srv.Store.Automations["auto_owned"] = &store.Automation{
		ID: "auto_owned", Name: "From wizard", SceneID: "scene_1",
	}

	mustStatus(t, do(t, srv, http.MethodDelete, "/api/scenes/scene_1", ""), http.StatusNoContent)

	if _, ok := srv.Store.Schedules["sch"]; ok {
		t.Error("a schedule targeting the scene survived")
	}
	if _, ok := srv.Store.Timers["tmr"]; ok {
		t.Error("a timer targeting the scene survived")
	}
	if _, ok := srv.Store.Automations["auto_owned"]; ok {
		t.Error("an automation owned by the scene survived")
	}
}

// Deleting a socket has the widest cascade of any delete; CascadeDeleteSocket
// has to stay in step with every field that can reference a socket id.
func TestDeleteSocketCascades(t *testing.T) {
	srv, _ := actionServer(t)
	srv.Store.Schedules["sch"] = &store.Schedule{
		ID: "sch", TargetType: "socket", TargetID: "s1", Action: "on", Time: "07:00",
	}
	srv.Store.Timers["tmr"] = &store.Timer{
		ID: "tmr", TargetType: "socket", TargetID: "s1", Action: "off",
	}
	srv.Store.Users["u1"] = &store.User{ID: "u1", Username: "kid", SocketIDs: []string{"s1", "s2"}}
	// A limited profile's allow-list must be pruned too. Seeding users turns
	// auth on, so this request has to be authenticated.
	user, pass := seedAdmin(t, srv)

	mustStatus(t, doAs(t, srv, user, pass, http.MethodDelete, "/api/sockets/s1", ""), http.StatusNoContent)

	if _, ok := srv.Store.Sockets["s1"]; ok {
		t.Error("socket survived")
	}
	if _, ok := srv.Store.Schedules["sch"]; ok {
		t.Error("a schedule targeting the socket survived")
	}
	if _, ok := srv.Store.Timers["tmr"]; ok {
		t.Error("a timer targeting the socket survived")
	}
	if got := srv.Store.Groups["group_1"].SocketIDs; len(got) != 1 || got[0] != "s2" {
		t.Errorf("group members = %v, want the deleted socket dropped", got)
	}
	if got := srv.Store.Users["u1"].SocketIDs; len(got) != 1 || got[0] != "s2" {
		t.Errorf("user's sockets = %v, want the deleted socket dropped", got)
	}
}
