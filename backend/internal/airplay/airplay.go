// Package airplay is HomeHub's AirPlay sender: it finds RAOP receivers on the
// LAN and pushes audio to them.
//
// It exists for the case the other bridges cannot serve. Sonos and KEF each
// stream a service themselves, or take a URL and fetch it; an AirPlay receiver
// does neither. It is a sink — something else holds the audio and the clock,
// and sends it. That something is HomeHub, which is already the single decoder
// for the media protocol's stream route (see docs/MEDIA-PROTOCOL.md), so the
// audio is already sitting here in exactly the form a receiver wants.
//
// # What this speaks, and what that does and does not exclude
//
// This sender speaks classic AirPlay — AirPlay 1, RAOP. That is a statement
// about the session HomeHub opens, and it is worth being careful about what it
// implies for the receivers it can reach, because the obvious reading is
// wrong.
//
// A receiver being "AirPlay 2" does not put it out of reach. shairport-sync in
// AirPlay 2 mode — which is what a current RoPieee runs — keeps answering
// classic senders on the same port. AirPlay 2 changes what an iPhone chooses
// to speak to the box, not what the box will accept. So a receiver somebody
// plays to from the Spotify app over AirPlay 2 is, in the ordinary case, a
// receiver this package drives too.
//
// What genuinely is out of reach is a receiver that *requires* the AirPlay 2
// handshake: Apple's own speakers, where a HomeKit pairing exchange comes
// first. Implementing that means SRP pairing, a PTP clock and a plist-shaped
// setup — a large piece of work that would buy nothing for a shairport-sync
// box, since the classic path already carries bit-exact audio to it with a
// clock of its own.
//
// The rule this package therefore holds to: **the advertisement is advice, the
// receiver is the authority.** A scan asks each box whether it takes a classic
// session (probeClassic) instead of inferring it from a TXT record, and a
// session that is refused tries the other shapes it might take before giving
// up. An earlier version of this trusted the advertisement and would have
// turned away a working RoPieee for describing itself as AirPlay 2.
//
// # What reaches the speaker
//
// 16-bit 44.1 kHz stereo, sent as raw PCM when the receiver advertises it
// (cn=0) and as uncompressed ALAC frames otherwise. Both are bit-exact:
// nothing here re-encodes, and there is no encoder dependency, which is the
// same trade internal/stream makes when it serves WAV rather than MP3. The
// advertisement picks the first attempt; the receiver's answer picks the rest.
//
// # Honesty about sync
//
// Every receiver in a cast is driven from one decode and one clock: the sender
// stamps each packet with an RTP timestamp, answers the receivers' timing
// requests, and broadcasts a sync packet a second, so all of them play the
// same sample at the same wall-clock moment. That is materially better than
// the stream route, where each speaker fills its own buffer and starts when it
// is ready. It is still not a vendor's own multi-room bus — the clock is
// HomeHub's and it is disciplined over UDP on a home network — so the media
// layer reports it as `clocked`, between `exact` and `buffered`, and this
// package must not imply better.
package airplay

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"homehub/internal/lanhost"
)

// DefaultPort is the RTSP port shairport-sync and most receivers listen on.
// Only a fallback: the port comes from the SRV record when discovered, and
// from the user when typed.
const DefaultPort = 7000

// ValidateHost checks a receiver address before it is put in a URL, with the
// same policy the other LAN bridges use.
func ValidateHost(host string) error {
	return lanhost.Policy{Noun: "receiver address"}.Validate(host)
}

// Codec is one audio format a receiver will accept, as advertised in its `cn`
// key. The numbers are AirPlay's, not ours.
type Codec int

const (
	CodecPCM     Codec = 0 // raw 16-bit samples, network byte order
	CodecALAC    Codec = 1 // Apple Lossless, 352-frame packets
	CodecAAC     Codec = 2 // not sent by this package
	CodecAACELD  Codec = 3 // not sent by this package
	codecUnknown Codec = -1
)

func (c Codec) String() string {
	switch c {
	case CodecPCM:
		return "pcm"
	case CodecALAC:
		return "alac"
	case CodecAAC:
		return "aac"
	case CodecAACELD:
		return "aac-eld"
	}
	return "unknown"
}

// Encryption is one scheme a receiver will accept, from its `et` key.
type Encryption int

const (
	EncryptionNone Encryption = 0 // cleartext RTP
	EncryptionRSA  Encryption = 1 // AES-128-CBC, key wrapped with Apple's RSA key
	// The rest — FairPlay (3), MFiSAP (4), FairPlay SAPv2.5 (5) — need a
	// key exchange this package does not implement. A receiver offering
	// only those is reported as unsupported rather than attempted.
	EncryptionFairPlay Encryption = 3
	EncryptionMFiSAP   Encryption = 4
)

