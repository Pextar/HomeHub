package media

// What the audio actually is by the time it comes out of a speaker, and who
// decided that.
//
// The question "am I hearing lossless?" has no single answer in a system like
// this, because the audio passes through two hands: the service that encoded
// it, and the path it took to the speaker. A lossless path carrying a lossy
// source is not lossless, and neither is a lossless source re-encoded on the
// way. So quality is reported as a chain with the weakest link named, rather
// than as one badge — a badge would have to pick a lie.
//
// The rule this file holds to is the same one the capability model holds to:
// say what is true, including when the truth is "this cannot be fixed here."
// A UI that offers to make Spotify lossless is worse than one that explains
// why it cannot be.
//
// That rule cuts both ways, which is what the Verdict type below is for. When
// a service adds a lossless tier that only some of the routes can carry, the
// honest report is neither "lossless" everywhere nor "lossy" everywhere: it is
// lossless where HomeHub can see it, capped where something in the chain caps
// it *with that thing named*, and "up to" where the speaker holds the account
// and never says what it negotiated.

import "fmt"

// Codec is an audio encoding, named the way a listener would name it.
type Codec string

const (
	CodecPCM     Codec = "pcm"
	CodecALAC    Codec = "alac"
	CodecFLAC    Codec = "flac"
	CodecVorbis  Codec = "vorbis"
	CodecAAC     Codec = "aac"
	CodecMP3     Codec = "mp3"
	CodecUnknown Codec = ""
)

// Lossless reports whether a codec preserves its input exactly. Note this is
// about the codec alone: PCM carrying a decoded Vorbis stream is a lossless
// container for audio that was already thrown away, which is exactly why
// Chain exists.
func (c Codec) Lossless() bool {
	switch c {
	case CodecPCM, CodecALAC, CodecFLAC:
		return true
	}
	return false
}

// Label is the codec as a listener would see it written.
func (c Codec) Label() string {
	switch c {
	case CodecPCM:
		return "PCM"
	case CodecALAC:
		return "ALAC"
	case CodecFLAC:
		return "FLAC"
	case CodecVorbis:
		return "Ogg Vorbis"
	case CodecAAC:
		return "AAC"
	case CodecMP3:
		return "MP3"
	}
	return "unknown"
}

// Quality is one description of audio.
//
// Every field is optional because the honest answer is often partial: a Sonos
// streaming Spotify from its own account link does not tell HomeHub what
// bitrate it negotiated, and inventing 320 because that is the likely value
// would be a number the UI renders in a monospace font as though it were
// measured.
type Quality struct {
	Codec      Codec `json:"codec,omitempty"`
	SampleRate int   `json:"sample_rate,omitempty"` // Hz
	BitDepth   int   `json:"bit_depth,omitempty"`
	Channels   int   `json:"channels,omitempty"`
	// BitrateKbps is zero for lossless and for unknown, which are told
	// apart by Lossless rather than by the number.
	BitrateKbps int `json:"bitrate_kbps,omitempty"`
	// Lossless is whether this stage preserves what it was handed.
	Lossless bool `json:"lossless"`
	// Approximate marks a description HomeHub could not measure — the
	// speaker's own session, whose parameters it never sees. The UI says
	// "up to" rather than stating it flatly.
	Approximate bool `json:"approximate,omitempty"`
}

// Label renders a quality the way it should be shown: codec first, then the
// number that matters for that codec.
func (q Quality) Label() string {
	if q.Codec == CodecUnknown && q.BitrateKbps == 0 && q.SampleRate == 0 {
		return "unknown"
	}
	base := q.Codec.Label()
	switch {
	case q.Lossless && q.SampleRate > 0 && q.BitDepth > 0 && q.Approximate:
		// The ceiling of a session HomeHub can't see into. "up to" belongs on
		// a lossless description for exactly the same reason it belongs on a
		// bitrate: it is the most the route can carry, not a measurement.
		return fmt.Sprintf("%s up to %s · %d-bit", base, khz(q.SampleRate), q.BitDepth)
	case q.Lossless && q.SampleRate > 0 && q.BitDepth > 0:
		return fmt.Sprintf("%s %s · %d-bit", base, khz(q.SampleRate), q.BitDepth)
	case q.BitrateKbps > 0 && q.Approximate:
		return fmt.Sprintf("%s up to %d kbps", base, q.BitrateKbps)
	case q.BitrateKbps > 0:
		return fmt.Sprintf("%s %d kbps", base, q.BitrateKbps)
	case q.SampleRate > 0:
		return fmt.Sprintf("%s %s", base, khz(q.SampleRate))
	}
	return base
}

