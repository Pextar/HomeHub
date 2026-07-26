package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homehub/internal/spotify"
	"homehub/internal/store"
)

func devices() []spotify.Device {
	return []spotify.Device{
		{ID: "phone", Name: "Petter's iPhone", Type: "Smartphone"},
		{ID: "kef-1", Name: "Living  Room", Type: "Speaker"},
		{ID: "car", Name: "Car", Type: "Automobile", Restricted: true},
	}
}

// Which Connect device a speaker is decides which room the music starts in,
// so the rules are exact: a pin wins, then a name, and nothing beyond that.
func TestMatchConnectDevice(t *testing.T) {
	t.Run("name match ignores case and double spaces", func(t *testing.T) {
		sp := store.KEFSpeaker{Name: "living room"}
		d, err := matchConnectDevice(sp, devices())
		if err != nil || d.ID != "kef-1" {
			t.Fatalf("got %+v, %v; want kef-1", d, err)
		}
	})

	t.Run("a pin wins over the name", func(t *testing.T) {
		sp := store.KEFSpeaker{Name: "Living Room", SpotifyDeviceID: "phone"}
		d, err := matchConnectDevice(sp, devices())
		if err != nil || d.ID != "phone" {
			t.Fatalf("got %+v, %v; want the pinned device", d, err)
		}
	})

	t.Run("a pinned id that rotated falls back to the pinned name", func(t *testing.T) {
		sp := store.KEFSpeaker{
			Name:              "Study", // renamed in HomeHub, still "Living Room" to Spotify
			SpotifyDeviceID:   "kef-old",
			SpotifyDeviceName: "Living Room",
		}
		d, err := matchConnectDevice(sp, devices())
		if err != nil || d.ID != "kef-1" {
			t.Fatalf("got %+v, %v; want the device re-found by name", d, err)
		}
	})

	t.Run("nothing matching is a fixable state, and says how", func(t *testing.T) {
		sp := store.KEFSpeaker{ID: "kef_1", Name: "Study"}
		_, err := matchConnectDevice(sp, devices())
		if !errors.Is(err, errNoConnectDevice) {
			t.Fatalf("err = %v, want errNoConnectDevice", err)
		}
		if !strings.Contains(err.Error(), "Study") || !strings.Contains(err.Error(), "Spotify app") {
			t.Errorf("err = %q, want the speaker named and the fix spelled out", err)
		}
		if got := kefSpotifyStatus(err); got != http.StatusConflict {
			t.Errorf("status = %d, want 409 so the frontend can prompt", got)
		}
	})

	t.Run("a pin that is asleep reads differently from no match at all", func(t *testing.T) {
		sp := store.KEFSpeaker{Name: "Study", SpotifyDeviceID: "gone", SpotifyDeviceName: "Kitchen"}
		_, err := matchConnectDevice(sp, devices())
		if err == nil || !strings.Contains(err.Error(), "Kitchen") ||
			!strings.Contains(err.Error(), "wake") {
			t.Errorf("err = %v, want the pinned name and advice to wake it", err)
		}
	})

	t.Run("a restricted device is refused by name, not silently", func(t *testing.T) {
		sp := store.KEFSpeaker{Name: "Car"}
		_, err := matchConnectDevice(sp, devices())
		if err == nil || !strings.Contains(err.Error(), "Car") {
			t.Errorf("err = %v, want the device named", err)
		}
	})

	t.Run("an unnamed speaker matches nothing", func(t *testing.T) {
		if _, err := matchConnectDevice(store.KEFSpeaker{}, devices()); err == nil {
			t.Error("a speaker with no name must not match the first device on the list")
		}
	})
}

// Spotify-side failures the user can act on answer 409; a refusal from
// Spotify itself stays a bad gateway.
func TestKEFSpotifyStatusMapping(t *testing.T) {
	cases := map[error]int{
		spotify.ErrNotConnected:         http.StatusConflict,
		spotify.ErrPlaybackScope:        http.StatusConflict,
		errNoConnectDevice:              http.StatusConflict,
		errors.New("spotify: HTTP 500"): http.StatusBadGateway,
	}
	for err, want := range cases {
		if got := kefSpotifyStatus(err); got != want {
			t.Errorf("%v → %d, want %d", err, got, want)
		}
	}
}

// The body is rejected before anything is dialled: the speaker in these tests
// is unreachable, so a test that got past validation would stall, not fail.
func TestKEFPlayItemValidatesTheRequest(t *testing.T) {
	cases := map[string]string{
		"no uri":            `{"service":"Spotify"}`,
		"another service":   `{"service":"TuneIn","uri":"spotify:track:x"}`,
		"not even a bridge": `{"service":"Sonos","uri":"spotify:track:x"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv, id := withKEFSpeaker(t)
			rec := httptest.NewRecorder()
			srv.kefPlayItem(rec, kefRequest(t, http.MethodPost, "/play-item", id, body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// Without a Spotify client wired there is nothing to play through, and the
// speaker must not be woken to find that out.
func TestKEFPlayItemWithoutSpotify(t *testing.T) {
	srv, id := withKEFSpeaker(t)
	rec := httptest.NewRecorder()
	srv.kefPlayItem(rec, kefRequest(t, http.MethodPost, "/play-item", id,
		`{"service":"Spotify","uri":"spotify:track:x","title":"Song"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

// Pinning round-trips through the store, and an empty id clears it — going
// back to matching by name rather than leaving a stale pairing behind.
func TestKEFSetSpotifyDevicePinAndClear(t *testing.T) {
	srv, id := withKEFSpeaker(t)

	rec := httptest.NewRecorder()
	srv.kefSetSpotifyDevice(rec, kefRequest(t, http.MethodPut, "/spotify", id,
		`{"device_id":"kef-1","device_name":"Living Room"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if sp := srv.Store.KEF[id]; sp.SpotifyDeviceID != "kef-1" || sp.SpotifyDeviceName != "Living Room" {
		t.Fatalf("stored %+v, want the pin", sp)
	}

	rec = httptest.NewRecorder()
	srv.kefSetSpotifyDevice(rec, kefRequest(t, http.MethodPut, "/spotify", id, `{"device_id":""}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("clear → %d, want 200", rec.Code)
	}
	if sp := srv.Store.KEF[id]; sp.SpotifyDeviceID != "" || sp.SpotifyDeviceName != "" {
		t.Errorf("stored %+v, want the pin cleared", sp)
	}
	// Clearing must not disturb the registration itself.
	if sp := srv.Store.KEF[id]; sp.Name != "Study" || sp.IP != "192.0.2.20" {
		t.Errorf("stored %+v, want name and address untouched", sp)
	}
}

func TestKEFSetSpotifyDeviceUnknownSpeaker(t *testing.T) {
	srv := testServer(t)
	rec := httptest.NewRecorder()
	srv.kefSetSpotifyDevice(rec, kefRequest(t, http.MethodPut, "/spotify", "kef_nope", `{"device_id":"x"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
