package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homehub/internal/store"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	st := store.New(t.TempDir(), nil)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	return &Server{Store: st, SPADir: t.TempDir()}
}

// The callback has to be reachable without credentials — speakers have none
// — so it must sit outside the API's auth middleware. It must also refuse
// anything it can't attribute to a subscription it issued.
func TestSonosEventRouteIsUnauthenticatedButGuarded(t *testing.T) {
	srv := testServer(t)
	h := srv.Handler()

	req := httptest.NewRequest("NOTIFY", sonosEventPath+"/deadbeef", strings.NewReader(
		`<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0"></e:propertyset>`))
	req.Header.Set("SID", "uuid:whatever")
	req.Header.Set("SEQ", "0")
	req.RemoteAddr = "192.168.1.42:3400"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// 412 is GENA's "that subscription is gone", which makes a stray sender
	// stop. Anything else — 401, 403, 404 — means the route is misplaced.
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("status = %d, want %d for an unknown token", rec.Code, http.StatusPreconditionFailed)
	}
}

// The route is bound to NOTIFY alone, so the same path in a browser still
// reaches the SPA rather than the event handler.
func TestSonosEventPathFallsThroughForBrowsers(t *testing.T) {
	srv := testServer(t)
	h := srv.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, sonosEventPath+"/deadbeef", nil))
	if rec.Code == http.StatusPreconditionFailed {
		t.Error("a GET reached the event handler; the route should be NOTIFY-only")
	}
}

// A speaker can only reach us on the address that faces its own subnet, and
// only over plain HTTP.
func TestSonosCallbackURL(t *testing.T) {
	srv := testServer(t)
	srv.HTTPPort = "8080"

	got, err := srv.sonosCallbackURL("192.168.1.42")
	if err != nil {
		t.Skipf("no route to a LAN address in this environment: %v", err)
	}
	if !strings.HasPrefix(got, "http://") {
		t.Errorf("callback = %q, want plain http — speakers will not post to TLS", got)
	}
	if !strings.HasSuffix(got, ":8080"+sonosEventPath) {
		t.Errorf("callback = %q, want it to end in :8080%s", got, sonosEventPath)
	}
	if strings.Contains(got, "0.0.0.0") || strings.Contains(got, "127.0.0.1") {
		t.Errorf("callback = %q, want a routable address", got)
	}
}

func TestSonosCallbackURLDefaultsPort(t *testing.T) {
	srv := testServer(t)
	got, err := srv.sonosCallbackURL("192.168.1.42")
	if err != nil {
		t.Skipf("no route to a LAN address in this environment: %v", err)
	}
	if !strings.HasSuffix(got, ":8080"+sonosEventPath) {
		t.Errorf("callback = %q, want the default :8080", got)
	}
}

func TestLocalAddrForRejectsUnroutable(t *testing.T) {
	if _, err := localAddrFor("not an ip"); err == nil {
		t.Error("localAddrFor(garbage) = nil, want an error")
	}
}

// Music changes are their own SSE topic, so a volume nudge doesn't make
// every open tab refetch every socket, scene and sensor in the house.
func TestSSEHubTopicsAreSeparate(t *testing.T) {
	hub := newSSEHub()
	c := hub.add()
	defer hub.remove(c)

	hub.broadcastTopic(topicMusic)
	got := c.take()
	if len(got) != 1 || got[0] != topicMusic {
		t.Fatalf("topics = %v, want [%s]", got, topicMusic)
	}
	if left := c.take(); len(left) != 0 {
		t.Errorf("topics = %v after draining, want none", left)
	}

	// A burst on one topic collapses, but must not swallow another that
	// arrived alongside it.
	hub.broadcastTopic(topicMusic)
	hub.broadcastTopic(topicMusic)
	hub.broadcast()
	got = c.take()
	if len(got) != 2 {
		t.Fatalf("topics = %v, want both %s and %s exactly once", got, topicMusic, topicChanged)
	}
	seen := map[string]bool{got[0]: true, got[1]: true}
	if !seen[topicMusic] || !seen[topicChanged] {
		t.Errorf("topics = %v, want both %s and %s", got, topicMusic, topicChanged)
	}
}

// A disconnected client must not keep the hub writing into it.
func TestSSEHubRemove(t *testing.T) {
	hub := newSSEHub()
	c := hub.add()
	hub.remove(c)
	hub.broadcastTopic(topicMusic)
	if got := c.take(); len(got) != 0 {
		t.Errorf("a removed client still received %v", got)
	}
}

// The per-speaker callback carries the token that guards the unauthenticated
// NOTIFY route, so it must not travel out in a diagnostic response.
func TestRedactTokenDropsTheCallbackToken(t *testing.T) {
	const withToken = "http://192.168.1.5:8080/sonos/event/9f2c1ab4"
	got := redactToken(withToken)
	if want := "http://192.168.1.5:8080/sonos/event"; got != want {
		t.Errorf("redactToken(%q) = %q, want %q", withToken, got, want)
	}
	if redactToken("") != "" {
		t.Error("redactToken invented a URL for an empty callback")
	}
}
