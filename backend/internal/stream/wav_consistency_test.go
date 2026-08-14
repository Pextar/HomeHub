package stream

import (
	"encoding/binary"
	"testing"

	"homehub/internal/media"
)

// The WAV constants here and media.CDQuality describe the same audio. A change
// to one that the other did not hear about would leave the AirPlay route
// packing samples in a format the WAV header denies — silence, or noise, on
// hardware nobody is standing next to.
func TestPCMConstantsAgreeWithTheMediaLayer(t *testing.T) {
	if SampleRate != media.CDQuality.SampleRate {
		t.Errorf("sample rate: stream says %d, media says %d",
			SampleRate, media.CDQuality.SampleRate)
	}
	if BitsPerSample != media.CDQuality.BitDepth {
		t.Errorf("bit depth: stream says %d, media says %d",
			BitsPerSample, media.CDQuality.BitDepth)
	}
	if Channels != media.CDQuality.Channels {
		t.Errorf("channels: stream says %d, media says %d",
			Channels, media.CDQuality.Channels)
	}
	if !media.CDQuality.LittleEndian {
		t.Error("librespot's pipe backend writes little-endian samples")
	}
}

// The header describes the samples, not the host's preferences.
//
// This is the whole of "never downsample" at this layer. The stream route does
// not resample and does not requantise — the 44-byte header is the entire
// conversion — so the one way it can damage audio is by describing it wrongly.
// A 24-bit/96 kHz source announced as 16-bit/44.1 is not played slightly
// worse; it is read at the wrong word length and the wrong rate, which is
// noise at whatever volume the room was left on.
func TestHeaderDescribesTheSourceRatherThanCDQuality(t *testing.T) {
	hiRes := media.PCMFormat{SampleRate: 96000, BitDepth: 24, Channels: 2, LittleEndian: true}
	h := WAVHeader(hiRes)

	if got := binary.LittleEndian.Uint32(h[24:28]); got != 96000 {
		t.Errorf("sample rate = %d, want 96000 — the header must not round to CD rate", got)
	}
	if got := binary.LittleEndian.Uint16(h[34:36]); got != 24 {
		t.Errorf("bits per sample = %d, want 24", got)
	}
	// Byte rate and block align are derived, and a stale derivation is how a
	// correct-looking header still plays at the wrong speed.
	if got := binary.LittleEndian.Uint32(h[28:32]); got != uint32(hiRes.BytesPerSecond()) {
		t.Errorf("byte rate = %d, want %d", got, hiRes.BytesPerSecond())
	}
	if got := binary.LittleEndian.Uint16(h[32:34]); got != 6 {
		t.Errorf("block align = %d, want 6 (2ch × 24-bit)", got)
	}
}

// A stream that declares no format still has to be described as something, and
// the something is CD quality — the format every decoder in this repo produced
// before PCMFormat existed. Silently emitting a zeroed header would be the
// mislabelling this whole change is about.
func TestUndeclaredFormatFallsBackRatherThanEmittingZeroes(t *testing.T) {
	for _, f := range []*media.PCMFormat{nil, {}} {
		h := headerFor(ContentTypeWAV, f)
		if got := binary.LittleEndian.Uint32(h[24:28]); got != SampleRate {
			t.Errorf("%+v: sample rate = %d, want the CD-quality fallback", f, got)
		}
		if got := binary.LittleEndian.Uint16(h[34:36]); got != BitsPerSample {
			t.Errorf("%+v: bit depth = %d, want the CD-quality fallback", f, got)
		}
	}
	// A source that frames itself needs no header from us.
	if h := headerFor("audio/flac", &media.PCMFormat{}); h != nil {
		t.Errorf("a framed source got %d bytes of WAV header prepended", len(h))
	}
}