// khz formats a sample rate the way audio equipment labels it: 44.1 kHz, not
// 44100 Hz, and no trailing ".0".
func khz(hz int) string {
	if hz%1000 == 0 {
		return fmt.Sprintf("%d kHz", hz/1000)
	}
	return fmt.Sprintf("%.1f kHz", float64(hz)/1000)
}

// Stage is one link in the chain, with the name of whatever is responsible for
// it — a service, a route, a speaker — because "not lossless" is only useful
// when it comes with "because of this."
type Stage struct {
	// Name is what to blame or credit: "Spotify", "AirPlay to Study".
	Name    string  `json:"name"`
	Quality Quality `json:"quality"`
	// Detail is one clause of explanation, shown under the label.
	Detail string `json:"detail,omitempty"`
}

// Verdict is the end-to-end answer, in the states it actually has rather than
// the two a boolean allows.
//
// The third state is the one Spotify's lossless tier created. On a route where
// the speaker holds the account, HomeHub never learns what quality that speaker
// negotiated: the ceiling is lossless, whether it was reached depends on the
// household's plan, its market and that speaker's own settings, and none of
// those are readable from here. Calling it lossless claims a measurement nobody
// took; calling it lossy tells a listener with a lossless plan that their
// system is worse than it is. So it gets its own value and the UI says "up to".
type Verdict string

const (
	// VerdictLossless: bit-exact end to end, and both stages are known
	// rather than inferred.
	VerdictLossless Verdict = "lossless"
	// VerdictUpTo: nothing in the chain is lossy, but at least one stage is
	// a ceiling rather than a measurement. LimitedBy is empty — nothing is
	// capping this, HomeHub simply cannot see the far end.
	VerdictUpTo Verdict = "up_to"
	// VerdictCapped: something in the chain is lossy and LimitedBy names it.
	VerdictCapped Verdict = "capped"
	// VerdictUnknown: the source says nothing about itself, so neither can
	// this report.
	VerdictUnknown Verdict = "unknown"
)

// Chain is the whole path, source to speaker.
type Chain struct {
	// Source is what the service hands over; Transport is how it travels.
	Source    Stage `json:"source"`
	Transport Stage `json:"transport"`
	// Verdict is the end-to-end answer and the field a badge should read.
	Verdict Verdict `json:"verdict"`
	// Lossless is true only for VerdictLossless — bit-exact and *known* to
	// be. It is deliberately false for VerdictUpTo: a chain that can carry
	// lossless is not a chain known to have carried it, and a caller that
	// wants the softer claim should read Verdict.
	Lossless bool `json:"lossless"`
	// LimitedBy names the stage that caps the result, empty when nothing
	// does. This is the field the UI leads with on VerdictCapped.
	LimitedBy string `json:"limited_by,omitempty"`
	// Summary is the chain in one sentence, fit to show a person.
	Summary string `json:"summary"`
	// Fix is what the user could change to improve it, or nil when there is
	// nothing honest to offer. Nil is a real answer and the UI must render
	// it as one rather than as a disabled button.
	Fix *Fix `json:"fix,omitempty"`
}

// Fix is an improvement actually available, phrased as the change it makes.
type Fix struct {
	// Setting is the machine-readable lever. "stream_quality" is the only
	// value that maps to a control — it is what PUT /api/media/quality
	// accepts. Empty means the improvement is real but there is nothing to
	// press: it is something the listener does to their zone, not something
	// HomeHub can do for them, and the UI must render it as a sentence.
	Setting string `json:"setting"`
	// Label is the offer in the user's words.
	Label string `json:"label"`
	// Detail says what it will and will not achieve, which for a lossy
	// source is most of the point.
	Detail string `json:"detail,omitempty"`
}

