package media

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeEndpoint is a capability declaration with just enough behaviour to be
// an Endpoint. Route resolution never calls the transport methods — that is
// the point of it being a pure function — so they are stubs.
type fakeEndpoint struct {
	desc Descriptor
}

func ep(name string, vendor Vendor, caps Capability, groupKey string) *fakeEndpoint {
	return &fakeEndpoint{Descriptor{
		ID: strings.ToLower(name), Name: name, Vendor: vendor,
		Caps: caps, GroupKey: groupKey,
	}}
}

func (f *fakeEndpoint) Descriptor() Descriptor { return f.desc }
func (f *fakeEndpoint) State(context.Context) (*NowPlaying, error) {
	return &NowPlaying{State: StateStopped}, nil
}
func (f *fakeEndpoint) Play(context.Context) error           { return nil }
func (f *fakeEndpoint) Pause(context.Context) error          { return nil }
func (f *fakeEndpoint) Next(context.Context) error           { return nil }
func (f *fakeEndpoint) Previous(context.Context) error       { return nil }
func (f *fakeEndpoint) SetVolume(context.Context, int) error { return nil }
func (f *fakeEndpoint) SetMute(context.Context, bool) error  { return nil }

// Capability shorthands matching what the real adapters declare, so the
// tests below exercise the actual shapes rather than invented ones.
const (
	sonosCaps = CapTransport | CapVolume | CapSeek | CapQueue | CapGroup |
		CapPlayURI | CapNativeService
	kefCaps = CapTransport | CapVolume | CapPlayURI | CapConnect | CapWake
)

func sonosEP(name string) *fakeEndpoint { return ep(name, VendorSonos, sonosCaps, "household-1") }
func kefEP(name string) *fakeEndpoint   { return ep(name, VendorKEF, kefCaps, "") }

// fakeProvider declares a route set and optionally implements the
// route-specific interfaces, selected by the flags.
type fakeProvider struct {
	routes      RouteSet
	native      bool
	connect     bool
	stream      bool
	streamAvail Availability
}

func (f *fakeProvider) ID() string              { return "fake" }
func (f *fakeProvider) Name() string            { return "Fake" }
func (f *fakeProvider) Available() Availability { return Availability{OK: true, Configured: true} }
func (f *fakeProvider) Search(context.Context, string, int) (*Results, error) {
	return &Results{}, nil
}
func (f *fakeProvider) Browse(context.Context, int) ([]Item, error) { return nil, nil }
func (f *fakeProvider) Routes() RouteSet                            { return f.routes }

// The optional interfaces are implemented by standalone types carrying no
// state of their own, so they can be embedded alongside *fakeProvider without
// the method sets colliding. They must not embed *fakeProvider themselves:
// composing by wrapping in struct{Provider; NativeProvider} would leave the
// outermost value promoting only Provider's methods, so a type assertion for
// an inner interface would fail and every case would silently fall through to
// the stream route — which is exactly the misdiagnosis this fixture exists to
// avoid.
type nativeImpl struct{}

func (nativeImpl) NativeItem(Vendor, Item, Account) (string, string, error) { return "u", "m", nil }
func (nativeImpl) ServiceName(Vendor) string                                { return "Fake" }

type connectImpl struct{}

func (connectImpl) ConnectDevices(context.Context) ([]ConnectDevice, error) { return nil, nil }
func (connectImpl) PlayOn(context.Context, string, Item) error              { return nil }

type streamImpl struct{ avail Availability }

func (streamImpl) OpenStream(context.Context, Item) (*Stream, error) { return nil, nil }
func (s streamImpl) StreamAvailable() Availability                   { return s.avail }

// fullProv is the shape of a fully capable service: it serves every route,
// so which one gets used is decided by the speakers and by Routes() alone.
type fullProv struct {
	*fakeProvider
	nativeImpl
	connectImpl
	streamImpl
}

