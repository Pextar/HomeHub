package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"homehub/internal/store"
)

// Characterisation tests for the CRUD surface.
//
// These pin the behaviour the handlers have today — status codes, error
// wording, merge semantics and cascade effects — so the boilerplate in
// them can be replaced by shared helpers without changing what the API
// does. Where the current behaviour looks inconsistent between resources
// the test says so rather than asserting what would be tidier: the point
// is to detect drift, not to endorse it.
//
// Everything goes through Handler() so routing, middleware and the handler
// are all covered; a route registered on the wrong method or path fails
// here too.

// do issues an unauthenticated request against the full router. That is
// enough for most tests: the API only enforces auth once at least one user
// exists, and the default fixture has none.
func do(t *testing.T, srv *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doAs(t, srv, "", "", method, path, body)
}

// doAs is do with HTTP basic credentials attached.
func doAs(t *testing.T, srv *Server, user, pass, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		r.SetBasicAuth(user, pass)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, r)
	return rec
}

// seedAdmin adds an admin account with a known password.
//
// Note that adding *any* user flips the whole API from open to
// authenticated, so a test that needs a user in the store must send
// credentials on every request from then on.
func seedAdmin(t *testing.T, srv *Server) (string, string) {
	t.Helper()
	const username, password = "admin", "hunter2hunter2"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	srv.Store.Users["admin_1"] = &store.User{
		ID: "admin_1", Username: username, Admin: true, Owner: true,
		PasswordHash: string(hash),
	}
	return username, password
}

// errorMessage pulls the message out of an error response body.
func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return body.Error
}

func mustStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, want, rec.Body.String())
	}
}

// ---------------------------------------------------------------- groups

func TestCreateGroupCharacterisation(t *testing.T) {
	t.Run("valid group is created with a generated id", func(t *testing.T) {
		srv := testServer(t)
		rec := do(t, srv, http.MethodPost, "/api/groups", `{"name":"Downstairs"}`)
		mustStatus(t, rec, http.StatusCreated)

		var g store.Group
		if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !strings.HasPrefix(g.ID, "group_") {
			t.Errorf("generated id = %q, want a group_ prefix", g.ID)
		}
		if g.Name != "Downstairs" {
			t.Errorf("name = %q", g.Name)
		}
		if len(srv.Store.Groups) != 1 {
			t.Errorf("store holds %d groups, want 1", len(srv.Store.Groups))
		}
	})

	t.Run("malformed JSON is rejected before the store is touched", func(t *testing.T) {
		srv := testServer(t)
		rec := do(t, srv, http.MethodPost, "/api/groups", `{"name":`)
		mustStatus(t, rec, http.StatusBadRequest)
		if msg := errorMessage(t, rec); !strings.HasPrefix(msg, "invalid JSON:") {
			t.Errorf("message = %q, want an invalid JSON prefix", msg)
		}
		if len(srv.Store.Groups) != 0 {
			t.Error("a malformed body created a group")
		}
	})

	t.Run("a client-supplied duplicate id is a conflict", func(t *testing.T) {
		srv := testServer(t)
		srv.Store.Groups["group_1"] = &store.Group{ID: "group_1", Name: "Existing"}
		rec := do(t, srv, http.MethodPost, "/api/groups", `{"id":"group_1","name":"Other"}`)
		mustStatus(t, rec, http.StatusConflict)
		if srv.Store.Groups["group_1"].Name != "Existing" {
			t.Error("the existing group was overwritten")
		}
	})

	// Quirk worth pinning: createGroup validates *before* looking at the id,
	// so an invalid body that also collides reports the validation problem
	// (400) rather than the collision (409). createRoom does the opposite —
	// see TestCreateRoomCharacterisation.
	t.Run("validation is reported ahead of an id collision", func(t *testing.T) {
		srv := testServer(t)
		srv.Store.Groups["group_1"] = &store.Group{ID: "group_1", Name: "Existing"}
		rec := do(t, srv, http.MethodPost, "/api/groups", `{"id":"group_1","name":"  "}`)
		mustStatus(t, rec, http.StatusBadRequest)
	})
}