// StreamQuality is the household's preference for the audio HomeHub decodes
// itself — the stream and AirPlay routes, the two paths where the bitrate is
// HomeHub's decision rather than the speaker's.
//
// It does nothing to the native and Connect routes: there the speaker holds
// the account and negotiates its own quality, and a setting that claimed
// otherwise would be a dial connected to nothing.
type StreamQuality string

const (
	// QualityBest decodes at the highest bitrate the service offers. The
	// default: the audio is re-served bit-exact over a LAN, so there is no
	// bandwidth reason to economise, and the decode is the only place
	// quality is lost.
	QualityBest StreamQuality = "best"
	// QualityBalanced is the service's middle tier.
	QualityBalanced StreamQuality = "balanced"
	// QualitySaver is the lowest tier, for a household on a metered or
	// congested link.
	QualitySaver StreamQuality = "saver"
)

// DefaultStreamQuality is what a household that has never chosen gets.
const DefaultStreamQuality = QualityBest

// Valid reports whether q is one of the three.
func (q StreamQuality) Valid() bool {
	switch q {
	case QualityBest, QualityBalanced, QualitySaver:
		return true
	}
	return false
}

// Normalize resolves an empty or unrecognised value to the default, so a
// hand-edited settings file cannot leave the decoder without a bitrate.
func (q StreamQuality) Normalize() StreamQuality {
	if q.Valid() {
		return q
	}
	return DefaultStreamQuality
}

// Bitrate is the kbps this preference asks the decoder for. The numbers are
// Spotify's tiers, which is what librespot takes; a second service with
// different tiers would map its own.
func (q StreamQuality) Bitrate() int {
	switch q.Normalize() {
	case QualityBalanced:
		return 160
	case QualitySaver:
		return 96
	}
	return 320
}

// Label is the preference as a person would read it.
func (q StreamQuality) Label() string {
	switch q.Normalize() {
	case QualityBalanced:
		return "Balanced"
	case QualitySaver:
		return "Data saver"
	}
	return "Best available"
}

// QualityReporter is a provider that can say what it delivers over a given
// route. A provider that doesn't implement it is reported as unknown rather
// than assumed — see the Quality doc comment on why guessing is worse.
type QualityReporter interface {
	// SourceQuality is what the service hands over for this route. The
	// route matters because the same service is a different thing over
	// each: a speaker's own account link negotiates its own bitrate, while
	// HomeHub's decoder uses the bitrate the household chose.
	SourceQuality(r Route) Quality
}

// QualityExplainer is a provider that can add a clause about its own source
// quality on a route. Optional and separate from QualityReporter because the
// caveat is the provider's to know: only Spotify knows that its lossless tier
// needs Premium with lossless switched on for that particular speaker, and only
// Spotify knows which of its routes HomeHub's decoder can't reach it on. A
// chain builder guessing at either would be inventing.
type QualityExplainer interface {
	SourceDetail(r Route) string
}

// FixSettingStreamQuality is the one Fix.Setting that maps to a control.
const FixSettingStreamQuality = "stream_quality"

// TransportQuality is what a route does to the audio between the service and
// the speaker. This is a property of the route alone, which is why it lives
// here rather than in a provider.
func TransportQuality(r Route) Quality {
	switch r {
	case RouteStream:
		// Decoded once and re-served as PCM. No re-encode: the 44-byte WAV
		// header is the entire conversion.
		return Quality{Codec: CodecPCM, SampleRate: 44100, BitDepth: 16, Channels: 2, Lossless: true}
	case RouteAirPlay:
		// PCM or uncompressed ALAC, depending on what the receiver asked
		// for. Both are bit-exact, so the distinction does not belong in a
		// quality report — the packing differs, the samples do not.
		return Quality{Codec: CodecPCM, SampleRate: 44100, BitDepth: 16, Channels: 2, Lossless: true}
	}
	// native, connect and group never re-encode: the speaker fetches the
	// service's own stream, so the transport adds nothing and takes nothing.
	return Quality{Lossless: true}
}