// nativeOnlyProv serves native/group but has neither Connect nor a decoder —
// the shape of a service that simply cannot reach a KEF.
type nativeOnlyProv struct {
	*fakeProvider
	nativeImpl
}

// streamOnlyProv is a service HomeHub can decode but that no speaker can
// serve for itself — the shape that matters for the two routes HomeHub
// decodes for, where the provider's own availability is the gate.
type streamOnlyProv struct {
	*fakeProvider
	streamImpl
}

// build returns the provider in the shape its flags describe. A flag left
// false means the interface is genuinely absent, which is how the "advertises
// a route it doesn't implement" case is constructed.
func (f *fakeProvider) build() Provider {
	switch {
	case f.native && f.connect && f.stream:
		return fullProv{f, nativeImpl{}, connectImpl{}, streamImpl{f.streamAvail}}
	case f.native:
		return nativeOnlyProv{f, nativeImpl{}}
	case f.stream:
		return streamOnlyProv{f, streamImpl{f.streamAvail}}
	default:
		return f
	}
}

// spotifyLike is the provider shape that matters most: Spotify serves all
// three routes, and its stream route is available when librespot is present.
func spotifyLike() Provider {
	return (&fakeProvider{
		routes:      RouteSet{RouteNative, RouteConnect, RouteGroup, RouteStream},
		native:      true,
		connect:     true,
		stream:      true,
		streamAvail: Availability{OK: true, Configured: true},
	}).build()
}

func TestResolveRoutes(t *testing.T) {
	tests := []struct {
		name      string
		provider  Provider
		endpoints []Endpoint
		wantRoute Route
		wantSync  Sync
		wantCoord string
		wantWake  int
	}{
		{
			// The regression guard that matters most: a lone Sonos must
			// keep taking the native path it took before this package
			// existed. If this ever picks stream, Sonos-only listening has
			// silently become transcoded.
			name:      "single sonos plays natively",
			provider:  spotifyLike(),
			endpoints: []Endpoint{sonosEP("Living Room")},
			wantRoute: RouteNative,
			wantSync:  SyncSingle,
			wantCoord: "Living Room",
		},
		{
			name:      "sonos pair groups natively",
			provider:  spotifyLike(),
			endpoints: []Endpoint{sonosEP("Living Room"), sonosEP("Kitchen")},
			wantRoute: RouteGroup,
			wantSync:  SyncExact,
			wantCoord: "Living Room",
		},
		{
			name:      "three sonos still group natively",
			provider:  spotifyLike(),
			endpoints: []Endpoint{sonosEP("A"), sonosEP("B"), sonosEP("C")},
			wantRoute: RouteGroup,
			wantSync:  SyncExact,
			wantCoord: "A",
		},
		{
			name:      "single kef uses connect and needs waking",
			provider:  spotifyLike(),
			endpoints: []Endpoint{kefEP("Study")},
			wantRoute: RouteConnect,
			wantSync:  SyncSingle,
			wantCoord: "Study",
			wantWake:  1,
		},
		{
			// The case this whole design exists for.
			name:      "mixed vendors fall through to stream",
			provider:  spotifyLike(),
			endpoints: []Endpoint{sonosEP("Living Room"), kefEP("Study")},
			wantRoute: RouteStream,
			wantSync:  SyncBuffered,
			wantWake:  1,
		},
		{
			name:      "two kefs stream, since Connect is one at a time",
			provider:  spotifyLike(),
			endpoints: []Endpoint{kefEP("Study"), kefEP("Bedroom")},
			wantRoute: RouteStream,
			wantSync:  SyncBuffered,
			wantWake:  2,
		},
		{
			// Two Sonos systems on one LAN can each stream natively but
			// cannot group with each other, so the fallback is stream even
			// though no speaker lacks CapNativeService.
			name:     "separate sonos households can't group",
			provider: spotifyLike(),
			endpoints: []Endpoint{
				ep("A", VendorSonos, sonosCaps, "household-1"),
				ep("B", VendorSonos, sonosCaps, "household-2"),
			},
			wantRoute: RouteStream,
			wantSync:  SyncBuffered,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := Resolve(tc.provider, tc.endpoints)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if plan.Route != tc.wantRoute {
				t.Errorf("route = %q, want %q (reason: %s)", plan.Route, tc.wantRoute, plan.Reason)
			}
			if plan.Sync != tc.wantSync {
				t.Errorf("sync = %q, want %q", plan.Sync, tc.wantSync)
			}
			if tc.wantCoord != "" {
				if plan.Coordinator == nil {
					t.Fatalf("coordinator = nil, want %q", tc.wantCoord)
				}
				if got := plan.Coordinator.Descriptor().Name; got != tc.wantCoord {
					t.Errorf("coordinator = %q, want %q", got, tc.wantCoord)
				}
			}
			if len(plan.Wake) != tc.wantWake {
				t.Errorf("wake = %d endpoints, want %d", len(plan.Wake), tc.wantWake)
			}
			if plan.Reason == "" {
				t.Error("reason is empty; every plan must explain itself")
			}
			if got := len(plan.Endpoints()); got != len(tc.endpoints) {
				t.Errorf("plan covers %d endpoints, want %d", got, len(tc.endpoints))
			}
		})
	}
}