func TestUpdateGroupCharacterisation(t *testing.T) {
	newSrv := func(t *testing.T) *Server {
		srv := testServer(t)
		srv.Store.Sockets["a"] = &store.Socket{ID: "a", Name: "Lamp", Code: "1"}
		srv.Store.Groups["group_1"] = &store.Group{
			ID: "group_1", Name: "Downstairs", SocketIDs: []string{"a"},
		}
		return srv
	}

	t.Run("an empty name leaves the existing one alone", func(t *testing.T) {
		srv := newSrv(t)
		rec := do(t, srv, http.MethodPut, "/api/groups/group_1", `{"name":"   "}`)
		mustStatus(t, rec, http.StatusOK)
		if got := srv.Store.Groups["group_1"].Name; got != "Downstairs" {
			t.Errorf("name = %q, want it unchanged", got)
		}
	})

	t.Run("omitted socket_ids are left alone but an empty array clears them", func(t *testing.T) {
		srv := newSrv(t)
		mustStatus(t, do(t, srv, http.MethodPut, "/api/groups/group_1", `{"name":"Up"}`), http.StatusOK)
		if got := srv.Store.Groups["group_1"].SocketIDs; len(got) != 1 {
			t.Errorf("socket_ids = %v, want them preserved when omitted", got)
		}
		mustStatus(t, do(t, srv, http.MethodPut, "/api/groups/group_1", `{"socket_ids":[]}`), http.StatusOK)
		if got := srv.Store.Groups["group_1"].SocketIDs; len(got) != 0 {
			t.Errorf("socket_ids = %v, want an explicit empty array to clear", got)
		}
	})

	// The merged record is revalidated in full, not just the changed fields.
	// A group still listing a socket that has since been deleted therefore
	// cannot be edited at all unless the stale member goes in the same
	// request — worth knowing before anyone "simplifies" the merge.
	t.Run("a stale member blocks an unrelated edit", func(t *testing.T) {
		srv := newSrv(t)
		delete(srv.Store.Sockets, "a")
		rec := do(t, srv, http.MethodPut, "/api/groups/group_1", `{"name":"Renamed"}`)
		mustStatus(t, rec, http.StatusBadRequest)
		if msg := errorMessage(t, rec); !strings.Contains(msg, "unknown socket") {
			t.Errorf("message = %q", msg)
		}
		// Clearing the stale member in the same request does go through.
		mustStatus(t, do(t, srv, http.MethodPut, "/api/groups/group_1",
			`{"name":"Renamed","socket_ids":[]}`), http.StatusOK)
	})

	t.Run("an unknown id is a not-found", func(t *testing.T) {
		srv := newSrv(t)
		rec := do(t, srv, http.MethodPut, "/api/groups/nope", `{"name":"X"}`)
		mustStatus(t, rec, http.StatusNotFound)
		if msg := errorMessage(t, rec); msg != "group not found" {
			t.Errorf("message = %q", msg)
		}
	})
}

func TestDeleteGroupCascades(t *testing.T) {
	srv := testServer(t)
	srv.Store.Groups["group_1"] = &store.Group{ID: "group_1", Name: "Downstairs"}
	srv.Store.Schedules["sch_hit"] = &store.Schedule{
		ID: "sch_hit", TargetType: "group", TargetID: "group_1", Action: "on", Time: "07:00",
	}
	srv.Store.Schedules["sch_miss"] = &store.Schedule{
		ID: "sch_miss", TargetType: "group", TargetID: "group_2", Action: "on", Time: "07:00",
	}
	srv.Store.Timers["tmr_hit"] = &store.Timer{
		ID: "tmr_hit", TargetType: "group", TargetID: "group_1", Action: "off",
	}
	srv.Store.Timers["tmr_miss"] = &store.Timer{
		ID: "tmr_miss", TargetType: "socket", TargetID: "group_1", Action: "off",
	}

	mustStatus(t, do(t, srv, http.MethodDelete, "/api/groups/group_1", ""), http.StatusNoContent)

	if _, ok := srv.Store.Groups["group_1"]; ok {
		t.Error("group survived the delete")
	}
	if _, ok := srv.Store.Schedules["sch_hit"]; ok {
		t.Error("a schedule targeting the group survived")
	}
	if _, ok := srv.Store.Schedules["sch_miss"]; !ok {
		t.Error("a schedule targeting a different group was deleted")
	}
	if _, ok := srv.Store.Timers["tmr_hit"]; ok {
		t.Error("a timer targeting the group survived")
	}
	// Same id, different target type — the cascade must key off both.
	if _, ok := srv.Store.Timers["tmr_miss"]; !ok {
		t.Error("a socket timer sharing the id was deleted")
	}

	mustStatus(t, do(t, srv, http.MethodDelete, "/api/groups/group_1", ""), http.StatusNotFound)
}

