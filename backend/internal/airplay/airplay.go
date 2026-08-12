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
// # What this speaks
//
// AirPlay 1 (RAOP), not AirPlay 2. That is a deliberate floor, not an
// aspiration deferred: RoPieee, and every other shairport-sync box, answers
// RAOP without pairing, and RAOP carries CD-quality audio with a real clock.
// AirPlay 2 adds pairing (HomeKit SRP + Curve25519), a PTP clock and buffered
// mode; it would be a much larger piece of work for receivers that already
// accept the simpler protocol. Apple's own AirPlay-2 speakers keep a RAOP
// service running for exactly this reason.
//
// # What reaches the speaker
//
// 16-bit 44.1 kHz stereo, sent as raw PCM when the receiver advertises it
// (cn=0) and as uncompressed ALAC frames when it does not. Both are bit-exact:
// nothing here re-encodes, and there is no encoder dependency, which is the
// same trade internal/stream makes when it serves WAV rather than MP3. The
// receiver's own advertisement decides which, because a capability that lies
// is worse than one that is absent — the rule the media layer is built on.
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

// Supported reports whether this package can actually drive the receiver, and
// when it cannot, why — in a sentence meant for a person, since it is shown
// next to the device in the scan results.
func (d Device) Supported() (bool, string) {
	if d.NeedsPassword {
		return false, "this receiver asks for a password, which HomeHub can't answer"
	}
	if _, ok := d.Codec(); !ok {
		var names []string
		for _, c := range d.Codecs {
			names = append(names, c.String())
		}
		return false, fmt.Sprintf("it only accepts %s, and HomeHub sends PCM or ALAC",
			strings.Join(names, "/"))
	}
	if _, ok := d.Cipher(); !ok {
		return false, "it only accepts FairPlay-encrypted audio, which HomeHub can't produce"
	}
	if d.Audio.SampleRate != 0 && d.Audio.SampleRate != SampleRate {
		return false, fmt.Sprintf("it wants %d Hz audio and HomeHub sends %d Hz",
			d.Audio.SampleRate, SampleRate)
	}
	return true, ""
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