// TestResolvePrefersNativeOverStream is the ordering guarantee stated in
// docs/MEDIA-PROTOCOL.md, asserted directly rather than inferred from the
// cases above: whenever a native route fits, stream must not be chosen.
func TestResolvePrefersNativeOverStream(t *testing.T) {
	p := spotifyLike()
	for _, eps := range [][]Endpoint{
		{sonosEP("A")},
		{sonosEP("A"), sonosEP("B")},
		{sonosEP("A"), sonosEP("B"), sonosEP("C")},
	} {
		plan, err := Resolve(p, eps)
		if err != nil {
			t.Fatalf("Resolve(%d sonos): %v", len(eps), err)
		}
		if plan.Route == RouteStream {
			t.Errorf("%d sonos speakers took the stream route; native was available", len(eps))
		}
		if plan.Sync == SyncBuffered {
			t.Errorf("%d sonos speakers reported buffered sync; should be exact", len(eps))
		}
	}
}

// TestResolveStreamUnavailable covers the missing-librespot case: the mixed
// zone has no other route, so it must fail with the reason the user needs
// rather than a bare "unsupported".
func TestResolveStreamUnavailable(t *testing.T) {
	p := (&fakeProvider{
		routes: RouteSet{RouteNative, RouteConnect, RouteGroup, RouteStream},
		native: true, connect: true, stream: true,
		streamAvail: Availability{OK: false, Reason: "librespot isn't installed"},
	}).build()

	_, err := Resolve(p, []Endpoint{sonosEP("Living Room"), kefEP("Study")})
	if err == nil {
		t.Fatal("expected failure when the decoder is unavailable")
	}
	if !errors.Is(err, ErrNoRoute) {
		t.Errorf("error does not unwrap to ErrNoRoute: %v", err)
	}
	if !strings.Contains(err.Error(), "librespot isn't installed") {
		t.Errorf("error should carry the provider's reason, got: %v", err)
	}
	// And it must name the speakers/limits for the routes it rejected, so
	// the message is actionable.
	if !strings.Contains(err.Error(), "one speaker at a time") {
		t.Errorf("error should explain why Connect was rejected, got: %v", err)
	}
}

// TestResolveProviderWithoutStream is the honest-failure case for a service
// that has no Connect and can't be decoded: a mixed zone simply can't work,
// and the error has to say which speaker is the problem.
func TestResolveProviderWithoutStream(t *testing.T) {
	p := (&fakeProvider{routes: RouteSet{RouteNative, RouteGroup}, native: true}).build()

	_, err := Resolve(p, []Endpoint{sonosEP("Living Room"), kefEP("Study")})
	if err == nil {
		t.Fatal("expected failure: KEF can't play this service at all")
	}
	if !strings.Contains(err.Error(), "Study") {
		t.Errorf("error should name the speaker that blocked it, got: %v", err)
	}
}