// ----------------------------------------------------------------- rooms

func TestCreateRoomCharacterisation(t *testing.T) {
	t.Run("valid room is created", func(t *testing.T) {
		srv := testServer(t)
		rec := do(t, srv, http.MethodPost, "/api/rooms", `{"name":"Kitchen"}`)
		mustStatus(t, rec, http.StatusCreated)
		if len(srv.Store.Rooms) != 1 {
			t.Fatalf("store holds %d rooms", len(srv.Store.Rooms))
		}
	})

	// The mirror of the group case above: createRoom assigns/checks the id
	// *before* validating, so a body that is both invalid and colliding
	// reports 409 where the group route reports 400.
	t.Run("an id collision is reported ahead of validation", func(t *testing.T) {
		srv := testServer(t)
		srv.Store.Rooms["room_1"] = &store.Room{ID: "room_1", Name: "Kitchen"}
		rec := do(t, srv, http.MethodPost, "/api/rooms", `{"id":"room_1","name":"  "}`)
		mustStatus(t, rec, http.StatusConflict)
	})
}

func TestRoomRenameCascades(t *testing.T) {
	srv := testServer(t)
	srv.Store.Rooms["room_1"] = &store.Room{ID: "room_1", Name: "Lounge"}
	srv.Store.Sockets["s1"] = &store.Socket{ID: "s1", Name: "Lamp", Code: "1", Room: "lounge"}
	srv.Store.Sensors["n1"] = &store.Sensor{ID: "n1", Name: "Temp", Kind: "temperature", Room: "LOUNGE"}
	srv.Store.Scenes["c1"] = &store.Scene{ID: "c1", Name: "Movie", Room: "Lounge"}
	srv.Store.Sockets["s2"] = &store.Socket{ID: "s2", Name: "Other", Code: "2", Room: "Kitchen"}

	mustStatus(t, do(t, srv, http.MethodPut, "/api/rooms/room_1", `{"name":"Living Room"}`), http.StatusOK)

	// The match is case-insensitive but the written value is the new canonical name.
	if got := srv.Store.Sockets["s1"].Room; got != "Living Room" {
		t.Errorf("socket room = %q, want the rename to cascade case-insensitively", got)
	}
	if got := srv.Store.Sensors["n1"].Room; got != "Living Room" {
		t.Errorf("sensor room = %q", got)
	}
	if got := srv.Store.Scenes["c1"].Room; got != "Living Room" {
		t.Errorf("scene room = %q", got)
	}
	if got := srv.Store.Sockets["s2"].Room; got != "Kitchen" {
		t.Errorf("unrelated socket room = %q, want it untouched", got)
	}
}

func TestDeleteRoomClearsTheNameEverywhere(t *testing.T) {
	srv := testServer(t)
	srv.Store.Rooms["room_1"] = &store.Room{ID: "room_1", Name: "Lounge"}
	srv.Store.Sockets["s1"] = &store.Socket{ID: "s1", Name: "Lamp", Code: "1", Room: "Lounge"}
	srv.Store.Sensors["n1"] = &store.Sensor{ID: "n1", Name: "Temp", Kind: "temperature", Room: "lounge"}
	srv.Store.Scenes["c1"] = &store.Scene{ID: "c1", Name: "Movie", Room: "Lounge"}

	mustStatus(t, do(t, srv, http.MethodDelete, "/api/rooms/room_1", ""), http.StatusNoContent)

	if got := srv.Store.Sockets["s1"].Room; got != "" {
		t.Errorf("socket room = %q, want it cleared", got)
	}
	if got := srv.Store.Sensors["n1"].Room; got != "" {
		t.Errorf("sensor room = %q, want it cleared", got)
	}
	if got := srv.Store.Scenes["c1"].Room; got != "" {
		t.Errorf("scene room = %q, want it cleared", got)
	}
}

