package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"homehub/internal/media"
	"homehub/internal/store"
)

// withReceiver registers an AirPlay receiver at an address that cannot answer.
// Nothing below should reach it: registration is the only handler that probes,
// and it is tested through the failure that produces.
func withReceiver(t *testing.T) *Server {
	t.Helper()
	srv := testServer(t)
	srv.Store.AirPlay["ap_1"] = &store.AirPlaySpeaker{
		ID: "ap_1", Name: "Study Pi", IP: "192.0.2.30", Port: 7000,
		DeviceID: "b827eb1234ab", Model: "ShairportSync",
		PCM: true, ALAC: true, Metadata: true, Volume: 35,
	}
	return srv
}

func TestAirPlayStatusListsRegisteredReceivers(t *testing.T) {
	srv := withReceiver(t)
	rec := httptest.NewRecorder()
	srv.airplayStatus(rec, httptest.NewRequest(http.MethodGet, "/api/airplay/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}

	var out []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Member  string `json:"member"`
		Casting bool   `json:"casting"`
		Volume  int    `json:"volume"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d receivers", len(out))
	}
	// The qualified id is what a zone stores, and it is carried so the
	// Music view never has to build one from a prefix it knows by heart.
	if out[0].Member != "airplay:ap_1" {
		t.Errorf("member = %q", out[0].Member)
	}
	if out[0].Casting {
		t.Error("nothing is casting on a fresh server")
	}
	if out[0].Volume != 35 {
		t.Errorf("volume = %d, want the stored level", out[0].Volume)
	}
}

// A receiver appears in the uniform endpoint list with the capabilities that
// decide its route — and, just as importantly, without the ones it lacks.
func TestAirPlayReceiverJoinsTheEndpointList(t *testing.T) {
	srv := withReceiver(t)
	rec := httptest.NewRecorder()
	srv.mediaEndpoints(rec, httptest.NewRequest(http.MethodGet, "/api/media/endpoints", nil))

	var out []struct {
		Name   string   `json:"name"`
		Vendor string   `json:"vendor"`
		Member string   `json:"member"`
		Caps   []string `json:"capabilities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	var found bool
	for _, e := range out {
		if e.Member != "airplay:ap_1" {
			continue
		}
		found = true
		if e.Vendor != "airplay" {
			t.Errorf("vendor = %q", e.Vendor)
		}
		caps := strings.Join(e.Caps, ",")
		if !strings.Contains(caps, "airplay") {
			t.Errorf("capabilities = %v, want airplay", e.Caps)
		}
		// The absences are load-bearing: a receiver claiming play_uri
		// would be handed a URL it cannot fetch.
		for _, forbidden := range []string{"play_uri", "native_service", "group", "queue", "seek"} {
			if strings.Contains(caps, forbidden) {
				t.Errorf("a receiver must not claim %q: %v", forbidden, e.Caps)
			}
		}
	}
	if !found {
		t.Fatal("the receiver is missing from the endpoint list")
	}
}

// A zone of receivers resolves to the AirPlay route, and the zone view says so
// before anything is played.
func TestZoneOfReceiversTakesTheAirPlayRoute(t *testing.T) {
	srv := withReceiver(t)
	srv.Store.AirPlay["ap_2"] = &store.AirPlaySpeaker{
		ID: "ap_2", Name: "Kitchen Pi", IP: "192.0.2.31", Port: 7000, ALAC: true,
	}
	id := createZone(t, srv, `{"name":"Upstairs","members":["airplay:ap_1","airplay:ap_2"]}`)

	rec := httptest.NewRecorder()
	srv.mediaZones(rec, httptest.NewRequest(http.MethodGet, "/api/media/zones", nil))

	var out []struct {
		ID      string       `json:"id"`
		Route   media.Route  `json:"route"`
		Sync    media.Sync   `json:"sync"`
		Reason  string       `json:"reason"`
		Problem string       `json:"problem"`
		Quality *media.Chain `json:"quality"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	var zone *struct {
		ID      string       `json:"id"`
		Route   media.Route  `json:"route"`
		Sync    media.Sync   `json:"sync"`
		Reason  string       `json:"reason"`
		Problem string       `json:"problem"`
		Quality *media.Chain `json:"quality"`
	}
	for i := range out {
		if out[i].ID == id {
			zone = &out[i]
		}
	}
	if zone == nil {
		t.Fatal("the zone is missing from the list")
	}

	// Spotify is unconfigured in a test server, so the stream route (and
	// with it AirPlay) reports unavailable — which is itself the thing to
	// assert: the reason must be the decoder's, not the speakers'.
	if zone.Problem != "" {
		if !strings.Contains(zone.Problem, "Spotify") {
			t.Errorf("problem should name the service that can't be reached: %q", zone.Problem)
		}
		return
	}
	if zone.Route != media.RouteAirPlay {
		t.Errorf("route = %q, want airplay", zone.Route)
	}
	if zone.Sync != media.SyncClocked {
		t.Errorf("sync = %q, want clocked", zone.Sync)
	}
	if zone.Quality == nil {
		t.Error("a resolvable zone should say what it will sound like")
	}
}

func TestAirPlayCreateRequiresAReachableReceiver(t *testing.T) {
	srv := testServer(t)
	rec := httptest.NewRecorder()
	srv.airplayCreateSpeaker(rec, httptest.NewRequest(http.MethodPost,
		"/api/airplay/speakers", strings.NewReader(`{"ip":"192.0.2.99","name":"Ghost"}`)))
	// Nothing answers at TEST-NET-1, so registration fails rather than
	// storing a receiver that can never play.
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502: %s", rec.Code, rec.Body)
	}
	if len(srv.Store.AirPlay) != 0 {
		t.Error("a receiver that never answered must not be registered")
	}
}

func TestAirPlayCreateRejectsAnUnsafeAddress(t *testing.T) {
	srv := testServer(t)
	rec := httptest.NewRecorder()
	srv.airplayCreateSpeaker(rec, httptest.NewRequest(http.MethodPost,
		"/api/airplay/speakers", strings.NewReader(`{"ip":"127.0.0.1","name":"Local"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
}

func TestAirPlayUpdateAndDelete(t *testing.T) {
	srv := withReceiver(t)

	rec := httptest.NewRecorder()
	req := mux.SetURLVars(httptest.NewRequest(http.MethodPut, "/api/airplay/speakers/ap_1",
		strings.NewReader(`{"name":"Study","room":"Study"}`)), map[string]string{"id": "ap_1"})
	srv.airplayUpdateSpeaker(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body)
	}
	if got := srv.Store.AirPlay["ap_1"]; got.Name != "Study" || got.Room != "Study" {
		t.Errorf("after update: %+v", got)
	}
	// What the receiver said it accepts is the receiver's answer, not the
	// household's, and an update must not quietly clear it.
	if !srv.Store.AirPlay["ap_1"].PCM {
		t.Error("an update should not drop the advertised codecs")
	}

	id := createZone(t, srv, `{"name":"Study zone","members":["airplay:ap_1"]}`)
	rec = httptest.NewRecorder()
	req = mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/api/airplay/speakers/ap_1", nil),
		map[string]string{"id": "ap_1"})
	srv.airplayDeleteSpeaker(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body)
	}
	if len(srv.Store.Zones[id].Members) != 0 {
		t.Errorf("the deleted receiver should be gone from its zone: %v",
			srv.Store.Zones[id].Members)
	}
}

// Volume is stored even with nothing playing, because a receiver only takes a
// level inside a session and the stored one is what the next cast opens with.
func TestAirPlayVolumeIsRememberedWhenNothingIsCasting(t *testing.T) {
	srv := withReceiver(t)
	rec := httptest.NewRecorder()
	req := mux.SetURLVars(httptest.NewRequest(http.MethodPut, "/api/airplay/ap_1/volume",
		strings.NewReader(`{"level":72}`)), map[string]string{"id": "ap_1"})
	srv.airplaySetVolume(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := srv.Store.AirPlay["ap_1"].Volume; got != 72 {
		t.Errorf("volume = %d, want 72", got)
	}

	rec = httptest.NewRecorder()
	req = mux.SetURLVars(httptest.NewRequest(http.MethodPut, "/api/airplay/ap_1/volume",
		strings.NewReader(`{"level":140}`)), map[string]string{"id": "ap_1"})
	srv.airplaySetVolume(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an impossible level should be refused, got %d", rec.Code)
	}
}

// A zone volume change must not fail because a receiver in the room is idle,
// and the level must not be lost either: it is what the next cast opens with.
func TestZoneVolumeRemembersIdleReceivers(t *testing.T) {
	srv := withReceiver(t)
	id := createZone(t, srv, `{"name":"Study zone","members":["airplay:ap_1"]}`)

	rec := httptest.NewRecorder()
	srv.mediaZoneVolume(rec, zoneRequest(t, http.MethodPut,
		"/api/media/zones/"+id+"/volume", id, `{"level":58}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := srv.Store.AirPlay["ap_1"].Volume; got != 58 {
		t.Errorf("stored volume = %d, want 58", got)
	}
}
