package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"homehub/internal/store"
)

// withKEFSpeaker registers a speaker at an address that cannot answer. Every
// test below must therefore reject its request before any network call — a
// test that reached the network would stall on the dial instead of failing.
func withKEFSpeaker(t *testing.T) (*Server, string) {
	t.Helper()
	srv := testServer(t)
	sp := &store.KEFSpeaker{
		ID:   "kef_test",
		Name: "Study",
		IP:   "192.0.2.20", // TEST-NET-1: routable-looking, never reachable
		MAC:  "a1b2c3d4e5f6",
	}
	srv.Store.KEF[sp.ID] = sp
	return srv, sp.ID
}

func kefRequest(t *testing.T, method, suffix, id, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, "/api/kef/"+id+suffix, strings.NewReader(body))
	return mux.SetURLVars(req, map[string]string{"id": id})
}

func TestKEFSetVolumeRejectsOutOfRange(t *testing.T) {
	for _, body := range []string{`{"level": 101}`, `{"level": -1}`} {
		srv, id := withKEFSpeaker(t)
		rec := httptest.NewRecorder()
		srv.kefSetVolume(rec, kefRequest(t, http.MethodPut, "/volume", id, body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s → %d, want 400", body, rec.Code)
		}
	}
}

func TestKEFSetSourceRejectsUnknownInput(t *testing.T) {
	srv, id := withKEFSpeaker(t)
	rec := httptest.NewRecorder()
	srv.kefSetSource(rec, kefRequest(t, http.MethodPut, "/source", id, `{"source":"turntable"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	// The error has to name the inputs the speaker does have, or the only
	// way to find them is reading the source.
	if !strings.Contains(rec.Body.String(), "optic") {
		t.Errorf("body = %s, want it to list the accepted sources", rec.Body.String())
	}
}

func TestKEFUpdateSettingsRejectsOutOfRange(t *testing.T) {
	cases := map[string]string{
		"desk gain above 0dB":   `{"desk_gain": 10}`,
		"wall gain below -6dB":  `{"wall_gain": -100}`,
		"treble beyond ±2dB":    `{"treble": 50}`,
		"high-pass below 50Hz":  `{"high_pass_freq": 30}`,
		"sub low-pass too high": `{"sub_lp_freq": 400}`,
		"sub gain beyond ±10dB": `{"sub_gain": -20}`,
		"max volume over 100":   `{"max_volume": 101}`,
		"unknown bass preset":   `{"bass_extension": "massive"}`,
		"unknown standby mode":  `{"standby_mode": "standby_5mins"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv, id := withKEFSpeaker(t)
			rec := httptest.NewRecorder()
			srv.kefUpdateSettings(rec, kefRequest(t, http.MethodPut, "/settings", id, body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestKEFUpdateSettingsRejectsEmptyPatch(t *testing.T) {
	// An empty patch would otherwise be a no-op round trip to the speaker.
	srv, id := withKEFSpeaker(t)
	rec := httptest.NewRecorder()
	srv.kefUpdateSettings(rec, kefRequest(t, http.MethodPut, "/settings", id, `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestKEFHandlersRejectUnknownSpeaker(t *testing.T) {
	srv := testServer(t)
	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"volume":   srv.kefSetVolume,
		"mute":     srv.kefSetMute,
		"source":   srv.kefSetSource,
		"power":    srv.kefSetPower,
		"settings": srv.kefUpdateSettings,
		"art":      srv.kefArt,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			call(rec, kefRequest(t, http.MethodPut, "/"+name, "kef_nope", `{}`))
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rec.Code)
			}
		})
	}
}

func TestKEFArtRefusesNonRelativePaths(t *testing.T) {
	// The proxy exists for mixed content, not as a way to fetch arbitrary
	// URLs through the server.
	for _, u := range []string{
		"http://evil.example/x", "//evil.example/x", "/../../etc/passwd", "relative", "",
	} {
		srv, id := withKEFSpeaker(t)
		req := httptest.NewRequest(http.MethodGet, "/api/kef/"+id+"/art?u="+u, nil)
		req = mux.SetURLVars(req, map[string]string{"id": id})
		rec := httptest.NewRecorder()
		srv.kefArt(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("u=%q → %d, want 400", u, rec.Code)
		}
	}
}

func TestKEFArtURLOnlyProxiesRelativePaths(t *testing.T) {
	// KEF gets its artwork from the streaming service, so most URLs are
	// already absolute and must pass through untouched.
	abs := "https://art.example/cover.jpg"
	if got := KEFArtURL("kef_1", abs); got != abs {
		t.Errorf("absolute art URL was rewritten to %q", got)
	}
	got := KEFArtURL("kef 1", "/art?id=5")
	if !strings.HasPrefix(got, "/api/kef/kef%201/art?u=") {
		t.Errorf("relative art URL = %q, want it proxied with the id escaped", got)
	}
}

func TestKEFStatusListsRegisteredSpeakers(t *testing.T) {
	// The status endpoint must report an unreachable speaker rather than
	// omitting it: the row is how the user gets to fixing the address.
	srv, id := withKEFSpeaker(t)
	req := httptest.NewRequest(http.MethodGet, "/api/kef/status", nil)
	// The cache is cold, so the handler reads the speaker synchronously.
	// Cancelling first makes that read give up on the dial instead of
	// waiting out the timeout against an address nothing answers.
	ctx, cancel := contextCanceled(req)
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	srv.kefStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Speakers []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Reachable bool   `json:"reachable"`
		} `json:"speakers"`
		Warm bool `json:"warm"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Speakers) != 1 || body.Speakers[0].ID != id {
		t.Fatalf("speakers = %+v", body.Speakers)
	}
	if body.Speakers[0].Reachable {
		t.Error("an address with nothing behind it reported itself reachable")
	}
	if body.Speakers[0].Name != "Study" {
		t.Errorf("name = %q", body.Speakers[0].Name)
	}
}

func TestKEFCreateRejectsBadAddress(t *testing.T) {
	srv := testServer(t)
	for _, ip := range []string{"", "127.0.0.1", "192.168.1.5:80", "not a host"} {
		req := httptest.NewRequest(http.MethodPost, "/api/kef/speakers",
			strings.NewReader(`{"ip":"`+ip+`"}`))
		rec := httptest.NewRecorder()
		srv.kefCreateSpeaker(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("ip=%q → %d, want 400", ip, rec.Code)
		}
	}
	if len(srv.Store.KEF) != 0 {
		t.Error("a rejected address still registered a speaker")
	}
}

func TestKEFDeleteRemovesTheSpeaker(t *testing.T) {
	srv, id := withKEFSpeaker(t)
	rec := httptest.NewRecorder()
	srv.kefDeleteSpeaker(rec, kefRequest(t, http.MethodDelete, "", id, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (%s)", rec.Code, rec.Body.String())
	}
	if len(srv.Store.KEF) != 0 {
		t.Error("the speaker is still registered")
	}
	// Deleting it twice is a 404, not a second 204.
	rec = httptest.NewRecorder()
	srv.kefDeleteSpeaker(rec, kefRequest(t, http.MethodDelete, "", id, ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
}

func TestKEFUpdateRejectsADuplicateAddress(t *testing.T) {
	// Two registrations pointing at one speaker would poll it twice and show
	// it twice; the validator is what stops that.
	srv, id := withKEFSpeaker(t)
	srv.Store.KEF["kef_other"] = &store.KEFSpeaker{
		ID: "kef_other", Name: "Loft", IP: "192.0.2.21", MAC: "aabbccddeeff",
	}
	rec := httptest.NewRecorder()
	srv.kefUpdateSpeaker(rec, kefRequest(t, http.MethodPut, "", id, `{"ip":"192.0.2.21"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if srv.Store.KEF[id].IP != "192.0.2.20" {
		t.Error("the rejected edit was applied anyway")
	}
}
