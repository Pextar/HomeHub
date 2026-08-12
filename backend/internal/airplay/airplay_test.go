package airplay

import (
	"strings"
	"testing"
)

// ropieeeTXT is what a stock RoPieee (shairport-sync) actually advertises.
// Kept verbatim from a real box rather than reduced to the keys under test:
// the point of the parse is that it survives a real record, extra keys and
// all.
var ropieeeTXT = map[string]string{
	"txtvers": "1", "ch": "2", "cn": "0,1", "et": "0,1", "sv": "false",
	"da": "true", "sr": "44100", "ss": "16", "pw": "false", "vn": "3",
	"tp": "UDP", "vs": "105.1", "am": "ShairportSync", "md": "0,1,2",
}

func TestDeviceFromRoPieeeAdvertisement(t *testing.T) {
	var d Device
	d.fromTXT(ropieeeTXT)

	if d.Model != "ShairportSync" || d.Version != "105.1" {
		t.Errorf("model/version = %q/%q", d.Model, d.Version)
	}
	if d.Audio != (Audio{SampleRate: 44100, BitDepth: 16, Channels: 2}) {
		t.Errorf("audio = %+v", d.Audio)
	}
	if d.NeedsPassword {
		t.Error("pw=false must not read as a password")
	}
	if !d.Metadata {
		t.Error("md=0,1,2 means it takes metadata")
	}

	// cn=0,1 offers PCM and ALAC; PCM wins because it needs no packing.
	if c, ok := d.Codec(); !ok || c != CodecPCM {
		t.Errorf("codec = %v, %v; want pcm", c, ok)
	}
	// et=0,1 offers cleartext and RSA; cleartext wins.
	if e, ok := d.Cipher(); !ok || e != EncryptionNone {
		t.Errorf("cipher = %v, %v; want none", e, ok)
	}
	if ok, why := d.Supported(); !ok {
		t.Errorf("a stock RoPieee must be supported, got %q", why)
	}
}

func TestDeviceDefaultsAudioWhenUnstated(t *testing.T) {
	// A terse responder that says nothing about the format. AirPlay 1 has
	// exactly one, so the defaults are facts rather than guesses.
	var d Device
	d.fromTXT(map[string]string{"am": "Old Box"})
	if d.Audio != (Audio{SampleRate: 44100, BitDepth: 16, Channels: 2}) {
		t.Errorf("audio = %+v, want CD quality defaults", d.Audio)
	}
	// No et at all is an old responder, not a refusal.
	if e, ok := d.Cipher(); !ok || e != EncryptionNone {
		t.Errorf("cipher = %v, %v; want cleartext", e, ok)
	}
}

// A receiver HomeHub cannot drive must say so in words the user can act on,
// and must say which reason applies — the scan lists the sentence next to the
// device, so a wrong one sends someone to fix the wrong thing.
func TestUnsupportedDevicesExplainThemselves(t *testing.T) {
	cases := []struct {
		name string
		txt  map[string]string
		want string
	}{
		{"password", map[string]string{"pw": "true", "cn": "0,1"}, "password"},
		{"aac only", map[string]string{"cn": "2,3", "et": "0"}, "aac"},
		{"fairplay only", map[string]string{"cn": "1", "et": "3,4"}, "FairPlay"},
		{"wrong rate", map[string]string{"cn": "1", "et": "0", "sr": "48000"}, "48000 Hz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d Device
			d.Name = "Box"
			d.fromTXT(tc.txt)
			ok, why := d.Supported()
			if ok {
				t.Fatalf("want unsupported, got supported")
			}
			if !strings.Contains(why, tc.want) {
				t.Errorf("reason %q should mention %q", why, tc.want)
			}
		})
	}
}

func TestALACOnlyReceiverGetsALAC(t *testing.T) {
	var d Device
	d.fromTXT(map[string]string{"cn": "1", "et": "1"})
	if c, ok := d.Codec(); !ok || c != CodecALAC {
		t.Errorf("codec = %v, %v; want alac", c, ok)
	}
	if e, ok := d.Cipher(); !ok || e != EncryptionRSA {
		t.Errorf("cipher = %v, %v; want rsa", e, ok)
	}
}

func TestSplitInstanceSeparatesIdentityFromName(t *testing.T) {
	cases := []struct{ in, wantID, wantName string }{
		{"B8:27:EB:12:34:AB@Living Room", "b827eb1234ab", "Living Room"},
		{"B827EB1234AB@ropieee", "b827eb1234ab", "ropieee"},
		{"Kitchen HomePod", "", "Kitchen HomePod"}, // the _airplay._tcp shape
		{"@Nameless", "", "@Nameless"},             // no id before the @: not a split
	}
	for _, tc := range cases {
		id, name := splitInstance(tc.in)
		if id != tc.wantID || name != tc.wantName {
			t.Errorf("splitInstance(%q) = %q/%q, want %q/%q",
				tc.in, id, name, tc.wantID, tc.wantName)
		}
	}
}