// transportDetail explains what a route does, in one clause.
func transportDetail(r Route) string {
	switch r {
	case RouteStream:
		return "HomeHub decodes once and re-serves the samples untouched over your network"
	case RouteAirPlay:
		return "HomeHub sends the samples untouched over AirPlay, on one clock"
	case RouteConnect:
		return "the service streams straight to the speaker"
	case RouteGroup:
		return "the speakers stream it themselves, grouped"
	}
	return "the speaker streams it itself"
}

// DescribeQuality builds the chain for a provider on a route, given what the
// household has asked HomeHub's own decoder for.
//
// It takes the route rather than the plan because that is all it needs, and
// because the caller has a plan only after resolving — a zone view wants to
// show what a tap *would* sound like, and resolving is exactly what tells it
// which route that is.
func DescribeQuality(p Provider, r Route, pref StreamQuality) Chain {
	source := Quality{}
	if qr, ok := p.(QualityReporter); ok {
		source = qr.SourceQuality(r)
	}
	transport := TransportQuality(r)
	transDetail := transportDetail(r)
	// A route that would have to reduce this source does not get to report
	// itself as a lossless carrier. It is lossless for audio it can carry,
	// and this is audio it cannot — so the honest transport stage is the
	// ceiling, named, with the note that HomeHub routes around it rather than
	// reducing to fit.
	if limit, decoded, yes := RouteReduces(p, r); yes {
		transport = Quality{
			Codec: CodecPCM, SampleRate: limit.SampleRate, BitDepth: limit.BitDepth,
			Channels: limit.Channels,
		}
		transDetail = fmt.Sprintf(
			"carries %s only; this decodes to %s, and HomeHub won't reduce it — a zone like this plays over HomeHub's stream instead",
			limit.Label(), decoded.Label())
	}

	c := Chain{
		Source:    Stage{Name: p.Name(), Quality: source},
		Transport: Stage{Name: routeLabel(r), Quality: transport, Detail: transDetail},
	}
	if qe, ok := p.(QualityExplainer); ok {
		c.Source.Detail = qe.SourceDetail(r)
	}

	switch {
	case source.Codec == CodecUnknown:
		// Nothing is known about the source. Say so rather than inferring
		// from the transport, which would report a lossy service as
		// lossless the moment it travelled over a lossless path.
		c.Verdict = VerdictUnknown
		c.LimitedBy = p.Name()
		c.Summary = fmt.Sprintf("%s doesn't say what quality it is sending", p.Name())
	case !source.Lossless:
		c.Verdict = VerdictCapped
		c.LimitedBy, c.Summary = cappedSource(p, r, source)
	case !transport.Lossless:
		c.Verdict = VerdictCapped
		c.LimitedBy = routeLabel(r)
		c.Summary = fmt.Sprintf("%s is lossless, but %s", p.Name(), transDetail)
	case source.Approximate || transport.Approximate:
		// Nothing caps this — LimitedBy stays empty — but the far end is a
		// ceiling rather than a reading, so the sentence says so plainly
		// instead of borrowing the flat claim below.
		c.Verdict = VerdictUpTo
		c.Summary = fmt.Sprintf("%s streams up to %s here; HomeHub can't see which quality %s settled on",
			p.Name(), source.Label(), routeLabel(r))
	default:
		c.Verdict = VerdictLossless
		c.Lossless = true
		c.Summary = fmt.Sprintf("%s at %s, bit-exact all the way to the speaker",
			p.Name(), source.Label())
	}
	c.Fix = fixFor(p, r, pref, source)
	return c
}

// cappedSource names who capped a lossy source and says it in a sentence.
//
// The distinction it draws is the whole reason this function exists. A service
// whose catalogue is lossy is capped by the service, and no route or setting
// changes that. A service that *has* a lossless tier which HomeHub's own
// decoder can't fetch is a different situation with a different answer: the
// service is not the limit, HomeHub is, and the listener has somewhere better
// to go. Blaming the catalogue in that second case is the kind of confident
// wrong sentence this whole file exists to avoid — it would tell someone their
// speakers can't do better when one of them can.
func cappedSource(p Provider, r Route, source Quality) (limitedBy, summary string) {
	if decodedRoute(r) && losslessOnSomeRoute(p) {
		return "HomeHub's decoder", fmt.Sprintf(
			"%s at %s — HomeHub decodes this path itself and can only fetch %s's compressed stream, so it caps below what %s can deliver",
			p.Name(), source.Label(), p.Name(), p.Name())
	}
	return p.Name(), fmt.Sprintf("%s at %s — %s, so nothing after it is lossless",
		p.Name(), source.Label(), lossyBecause(p.Name()))
}