// Classic is whether a receiver accepts the classic (AirPlay 1) session this
// package sends.
//
// Tri-state on purpose. The mDNS advertisement describes what a receiver
// *prefers*, which on an AirPlay 2 box is a different question from what it
// will *accept* — so the advertisement is treated as advice and the receiver
// is asked. Until it has been asked, the honest value is "unknown", and an
// unknown is attempted rather than refused.
type Classic int

const (
	// ClassicUnknown is a receiver nobody has asked yet.
	ClassicUnknown Classic = iota
	// ClassicYes is a receiver that listed ANNOUNCE in its own OPTIONS
	// reply — it takes the classic session.
	ClassicYes
	// ClassicNo is a receiver that answered and refused: no ANNOUNCE in its
	// methods, or it would not talk to us at all.
	ClassicNo
)

func (c Classic) String() string {
	switch c {
	case ClassicYes:
		return "yes"
	case ClassicNo:
		return "no"
	}
	return "unknown"
}

// MarshalJSON encodes the three states as words rather than as 0/1/2, so the
// frontend cannot mistake the middle one for a boolean — which is the whole
// point of there being three.
func (c Classic) MarshalJSON() ([]byte, error) {
	return []byte(`"` + c.String() + `"`), nil
}

// Audio is the format a receiver takes. AirPlay 1 is 44.1/16/2 in practice —
// the fields exist because receivers advertise them and a value that disagrees
// is worth showing the user rather than silently overriding.
type Audio struct {
	SampleRate int `json:"sample_rate"`
	BitDepth   int `json:"bit_depth"`
	Channels   int `json:"channels"`
}

