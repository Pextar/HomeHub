package spotify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const playerScope = "user-read-playback-state user-modify-playback-state"

// The body Spotify sends for a phone playing a track, trimmed to the fields
// this package reads.
const playingBody = `{
  "device": {"id":"dev1","is_active":true,"name":"Petter's iPhone","type":"Smartphone","volume_percent":63},
  "is_playing": true,
  "progress_ms": 42000,
  "item": {
    "name":"Kaos","uri":"spotify:track:1","duration_ms":215000,
    "artists":[{"name":"Familjen"}],
    "album":{"name":"Det snurrar i min skalle","images":[{"url":"http://art/1.jpg"}]}
  }
}`

func TestPlaybackReadsTheSession(t *testing.T) {
	c := connected(t, playerScope, func(r *http.Request) *http.Response {
		if !strings.Contains(r.URL.Path, "/me/player") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		return jsonResponse(200, playingBody)
	})

	pb, err := c.Playback(context.Background())
	if err != nil {
		t.Fatalf("playback: %v", err)
	}
	if pb == nil {
		t.Fatal("want a session")
	}
	if pb.DeviceID != "dev1" || pb.DeviceName != "Petter's iPhone" {
		t.Errorf("device = %q/%q", pb.DeviceID, pb.DeviceName)
	}
	if !pb.Playing || pb.ProgressMS != 42000 || pb.DurationMS != 215000 {
		t.Errorf("state = %+v", pb)
	}
	if pb.Volume != 63 {
		t.Errorf("volume = %d", pb.Volume)
	}
	if pb.Item == nil || pb.Item.Name != "Kaos" || pb.Item.Sub != "Familjen" {
		t.Errorf("item = %+v", pb.Item)
	}
	if pb.At.IsZero() {
		t.Error("a reading must say when it was taken")
	}
}

// Nothing playing anywhere is a 204 with no body. It is the state an idle
// household is in most of the day, so it must read as "nothing", not as a
// failure — and certainly not as a JSON parse error.
func TestPlaybackTreatsNothingPlayingAsNothing(t *testing.T) {
	c := connected(t, playerScope, func(r *http.Request) *http.Response {
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}
	})

	pb, err := c.Playback(context.Background())
	if err != nil {
		t.Fatalf("an idle account is not an error: %v", err)
	}
	if pb != nil {
		t.Errorf("want nil, got %+v", pb)
	}
}

// A device with no volume of its own reports -1 rather than 0, so a client
// cannot render "no slider" as "silent".
func TestPlaybackDistinguishesNoVolumeFromZero(t *testing.T) {
	c := connected(t, playerScope, func(r *http.Request) *http.Response {
		return jsonResponse(200, `{"device":{"id":"tv","name":"Telly","volume_percent":null},
			"is_playing":false}`)
	})
	pb, err := c.Playback(context.Background())
	if err != nil || pb == nil {
		t.Fatalf("playback: %v", err)
	}
	if pb.Volume != -1 {
		t.Errorf("volume = %d, want -1 for a device with none", pb.Volume)
	}
}

func TestTransferMovesTheSession(t *testing.T) {
	var body map[string]any
	c := connected(t, playerScope, func(r *http.Request) *http.Response {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/me/player" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		return jsonResponse(204, "")
	})

	if err := c.Transfer(context.Background(), "dev2", true); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	ids, _ := body["device_ids"].([]any)
	if len(ids) != 1 || ids[0] != "dev2" {
		t.Errorf("device_ids = %v", body["device_ids"])
	}
	if body["play"] != true {
		t.Errorf("play = %v, want true", body["play"])
	}
}

// A device that answers 202 is awake but not ready — the same "took a second"
// case PlayOn handles, and worth one retry rather than an error the user
// cannot act on.
func TestTransferRetriesADeviceThatIsNotReadyYet(t *testing.T) {
	var calls int
	c := connected(t, playerScope, func(r *http.Request) *http.Response {
		calls++
		if calls == 1 {
			return jsonResponse(202, "")
		}
		return jsonResponse(204, "")
	})

	if err := c.Transfer(context.Background(), "dev2", true); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want a retry", calls)
	}
}

func TestTransferGivesUpWithSomethingActionable(t *testing.T) {
	c := connected(t, playerScope, func(r *http.Request) *http.Response {
		return jsonResponse(202, "")
	})
	err := c.Transfer(context.Background(), "dev2", true)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "wake it") {
		t.Errorf("error should say what to do: %v", err)
	}
}

func TestTransferNeedsADevice(t *testing.T) {
	c := connected(t, playerScope, func(r *http.Request) *http.Response {
		t.Error("nothing should reach the network")
		return jsonResponse(500, "")
	})
	if err := c.Transfer(context.Background(), "  ", true); err == nil {
		t.Error("an empty device id is a caller bug, not a request")
	}
}

// The player scope is what both reading and moving need. A login made before
// HomeHub asked for it must fail with the sentence that fixes it, before any
// request goes out.
func TestPlayerCallsRequireTheScope(t *testing.T) {
	c := connected(t, "user-read-private", func(r *http.Request) *http.Response {
		t.Error("nothing should reach the network without the scope")
		return jsonResponse(500, "")
	})
	if _, err := c.Playback(context.Background()); err == nil {
		t.Error("reading the player needs the scope")
	}
	if err := c.Transfer(context.Background(), "dev1", true); err == nil {
		t.Error("moving the session needs the scope")
	}
}

func TestSetDeviceVolumeSendsThePercentAndClamps(t *testing.T) {
	var got string
	c := connected(t, playerScope, func(r *http.Request) *http.Response {
		got = r.URL.RawQuery
		return jsonResponse(204, "")
	})

	if err := c.For("").SetDeviceVolume(context.Background(), "dev1", 140); err != nil {
		t.Fatalf("volume: %v", err)
	}
	if !strings.Contains(got, "volume_percent=100") {
		t.Errorf("query = %q, want the level clamped to 100", got)
	}
	if !strings.Contains(got, "device_id=dev1") {
		t.Errorf("query = %q, want the device named", got)
	}
}