// decodedRoute reports whether HomeHub holds the audio on this route — the two
// where its own decoder, rather than a speaker's account link, sets the ceiling.
func decodedRoute(r Route) bool { return r == RouteStream || r == RouteAirPlay }

// losslessOnSomeRoute reports whether this provider hands over lossless audio
// on any route where a speaker streams it directly. That is what makes a lossy
// decoded route HomeHub's limitation rather than the service's.
func losslessOnSomeRoute(p Provider) bool {
	qr, ok := p.(QualityReporter)
	if !ok {
		return false
	}
	routes := p.Routes()
	for _, r := range []Route{RouteNative, RouteGroup, RouteConnect} {
		if routes.Has(r) && qr.SourceQuality(r).Lossless {
			return true
		}
	}
	return false
}

// fixFor offers the one change that genuinely improves this chain, or nothing.
//
// Two improvements can be real here, and they are ranked by how much they buy.
// The first is the setting: how hard HomeHub's decoder asks the service to
// compress, which only moves the two routes HomeHub decodes for. The second
// only exists for a service with a lossless tier HomeHub's decoder can't
// fetch — there the listener can leave the decoded route entirely by playing to
// a single speaker that streams the service itself, and that is worth more than
// any bitrate. It is offered with an empty Setting because it is a thing they
// do to their zone, not a switch HomeHub owns.
//
// On a source that is lossy at its best everywhere, there is still no fix, and
// saying so remains the honest answer.
func fixFor(p Provider, r Route, pref StreamQuality, source Quality) *Fix {
	if !decodedRoute(r) {
		return nil
	}
	if pref.Normalize() != QualityBest {
		detail := fmt.Sprintf("Decoding at %d kbps rather than %d, because this household picked %s.",
			pref.Bitrate(), QualityBest.Bitrate(), pref.Label())
		if !source.Lossless {
			detail += " " + decodeCeiling(p)
		}
		return &Fix{
			Setting: FixSettingStreamQuality,
			Label:   fmt.Sprintf("Switch to %s", QualityBest.Label()),
			Detail:  detail,
		}
	}
	if !source.Lossless && losslessOnSomeRoute(p) {
		return &Fix{
			Label: fmt.Sprintf("Play this to one speaker that streams %s itself", p.Name()),
			Detail: fmt.Sprintf(
				"A single speaker on its own %s account link can fetch the lossless stream. HomeHub's decoder can't, so any zone it has to decode for caps here however this setting is set.",
				p.Name()),
		}
	}
	return nil
}

// decodeCeiling is the clause that says what the decoder's best still isn't,
// which differs by whether the service has a better tier at all.
func decodeCeiling(p Provider) string {
	if losslessOnSomeRoute(p) {
		return fmt.Sprintf("Even at best, HomeHub's decoder fetches %s's compressed stream rather than its lossless one.", p.Name())
	}
	return fmt.Sprintf("%s will still be compressed at the source — this is as close to the original as %s goes.",
		p.Name(), p.Name())
}

// lossyBecause is the half-sentence that says whose limit it is. Split out
// because it is the part most likely to need rewording per service, and
// because burying it in a format string is how it gets missed.
func lossyBecause(provider string) string {
	return fmt.Sprintf("%s's catalogue is compressed at the source", provider)
}

// routeLabel names a route for a person.
func routeLabel(r Route) string {
	switch r {
	case RouteNative:
		return "the speaker's own stream"
	case RouteGroup:
		return "the group's own stream"
	case RouteConnect:
		return "Connect"
	case RouteAirPlay:
		return "AirPlay"
	case RouteStream:
		return "HomeHub's stream"
	}
	return string(r)
}
