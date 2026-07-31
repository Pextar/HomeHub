package api

import (
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"homehub/internal/store"
)

// Kid profiles get the household's music: browse + playback control on Sonos
// and Spotify search, so their playful surface is a real music player. What
// they never get is configuration — discovery, device management, settings,
// the event monitor, KEF, and the Spotify account itself stay admin-only,
// and a limited profile that isn't a kid keeps its 403 everywhere.

// seedKid adds a kid profile (a limited one with the kid flag) with a known
// password, so tests can authenticate with basic auth like the other roles.
func seedKid(t *testing.T, srv *Server, id, username string) (string, string) {
	t.Helper()
	const password = "kid-pass-1234"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	srv.Store.Users[id] = &store.User{
		ID: id, Username: username, Admin: false, Kid: true,
		PasswordHash: string(hash),
	}
	return username, password
}

func TestKidReachesMusicBrowseAndControl(t *testing.T) {
	srv, _ := actionServer(t)
	admin, adminPass := seedAdmin(t, srv)
	kid, kidPass := seedKid(t, srv, "u_kid", "kid")

	// Sonos status is fully reachable and answers 200 with the (empty)
	// topology, exactly as it does for an admin.
	mustStatus(t, doAs(t, srv, kid, kidPass, http.MethodGet, "/api/sonos/status", ""), http.StatusOK)

	// The remaining music routes share one assertion: whatever they answer
	// (404 for an unknown speaker, 503 while Spotify isn't wired in the
	// fixture), it must equal the admin's answer — never a 403. Authz is
	// the only thing this pins; handler behaviour has its own tests.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/sonos/sp1/play", ""},
		{http.MethodPost, "/api/sonos/sp1/pause", ""},
		{http.MethodPost, "/api/sonos/sp1/next", ""},
		{http.MethodPost, "/api/sonos/sp1/previous", ""},
		{http.MethodPost, "/api/sonos/sp1/leave", ""},
		{http.MethodPost, "/api/sonos/sp1/join", `{"target_id":"sp2"}`},
		{http.MethodPut, "/api/sonos/sp1/volume", `{"level":10}`},
		{http.MethodPut, "/api/sonos/sp1/mute", `{"muted":true}`},
		{http.MethodGet, "/api/sonos/sp1/art", ""},
		{http.MethodPost, "/api/sonos/sp1/play-item", `{"service":"spotify","uri":"spotify:track:x","title":"X"}`},
		{http.MethodPut, "/api/sonos/sp1/seek", `{"position":"0:01:00"}`},
		{http.MethodPut, "/api/sonos/sp1/playmode", `{"shuffle":true,"repeat":"off"}`},
		{http.MethodPut, "/api/sonos/sp1/crossfade", `{"enabled":true}`},
		{http.MethodPut, "/api/sonos/sp1/autoplay", `{"enabled":true}`},
		{http.MethodGet, "/api/sonos/sp1/queue", ""},
		{http.MethodDelete, "/api/sonos/sp1/queue", ""},
		{http.MethodDelete, "/api/sonos/sp1/queue/2", ""},
		{http.MethodGet, "/api/spotify/status", ""},
		{http.MethodGet, "/api/spotify/search?q=adele", ""},
		{http.MethodGet, "/api/spotify/playlists", ""},
		{http.MethodGet, "/api/spotify/artist?uri=spotify:artist:x", ""},
		{http.MethodGet, "/api/spotify/context?uri=spotify:album:x", ""},
	} {
		got := doAs(t, srv, kid, kidPass, tc.method, tc.path, tc.body)
		if got.Code == http.StatusForbidden {
			t.Errorf("%s %s as a kid = 403, want the admin's answer", tc.method, tc.path)
			continue
		}
		want := doAs(t, srv, admin, adminPass, tc.method, tc.path, tc.body)
		if got.Code != want.Code {
			t.Errorf("%s %s as a kid = %d, admin got %d — the gate should treat them alike",
				tc.method, tc.path, got.Code, want.Code)
		}
	}
}

func TestKidIsRefusedConfigurationAndOtherBridges(t *testing.T) {
	srv, _ := actionServer(t)
	seedAdmin(t, srv)
	kid, kidPass := seedKid(t, srv, "u_kid", "kid")

	for _, tc := range []struct{ method, path, body string }{
		// Sonos setup and diagnostics.
		{http.MethodGet, "/api/sonos/discover", ""},
		{http.MethodGet, "/api/sonos/events", ""},
		{http.MethodPost, "/api/sonos/events/retry", ""},
		{http.MethodPost, "/api/sonos/speakers", `{"ip":"1.2.3.4"}`},
		{http.MethodPut, "/api/sonos/speakers/sp1", `{"name":"X"}`},
		{http.MethodDelete, "/api/sonos/speakers/sp1", ""},
		{http.MethodGet, "/api/sonos/sp1/settings", ""},
		{http.MethodPut, "/api/sonos/sp1/settings", `{}`},
		{http.MethodGet, "/api/sonos/sp1/image", ""},
		{http.MethodGet, "/api/sonos/sp1/favorites", ""},
		// The Spotify account is the grown-ups' to connect and drop.
		{http.MethodPut, "/api/spotify/config", `{"client_id":"x"}`},
		{http.MethodGet, "/api/spotify/login", ""},
		{http.MethodPost, "/api/spotify/exchange", `{"url":"x"}`},
		{http.MethodPost, "/api/spotify/disconnect", ""},
		// KEF and the vendor-neutral media layer stay out of the kid surface.
		{http.MethodGet, "/api/kef/status", ""},
		{http.MethodPost, "/api/kef/k1/play", ""},
		{http.MethodGet, "/api/media/endpoints", ""},
		{http.MethodGet, "/api/media/search?q=x", ""},
	} {
		rec := doAs(t, srv, kid, kidPass, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a kid = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}

func TestLimitedNonKidKeepsIts403OnMusic(t *testing.T) {
	srv, _, _, limited, limitedPass := authzServer(t) // limited may touch s1 only

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/sonos/status", ""},
		{http.MethodPost, "/api/sonos/sp1/play", ""},
		{http.MethodPut, "/api/sonos/sp1/volume", `{"level":10}`},
		{http.MethodGet, "/api/sonos/sp1/queue", ""},
		{http.MethodGet, "/api/spotify/status", ""},
		{http.MethodGet, "/api/spotify/search?q=adele", ""},
		{http.MethodGet, "/api/spotify/playlists", ""},
	} {
		rec := doAs(t, srv, limited, limitedPass, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a limited profile = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}
