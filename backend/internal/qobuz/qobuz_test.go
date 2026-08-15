package qobuz

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newTestClient wires a client to a fake Qobuz. The data dir is a temp dir so
// the persistence path is exercised rather than stubbed — a client that only
// works with saving disabled would pass every test and fail on first restart.
func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Redirect every call at the fake server, keeping the real path and
	// query so assertions see exactly what the client would have sent.
	c.HTTP = &http.Client{Transport: roundTripTo(srv.URL)}
	c.Now = func() time.Time { return time.Unix(1700000000, 0) }
	if err := c.SetApp("app-123", "s3cr3t"); err != nil {
		t.Fatalf("set app: %v", err)
	}
	return c, srv
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func roundTripTo(base string) http.RoundTripper {
	u, _ := url.Parse(base)
	return rtFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme, r.URL.Host = u.Scheme, u.Host
		return http.DefaultTransport.RoundTrip(r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// The signature is the one piece of this client that cannot be checked against
// a live server without credentials, and the one that fails silently-ish when
// wrong — Qobuz answers 400 and the audio simply never arrives. So it is
// pinned here against an independently computed digest.
func TestFileURLSignature(t *testing.T) {
	const ts = "1700000000"
	// Built by hand from Qobuz's documented scheme: the path with slashes
	// removed, then every parameter as {name}{value} in alphabetical order,
	// then the timestamp, then the secret.
	want := md5Hex("trackgetFileUrl" + "format_id" + "27" + "intent" + "stream" +
		"track_id" + "555" + ts + "s3cr3t")

	var gotSig, gotTS string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.URL.Query().Get("request_sig")
		gotTS = r.URL.Query().Get("request_ts")
		writeJSON(w, map[string]any{
			"track_id": 555, "format_id": 27, "mime_type": "audio/flac",
			"url": "https://cdn.example/track.flac", "sampling_rate": 96.0,
			"bit_depth": 24, "duration": 300,
		})
	})
	mustLogin(t, c)

	f, err := c.FileURL(context.Background(), "555", FormatHiRes192)
	if err != nil {
		t.Fatalf("file url: %v", err)
	}
	if gotSig != want {
		t.Errorf("request_sig = %s, want %s", gotSig, want)
	}
	if gotTS != ts {
		t.Errorf("request_ts = %s, want %s", gotTS, ts)
	}
	// The response is believed over the request: this asked for 192 and got
	// 96, and reporting 192 downstream would put a number on screen that no
	// speaker ever received.
	if f.SampleRate != 96000 || f.BitDepth != 24 {
		t.Errorf("format = %d Hz/%d-bit, want 96000/24", f.SampleRate, f.BitDepth)
	}
	if !f.Lossless() {
		t.Error("format 27 is FLAC and must report lossless")
	}
}

// app_id, request_ts and request_sig must not be folded into the digest they
// produce. Getting this wrong is the classic way to sign a request that can
// never validate.
func TestSignatureExcludesItsOwnParameters(t *testing.T) {
	q := url.Values{
		"track_id": {"1"}, "format_id": {"6"}, "intent": {"stream"},
		"app_id": {"app-123"}, "request_ts": {"999"}, "request_sig": {"stale"},
	}
	got := sign("/track/getFileUrl", q, "999", "s3cr3t")
	want := md5Hex("trackgetFileUrl" + "format_id6" + "intentstream" + "track_id1" + "999" + "s3cr3t")
	if got != want {
		t.Errorf("sign = %s, want %s", got, want)
	}
}

// Parameters are ordered by name, not by insertion. url.Values is a map, so an
// implementation that iterated it directly would pass locally and fail
// randomly in production.
func TestSignatureIsOrderIndependent(t *testing.T) {
	a := url.Values{"track_id": {"1"}, "format_id": {"6"}, "intent": {"stream"}}
	b := url.Values{"intent": {"stream"}, "format_id": {"6"}, "track_id": {"1"}}
	if sign("/track/getFileUrl", a, "1", "x") != sign("/track/getFileUrl", b, "1", "x") {
		t.Error("the digest depends on map iteration order")
	}
}