// Rooms are listed with per-room socket counts rather than as bare records.
func TestGetRoomsReportsSocketCounts(t *testing.T) {
	srv := testServer(t)
	srv.Store.Rooms["room_1"] = &store.Room{ID: "room_1", Name: "Lounge"}
	srv.Store.Sockets["s1"] = &store.Socket{ID: "s1", Name: "A", Code: "1", Room: "Lounge", State: true}
	srv.Store.Sockets["s2"] = &store.Socket{ID: "s2", Name: "B", Code: "2", Room: "lounge"}

	rec := do(t, srv, http.MethodGet, "/api/rooms", "")
	mustStatus(t, rec, http.StatusOK)

	var out []roomSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d rooms", len(out))
	}
	if out[0].Sockets != 2 || out[0].On != 1 {
		t.Errorf("counts = %d sockets / %d on, want 2/1", out[0].Sockets, out[0].On)
	}
}

// --------------------------------------------------------------- generic

// Every collection sorts by name case-insensitively, so the UI's ordering
// doesn't depend on map iteration order.
func TestCollectionsSortByNameCaseInsensitively(t *testing.T) {
	srv := testServer(t)
	for _, n := range []string{"zebra", "Apple", "monkey"} {
		mustStatus(t, do(t, srv, http.MethodPost, "/api/groups",
			fmt.Sprintf(`{"name":%q}`, n)), http.StatusCreated)
	}
	rec := do(t, srv, http.MethodGet, "/api/groups", "")
	mustStatus(t, rec, http.StatusOK)

	var got []store.Group
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{"Apple", "monkey", "zebra"}
	for i, w := range want {
		if got[i].Name != w {
			t.Fatalf("order = %v, want %v", groupNames(got), want)
		}
	}
}

func groupNames(gs []store.Group) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Name
	}
	return out
}

// The not-found message names the resource, which the UI surfaces directly.
func TestNotFoundMessagesNameTheResource(t *testing.T) {
	srv := testServer(t)
	for _, tc := range []struct{ method, path, want string }{
		{http.MethodGet, "/api/groups/nope", "group not found"},
		{http.MethodPut, "/api/groups/nope", "group not found"},
		{http.MethodDelete, "/api/groups/nope", "group not found"},
		{http.MethodGet, "/api/scenes/nope", "scene not found"},
		{http.MethodDelete, "/api/scenes/nope", "scene not found"},
	} {
		body := ""
		if tc.method == http.MethodPut {
			body = `{"name":"X"}`
		}
		rec := do(t, srv, tc.method, tc.path, body)
		mustStatus(t, rec, http.StatusNotFound)
		if msg := errorMessage(t, rec); msg != tc.want {
			t.Errorf("%s %s message = %q, want %q", tc.method, tc.path, msg, tc.want)
		}
	}
}

// Malformed JSON is a 400 with a consistent prefix on every write route.
func TestMalformedJSONIsRejectedConsistently(t *testing.T) {
	srv := testServer(t)
	for _, path := range []string{
		"/api/groups", "/api/rooms", "/api/scenes", "/api/schedules",
		"/api/sensors", "/api/automations",
	} {
		rec := do(t, srv, http.MethodPost, path, `{`)
		mustStatus(t, rec, http.StatusBadRequest)
		if msg := errorMessage(t, rec); !strings.HasPrefix(msg, "invalid JSON:") {
			t.Errorf("POST %s message = %q, want an invalid JSON prefix", path, msg)
		}
	}
}
