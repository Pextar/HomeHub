package spotify

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// roundTripFunc lets a test stand in for Spotify's API without a server.
type roundTripFunc func(*http.Request) *http.Response

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

// connected returns a client with a live-looking token, so calls reach the
// stubbed transport instead of stopping at the credential checks.
func connected(t *testing.T, scope string, rt roundTripFunc) *Client {
	t.Helper()
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.p = persisted{
		ClientID: "cid",
		Household: &accountState{
			RefreshToken: "refresh", AccessToken: "access",
			Expiry: time.Now().Add(time.Hour), Scope: scope,
		},
	}
	c.HTTP = &http.Client{Transport: rt}
	return c
}

const fullScope = "user-read-private user-read-playback-state user-modify-playback-state"

// A login made before the player scopes existed searches fine and cannot
// play. That has to be reported as "reconnect", not as Spotify's 403, and it
// must be caught before any request goes out.
func TestPlaybackNeedsTheGrant(t *testing.T) {
	var calls int
	rt := roundTripFunc(func(*http.Request) *http.Response {
		calls++
		return jsonResponse(204, "")
	})

	old := connected(t, "user-read-private playlist-read-private", rt)
	if err := old.PlayOn(context.Background(), "dev", "spotify:track:x"); !errors.Is(err, ErrPlaybackScope) {
		t.Errorf("PlayOn without the player scopes = %v, want ErrPlaybackScope", err)
	}
	if _, err := old.Devices(context.Background()); !errors.Is(err, ErrPlaybackScope) {
		t.Errorf("Devices without the player scopes = %v, want ErrPlaybackScope", err)
	}
	if calls != 0 {
		t.Errorf("%d requests went out; a missing grant must fail locally", calls)
	}
	if old.Status().Playback {
		t.Error("Status.Playback must be false without both player scopes")
	}
	if !connected(t, fullScope, rt).Status().Playback {
		t.Error("Status.Playback must be true with both player scopes")
	}

	// No account at all is a different problem with a different prompt.
	blank, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := blank.PlayOn(context.Background(), "dev", "spotify:track:x"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("PlayOn with no account = %v, want ErrNotConnected", err)
	}
}

// A track plays on its own; an album or playlist plays as a context so the
// rest of it follows. Anything else is refused rather than sent.
func TestPlayOnSendsTheRightBody(t *testing.T) {
	cases := []struct {
		uri  string
		want string
	}{
		{"spotify:track:4cOdK2wGLETKBW3PvgPWqT", `{"uris":["spotify:track:4cOdK2wGLETKBW3PvgPWqT"]}`},
		{"spotify:album:1DFixLWuPkv3KT3TnV35m3", `{"context_uri":"spotify:album:1DFixLWuPkv3KT3TnV35m3"}`},
		{"spotify:playlist:37i9dQZF1DXcBWIGoYBM5M", `{"context_uri":"spotify:playlist:37i9dQZF1DXcBWIGoYBM5M"}`},
	}
	for _, tc := range cases {
		var gotBody, gotQuery, gotMethod string
		c := connected(t, fullScope, func(r *http.Request) *http.Response {
			raw, _ := io.ReadAll(r.Body)
			gotBody, gotQuery, gotMethod = string(raw), r.URL.RawQuery, r.Method
			return jsonResponse(204, "")
		})
		if err := c.PlayOn(context.Background(), "dev-1", tc.uri); err != nil {
			t.Fatalf("PlayOn(%s): %v", tc.uri, err)
		}
		if gotMethod != http.MethodPut {
			t.Errorf("method = %s, want PUT", gotMethod)
		}
		if gotQuery != "device_id=dev-1" {
			t.Errorf("query = %q, want the device named", gotQuery)
		}
		if gotBody != tc.want {
			t.Errorf("body for %s = %s, want %s", tc.uri, gotBody, tc.want)
		}
	}

	c := connected(t, fullScope, func(*http.Request) *http.Response { return jsonResponse(204, "") })
	if err := c.PlayOn(context.Background(), "dev-1", "spotify:show:xyz"); err == nil {
		t.Error("a podcast show is not something this can start; want an error")
	}
	if err := c.PlayOn(context.Background(), "", "spotify:track:x"); err == nil {
		t.Error("an empty device id must be refused")
	}
}