// TestResolveDeclaredRouteNotImplemented guards the mismatch between what a
// provider advertises and what it implements. Advertising RouteNative without
// implementing NativeProvider must be rejected, not panic on a type assertion.
func TestResolveDeclaredRouteNotImplemented(t *testing.T) {
	p := (&fakeProvider{routes: RouteSet{RouteNative}}).build() // native flag off

	_, err := Resolve(p, []Endpoint{sonosEP("Living Room")})
	if err == nil {
		t.Fatal("expected failure: provider advertises native but doesn't implement it")
	}
	if !errors.Is(err, ErrNoRoute) {
		t.Errorf("want ErrNoRoute, got %v", err)
	}
}

func TestResolveEmptyZone(t *testing.T) {
	if _, err := Resolve(spotifyLike(), nil); !errors.Is(err, ErrEmptyZone) {
		t.Errorf("want ErrEmptyZone, got %v", err)
	}
}

// TestCapabilityHas covers the multi-bit case the route predicates rely on:
// Has(a|b) must mean "both", not "either".
func TestCapabilityHas(t *testing.T) {
	c := CapTransport | CapVolume | CapGroup
	if !c.Has(CapTransport | CapGroup) {
		t.Error("Has should be true when every requested bit is present")
	}
	if c.Has(CapTransport | CapConnect) {
		t.Error("Has must be false when any requested bit is missing")
	}
	if c.Has(CapSeek) {
		t.Error("Has must be false for an absent single capability")
	}
}

func TestCapabilityJSON(t *testing.T) {
	got, err := (CapTransport | CapVolume).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(got) != `["transport","volume"]` {
		t.Errorf("got %s", got)
	}
	// An empty set must encode as [] rather than null, so the frontend can
	// iterate without a nil check.
	got, err = Capability(0).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(got) != `[]` {
		t.Errorf("empty set encoded as %s, want []", got)
	}
}

func TestNowPlayingSyncWire(t *testing.T) {
	n := &NowPlaying{Position: 90 * time.Second, Duration: 3*time.Minute + 30*time.Second}
	n.SyncWire()
	if n.PositionMS != 90000 || n.DurationMS != 210000 {
		t.Errorf("got position=%d duration=%d, want 90000/210000", n.PositionMS, n.DurationMS)
	}
}

// TestRouteSyncSingle guards a subtle case: a one-endpoint zone on the stream
// route is still "single", because there is nothing for it to be out of sync
// with. Reporting "buffered" there would make the UI warn about a problem
// that doesn't exist.
func TestRouteSyncSingle(t *testing.T) {
	if got := RouteStream.Sync(1); got != SyncSingle {
		t.Errorf("one endpoint on stream = %q, want %q", got, SyncSingle)
	}
	if got := RouteStream.Sync(2); got != SyncBuffered {
		t.Errorf("two endpoints on stream = %q, want %q", got, SyncBuffered)
	}
}

// hiResProv decodes above what AirPlay can carry — the case the whole
// never-downsample rule exists for, and the one no provider in this repo hits
// yet. It is written as a test fixture rather than waited for, because the
// router's behaviour on the day it appears should be settled now.
type hiResProv struct {
	*fakeProvider
	streamImpl
	format PCMFormat
}

func (p hiResProv) DecodedFormat() PCMFormat { return p.format }

func hiRes(f PCMFormat) Provider {
	return hiResProv{
		fakeProvider: &fakeProvider{routes: RouteSet{RouteAirPlay, RouteStream}},
		streamImpl:   streamImpl{Availability{OK: true, Configured: true}},
		format:       f,
	}
}