func TestLoginStoresTokenAndEntitlement(t *testing.T) {
	var gotPassword, gotEmail string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotEmail = r.URL.Query().Get("email")
		gotPassword = r.URL.Query().Get("password")
		writeJSON(w, map[string]any{
			"user_auth_token": "tok-abc",
			"user": map[string]any{
				"display_name": "Petter",
				"credential": map[string]any{
					"description": "Studio Premier",
					"parameters": map[string]any{
						"label": "Studio Premier", "lossless_streaming": true,
						"hires_streaming": true, "lossy_streaming": true,
					},
				},
			},
		})
	})

	if err := c.Login(context.Background(), "me@example.com", "hunter2"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if gotEmail != "me@example.com" {
		t.Errorf("email = %q", gotEmail)
	}
	// The password goes as its MD5, and the plaintext must never appear.
	if gotPassword != md5Hex("hunter2") {
		t.Errorf("password = %q, want the md5 digest", gotPassword)
	}
	if strings.Contains(gotPassword, "hunter2") {
		t.Error("the plaintext password was sent")
	}

	st := c.Status()
	if !st.Connected || st.DisplayName != "Petter" || st.Plan != "Studio Premier" {
		t.Errorf("status = %+v", st)
	}
	if st.MaxFormat != FormatHiRes192 {
		t.Errorf("max format = %d, want hi-res", st.MaxFormat)
	}

	// And it survives a restart, because the token is what every later call
	// depends on.
	reloaded, err := New(c.dataDir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.Status().Connected {
		t.Error("the session did not survive a restart")
	}
}

// A CD-only subscription must not be recorded as hi-res capable. Quality
// reporting is built on this number, so an optimistic reading here becomes a
// "24-bit" badge over audio that arrived at 16.
func TestEntitlementFollowsTheSubscription(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
		want   FormatID
	}{
		{"hi-res", map[string]any{"hires_streaming": true, "lossless_streaming": true}, FormatHiRes192},
		{"lossless only", map[string]any{"lossless_streaming": true}, FormatCD},
		{"lossy only", map[string]any{"lossy_streaming": true}, FormatMP3320},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]any{
					"user_auth_token": "tok",
					"user":            map[string]any{"credential": map[string]any{"parameters": tc.params}},
				})
			})
			if err := c.Login(context.Background(), "a@b.c", "pw"); err != nil {
				t.Fatalf("login: %v", err)
			}
			if got := c.MaxFormat(); got != tc.want {
				t.Errorf("max format = %d, want %d", got, tc.want)
			}
		})
	}
}

// An account with nothing recorded gets CD rather than nothing. Every Qobuz
// subscription that streams at all streams lossless, so a zero here would
// report a household as worse off than any real one is.
func TestUnknownEntitlementFloorsAtCD(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if got := c.MaxFormat(); got != FormatCD {
		t.Errorf("max format = %d, want CD", got)
	}
}

func TestSearchMapsTheCatalogue(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api.json/0.2/catalog/search" {
			t.Errorf("path = %s", got)
		}
		writeJSON(w, map[string]any{
			"tracks": map[string]any{"items": []map[string]any{{
				"id": 42, "title": "Blue in Green", "duration": 337,
				"maximum_bit_depth": 24, "maximum_sampling_rate": 192.0,
				"hires_streamable": true, "streamable": true,
				"performer": map[string]any{"name": "Miles Davis"},
				"album": map[string]any{
					"title": "Kind of Blue",
					"image": map[string]any{"large": "https://img/large.jpg"},
				},
			}}},
			"albums": map[string]any{"items": []map[string]any{{
				"id": "abc", "title": "Kind of Blue", "streamable": true,
				"maximum_bit_depth": 24, "maximum_sampling_rate": 44.1,
				"artist": map[string]any{"name": "Miles Davis"},
			}}},
		})
	})

	res, err := c.Search(context.Background(), "kind of blue", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Tracks) != 1 || len(res.Albums) != 1 {
		t.Fatalf("results = %d tracks, %d albums", len(res.Tracks), len(res.Albums))
	}
	tr := res.Tracks[0]
	if tr.URI != "qobuz:track:42" {
		t.Errorf("uri = %q", tr.URI)
	}
	if tr.Sub != "Miles Davis · Kind of Blue" {
		t.Errorf("subtitle = %q", tr.Sub)
	}
	if tr.SampleRate != 192000 || tr.BitDepth != 24 {
		t.Errorf("track format = %d Hz/%d-bit", tr.SampleRate, tr.BitDepth)
	}
	if tr.ArtURL != "https://img/large.jpg" {
		t.Errorf("art = %q", tr.ArtURL)
	}
	// 44.1 is the one rate that is not a whole number of kHz, and rounding
	// it to 44000 would be wrong everywhere it is later displayed.
	if got := res.Albums[0].SampleRate; got != 44100 {
		t.Errorf("album rate = %d, want 44100", got)
	}
}

