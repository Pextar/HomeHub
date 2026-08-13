package stream

import (
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