// The payoff: a source AirPlay cannot carry does not fail, and is not reduced.
// It takes the next route down, which serves any format intact.
func TestHiResRoutesAroundAirPlayRatherThanBeingReduced(t *testing.T) {
	eps := []Endpoint{
		ep("Study", VendorKEF, CapAirPlay|CapPlayURI, ""),
		ep("Hall", VendorKEF, CapAirPlay|CapPlayURI, ""),
	}
	p := hiRes(PCMFormat{SampleRate: 96000, BitDepth: 24, Channels: 2, LittleEndian: true})

	plan, err := Resolve(p, eps)
	if err != nil {
		t.Fatalf("a hi-res source must still play: %v", err)
	}
	if plan.Route != RouteStream {
		t.Errorf("route = %s, want stream — AirPlay would have had to reduce it", plan.Route)
	}
}

// And the rejection says which two formats disagree. "AirPlay unavailable"
// would leave someone unable to tell a network fault from a format they chose.
func TestAirPlayRejectionNamesBothFormats(t *testing.T) {
	eps := []Endpoint{ep("Study", VendorKEF, CapAirPlay, "")}
	p := hiRes(PCMFormat{SampleRate: 44100, BitDepth: 24, Channels: 2, LittleEndian: true})

	_, err := Resolve(p, eps)
	if err == nil {
		t.Fatal("an AirPlay-only zone can't carry 24-bit, so nothing should serve it")
	}
	var rerr *RouteError
	if !errors.As(err, &rerr) {
		t.Fatalf("error = %v, want a RouteError naming each rejection", err)
	}
	var reason string
	for _, b := range rerr.Blocked {
		if b.Route == RouteAirPlay {
			reason = b.Reason
		}
	}
	for _, want := range []string{"44.1 kHz · 16-bit", "44.1 kHz · 24-bit", "won't reduce"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q should mention %q", reason, want)
		}
	}
}

// CD-quality sources are untouched by any of this: AirPlay carries them
// exactly, and it must stay ranked above the clockless stream route for them.
func TestCDQualityStillPrefersAirPlay(t *testing.T) {
	eps := []Endpoint{
		ep("Study", VendorKEF, CapAirPlay|CapPlayURI, ""),
		ep("Hall", VendorKEF, CapAirPlay|CapPlayURI, ""),
	}
	plan, err := Resolve(hiRes(CDQuality), eps)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Route != RouteAirPlay {
		t.Errorf("route = %s, want airplay — nothing is being reduced here", plan.Route)
	}
}

// A carrier holds anything at or below itself, and nothing above it in any
// single dimension. The asymmetry is the point: 16-bit audio over a 24-bit
// link is wasteful and lossless, 24-bit over a 16-bit link is not.
func TestCarriesIsOneDirectional(t *testing.T) {
	cases := []struct {
		name  string
		limit PCMFormat
		src   PCMFormat
		want  bool
	}{
		{"same", CDQuality, CDQuality, true},
		{"deeper words", CDQuality, PCMFormat{44100, 24, 2, true}, false},
		{"faster rate", CDQuality, PCMFormat{96000, 16, 2, true}, false},
		{"more channels", CDQuality, PCMFormat{44100, 16, 6, true}, false},
		{"below the carrier is fine", PCMFormat{96000, 24, 2, true}, CDQuality, true},
	}
	for _, tc := range cases {
		if got := tc.limit.Carries(tc.src); got != tc.want {
			t.Errorf("%s: Carries = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A provider that cannot say what it decodes to is not assumed to be hi-res.
// Guessing the other way would silently strip AirPlay from every zone the
// moment a provider forgot to implement one optional interface.
func TestASilentProviderIsNotAssumedHiRes(t *testing.T) {
	eps := []Endpoint{ep("Study", VendorKEF, CapAirPlay, "")}
	p := &fakeProvider{routes: RouteSet{RouteAirPlay}, stream: true,
		streamAvail: Availability{OK: true, Configured: true}}
	plan, err := Resolve(p.build(), eps)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Route != RouteAirPlay {
		t.Errorf("route = %s, want airplay", plan.Route)
	}
}
