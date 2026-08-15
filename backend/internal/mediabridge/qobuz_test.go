package mediabridge

import (
	"context"
	"strings"
	"testing"

	"homehub/internal/media"
	"homehub/internal/qobuz"
)

// fakeAccount is a Qobuz account at a chosen entitlement. Quality reporting is
// built entirely on MaxFormat, so being able to vary it is the whole point of
// the provider taking an interface.
type fakeAccount struct {
	status qobuz.Status
	max    qobuz.FormatID
	// tracks is what a URI expands to, so ItemFormat can be exercised
	// against catalogue entries of different formats.
	tracks []qobuz.Item
}

func (a fakeAccount) Status() qobuz.Status      { return a.status }
func (a fakeAccount) MaxFormat() qobuz.FormatID { return a.max }
func (a fakeAccount) Search(context.Context, string, int) (*qobuz.Results, error) {
	return &qobuz.Results{}, nil
}
func (a fakeAccount) Favorites(context.Context, int) ([]qobuz.Item, error) { return nil, nil }
func (a fakeAccount) Tracks(context.Context, string) ([]qobuz.Item, error) {
	return a.tracks, nil
}

// withEntitlement builds a signed-in provider entitled to max.
func withEntitlement(t *testing.T, max qobuz.FormatID) *QobuzProvider {
	t.Helper()
	return NewQobuzProvider(fakeAccount{
		status: qobuz.Status{Configured: true, Connected: true, Plan: "Studio", MaxFormat: max},
		max:    max,
	}, nil)
}

// The claim this whole provider exists to make. A lossless source over a
// lossless transport is the one chain in this system that can honestly reach
// "lossless" — but only as a ceiling, because the track decides its own rate
// within the entitlement.
func TestQobuzReportsLosslessAsACeiling(t *testing.T) {
	p := withEntitlement(t, qobuz.FormatHiRes192)
	q := p.SourceQuality(media.RouteStream)

	if !q.Lossless || q.Codec != media.CodecFLAC {
		t.Errorf("source = %+v, want lossless FLAC", q)
	}
	if q.SampleRate != 192000 || q.BitDepth != 24 {
		t.Errorf("ceiling = %d Hz/%d-bit, want 192000/24", q.SampleRate, q.BitDepth)
	}
	// Approximate, because a 16-bit album on a hi-res plan arrives at 16-bit
	// and printing 24 for it would be inventing.
	if !q.Approximate {
		t.Error("the entitlement is a ceiling, not a measurement")
	}

	chain := media.DescribeQuality(p, media.RouteStream, media.QualityBest)
	if chain.Verdict != media.VerdictUpTo {
		t.Errorf("verdict = %q, want up_to", chain.Verdict)
	}
	if chain.LimitedBy != "" {
		t.Errorf("nothing caps this chain, but it blames %q", chain.LimitedBy)
	}
	// And there is nothing to offer: this is as good as the system gets.
	if chain.Fix != nil {
		t.Errorf("nothing to fix, got %+v", chain.Fix)
	}
}

// A CD-only subscription is still lossless, and must not be dressed up as
// hi-res. This is the number a "24-bit" badge would be built on.
func TestQobuzCDPlanIsLosslessButNotHiRes(t *testing.T) {
	q := withEntitlement(t, qobuz.FormatCD).SourceQuality(media.RouteStream)
	if !q.Lossless {
		t.Error("a CD-quality FLAC is lossless")
	}
	if q.SampleRate != 44100 || q.BitDepth != 16 {
		t.Errorf("ceiling = %d Hz/%d-bit, want CD quality", q.SampleRate, q.BitDepth)
	}
}

// A subscription with no lossless entitlement must not be reported as
// lossless just because the provider is Qobuz. The point of this route is the
// FLAC, and without it there is nothing to claim.
func TestQobuzWithoutLosslessEntitlementSaysSo(t *testing.T) {
	p := withEntitlement(t, qobuz.FormatMP3320)

	q := p.SourceQuality(media.RouteStream)
	if q.Lossless || q.Codec != media.CodecMP3 {
		t.Errorf("source = %+v, want a lossy report", q)
	}
	chain := media.DescribeQuality(p, media.RouteStream, media.QualityBest)
	if chain.Verdict != media.VerdictCapped {
		t.Errorf("verdict = %q, want capped", chain.Verdict)
	}
	// And streaming is refused before the tap rather than quietly serving
	// MP3 down a path the UI calls lossless.
	if av := p.StreamAvailable(); av.OK {
		t.Error("a lossy-only subscription must not be streamable here")
	}
}

// The router has to know the decode is hi-res before opening anything, or it
// plans an AirPlay cast that has to be refused once the file arrives.
func TestQobuzDeclaresItsDecodedFormat(t *testing.T) {
	got := withEntitlement(t, qobuz.FormatHiRes192).DecodedFormat()
	want := media.PCMFormat{SampleRate: 192000, BitDepth: 24, Channels: 2, LittleEndian: true}
	if got != want {
		t.Errorf("decoded format = %+v, want %+v", got, want)
	}
	if media.CDQuality.Carries(got) {
		t.Error("AirPlay must not be considered able to carry a 24-bit decode")
	}
}

