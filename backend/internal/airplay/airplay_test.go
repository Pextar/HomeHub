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