// A purchase-only track must fail with something a listener can act on, not
// with an empty URL that becomes a confusing decoder error later.
func TestUnstreamableTrackIsRefusedClearly(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"track_id": 1, "url": "", "streamable": false})
	})
	mustLogin(t, c)

	_, err := c.FileURL(context.Background(), "1", FormatCD)
	if err == nil || !strings.Contains(err.Error(), "isn't streamable") {
		t.Errorf("error = %v, want the not-streamable message", err)
	}
}

// Calls that need a session must say so rather than sending an anonymous
// request and reporting whatever Qobuz says about it.
func TestSignedCallsRequireASession(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should have been made")
	})
	if _, err := c.FileURL(context.Background(), "1", FormatCD); err != ErrNotLoggedIn {
		t.Errorf("error = %v, want ErrNotLoggedIn", err)
	}
	if _, err := c.Favorites(context.Background(), 10); err != ErrNotLoggedIn {
		t.Errorf("favorites error = %v, want ErrNotLoggedIn", err)
	}
}

// Without app credentials nothing is attempted at all — the household has to
// supply their own, and a request without them is guaranteed to fail.
func TestNoAppCredentialsIsItsOwnError(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := c.Login(context.Background(), "a@b.c", "pw"); err != ErrNoApp {
		t.Errorf("login error = %v, want ErrNoApp", err)
	}
	if st := c.Status(); st.Configured {
		t.Error("a client with no credentials must not report itself configured")
	}
}

// Changing the application invalidates the session issued under the old one.
// Keeping it would leave a token that fails on the next call for a reason
// nobody could see.
func TestChangingTheAppDropsTheSession(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"user_auth_token": "tok",
			"user": map[string]any{"credential": map[string]any{"parameters": map[string]any{"lossless_streaming": true}}}})
	})
	mustLogin(t, c)
	if !c.Status().Connected {
		t.Fatal("precondition: connected")
	}
	if err := c.SetApp("different-app", "other-secret"); err != nil {
		t.Fatalf("set app: %v", err)
	}
	if c.Status().Connected {
		t.Error("the old session survived an app change")
	}
}

// Disconnecting forgets the listener, not the installation.
func TestDisconnectKeepsAppCredentials(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"user_auth_token": "tok",
			"user": map[string]any{"credential": map[string]any{"parameters": map[string]any{"lossless_streaming": true}}}})
	})
	mustLogin(t, c)
	if err := c.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	st := c.Status()
	if st.Connected {
		t.Error("still connected after disconnect")
	}
	if !st.Configured {
		t.Error("disconnect threw away the app credentials too")
	}
}

func TestParseURI(t *testing.T) {
	kind, id, err := ParseURI("qobuz:album:abc123")
	if err != nil || kind != "album" || id != "abc123" {
		t.Errorf("parse = %q/%q/%v", kind, id, err)
	}
	for _, bad := range []string{"", "qobuz:track", "spotify:track:1", "qobuz::1", "qobuz:track:"} {
		if _, _, err := ParseURI(bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func mustLogin(t *testing.T, c *Client) {
	t.Helper()
	c.mu.Lock()
	c.p.AuthToken = "tok"
	c.mu.Unlock()
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