// Routing is decided by the track, not the subscription. A hi-res *plan* that
// blocked AirPlay for everything would refuse CD-quality albums on AirPlay-only
// receivers entirely — which is what this asserts can no longer happen.
func TestAirPlayIsDecidedByTheTrackNotThePlan(t *testing.T) {
	hi := media.PCMFormat{SampleRate: 192000, BitDepth: 24, Channels: 2, LittleEndian: true}
	if _, yes := media.RouteReduces(media.RouteAirPlay, &hi); !yes {
		t.Error("AirPlay can't carry 24/192 and should say so")
	}
	cd := media.CDQuality
	if _, yes := media.RouteReduces(media.RouteAirPlay, &cd); yes {
		t.Error("a CD-quality track fits AirPlay exactly and must keep it")
	}
	// Even on a plan entitled to far more.
	if got := withEntitlement(t, qobuz.FormatHiRes192).ItemFormatFor(cd.SampleRate, cd.BitDepth); got != cd {
		t.Errorf("a CD track on a hi-res plan resolves to %+v, want CD quality", got)
	}
}

// The track and the subscription both cap, and the smaller wins. Getting this
// backwards in either direction is a real failure: take only the plan and a CD
// album is refused on AirPlay; take only the catalogue and a CD-only plan is
// reported as playing 24-bit.
func TestItemFormatTakesTheSmallerOfTrackAndPlan(t *testing.T) {
	cases := []struct {
		name      string
		plan      qobuz.FormatID
		rate, dep int
		wantRate  int
		wantDepth int
	}{
		{"hi-res plan, hi-res track", qobuz.FormatHiRes192, 192000, 24, 192000, 24},
		{"hi-res plan, CD track", qobuz.FormatHiRes192, 44100, 16, 44100, 16},
		{"CD plan, hi-res track", qobuz.FormatCD, 192000, 24, 44100, 16},
		{"hi-res plan, 96k track", qobuz.FormatHiRes192, 96000, 24, 96000, 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withEntitlement(t, tc.plan).ItemFormatFor(tc.rate, tc.dep)
			if got.SampleRate != tc.wantRate || got.BitDepth != tc.wantDepth {
				t.Errorf("format = %d Hz/%d-bit, want %d/%d",
					got.SampleRate, got.BitDepth, tc.wantRate, tc.wantDepth)
			}
		})
	}
}

// A catalogue that says nothing about a track leaves the entitlement as the
// only thing known, and that is a ceiling — the one case where being
// conservative is right, because guessing low would claim a quality nobody
// verified.
func TestItemFormatFallsBackToTheEntitlement(t *testing.T) {
	got := withEntitlement(t, qobuz.FormatHiRes96).ItemFormatFor(0, 0)
	if got.SampleRate != 96000 || got.BitDepth != 24 {
		t.Errorf("format = %+v, want the entitlement ceiling", got)
	}
}

// End to end through the catalogue: a CD-quality album on a hi-res plan
// resolves to CD quality, which is what keeps it on AirPlay.
func TestItemFormatReadsTheCatalogue(t *testing.T) {
	p := NewQobuzProvider(fakeAccount{
		status: qobuz.Status{Configured: true, Connected: true, MaxFormat: qobuz.FormatHiRes192},
		max:    qobuz.FormatHiRes192,
		tracks: []qobuz.Item{{ID: "1", SampleRate: 44100, BitDepth: 16}},
	}, nil)

	got, err := p.ItemFormat(context.Background(), media.Item{URI: "qobuz:album:x"})
	if err != nil {
		t.Fatalf("item format: %v", err)
	}
	if got != media.CDQuality {
		t.Errorf("format = %+v, want CD quality", got)
	}
	if _, yes := media.RouteReduces(media.RouteAirPlay, &got); yes {
		t.Error("a CD album must not be pushed off AirPlay by a hi-res plan")
	}
}

// An unconfigured provider explains which of the two credentials is missing,
// because they are issued to different parties and found in different places.
func TestQobuzNamesWhichSetupStepIsMissing(t *testing.T) {
	blank := NewQobuzProvider(fakeAccount{}, nil)
	if av := blank.Available(); av.OK || !strings.Contains(av.Reason, "app ID") {
		t.Errorf("reason = %q, want the app-credentials step", av.Reason)
	}

	configured := NewQobuzProvider(fakeAccount{status: qobuz.Status{Configured: true}}, nil)
	av := configured.Available()
	if av.OK || !strings.Contains(av.Reason, "Sign in") {
		t.Errorf("reason = %q, want the sign-in step", av.Reason)
	}
	if !av.Configured {
		t.Error("app credentials are present, so the provider is configured")
	}

	// And a server with no Qobuz client at all must not panic on the way to
	// saying so — the nil-interface trap the API layer guards against.
	if av := NewQobuzProvider(nil, nil).Available(); av.OK || av.Reason == "" {
		t.Errorf("an unwired provider must explain itself, got %+v", av)
	}
}

// Qobuz advertises only the routes where HomeHub holds the audio. Advertising
// a route it cannot serve would have the router pick it and fail at the tap.
func TestQobuzAdvertisesOnlyDecodedRoutes(t *testing.T) {
	routes := withEntitlement(t, qobuz.FormatCD).Routes()
	for _, r := range []media.Route{media.RouteNative, media.RouteConnect, media.RouteGroup} {
		if routes.Has(r) {
			t.Errorf("%s is advertised, but no speaker holds a Qobuz account link", r)
		}
	}
	for _, r := range []media.Route{media.RouteStream, media.RouteAirPlay} {
		if !routes.Has(r) {
			t.Errorf("%s should be advertised", r)
		}
	}
}
