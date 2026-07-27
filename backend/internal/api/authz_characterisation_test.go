package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"homehub/internal/store"
)

// Characterisation tests for who is allowed to do what.
//
// Authorisation is currently spread across three mechanisms — the
// requireAdmin route wrapper, canAccess filtering inside list handlers, and
// per-handler checks in createSchedule — so it is easy to drop one while
// moving handler bodies around. These tests pin the observable rules so a
// refactor that widens access fails loudly.

// seedLimited adds a non-admin profile restricted to the given sockets.
func seedLimited(t *testing.T, srv *Server, id, username string, socketIDs ...string) (string, string) {
	t.Helper()
	const password = "limited-pass-1234"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	srv.Store.Users[id] = &store.User{
		ID: id, Username: username, Admin: false,
		PasswordHash: string(hash), SocketIDs: socketIDs,
	}
	return username, password
}

// authzServer has two sockets, an admin, and a limited profile that may
// only touch the first socket.
func authzServer(t *testing.T) (srv *Server, admin, adminPass, limited, limitedPass string) {
	t.Helper()
	srv, _ = actionServer(t)
	admin, adminPass = seedAdmin(t, srv)
	limited, limitedPass = seedLimited(t, srv, "u_kid", "kid", "s1")
	return srv, admin, adminPass, limited, limitedPass
}

// The whole-home surfaces are admin-only. A limited profile gets 403, not
// an empty list, so the UI can hide the section outright.
func TestAdminOnlyRoutesRejectLimitedProfiles(t *testing.T) {
	srv, _, _, limited, limitedPass := authzServer(t)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/groups", ""},
		{http.MethodPost, "/api/groups", `{"name":"X"}`},
		{http.MethodGet, "/api/scenes", ""},
		{http.MethodPost, "/api/scenes", `{"name":"X"}`},
		{http.MethodGet, "/api/sensors", ""},
		{http.MethodGet, "/api/automations", ""},
		{http.MethodGet, "/api/activity", ""},
		{http.MethodGet, "/api/users", ""},
		{http.MethodPost, "/api/sockets", `{"name":"X","code":"9","protocol":"nexa"}`},
		{http.MethodDelete, "/api/sockets/s1", ""},
		{http.MethodGet, "/api/export", ""},
	} {
		rec := doAs(t, srv, limited, limitedPass, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a limited profile = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}

func TestAdminReachesTheSameRoutes(t *testing.T) {
	srv, admin, adminPass, _, _ := authzServer(t)
	for _, path := range []string{
		"/api/groups", "/api/scenes", "/api/sensors",
		"/api/automations", "/api/activity", "/api/users",
	} {
		rec := doAs(t, srv, admin, adminPass, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s as admin = %d, want 200 (%s)", path, rec.Code, rec.Body.String())
		}
	}
}

// Socket lists are filtered rather than refused: a limited profile sees
// exactly its own devices.
func TestSocketListIsFilteredForLimitedProfiles(t *testing.T) {
	srv, admin, adminPass, limited, limitedPass := authzServer(t)

	rec := doAs(t, srv, limited, limitedPass, http.MethodGet, "/api/sockets", "")
	mustStatus(t, rec, http.StatusOK)
	var got []store.Socket
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != "s1" {
		t.Errorf("limited profile saw %d sockets (%+v), want just s1", len(got), got)
	}

	rec = doAs(t, srv, admin, adminPass, http.MethodGet, "/api/sockets", "")
	mustStatus(t, rec, http.StatusOK)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("admin saw %d sockets, want all 3", len(got))
	}
}

// Control of a socket outside the allow-list is refused even though the
// route itself is not admin-gated.
func TestSocketControlIsGatedPerSocket(t *testing.T) {
	srv, _, _, limited, limitedPass := authzServer(t)

	mustStatus(t, doAs(t, srv, limited, limitedPass,
		http.MethodPost, "/api/sockets/s1/on", ""), http.StatusOK)

	rec := doAs(t, srv, limited, limitedPass, http.MethodPost, "/api/sockets/s2/on", "")
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Errorf("controlling someone else's socket = %d, want 403 or 404", rec.Code)
	}
	if srv.Store.Sockets["s2"].State {
		t.Error("a socket outside the allow-list was switched")
	}
}

// Schedules are readable and writable by everyone, but a limited profile
// may only schedule its own sockets — never a group, room or scene.
func TestScheduleCreationIsRestrictedForLimitedProfiles(t *testing.T) {
	srv, _, _, limited, limitedPass := authzServer(t)

	t.Run("own socket is allowed", func(t *testing.T) {
		rec := doAs(t, srv, limited, limitedPass, http.MethodPost, "/api/schedules",
			`{"target_type":"socket","target_id":"s1","action":"on","time":"07:00","days":[1]}`)
		mustStatus(t, rec, http.StatusCreated)
	})

	t.Run("another profile's socket is refused", func(t *testing.T) {
		rec := doAs(t, srv, limited, limitedPass, http.MethodPost, "/api/schedules",
			`{"target_type":"socket","target_id":"s2","action":"on","time":"07:00","days":[1]}`)
		mustStatus(t, rec, http.StatusForbidden)
	})

	t.Run("a group target is refused outright", func(t *testing.T) {
		rec := doAs(t, srv, limited, limitedPass, http.MethodPost, "/api/schedules",
			`{"target_type":"group","target_id":"group_1","action":"on","time":"07:00","days":[1]}`)
		mustStatus(t, rec, http.StatusForbidden)
	})

	t.Run("the bulk enable switch stays admin-only", func(t *testing.T) {
		rec := doAs(t, srv, limited, limitedPass, http.MethodPost, "/api/schedules/all/disable", "")
		mustStatus(t, rec, http.StatusForbidden)
	})
}

// With no users configured the API is deliberately open, which is what
// makes first-run setup possible.
func TestApiIsOpenUntilTheFirstUserExists(t *testing.T) {
	srv, _ := actionServer(t)
	mustStatus(t, do(t, srv, http.MethodGet, "/api/groups", ""), http.StatusOK)

	seedAdmin(t, srv)
	rec := do(t, srv, http.MethodGet, "/api/groups", "")
	mustStatus(t, rec, http.StatusUnauthorized)
}
