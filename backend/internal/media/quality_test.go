package media

import (
	"strings"
	"testing"
)

// lossyProv is a service like Spotify: compressed at the source, and it knows
// it. The bitrate it reports depends on the route, because on the routes
// HomeHub decodes for, the bitrate is HomeHub's choice.
type lossyProv struct {
	*fakeProvider
	streamImpl
	bitrate int
}

func (p lossyProv) SourceQuality(r Route) Quality {
	q := Quality{Codec: CodecVorbis, SampleRate: 44100, Channels: 2}
	switch r {
	case RouteStream, RouteAirPlay:
		q.BitrateKbps = p.bitrate
	default:
		q.BitrateKbps = 320
		q.Approximate = true
	}
	return q
}

func lossy(bitrate int) Provider {
	return lossyProv{
		fakeProvider: &fakeProvider{routes: RouteSet{RouteNative, RouteAirPlay, RouteStream}},
		streamImpl:   streamImpl{Availability{OK: true, Configured: true}},
		bitrate:      bitrate,
	}
}

// losslessProv is the service this codebase does not have yet, and the reason
// the chain is modelled as two stages rather than one boolean: the day a
// lossless source appears, the transport's honesty has to still hold.
type losslessProv struct {
	*fakeProvider
	streamImpl
}

func (losslessProv) SourceQuality(Route) Quality {
	return Quality{Codec: CodecFLAC, SampleRate: 44100, BitDepth: 16, Channels: 2, Lossless: true}
}

func lossless() Provider {
	return losslessProv{
		fakeProvider: &fakeProvider{routes: RouteSet{RouteAirPlay, RouteStream}},
		streamImpl:   streamImpl{Availability{OK: true, Configured: true}},
	}
}

// The central claim: a lossless transport carrying a lossy source is not
// lossless, and the blame lands on the service rather than on the route.
// Getting this backwards would have the UI tell a user their Spotify is
// bit-perfect because it travelled over AirPlay.
func TestLossySourceIsNeverLosslessHoweverItTravels(t *testing.T) {
	for _, route := range []Route{RouteNative, RouteAirPlay, RouteStream} {
		chain := DescribeQuality(lossy(320), route, QualityBest)
		if chain.Lossless {
			t.Errorf("%s: a Vorbis source must not report lossless", route)
		}
		if chain.LimitedBy != "Fake" {
			t.Errorf("%s: limited by %q, want the service", route, chain.LimitedBy)
		}
		if !strings.Contains(chain.Summary, "compressed at the source") {
			t.Errorf("%s: summary should say whose limit it is: %q", route, chain.Summary)
		}
		// The transport still reports itself honestly — it is what makes
		// "the source is the limit" a claim rather than a shrug.
		if !chain.Transport.Quality.Lossless {
			t.Errorf("%s: the transport does not re-encode and should say so", route)
		}
	}
}

func TestLosslessSourceOverALosslessTransport(t *testing.T) {
	chain := DescribeQuality(lossless(), RouteAirPlay, QualityBest)
	if !chain.Lossless {
		t.Error("FLAC over AirPlay is bit-exact end to end")
	}
	if chain.LimitedBy != "" {
		t.Errorf("nothing limits it, got %q", chain.LimitedBy)
	}
	if !strings.Contains(chain.Summary, "bit-exact") {
		t.Errorf("summary = %q", chain.Summary)
	}
	if chain.Fix != nil {
		t.Errorf("nothing to fix, got %+v", chain.Fix)
	}
}

// A provider that says nothing must not be reported as lossless because the
// route happened to be. Unknown is its own answer.
func TestUnknownSourceIsNotAssumedGood(t *testing.T) {
	p := (&fakeProvider{routes: RouteSet{RouteNative}, native: true}).build()
	chain := DescribeQuality(p, RouteNative, QualityBest)
	if chain.Lossless {
		t.Error("an unreported source must not read as lossless")
	}
	if !strings.Contains(chain.Summary, "doesn't say") {
		t.Errorf("summary = %q", chain.Summary)
	}
}

