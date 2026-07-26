package mediabridge

import (
	"errors"
	"strings"
	"testing"
	"time"

	"homehub/internal/kef"
	"homehub/internal/media"
	"homehub/internal/store"
)

// Compile-time proof that the adapters are what they claim to be. A missing
// method here is a build failure rather than a runtime type assertion that
// quietly sends a route down the wrong path.
var (
	_ media.Endpoint            = (*SonosEndpoint)(nil)
	_ media.Seeker              = (*SonosEndpoint)(nil)
	_ media.Queuer              = (*SonosEndpoint)(nil)
	_ media.Grouper             = (*SonosEndpoint)(nil)
	_ media.URIPlayer           = (*SonosEndpoint)(nil)
	_ media.NativeServicePlayer = (*SonosEndpoint)(nil)

	_ media.Endpoint      = (*KEFEndpoint)(nil)
	_ media.URIPlayer     = (*KEFEndpoint)(nil)
	_ media.ConnectTarget = (*KEFEndpoint)(nil)
	_ media.Waker         = (*KEFEndpoint)(nil)

	_ media.Provider        = (*SpotifyProvider)(nil)
	_ media.NativeProvider  = (*SpotifyProvider)(nil)
	_ media.ConnectProvider = (*SpotifyProvider)(nil)
	_ media.StreamProvider  = (*SpotifyProvider)(nil)
)

// TestCapabilitiesMatchInterfaces is the invariant the route engine depends
// on: a declared capability must be backed by the matching interface.
//
// The engine picks a route from capabilities alone and then type-asserts to
// call it. A capability without its interface therefore turns into a failed
// assertion at the moment a user taps play — the worst possible time to find
// out. This catches it at test time instead.
func TestCapabilitiesMatchInterfaces(t *testing.T) {
	endpoints := []media.Endpoint{
		NewSonosEndpoint(store.SonosSpeaker{ID: "s1", Name: "Living Room", UUID: "RINCON_1"}, "", nil),
		NewKEFEndpoint(store.KEFSpeaker{ID: "k1", Name: "Study"}, nil),
	}

	// Each capability and the interface it promises. Capabilities with no
	// interface behind them (CapTransport, CapVolume) are covered by
	// media.Endpoint itself and so are absent here.
	checks := []struct {
		cap  media.Capability
		name string
		ok   func(media.Endpoint) bool
	}{
		{media.CapSeek, "media.Seeker", func(e media.Endpoint) bool {
			_, ok := e.(media.Seeker)
			return ok
		}},
		{media.CapQueue, "media.Queuer", func(e media.Endpoint) bool {
			_, ok := e.(media.Queuer)
			return ok
		}},
		{media.CapGroup, "media.Grouper", func(e media.Endpoint) bool {
			_, ok := e.(media.Grouper)
			return ok
		}},
		{media.CapPlayURI, "media.URIPlayer", func(e media.Endpoint) bool {
			_, ok := e.(media.URIPlayer)
			return ok
		}},
		{media.CapNativeService, "media.NativeServicePlayer", func(e media.Endpoint) bool {
			_, ok := e.(media.NativeServicePlayer)
			return ok
		}},
		{media.CapConnect, "media.ConnectTarget", func(e media.Endpoint) bool {
			_, ok := e.(media.ConnectTarget)
			return ok
		}},
		{media.CapWake, "media.Waker", func(e media.Endpoint) bool {
			_, ok := e.(media.Waker)
			return ok
		}},
	}

	for _, e := range endpoints {
		d := e.Descriptor()
		for _, c := range checks {
			declared := d.Caps.Has(c.cap)
			implemented := c.ok(e)
			if declared && !implemented {
				t.Errorf("%s declares %v but does not implement %s",
					d.Name, c.cap, c.name)
			}
			// The reverse is allowed but suspicious: an endpoint that can
			// do something it doesn't advertise will simply never be asked.
			if implemented && !declared {
				t.Errorf("%s implements %s but does not declare %v — the route engine will never use it",
					d.Name, c.name, c.cap)
			}
		}
	}
}

