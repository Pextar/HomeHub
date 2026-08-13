package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homehub/internal/media"
	"homehub/internal/spotify"
	"homehub/internal/store"
)

// A server with no Spotify client at all is the state most of these handlers
// have to survive: the integration is optional everywhere else, and this
// surface must not be the one that panics.
func TestConnectHandlersSurviveNoSpotify(t *testing.T) {
	srv := testServer(t)
	for _, tc := range []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
		req  *http.Request
	}{
		{"list", srv.spotifyConnect,
			httptest.NewRequest(http.MethodGet, "/api/spotify/connect", nil)},
		{"transfer", srv.spotifyConnectTransfer,
			httptest.NewRequest(http.MethodPut, "/api/spotify/connect/transfer",
				strings.NewReader(`{"device_id":"d1"}`))},
		{"volume", srv.spotifyConnectVolume,
			httptest.NewRequest(http.MethodPut, "/api/spotify/connect/volume",
				strings.NewReader(`{"device_id":"d1","level":40}`))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.call(rec, tc.req)
			if rec.Code < 400 {
				t.Errorf("status = %d, want a refusal rather than a nil deref", rec.Code)
			}
		})
	}
}

func TestConnectTransferValidatesItsBody(t *testing.T) {
	srv := testServer(t)
	srv.Spotify = spotifyClient(t)

	for _, body := range []string{`{}`, `{"device_id":"   "}`} {
		rec := httptest.NewRecorder()
		srv.spotifyConnectTransfer(rec, httptest.NewRequest(http.MethodPut,
			"/api/spotify/connect/transfer", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s → %d, want 400", body, rec.Code)
		}
	}
}

func TestConnectVolumeRequiresALevel(t *testing.T) {
	srv := testServer(t)
	srv.Spotify = spotifyClient(t)

	rec := httptest.NewRecorder()
	srv.spotifyConnectVolume(rec, httptest.NewRequest(http.MethodPut,
		"/api/spotify/connect/volume", strings.NewReader(`{"device_id":"d1"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	// Zero is a real level — silence — and must not be read as "absent",
	// which is why the field is a pointer.
	if strings.Contains(rec.Body.String(), "device_id") {
		t.Errorf("the missing field is the level, not the device: %s", rec.Body)
	}
}

// An account linked before HomeHub asked for the player scope can search but
// cannot touch this screen. The answer has to be the sentence that fixes it.
func TestConnectNeedsThePlayerScope(t *testing.T) {
	srv := testServer(t)
	srv.Spotify = spotifyClient(t)

	rec := httptest.NewRecorder()
	srv.spotifyConnect(rec, httptest.NewRequest(http.MethodGet, "/api/spotify/connect", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "econnect") { // "reconnect"/"Reconnect"
		t.Errorf("the error should say how to fix it: %s", rec.Body)
	}
}

// The picker's job is to be honest about which of these devices are already
// something HomeHub knows: its own decoder, and the speakers a KEF pin has
// already matched. Both would otherwise look like strangers with familiar
// names.
func TestDescribeConnectDevicesNamesWhatHomeHubKnows(t *testing.T) {
	srv := testServer(t)
	srv.Store.KEF["kef_1"] = &store.KEFSpeaker{
		ID: "kef_1", Name: "Study", IP: "192.0.2.20", MAC: "a1b2c3d4e5f6",
		SpotifyDeviceID: "kef-connect-id",
	}
	srv.Store.KEF["kef_2"] = &store.KEFSpeaker{
		ID: "kef_2", Name: "Kitchen", IP: "192.0.2.21", MAC: "b1b2c3d4e5f6",
	}
	// Creating the decoder is what gives it a name to match against.
	_ = srv.decoder()

	got := srv.describeConnectDevices([]spotify.Device{
		{ID: "kef-connect-id", Name: "LSX II", Type: "Speaker"},
		{ID: "other", Name: "kitchen", Type: "Speaker"}, // matched by name
		{ID: "phone", Name: "Petter's iPhone", Type: "Smartphone"},
		{ID: "hub", Name: srv.decoderDeviceName(), Type: "Speaker"},
	})
	if len(got) != 4 {
		t.Fatalf("got %d devices", len(got))
	}
	if got[0].Speaker != "Study" {
		t.Errorf("a pinned device should name its speaker, got %q", got[0].Speaker)
	}
	if got[1].Speaker != "Kitchen" {
		t.Errorf("a name match should name its speaker, got %q", got[1].Speaker)
	}
	if got[2].Speaker != "" || got[2].HomeHub {
		t.Errorf("a phone is nobody's speaker: %+v", got[2])
	}
	if !got[3].HomeHub {
		t.Error("HomeHub's own decoder should be marked as itself")
	}
}

// Moving the session is only a warning when HomeHub is holding it. A Sonos
// streaming from its own account link keeps playing whatever a phone does, and
// warning about that would be a lie in the other direction.
func TestInterruptsOnlyNamesRoomsHomeHubIsFeeding(t *testing.T) {
	srv := testServer(t)
	_ = srv.decoder()
	hub := srv.decoderDeviceName()

	srv.Store.Zones["z1"] = &store.Zone{ID: "z1", Name: "Downstairs"}
	srv.setZoneSession("z1", &media.Session{Route: media.RouteAirPlay})

	if got := srv.connectInterrupts(&spotify.Playback{DeviceName: hub}); got != "Downstairs" {
		t.Errorf("interrupts = %q, want the zone HomeHub is feeding", got)
	}
	// The session is on a phone: nothing of HomeHub's is at risk.
	if got := srv.connectInterrupts(&spotify.Playback{DeviceName: "Someone's iPhone"}); got != "" {
		t.Errorf("interrupts = %q, want nothing", got)
	}
	// Nothing playing at all.
	if got := srv.connectInterrupts(nil); got != "" {
		t.Errorf("interrupts = %q, want nothing", got)
	}

	// A natively grouped zone does not hold the account's session, so it is
	// not at risk either.
	srv.Store.Zones["z2"] = &store.Zone{ID: "z2", Name: "Living Room"}
	srv.setZoneSession("z2", &media.Session{Route: media.RouteNative})
	got := srv.connectInterrupts(&spotify.Playback{DeviceName: hub})
	if strings.Contains(got, "Living Room") {
		t.Errorf("a native zone is not interrupted by a transfer: %q", got)
	}
}

// After the session moves away, HomeHub must stop claiming the rooms it was
// feeding — otherwise the Music view shows a stream nobody is receiving.
func TestReleaseDecodedZonesEndsOnlyTheDecodedOnes(t *testing.T) {
	srv := testServer(t)
	srv.setZoneSession("streamed", &media.Session{Route: media.RouteStream})
	srv.setZoneSession("cast", &media.Session{Route: media.RouteAirPlay})
	srv.setZoneSession("native", &media.Session{Route: media.RouteNative})

	srv.releaseDecodedZones()

	srv.zoneMu.Lock()
	defer srv.zoneMu.Unlock()
	if _, ok := srv.zoneSessions["streamed"]; ok {
		t.Error("the streamed zone should have been released")
	}
	if _, ok := srv.zoneSessions["cast"]; ok {
		t.Error("the cast zone should have been released")
	}
	if _, ok := srv.zoneSessions["native"]; !ok {
		t.Error("a natively played zone keeps playing and keeps its session")
	}
}

// spotifyClient is a configured-but-unconnected client: enough for the
// handlers to get past their nil checks and stop at the account state, which
// is where the interesting refusals live.
func spotifyClient(t *testing.T) *spotify.Client {
	t.Helper()
	c, err := spotify.New(t.TempDir())
	if err != nil {
		t.Fatalf("spotify client: %v", err)
	}
	if err := c.SetClientID("test-client-id"); err != nil {
		t.Fatalf("client id: %v", err)
	}
	return c
}

// The wire shape the frontend reads, pinned: a client that guessed at these
// names would fail silently rather than at compile time.
func TestConnectResponseShape(t *testing.T) {
	srv := testServer(t)
	devices := srv.describeConnectDevices([]spotify.Device{
		{ID: "d1", Name: "Phone", Type: "Smartphone", Active: true, Volume: 50},
	})
	raw, err := json.Marshal(map[string]any{
		"devices": devices, "playing": nil, "interrupts": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id":"d1"`, `"active":true`, `"homehub":false`, `"volume":50`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("response is missing %s: %s", want, raw)
		}
	}
}
