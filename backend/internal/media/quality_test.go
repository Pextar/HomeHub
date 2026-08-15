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
		// A lossless ceiling hedges exactly as a bitrate ceiling does.
		{Quality{Codec: CodecFLAC, SampleRate: 44100, BitDepth: 24, Lossless: true, Approximate: true},
			"FLAC up to 44.1 kHz · 24-bit"},
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

// tieredProv is the shape Spotify took on the day it launched a lossless tier:
// lossless on the routes a speaker serves from its own account link, and lossy
// on the two HomeHub decodes for, because the decoder it runs can only fetch
// the compressed stream. It is the case that broke the old one-answer-per-
// service model, and the reason a chain has to ask the route.
type tieredProv struct {
	*fakeProvider
	streamImpl
	bitrate int
}

func (p tieredProv) SourceQuality(r Route) Quality {
	if r == RouteStream || r == RouteAirPlay {
		return Quality{Codec: CodecVorbis, SampleRate: 44100, Channels: 2, BitrateKbps: p.bitrate}
	}
	return Quality{Codec: CodecFLAC, SampleRate: 44100, BitDepth: 24, Channels: 2,
		Lossless: true, Approximate: true}
}

func (tieredProv) SourceDetail(r Route) string { return "the caveat for " + string(r) }

func tiered(bitrate int) Provider {
	return tieredProv{
		fakeProvider: &fakeProvider{routes: RouteSet{
			RouteNative, RouteConnect, RouteAirPlay, RouteStream}},
		streamImpl: streamImpl{Availability{OK: true, Configured: true}},
		bitrate:    bitrate,
	}
}

// A ceiling is not a measurement. When the speaker holds the account, HomeHub
// never learns what it negotiated — so the answer is "up to", nothing is
// blamed, and the flat lossless claim is withheld. Both of the wrong answers
// here are bad in their own direction: "lossless" invents a reading, and "not
// lossless" tells someone with a lossless plan their system is worse than it is.
func TestSpeakerHeldLosslessIsACeilingNotAClaim(t *testing.T) {
	for _, r := range []Route{RouteNative, RouteConnect} {
		chain := DescribeQuality(tiered(320), r, QualityBest)
		if chain.Verdict != VerdictUpTo {
			t.Errorf("%s: verdict = %q, want up_to", r, chain.Verdict)
		}
		if chain.Lossless {
			t.Errorf("%s: an unmeasured ceiling must not read as measured lossless", r)
		}
		if chain.LimitedBy != "" {
			t.Errorf("%s: nothing caps this, but it blames %q", r, chain.LimitedBy)
		}
		if !strings.Contains(chain.Summary, "up to") {
			t.Errorf("%s: summary should hedge: %q", r, chain.Summary)
		}
		if chain.Fix != nil {
			t.Errorf("%s: the speaker chooses here, so there is nothing to offer", r)
		}
	}
}

// The attribution that matters. When the service has a lossless tier and
// HomeHub's decoder is what can't fetch it, blaming the service's catalogue
// would tell a listener nothing can be done — when in fact one of their
// speakers can do better, and the fix says so.
func TestDecodedRouteBlamesTheDecoderNotTheService(t *testing.T) {
	for _, r := range []Route{RouteStream, RouteAirPlay} {
		chain := DescribeQuality(tiered(320), r, QualityBest)
		if chain.Verdict != VerdictCapped {
			t.Errorf("%s: verdict = %q, want capped", r, chain.Verdict)
		}
		if chain.LimitedBy != "HomeHub's decoder" {
			t.Errorf("%s: limited by %q, want the decoder", r, chain.LimitedBy)
		}
		if strings.Contains(chain.Summary, "compressed at the source") {
			t.Errorf("%s: this service is not the limit: %q", r, chain.Summary)
		}
		fix := chain.Fix
		if fix == nil {
			t.Fatalf("%s: leaving the decoded route is a real improvement", r)
		}
		if fix.Setting != "" {
			t.Errorf("%s: this is not a switch HomeHub owns, got setting %q", r, fix.Setting)
		}
		if !strings.Contains(fix.Label, "one speaker") {
			t.Errorf("%s: fix should name the move: %q", r, fix.Label)
		}
	}
}

// A service that is lossy everywhere keeps the old answer. The decoder is only
// to blame when there is something better it failed to reach.
func TestAServiceLossyEverywhereIsStillTheLimit(t *testing.T) {
	chain := DescribeQuality(lossy(320), RouteStream, QualityBest)
	if chain.LimitedBy != "Fake" {
		t.Errorf("limited by %q, want the service", chain.LimitedBy)
	}
	if chain.Fix != nil {
		t.Errorf("there is nowhere better to send them, got %+v", chain.Fix)
	}
}

// Below-best decoding still offers the setting first — it is the bigger,
// pressable win — but must not claim the service is the ceiling when it isn't.
func TestBelowBestFixNamesTheDecoderCeiling(t *testing.T) {
	chain := DescribeQuality(tiered(96), RouteStream, QualitySaver)
	if chain.Fix == nil || chain.Fix.Setting != FixSettingStreamQuality {
		t.Fatalf("the pressable lever comes first, got %+v", chain.Fix)
	}
	if strings.Contains(chain.Fix.Detail, "compressed at the source") {
		t.Errorf("that is the decoder's ceiling, not the service's: %q", chain.Fix.Detail)
	}
	if !strings.Contains(chain.Fix.Detail, "Even at best") {
		t.Errorf("detail should say what best still isn't: %q", chain.Fix.Detail)
	}
}

// The provider's own caveat reaches the chain, on every route. It is the only
// place a listener learns why the decoded routes cap where they do.
func TestSourceCaveatIsCarriedThrough(t *testing.T) {
	for _, r := range []Route{RouteNative, RouteStream} {
		if got := DescribeQuality(tiered(320), r, QualityBest).Source.Detail; got != "the caveat for "+string(r) {
			t.Errorf("%s: source detail = %q", r, got)
		}
	}
	// A provider without one is not made to invent one.
	if got := DescribeQuality(lossy(320), RouteStream, QualityBest).Source.Detail; got != "" {
		t.Errorf("detail = %q, want none", got)
	}
}

// Verdicts, pinned. These four are what a badge switches on, and collapsing
// any two of them back into a boolean is the regression this guards.
func TestVerdictsCoverTheFourAnswers(t *testing.T) {
	unknown := (&fakeProvider{routes: RouteSet{RouteNative}, native: true}).build()
	cases := []struct {
		name string
		got  Verdict
		want Verdict
	}{
		{"measured lossless", DescribeQuality(lossless(), RouteAirPlay, QualityBest).Verdict, VerdictLossless},
		{"ceiling", DescribeQuality(tiered(320), RouteNative, QualityBest).Verdict, VerdictUpTo},
		{"capped", DescribeQuality(lossy(320), RouteStream, QualityBest).Verdict, VerdictCapped},
		{"silent source", DescribeQuality(unknown, RouteNative, QualityBest).Verdict, VerdictUnknown},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: verdict = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}