// TestKEFDoesNotClaimGrouping guards the asymmetry the design rests on. If a
// KEF ever declared CapGroup, the route engine would try to group it with a
// Sonos and the cross-vendor case would break in a way that is hard to trace
// back to here.
func TestKEFDoesNotClaimGrouping(t *testing.T) {
	d := NewKEFEndpoint(store.KEFSpeaker{ID: "k1", Name: "Study"}, nil).Descriptor()
	if d.Caps.Has(media.CapGroup) {
		t.Error("KEF must not declare CapGroup: the speakers have no native grouping")
	}
	if d.Caps.Has(media.CapNativeService) {
		t.Error("KEF must not declare CapNativeService: its API cannot accept content")
	}
	if d.GroupKey != "" {
		t.Errorf("KEF GroupKey = %q, want empty — there is no grouping domain to belong to", d.GroupKey)
	}
}

// TestSonosGroupKey checks that speakers default into one grouping domain,
// and that an explicit household separates them.
func TestSonosGroupKey(t *testing.T) {
	a := NewSonosEndpoint(store.SonosSpeaker{ID: "a", Name: "A"}, "", nil).Descriptor()
	b := NewSonosEndpoint(store.SonosSpeaker{ID: "b", Name: "B"}, "", nil).Descriptor()
	if a.GroupKey == "" || a.GroupKey != b.GroupKey {
		t.Errorf("speakers with no household should share a group key, got %q and %q",
			a.GroupKey, b.GroupKey)
	}
	c := NewSonosEndpoint(store.SonosSpeaker{ID: "c", Name: "C"}, "other", nil).Descriptor()
	if c.GroupKey == a.GroupKey {
		t.Error("an explicit household must not collide with the default")
	}
}