// The fix is offered exactly where it works. On the routes the speaker serves
// for itself, the setting reaches nothing, and offering it there would be a
// control wired to a dead end.
func TestFixIsOfferedOnlyWhereTheSettingReaches(t *testing.T) {
	cases := []struct {
		route Route
		pref  StreamQuality
		want  bool
	}{
		{RouteStream, QualitySaver, true},
		{RouteAirPlay, QualityBalanced, true},
		{RouteStream, QualityBest, false},  // already at the top
		{RouteNative, QualitySaver, false}, // the speaker chooses, not us
	}
	for _, tc := range cases {
		chain := DescribeQuality(lossy(tc.pref.Bitrate()), tc.route, tc.pref)
		if got := chain.Fix != nil; got != tc.want {
			t.Errorf("%s at %s: fix offered = %v, want %v", tc.route, tc.pref, got, tc.want)
		}
	}
}

// When a fix exists on a lossy source it must not oversell itself: raising
// the bitrate does not make Spotify lossless, and a user who taps it and
// still sees "not lossless" was misled by the button.
func TestFixOnALossySourceSaysWhatItCannotDo(t *testing.T) {
	chain := DescribeQuality(lossy(96), RouteAirPlay, QualitySaver)
	if chain.Fix == nil {
		t.Fatal("a saver-quality decode has room to improve")
	}
	if !strings.Contains(chain.Fix.Detail, "still be compressed at the source") {
		t.Errorf("detail should not promise lossless: %q", chain.Fix.Detail)
	}
	if chain.Fix.Setting != "stream_quality" {
		t.Errorf("setting = %q", chain.Fix.Setting)
	}
}

func TestStreamQualityMapsToBitrates(t *testing.T) {
	cases := map[StreamQuality]int{
		QualityBest:     320,
		QualityBalanced: 160,
		QualitySaver:    96,
		"":              320, // never chosen resolves to the default
		"nonsense":      320, // and so does a hand-edited settings file
	}
	for q, want := range cases {
		if got := q.Bitrate(); got != want {
			t.Errorf("%q → %d kbps, want %d", q, got, want)
		}
	}
	if QualityBest.Normalize() != QualityBest || StreamQuality("x").Valid() {
		t.Error("Valid/Normalize disagree about what is a real setting")
	}
}

func TestQualityLabelsReadLikeEquipmentLabels(t *testing.T) {
	cases := []struct {
		q    Quality
		want string
	}{
		{Quality{Codec: CodecPCM, SampleRate: 44100, BitDepth: 16, Lossless: true},
			"PCM 44.1 kHz · 16-bit"},
		{Quality{Codec: CodecVorbis, BitrateKbps: 320}, "Ogg Vorbis 320 kbps"},
		{Quality{Codec: CodecVorbis, BitrateKbps: 320, Approximate: true},
			"Ogg Vorbis up to 320 kbps"},
		{Quality{Codec: CodecALAC, SampleRate: 48000, BitDepth: 24, Lossless: true},
			"ALAC 48 kHz · 24-bit"},
		{Quality{}, "unknown"},
	}
	for _, tc := range cases {
		if got := tc.q.Label(); got != tc.want {
			t.Errorf("label = %q, want %q", got, tc.want)
		}
	}
}

// The transport claim per route, pinned. If someone adds a re-encode to a
// route later, this is what should stop them shipping it silently.
func TestTransportQualityPerRoute(t *testing.T) {
	for _, r := range []Route{RouteNative, RouteGroup, RouteConnect, RouteStream, RouteAirPlay} {
		if !TransportQuality(r).Lossless {
			t.Errorf("%s: no route in this system re-encodes", r)
		}
	}
	if q := TransportQuality(RouteAirPlay); q.SampleRate != 44100 || q.BitDepth != 16 {
		t.Errorf("airplay carries CD quality, got %+v", q)
	}
}
