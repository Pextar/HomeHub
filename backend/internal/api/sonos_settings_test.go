package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"homehub/internal/store"
)

// contextCanceled returns an already-cancelled child of the request's context,
// so a call that gets as far as the network gives up immediately rather than
// waiting out a connect timeout.
func contextCanceled(r *http.Request) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	return ctx, cancel
}

// settingsRequest builds a PUT for the settings handler with the {id} route
// var already bound, so the handler can be exercised without the router.
func settingsRequest(t *testing.T, id, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/sonos/"+id+"/settings", strings.NewReader(body))
	return mux.SetURLVars(req, map[string]string{"id": id})
}

// withSpeaker registers a speaker at an address that cannot answer. Every test
// below must therefore reject its request before any network call — a test that
// reaches the network would stall on the dial instead of failing fast.
func withSpeaker(t *testing.T) (*Server, string) {
	t.Helper()
	srv := testServer(t)
	sp := &store.SonosSpeaker{
		ID:   "sonos_test",
		Name: "Kitchen",
		IP:   "192.0.2.10", // TEST-NET-1: routable-looking, never reachable
		UUID: "RINCON_TEST0140001400",
	}
	srv.Store.Sonos[sp.ID] = sp
	return srv, sp.ID
}

func TestSonosUpdateSettingsRejectsOutOfRange(t *testing.T) {
	cases := map[string]string{
		"bass above range":     `{"bass": 11}`,
		"bass below range":     `{"bass": -11}`,
		"treble above range":   `{"treble": 99}`,
		"sub gain above range": `{"sub_gain": 16}`,
		"sleep timer negative": `{"sleep_minutes": -1}`,
		"sleep timer too long": `{"sleep_minutes": 601}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv, id := withSpeaker(t)
			rec := httptest.NewRecorder()
			srv.sonosUpdateSettings(rec, settingsRequest(t, id, body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d — a bad value must be rejected before it reaches a speaker",
					rec.Code, http.StatusBadRequest)
			}
		})
	}
}

// An empty patch is a bug in the caller, not a no-op worth pretending to do.
func TestSonosUpdateSettingsRejectsEmptyPatch(t *testing.T) {
	srv, id := withSpeaker(t)
	rec := httptest.NewRecorder()
	srv.sonosUpdateSettings(rec, settingsRequest(t, id, `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for a patch with no settings in it", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "no settings") {
		t.Errorf("body = %q, want it to say the patch was empty", rec.Body.String())
	}
}

func TestSonosUpdateSettingsUnknownSpeaker(t *testing.T) {
	srv := testServer(t)
	rec := httptest.NewRecorder()
	srv.sonosUpdateSettings(rec, settingsRequest(t, "sonos_nope", `{"bass": 0}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// Boundary values are legal, so they must get past validation and on to the
// speaker — which then fails to answer. A 502 proves validation let them
// through; a 400 would mean the range check is off by one.
func TestSonosUpdateSettingsAcceptsBoundaryValues(t *testing.T) {
	for _, body := range []string{
		`{"bass": 10}`, `{"bass": -10}`, `{"sub_gain": 15}`,
		`{"sub_gain": -15}`, `{"sleep_minutes": 0}`, `{"sleep_minutes": 600}`,
	} {
		srv, id := withSpeaker(t)
		rec := httptest.NewRecorder()
		req := settingsRequest(t, id, body)
		// Cancel the context up front: the value is accepted, the dial is
		// then abandoned instead of waiting out a real connect timeout.
		ctx, cancel := contextCanceled(req)
		defer cancel()
		srv.sonosUpdateSettings(rec, req.WithContext(ctx))
		if rec.Code != http.StatusBadGateway {
			t.Errorf("%s: status = %d, want %d (accepted, then unreachable)",
				body, rec.Code, http.StatusBadGateway)
		}
	}
}