func TestParseClock(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"0:00:00", 0},
		{"0:03:45", 3*time.Minute + 45*time.Second},
		{"1:02:03", time.Hour + 2*time.Minute + 3*time.Second},
		{"  0:01:00  ", time.Minute},
		// Speakers really do return these for live streams; both must be
		// zero rather than a partial parse.
		{"NOT_IMPLEMENTED", 0},
		{"", 0},
		{"1:2", 0},
		{"a:b:c", 0},
		{"-1:00:00", 0},
	}
	for _, tc := range tests {
		if got := parseClock(tc.in); got != tc.want {
			t.Errorf("parseClock(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestFormatClock(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0:00:00"},
		{45 * time.Second, "0:00:45"},
		{3*time.Minute + 5*time.Second, "0:03:05"},
		{time.Hour + 2*time.Minute + 3*time.Second, "1:02:03"},
		// Clamped rather than rendered negative, which the speaker rejects.
		{-time.Second, "0:00:00"},
	}
	for _, tc := range tests {
		if got := formatClock(tc.in); got != tc.want {
			t.Errorf("formatClock(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Round trip, since Seek depends on both directions agreeing.
	for _, d := range []time.Duration{0, time.Second, 90 * time.Second, 3661 * time.Second} {
		if got := parseClock(formatClock(d)); got != d {
			t.Errorf("round trip of %v gave %v", d, got)
		}
	}
}

func TestSonosPlayState(t *testing.T) {
	tests := map[string]media.PlayState{
		"PLAYING":         media.StatePlaying,
		"PAUSED_PLAYBACK": media.StatePaused,
		"TRANSITIONING":   media.StateTransitioning,
		"STOPPED":         media.StateStopped,
		"playing":         media.StatePlaying,
		// An unrecognised value must understate rather than guess.
		"WHO_KNOWS": media.StateStopped,
		"":          media.StateStopped,
	}
	for in, want := range tests {
		if got := sonosPlayState(in); got != want {
			t.Errorf("sonosPlayState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKEFPlayState(t *testing.T) {
	tests := []struct {
		name string
		st   *kef.State
		want media.PlayState
	}{
		{"playing", &kef.State{PoweredOn: true, Status: kef.StatusPlaying}, media.StatePlaying},
		{"paused", &kef.State{PoweredOn: true, Status: kef.StatusPaused}, media.StatePaused},
		{"stopped", &kef.State{PoweredOn: true, Status: kef.StatusStopped}, media.StateStopped},
		{
			// The case worth pinning: a sleeping speaker's player field is
			// stale, and showing a resumable transport for a speaker that is
			// off would be a lie the user can't act on.
			name: "standby overrides a stale playing status",
			st:   &kef.State{PoweredOn: false, Status: kef.StatusPlaying},
			want: media.StateStopped,
		},
	}
	for _, tc := range tests {
		if got := kefPlayState(tc.st); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestStreamDIDL(t *testing.T) {
	didl := streamDIDL("http://192.168.1.5:8080/stream.mp3", media.Metadata{
		Title:       "Song & Dance",
		Artist:      "A <Band>",
		Album:       "Album",
		ContentType: "audio/flac",
		Live:        true,
	})

	// The class is what stops Sonos showing a scrubber it can't honour and
	// trying to advance past an endless source.
	if !strings.Contains(didl, "object.item.audioItem.audioBroadcast") {
		t.Error("a live stream must use the audioBroadcast class")
	}
	// The advertised MIME type has to be the one actually served: Sonos
	// picks a decoder from this before fetching a byte.
	if !strings.Contains(didl, "http-get:*:audio/flac:*") {
		t.Errorf("content type missing from protocolInfo:\n%s", didl)
	}
	// Metadata that reaches a speaker unescaped produces a document the
	// speaker rejects, which presents as silence rather than as an error.
	if !strings.Contains(didl, "Song &amp; Dance") {
		t.Error("title not XML-escaped")
	}
	if !strings.Contains(didl, "A &lt;Band&gt;") {
		t.Error("artist not XML-escaped")
	}
	if strings.Contains(didl, "<Band>") {
		t.Error("raw angle brackets leaked into the document")
	}

	// A non-live stream is a track, so the speaker shows position.
	didl = streamDIDL("http://h/s.mp3", media.Metadata{Title: "T"})
	if !strings.Contains(didl, "object.item.audioItem.musicTrack") {
		t.Error("a non-live stream should be a musicTrack")
	}
	// And with no content type given, the adapter must still advertise
	// something rather than an empty protocolInfo.
	if !strings.Contains(didl, "http-get:*:audio/mpeg:*") {
		t.Errorf("missing default content type:\n%s", didl)
	}
}

// fakeTarget is a ConnectTarget with settable hints.
type fakeTarget struct {
	id    string
	names []string
}

func (f fakeTarget) ConnectHint() (string, []string) { return f.id, f.names }

func TestMatchConnectDevice(t *testing.T) {
	devices := []media.ConnectDevice{
		{ID: "dev-1", Name: "Study"},
		{ID: "dev-2", Name: "Living  Room"},
		{ID: "dev-3", Name: "Car", Restricted: true},
	}

	t.Run("pinned id wins", func(t *testing.T) {
		got, err := MatchConnectDevice(fakeTarget{id: "dev-2", names: []string{"Study"}}, "Study", devices)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "dev-2" {
			t.Errorf("got %q, want dev-2 — a pin must beat a name match", got.ID)
		}
	})

	t.Run("name match folds whitespace and case", func(t *testing.T) {
		got, err := MatchConnectDevice(fakeTarget{names: []string{"living room"}}, "living room", devices)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "dev-2" {
			t.Errorf("got %q, want dev-2", got.ID)
		}
	})

	t.Run("falls back to the next name when the first misses", func(t *testing.T) {
		got, err := MatchConnectDevice(fakeTarget{names: []string{"Old Name", "Study"}}, "Study", devices)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "dev-1" {
			t.Errorf("got %q, want dev-1", got.ID)
		}
	})

	t.Run("a stale pin falls through to the name", func(t *testing.T) {
		// Spotify rotates device ids on re-registration, so a pin that no
		// longer resolves must not stop the name from matching.
		got, err := MatchConnectDevice(fakeTarget{id: "gone", names: []string{"Study"}}, "Study", devices)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "dev-1" {
			t.Errorf("got %q, want dev-1", got.ID)
		}
	})

	t.Run("restricted device is rejected with a reason", func(t *testing.T) {
		_, err := MatchConnectDevice(fakeTarget{names: []string{"Car"}}, "Car", devices)
		if err == nil {
			t.Fatal("expected a restricted device to be rejected")
		}
		if !errors.Is(err, ErrNoConnectDevice) {
			t.Errorf("want ErrNoConnectDevice, got %v", err)
		}
		if !strings.Contains(err.Error(), "Car") {
			t.Errorf("error should name the device: %v", err)
		}
	})

	t.Run("no match explains what to do", func(t *testing.T) {
		_, err := MatchConnectDevice(fakeTarget{names: []string{"Bedroom"}}, "Bedroom", devices)
		if err == nil {
			t.Fatal("expected no match")
		}
		if !strings.Contains(err.Error(), "Bedroom") {
			t.Errorf("error should name the speaker: %v", err)
		}
	})

	t.Run("nothing is guessed when there are no usable hints", func(t *testing.T) {
		// An endpoint with no pin and no names must not fall back to
		// "whatever device is first" — that plays in the wrong room.
		_, err := MatchConnectDevice(fakeTarget{names: []string{"", "  "}}, "Unnamed", devices)
		if err == nil {
			t.Fatal("expected no match when there is nothing to match on")
		}
	})
}

// TestSpotifyProviderNilClient covers the unwired integration: every method
// must report rather than panic, which is what the API layer relies on.
func TestSpotifyProviderNilClient(t *testing.T) {
	p := NewSpotifyProvider(nil, nil)
	av := p.Available()
	if av.OK {
		t.Error("a provider with no client must not report itself available")
	}
	if av.Reason == "" {
		t.Error("unavailability must carry a reason the user can act on")
	}
	if _, err := p.Search(t.Context(), "anything", 10); err == nil {
		t.Error("Search should fail when the provider is unavailable")
	}
	if _, err := p.Browse(t.Context(), 10); err == nil {
		t.Error("Browse should fail when the provider is unavailable")
	}
	if sa := p.StreamAvailable(); sa.OK {
		t.Error("stream must not be available without a client")
	}
}

// TestSpotifyNativeItemRejectsNonSonos pins the honesty rule: only Sonos has a
// native Spotify integration, and the provider must say so rather than hand
// back a URI the speaker would ignore.
func TestSpotifyNativeItemRejectsNonSonos(t *testing.T) {
	p := NewSpotifyProvider(nil, nil)
	_, _, err := p.NativeItem(media.VendorKEF,
		media.Item{URI: "spotify:track:x", Title: "T"}, media.Account{SID: 1})
	if err == nil {
		t.Fatal("expected KEF to be rejected for native Spotify")
	}
	if !strings.Contains(err.Error(), "kef") {
		t.Errorf("error should name the vendor: %v", err)
	}
}

// TestSpotifyRoutesCoverEveryPath asserts the provider advertises all four
// routes. Dropping one here would silently remove a capability from every
// zone, and the route engine would report it as a speaker limitation.
func TestSpotifyRoutesCoverEveryPath(t *testing.T) {
	routes := NewSpotifyProvider(nil, nil).Routes()
	for _, want := range []media.Route{
		media.RouteNative, media.RouteGroup, media.RouteConnect, media.RouteStream,
	} {
		if !routes.Has(want) {
			t.Errorf("Spotify should advertise the %q route", want)
		}
	}
}