// Device is one AirPlay receiver as it advertises itself.
//
// Everything here except Name/IP/Port comes from the mDNS TXT record, which is
// the receiver describing its own abilities. It is kept rather than reduced to
// a boolean because it is what decides how audio is sent, and because a
// receiver that turns out to be unsupported should be able to say why in the
// user's words: "it only offers FairPlay" is actionable, "unsupported" is not.
type Device struct {
	// Name is the friendly name — for shairport-sync, what its config calls
	// the box, which is what RoPieee's own settings page shows.
	Name string `json:"name"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
	// ID is the receiver's stable identity: the MAC-shaped prefix of the
	// RAOP instance name ("B827EB1234AB@Kitchen"), normalised lower case
	// with no separators. Empty when the instance name has no prefix, in
	// which case the address is all there is to go on.
	ID string `json:"id"`
	// Model is the `am` key — "ShairportSync" for RoPieee, "AppleTV6,2" for
	// an Apple TV, and so on. Shown, never branched on.
	Model   string `json:"model,omitempty"`
	Version string `json:"version,omitempty"` // `vs`, the AirPlay version string

	Codecs     []Codec      `json:"-"`
	Encryption []Encryption `json:"-"`
	Audio      Audio        `json:"audio"`
	// NeedsPassword is the `pw` key. A receiver with a password set cannot
	// be driven by this package — RAOP password auth is a challenge the
	// sender answers, and shairport-sync users who set one are asking for
	// exactly the exclusion it provides — so it is surfaced as a reason.
	NeedsPassword bool `json:"needs_password"`
	// Metadata is whether the receiver accepts track info (`md`), which is
	// how RoPieee's display and Roon's "now playing" get filled in.
	Metadata bool `json:"metadata"`
	// AirPlay2 is whether the box also advertises the AirPlay 2 service.
	//
	// It does not mean HomeHub cannot drive it, and reading it that way is
	// the mistake worth naming here: shairport-sync in AirPlay 2 mode — what
	// a current RoPieee runs — keeps answering classic senders on the same
	// port, which is the session this package opens. What AirPlay 2 changes
	// is what an *iPhone* chooses to speak to it, not what the box will
	// accept. So this is carried for display and for a better refusal
	// message, and `Classic` is what actually decides.
	AirPlay2 bool `json:"airplay2,omitempty"`
	// Classic is whether the receiver takes the session this package sends,
	// as answered by the receiver itself. Three states, because "nobody has
	// asked" is not "no" — a scan asks, and a hand-typed address is asked at
	// registration.
	Classic Classic `json:"classic"`
	// Registered is set by the API layer when this device is already one of
	// the household's speakers, so a scan can show it as already added.
	Registered bool `json:"registered"`
}

// Addr is host:port for the RTSP control channel.
func (d Device) Addr() string { return fmt.Sprintf("%s:%d", d.IP, d.Port) }

func (d Device) String() string {
	return fmt.Sprintf("%s (%s, %s)", d.Name, d.Model, d.Addr())
}

// Codec picks what to send. PCM is preferred over ALAC when the receiver takes
// both: both are bit-exact at this bit depth, and PCM needs no frame packing
// at all, so the cheaper one wins. Returns false when the receiver offers
// nothing this package can produce.
func (d Device) Codec() (Codec, bool) {
	var alac bool
	for _, c := range d.Codecs {
		switch c {
		case CodecPCM:
			return CodecPCM, true
		case CodecALAC:
			alac = true
		}
	}
	if alac {
		return CodecALAC, true
	}
	return codecUnknown, false
}

// Cipher picks the encryption to use, preferring none. A receiver that will
// take cleartext is sent cleartext: the audio is already crossing the user's
// own LAN, the key exchange exists to satisfy Apple's licensing rather than
// the user's threat model, and skipping it removes a per-packet AES pass.
func (d Device) Cipher() (Encryption, bool) {
	var rsa bool
	for _, e := range d.Encryption {
		switch e {
		case EncryptionNone:
			return EncryptionNone, true
		case EncryptionRSA:
			rsa = true
		}
	}
	if rsa {
		return EncryptionRSA, true
	}
	// A receiver that advertised nothing usable. Treated as cleartext only
	// if it advertised nothing at all — an empty `et` is an old or terse
	// responder, not a refusal.
	if len(d.Encryption) == 0 {
		return EncryptionNone, true
	}
	return EncryptionNone, false
}

// Supported reports whether this package can drive the receiver, and when it
// cannot, why — in a sentence meant for a person, since it is shown next to
// the device in the scan results.
//
// The order of the checks is the argument. What the receiver *answered* comes
// first, because that is evidence; what it *advertised* comes second, because
// on an AirPlay 2 box the advertisement describes the protocol an iPhone would
// pick and not the one this package sends. An earlier version of this had the
// advertisement first, and it would have refused a perfectly usable RoPieee
// for saying "I am AirPlay 2" — a box that plays Spotify from a phone all day
// and would have played from HomeHub too.
func (d Device) Supported() (bool, string) {
	if d.NeedsPassword {
		return false, "this receiver asks for a password, which HomeHub can't answer"
	}
	if d.Classic == ClassicNo {
		if d.AirPlay2 {
			return false, "this is an AirPlay 2 receiver that won't take a classic session — " +
				"Apple's own speakers need a pairing exchange HomeHub can't perform"
		}
		return false, "it answered, but not to the AirPlay audio session HomeHub opens"
	}
	// A rate mismatch is a hard no whatever the receiver says about codecs:
	// resampling is the one lossy step this path must not take silently.
	if d.Audio.SampleRate != 0 && d.Audio.SampleRate != SampleRate {
		return false, fmt.Sprintf("it wants %d Hz audio and HomeHub sends %d Hz",
			d.Audio.SampleRate, SampleRate)
	}
	// Codecs and ciphers are only refused when the receiver listed some and
	// none of them is ours. An empty list is an AirPlay 2 advertisement that
	// simply has no classic keys in it, not a refusal — and the ANNOUNCE is
	// where a receiver that meant it says so.
	if len(d.Codecs) > 0 {
		if _, ok := d.Codec(); !ok {
			var names []string
			for _, c := range d.Codecs {
				names = append(names, c.String())
			}
			return false, fmt.Sprintf("it only accepts %s, and HomeHub sends PCM or ALAC",
				strings.Join(names, "/"))
		}
	}
	if len(d.Encryption) > 0 {
		if _, ok := d.Cipher(); !ok {
			return false, "it only accepts FairPlay-encrypted audio, which HomeHub can't produce"
		}
	}
	return true, ""
}

// attempt is one way of asking a receiver to take a session.
type attempt struct {
	codec  Codec
	cipher Encryption
}

// attempts lists the sessions to try, best first.
//
// More than one, because the advertisement can be wrong or absent and the only
// authority is the ANNOUNCE. A receiver that publishes no classic keys at all
// — the AirPlay 2 case — would otherwise be refused on a guess; instead it is
// offered ALAC in the clear, then ALAC with a key, which is what every RAOP
// receiver ever built accepts.
//
// Capped at four: past that a "no" is a no, and each attempt is a round trip
// standing between a tap and the music.
func (d Device) attempts() []attempt {
	codecs := []Codec{}
	if c, ok := d.Codec(); ok {
		codecs = append(codecs, c)
	}
	// ALAC last-resort, always. It is the one format the protocol has
	// required since the first AirPort Express.
	if !hasCodec(codecs, CodecALAC) {
		codecs = append(codecs, CodecALAC)
	}

	ciphers := []Encryption{}
	if c, ok := d.Cipher(); ok {
		ciphers = append(ciphers, c)
	}
	if !hasCipher(ciphers, EncryptionRSA) {
		ciphers = append(ciphers, EncryptionRSA)
	}

	out := make([]attempt, 0, len(codecs)*len(ciphers))
	for _, c := range codecs {
		for _, e := range ciphers {
			out = append(out, attempt{codec: c, cipher: e})
		}
	}
	return out
}

func hasCodec(list []Codec, want Codec) bool {
	for _, c := range list {
		if c == want {
			return true
		}
	}
	return false
}

func hasCipher(list []Encryption, want Encryption) bool {
	for _, e := range list {
		if e == want {
			return true
		}
	}
	return false
}

// fromTXT fills the advertised fields from a service's TXT record. Unknown and
// malformed keys are ignored rather than rejected: TXT records vary by
// firmware, and a receiver that says one thing this package doesn't understand
// is still a receiver.
func (d *Device) fromTXT(txt map[string]string) {
	for k, v := range txt {
		switch strings.ToLower(k) {
		case "am":
			d.Model = v
		case "vs":
			d.Version = v
		case "cn":
			for _, n := range parseNumList(v) {
				d.Codecs = append(d.Codecs, Codec(n))
			}
		case "et":
			for _, n := range parseNumList(v) {
				d.Encryption = append(d.Encryption, Encryption(n))
			}
		case "sr":
			d.Audio.SampleRate = atoiOr(v, 0)
		case "ss":
			d.Audio.BitDepth = atoiOr(v, 0)
		case "ch":
			d.Audio.Channels = atoiOr(v, 0)
		case "pw":
			d.NeedsPassword = isTrue(v)
		case "md":
			// Any value at all means it takes some metadata; the digits
			// distinguish text from artwork from progress, and this package
			// sends all three or none.
			d.Metadata = strings.TrimSpace(v) != ""
		}
	}
	if d.Audio.SampleRate == 0 {
		d.Audio.SampleRate = SampleRate
	}
	if d.Audio.BitDepth == 0 {
		d.Audio.BitDepth = BitsPerSample
	}
	if d.Audio.Channels == 0 {
		d.Audio.Channels = Channels
	}
}

// parseNumList splits a comma-separated numeric TXT value ("0,1,3"). Junk
// entries are skipped: a firmware that writes something unexpected in one
// position should not cost the receiver the positions it got right.
func parseNumList(v string) []int {
	var out []int
	for _, part := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func atoiOr(v string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// isTrue reads the boolean spelling AirPlay TXT records use: "true"/"false",
// occasionally "1"/"0".
func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true
	}
	return false
}

// fromAirPlayTXT reads the `_airplay._tcp` advertisement, which an AirPlay 2
// receiver publishes alongside its classic one.
//
// Only the keys that mean something here are read. Notably absent is any
// attempt to decode the `features`/`flags` bitfields into "needs pairing":
// those bits are undocumented, their meanings differ between firmwares, and a
// wrong guess would refuse a working receiver on the strength of a bit nobody
// can check. Whether a receiver will take a classic session is a question with
// an authoritative answer — ask it — and probeClassic does.
func (d *Device) fromAirPlayTXT(txt map[string]string) {
	for k, v := range txt {
		switch strings.ToLower(k) {
		case "features", "ft":
			// Presence, not contents: a box publishing a feature bitfield on
			// this service is an AirPlay 2 receiver.
			if strings.TrimSpace(v) != "" {
				d.AirPlay2 = true
			}
		case "srcvers":
			d.AirPlay2 = true
			if d.Version == "" {
				d.Version = v
			}
		case "model":
			if d.Model == "" {
				d.Model = v
			}
		case "pw":
			// Some firmwares carry the password flag here rather than on the
			// audio service. Only ever set, never cleared: a receiver that
			// says "password" anywhere means it.
			if isTrue(v) {
				d.NeedsPassword = true
			}
		}
	}
}

// splitInstance separates a RAOP instance name into its MAC prefix and the
// friendly name: "B827EB1234AB@Living Room" → ("b827eb1234ab", "Living Room").
// A name with no prefix keeps the whole string as the name, which is what the
// _airplay._tcp advertisement looks like.
func splitInstance(instance string) (id, name string) {
	if at := strings.Index(instance, "@"); at > 0 {
		return NormalizeID(instance[:at]), instance[at+1:]
	}
	return "", instance
}

// NormalizeID reduces an identity to lower-case hex with no separators, so the
// same receiver read through two firmware versions — one colon-separated, one
// not — is still one device. Exported because the store validates against it.
func NormalizeID(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sortDevices orders a scan for display: named receivers alphabetically, then
// by address so two boxes with the same name keep a stable order.
func sortDevices(devs []Device) {
	sort.Slice(devs, func(i, j int) bool {
		if !strings.EqualFold(devs[i].Name, devs[j].Name) {
			return strings.ToLower(devs[i].Name) < strings.ToLower(devs[j].Name)
		}
		return devs[i].Addr() < devs[j].Addr()
	})
}
