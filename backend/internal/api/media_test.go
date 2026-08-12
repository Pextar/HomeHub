package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/media"
	"homehub/internal/store"
)

// withSpeakers registers one Sonos and one KEF at addresses that cannot
// answer, so any test that reached the network would stall rather than fail
// quietly. Everything below must resolve before touching a speaker.
func withSpeakers(t *testing.T) *Server {
	t.Helper()
	srv := testServer(t)
	srv.Store.Sonos["son_1"] = &store.SonosSpeaker{
		ID: "son_1", Name: "Living Room", IP: "192.0.2.10", UUID: "RINCON_TEST01",
	}
	srv.Store.KEF["kef_1"] = &store.KEFSpeaker{
		ID: "kef_1", Name: "Study", IP: "192.0.2.20", MAC: "a1b2c3d4e5f6",
	}
	return srv
}

func zoneRequest(t *testing.T, method, path, id, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if id != "" {
		req = mux.SetURLVars(req, map[string]string{"id": id})
	}
	return req
}

// createZone posts a zone and returns its id.
func createZone(t *testing.T, srv *Server, body string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mediaCreateZone(rec, zoneRequest(t, http.MethodPost, "/api/media/zones", "", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating zone: %d %s", rec.Code, rec.Body.String())
	}
	var z store.Zone
	if err := json.Unmarshal(rec.Body.Bytes(), &z); err != nil {
		t.Fatalf("decoding zone: %v", err)
	}
	return z.ID
}

func TestMediaEndpointsReportsCapabilities(t *testing.T) {
	srv := withSpeakers(t)
	rec := httptest.NewRecorder()
	srv.mediaEndpoints(rec, httptest.NewRequest(http.MethodGet, "/api/media/endpoints", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var out []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Vendor string   `json:"vendor"`
		Caps   []string `json:"capabilities"`
		Member string   `json:"member"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d endpoints, want 2", len(out))
	}

	byName := map[string]int{}
	for i, e := range out {
		byName[e.Name] = i
	}
	sonosIdx, ok := byName["Living Room"]
	if !ok {
		t.Fatal("Sonos speaker missing from the endpoint list")
	}
	kefIdx, ok := byName["Study"]
	if !ok {
		t.Fatal("KEF speaker missing from the endpoint list")
	}

	// Capabilities are names, not bit values — the frontend keys off these
	// strings, so a rename here is a breaking change and should fail loudly.
	if !hasCap(out[sonosIdx].Caps, "native_service") {
		t.Errorf("Sonos should declare native_service, got %v", out[sonosIdx].Caps)
	}
	if !hasCap(out[sonosIdx].Caps, "group") {
		t.Errorf("Sonos should declare group, got %v", out[sonosIdx].Caps)
	}
	if hasCap(out[kefIdx].Caps, "group") {
		t.Errorf("KEF must not declare group, got %v", out[kefIdx].Caps)
	}
	if !hasCap(out[kefIdx].Caps, "connect") || !hasCap(out[kefIdx].Caps, "wake") {
		t.Errorf("KEF should declare connect and wake, got %v", out[kefIdx].Caps)
	}

	// The member id is what a zone stores, and it has to be bridge-qualified
	// or two speakers with the same bare id would be indistinguishable.
	if out[sonosIdx].Member != "sonos:son_1" {
		t.Errorf("Sonos member = %q, want sonos:son_1", out[sonosIdx].Member)
	}
	if out[kefIdx].Member != "kef:kef_1" {
		t.Errorf("KEF member = %q, want kef:kef_1", out[kefIdx].Member)
	}
}

func hasCap(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

func TestCreateZoneValidatesMembers(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"valid mixed zone", `{"name":"Downstairs","members":["sonos:son_1","kef:kef_1"]}`, http.StatusCreated},
		{"valid single", `{"name":"Just Sonos","members":["sonos:son_1"]}`, http.StatusCreated},
		// An empty zone is allowed: a user clearing members in the UI is
		// still editing, and refusing would lose the name they typed.
		{"empty members", `{"name":"Empty","members":[]}`, http.StatusCreated},
		{"no name", `{"name":"","members":["sonos:son_1"]}`, http.StatusBadRequest},
		{"unknown speaker", `{"name":"Ghost","members":["sonos:nope"]}`, http.StatusBadRequest},
		// An unqualified id is ambiguous between the bridges and must be
		// rejected rather than guessed at.
		{"unqualified member", `{"name":"Bare","members":["son_1"]}`, http.StatusBadRequest},
		{"unknown bridge", `{"name":"Alien","members":["bose:x"]}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := withSpeakers(t)
			rec := httptest.NewRecorder()
			srv.mediaCreateZone(rec, zoneRequest(t, http.MethodPost, "/api/media/zones", "", tc.body))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestCreateZoneRejectsDuplicateName(t *testing.T) {
	srv := withSpeakers(t)
	createZone(t, srv, `{"name":"Downstairs","members":["sonos:son_1"]}`)

	rec := httptest.NewRecorder()
	srv.mediaCreateZone(rec, zoneRequest(t, http.MethodPost, "/api/media/zones", "",
		`{"name":"downstairs","members":["kef:kef_1"]}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a duplicate name", rec.Code)
	}
}

func TestZoneDeduplicatesMembers(t *testing.T) {
	srv := withSpeakers(t)
	id := createZone(t, srv,
		`{"name":"Dupes","members":["sonos:son_1","sonos:son_1","kef:kef_1"]}`)

	srv.Store.Mu.RLock()
	members := srv.Store.Zones[id].Members
	srv.Store.Mu.RUnlock()

	// A speaker listed twice would be sent every command twice and, on the
	// group route, told to join a group it already leads.
	if len(members) != 2 {
		t.Errorf("members = %v, want the duplicate collapsed", members)
	}
}

func TestUpdateZoneReplacesMembers(t *testing.T) {
	srv := withSpeakers(t)
	id := createZone(t, srv, `{"name":"Downstairs","members":["sonos:son_1","kef:kef_1"]}`)

	rec := httptest.NewRecorder()
	srv.mediaUpdateZone(rec, zoneRequest(t, http.MethodPut, "/api/media/zones/"+id, id,
		`{"name":"Downstairs","members":["sonos:son_1"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	srv.Store.Mu.RLock()
	members := srv.Store.Zones[id].Members
	srv.Store.Mu.RUnlock()
	// Wholesale replacement, not a merge — there'd be no way to express a
	// removal otherwise.
	if len(members) != 1 || members[0] != "sonos:son_1" {
		t.Errorf("members = %v, want just the Sonos", members)
	}
}

func TestDeleteZone(t *testing.T) {
	srv := withSpeakers(t)
	id := createZone(t, srv, `{"name":"Gone","members":["sonos:son_1"]}`)

	rec := httptest.NewRecorder()
	srv.mediaDeleteZone(rec, zoneRequest(t, http.MethodDelete, "/api/media/zones/"+id, id, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.mediaDeleteZone(rec, zoneRequest(t, http.MethodDelete, "/api/media/zones/"+id, id, ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
}

// TestDeletingSpeakerCascadesToZones is the sibling of CascadeDeleteSocket.
// A dangling member would fail validation on the next unrelated edit of that
// zone, presenting as "no such speaker" for a change the user didn't make.
func TestDeletingSpeakerCascadesToZones(t *testing.T) {
	srv := withSpeakers(t)
	id := createZone(t, srv, `{"name":"Downstairs","members":["sonos:son_1","kef:kef_1"]}`)

	rec := httptest.NewRecorder()
	srv.kefDeleteSpeaker(rec, zoneRequest(t, http.MethodDelete, "/api/kef/speakers/kef_1", "kef_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("deleting speaker = %d: %s", rec.Code, rec.Body.String())
	}

	srv.Store.Mu.RLock()
	members := srv.Store.Zones[id].Members
	srv.Store.Mu.RUnlock()
	if len(members) != 1 || members[0] != "sonos:son_1" {
		t.Errorf("members = %v, want the deleted speaker dropped", members)
	}
}

// TestZoneRoutesExplainsTheChoice is what lets the UI be honest before a user
// taps play, so the reasoning has to survive to the response.
func TestZoneRoutesExplainsTheChoice(t *testing.T) {
	srv := withSpeakers(t)

	t.Run("sonos only reports a native route", func(t *testing.T) {
		id := createZone(t, srv, `{"name":"Sonos Only","members":["sonos:son_1"]}`)
		rec := httptest.NewRecorder()
		srv.mediaZoneRoutes(rec, zoneRequest(t, http.MethodGet, "/api/media/zones/"+id+"/routes", id, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		// Spotify is unconfigured in tests, so no route resolves and the
		// response must explain that rather than silently offering one.
		if out["route"] == nil && out["problem"] == nil {
			t.Errorf("response says neither which route nor why not: %s", rec.Body.String())
		}
	})

	t.Run("missing zone is 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.mediaZoneRoutes(rec, zoneRequest(t, http.MethodGet, "/api/media/zones/nope/routes", "nope", ""))
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

// TestPlayToEmptyZoneIsRejected covers the split between what may be stored
// and what may be played: an empty zone is a legal thing to have and an
// illegal thing to play to.
func TestPlayToEmptyZoneIsRejected(t *testing.T) {
	srv := withSpeakers(t)
	id := createZone(t, srv, `{"name":"Empty","members":[]}`)

	rec := httptest.NewRecorder()
	srv.mediaZonePlay(rec, zoneRequest(t, http.MethodPost, "/api/media/zones/"+id+"/play", id,
		`{"provider":"spotify","uri":"spotify:track:x","title":"T"}`))
	if rec.Code == http.StatusOK || rec.Code == http.StatusNoContent {
		t.Errorf("playing to an empty zone succeeded (%d)", rec.Code)
	}
}

func TestPlayRequiresURI(t *testing.T) {
	srv := withSpeakers(t)
	id := createZone(t, srv, `{"name":"Z","members":["sonos:son_1"]}`)

	rec := httptest.NewRecorder()
	srv.mediaZonePlay(rec, zoneRequest(t, http.MethodPost, "/api/media/zones/"+id+"/play", id,
		`{"provider":"spotify","uri":"","title":"T"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestPlayWithUnconfiguredSpotifyIsActionable checks the status mapping: an
// unconfigured account is something the user fixes, so it must be a 409 the
// frontend can prompt on rather than a 502 that reads as a broken server.
func TestPlayWithUnconfiguredSpotifyIsActionable(t *testing.T) {
	srv := withSpeakers(t)
	id := createZone(t, srv, `{"name":"Z","members":["sonos:son_1"]}`)

	rec := httptest.NewRecorder()
	srv.mediaZonePlay(rec, zoneRequest(t, http.MethodPost, "/api/media/zones/"+id+"/play", id,
		`{"provider":"spotify","uri":"spotify:track:x","title":"T"}`))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 for an account the user can connect", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Error("the response must say what to do about it")
	}
}

func TestPlayRejectsUnknownProvider(t *testing.T) {
	srv := withSpeakers(t)
	id := createZone(t, srv, `{"name":"Z","members":["sonos:son_1"]}`)

	rec := httptest.NewRecorder()
	srv.mediaZonePlay(rec, zoneRequest(t, http.MethodPost, "/api/media/zones/"+id+"/play", id,
		`{"provider":"tidal","uri":"x","title":"T"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestMediaSearchRequiresQuery(t *testing.T) {
	srv := withSpeakers(t)
	rec := httptest.NewRecorder()
	srv.mediaSearch(rec, httptest.NewRequest(http.MethodGet, "/api/media/search?q=", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestMediaProvidersReportsStreamingSeparately matters because "you can't
// search" and "you can't play to mixed speakers" are different problems with
// different fixes, and collapsing them would mislead.
func TestMediaProvidersReportsStreamingSeparately(t *testing.T) {
	srv := withSpeakers(t)
	rec := httptest.NewRecorder()
	srv.mediaProviders(rec, httptest.NewRequest(http.MethodGet, "/api/media/providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var out []struct {
		ID        string             `json:"id"`
		Routes    []string           `json:"routes"`
		Avail     media.Availability `json:"availability"`
		Streaming media.Availability `json:"streaming"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(out) != 1 || out[0].ID != "spotify" {
		t.Fatalf("got %+v, want one spotify provider", out)
	}
	if len(out[0].Routes) != 5 {
		t.Errorf("routes = %v, want every route", out[0].Routes)
	}
	// Neither is available in a test server, and both must say why.
	if out[0].Avail.Reason == "" {
		t.Error("an unavailable provider must carry a reason")
	}
	if out[0].Streaming.Reason == "" {
		t.Error("unavailable streaming must carry its own reason")
	}
}

// TestZonesListIncludesLiveState exercises the read path end to end. The
// speakers are unreachable, so this also pins that an unreachable zone still
// renders rather than failing the whole request.
//
// The request carries a short deadline of its own. The handler budgets
// mediaTimeout for a zone read, and reaching a speaker at an unroutable
// address costs whatever the host's network does with the packets — a fast
// refusal on a developer's machine, a silent blackhole on a CI runner, where
// the difference is minutes of a hung test rather than a failure. Since
// context.WithTimeout takes the earlier of the two deadlines, capping it here
// makes the test measure what it is about — that the zone renders without
// live state — instead of measuring the network.
func TestZonesListIncludesLiveState(t *testing.T) {
	srv := withSpeakers(t)
	createZone(t, srv, `{"name":"Downstairs","members":["sonos:son_1"]}`)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	rec := httptest.NewRecorder()
	srv.mediaZones(rec, httptest.NewRequest(http.MethodGet, "/api/media/zones", nil).WithContext(ctx))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var out []struct {
		Name     string `json:"name"`
		Speakers []struct {
			Member string `json:"member"`
			Name   string `json:"name"`
		} `json:"speakers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(out) != 1 || out[0].Name != "Downstairs" {
		t.Fatalf("got %+v, want one zone", out)
	}
	if len(out[0].Speakers) != 1 || out[0].Speakers[0].Name != "Living Room" {
		t.Errorf("speakers = %+v, want the Sonos named", out[0].Speakers)
	}
}

// TestStreamHandlerBeforeAnyPlaybackIs404 pins the honest answer for a stream
// id that was never published — the same one an expired id gets.
func TestStreamHandlerBeforeAnyPlaybackIs404(t *testing.T) {
	srv := withSpeakers(t)
	rec := httptest.NewRecorder()
	srv.streamHandler().ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, streamPath+"/deadbeef", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