func TestTrimServiceLeavesTheInstanceName(t *testing.T) {
	got := trimService("B827EB1234AB@ropieee._raop._tcp.local.", raopService)
	if got != "B827EB1234AB@ropieee" {
		t.Errorf("got %q", got)
	}
}

// ── AirPlay 2 receivers ──────────────────────────────────────────────────
// The case this package got wrong first time round. A box advertising
// AirPlay 2 is not out of reach: shairport-sync in AirPlay 2 mode — a current
// RoPieee — still answers classic senders, which is why somebody can play to
// it from the Spotify app and from HomeHub both. What decides is the
// receiver's answer, not its advertisement.

func TestAirPlay2ReceiverIsNotRefusedForBeingAirPlay2(t *testing.T) {
	var d Device
	d.Name = "RoPieee"
	// The AirPlay 2 service, with no classic audio keys in it at all.
	d.fromTXT(nil)
	d.fromAirPlayTXT(map[string]string{
		"features": "0x405FCA00,0x1C340", "srcvers": "366.0", "model": "ShairportSync",
		"deviceid": "B8:27:EB:12:34:AB", "pk": "3b6e…",
	})
	if !d.AirPlay2 {
		t.Error("a features/srcvers advertisement is an AirPlay 2 receiver")
	}

	// Nobody has asked it anything yet: unknown, and unknown is attempted.
	if d.Classic != ClassicUnknown {
		t.Errorf("classic = %v before asking", d.Classic)
	}
	if ok, why := d.Supported(); !ok {
		t.Fatalf("an unasked AirPlay 2 receiver must not be refused: %q", why)
	}

	// And it must be offered something a RAOP receiver actually takes.
	got := d.attempts()
	if len(got) == 0 {
		t.Fatal("no attempts for a receiver with no advertised codecs")
	}
	if got[0].codec != CodecALAC {
		t.Errorf("first attempt = %v, want ALAC — the universal format", got[0].codec)
	}
	var sawClear, sawRSA bool
	for _, a := range got {
		sawClear = sawClear || a.cipher == EncryptionNone
		sawRSA = sawRSA || a.cipher == EncryptionRSA
	}
	if !sawClear || !sawRSA {
		t.Errorf("both ciphers should be tried, got %+v", got)
	}
}

// Once asked, a refusal is honoured — and named as the AirPlay 2 pairing it
// almost certainly is, because that is what tells the user it is an Apple
// speaker rather than a broken network.
func TestReceiverThatRefusedTheClassicSessionIsRefused(t *testing.T) {
	d := Device{Name: "Living Room", AirPlay2: true, Classic: ClassicNo}
	ok, why := d.Supported()
	if ok {
		t.Fatal("a receiver that said no is a no")
	}
	if !strings.Contains(why, "pairing") {
		t.Errorf("reason should name what it needs: %q", why)
	}

	// The same refusal from a box that never claimed AirPlay 2 gets a plainer
	// sentence — guessing "pairing" there would send someone looking for a
	// setting that doesn't exist.
	_, why = Device{Name: "Mystery", Classic: ClassicNo}.Supported()
	if strings.Contains(why, "pairing") {
		t.Errorf("reason should not invent a cause: %q", why)
	}
}

// A stock classic receiver still gets its advertised best first: PCM needs no
// frame packing, so where it is genuinely offered it is genuinely preferred.
func TestAttemptsLeadWithWhatTheReceiverAdvertised(t *testing.T) {
	var d Device
	d.fromTXT(ropieeeTXT)
	got := d.attempts()
	if got[0].codec != CodecPCM || got[0].cipher != EncryptionNone {
		t.Errorf("first attempt = %+v, want PCM in the clear", got[0])
	}
	// ALAC is still in the list: an advertisement that says PCM and a
	// firmware that only implements ALAC is a real combination.
	var sawALAC bool
	for _, a := range got {
		sawALAC = sawALAC || a.codec == CodecALAC
	}
	if !sawALAC {
		t.Error("ALAC should remain as the fallback")
	}
}

// A receiver that lists only FairPlay is still refused: that is the
// advertisement saying something specific about the classic session, not the
// absence of an answer.
func TestFairPlayOnlyIsStillRefused(t *testing.T) {
	var d Device
	d.Name = "Apple TV"
	d.fromTXT(map[string]string{"cn": "1", "et": "3,5"})
	if ok, why := d.Supported(); ok {
		t.Error("FairPlay-only cannot be driven")
	} else if !strings.Contains(why, "FairPlay") {
		t.Errorf("reason = %q", why)
	}
}

// A password is a refusal wherever it is advertised, including on the
// AirPlay 2 service — a user who set one is asking for exactly this.
func TestPasswordOnEitherServiceRefuses(t *testing.T) {
	var d Device
	d.fromTXT(map[string]string{"cn": "0,1", "et": "0,1"})
	d.fromAirPlayTXT(map[string]string{"pw": "true", "features": "0x1"})
	if ok, _ := d.Supported(); ok {
		t.Error("a password on the AirPlay 2 service still means password")
	}
}