// 202 means "reachable, not ready" — a speaker that just woke. One retry is
// the difference between "didn't work" and "took a second".
func TestPlayOnRetriesWhenTheSpeakerIsntReadyYet(t *testing.T) {
	var calls int
	c := connected(t, fullScope, func(*http.Request) *http.Response {
		calls++
		if calls == 1 {
			return jsonResponse(http.StatusAccepted, "")
		}
		return jsonResponse(204, "")
	})
	if err := c.PlayOn(context.Background(), "dev", "spotify:track:x"); err != nil {
		t.Fatalf("a 202 then a 204 should succeed, got %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry)", calls)
	}

	// Still not ready on the retry: say so in words the user can act on,
	// rather than leaking the status code.
	stuck := connected(t, fullScope, func(*http.Request) *http.Response {
		return jsonResponse(http.StatusAccepted, "")
	})
	err := stuck.PlayOn(context.Background(), "dev", "spotify:track:x")
	if err == nil || !strings.Contains(err.Error(), "wake it") {
		t.Errorf("persistent 202 = %v, want advice to wake the speaker", err)
	}
}

// The two refusals a user is most likely to hit have to name their cause:
// no Premium, and a speaker that fell asleep between listing and playing.
func TestPlayOnExplainsRefusals(t *testing.T) {
	premium := connected(t, fullScope, func(*http.Request) *http.Response {
		return jsonResponse(http.StatusForbidden,
			`{"error":{"status":403,"message":"Player command failed: Premium required","reason":"PREMIUM_REQUIRED"}}`)
	})
	err := premium.PlayOn(context.Background(), "dev", "spotify:track:x")
	if err == nil || !strings.Contains(err.Error(), "Premium") {
		t.Errorf("403 = %v, want it to name Premium", err)
	}

	gone := connected(t, fullScope, func(*http.Request) *http.Response {
		return jsonResponse(http.StatusNotFound, `{"error":{"status":404,"message":"Device not found"}}`)
	})
	err = gone.PlayOn(context.Background(), "dev", "spotify:track:x")
	if err == nil || !strings.Contains(err.Error(), "wake it") {
		t.Errorf("404 = %v, want advice to wake the speaker", err)
	}
}

func TestDevicesParsesTheList(t *testing.T) {
	c := connected(t, fullScope, func(r *http.Request) *http.Response {
		if r.URL.Path != "/v1/me/player/devices" {
			t.Errorf("path = %s", r.URL.Path)
		}
		return jsonResponse(200, `{"devices":[
			{"id":"abc","is_active":true,"is_restricted":false,"name":" Living Room ","type":"Speaker","volume_percent":42},
			{"id":"def","is_active":false,"is_restricted":true,"name":"Car","type":"Automobile","volume_percent":null}
		]}`)
	})
	devs, err := c.Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("got %d devices, want 2", len(devs))
	}
	if devs[0].Name != "Living Room" || !devs[0].Active || devs[0].Volume != 42 {
		t.Errorf("first device = %+v", devs[0])
	}
	// A null volume must not be mistaken for silence, and a restricted
	// device has to stay flagged so the caller can refuse it by name.
	if devs[1].Volume != 0 || !devs[1].Restricted {
		t.Errorf("second device = %+v", devs[1])
	}
}

// The grant Spotify reports is what gets stored — including on a refresh,
// which is how a login saved by an older build learns what it has.
func TestRefreshRecordsTheGrant(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Expired, no scope — the legacy flat shape, which New folds into the
	// household account on the next load; written here directly.
	c.p = persisted{ClientID: "cid", Household: &accountState{RefreshToken: "refresh"}}
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) *http.Response {
		return jsonResponse(200, `{"access_token":"fresh","expires_in":3600,"scope":"`+fullScope+`"}`)
	})}
	if _, err := c.For("").accessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !c.Status().Playback {
		t.Error("a refresh that reports the player scopes should leave Playback true")
	}
}
