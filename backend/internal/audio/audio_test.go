package audio

import (
	"testing"

	"homehub/internal/media"
)

// With nowhere to serve from, the engine must hand back a nil interface
// rather than a host. A host built on an empty base URL would point speakers
// at something they cannot fetch, and the failure would surface several
// seconds later as silence instead of immediately as "no stream host
// configured".
func TestStreamHostIsNilWithNoReachableAddress(t *testing.T) {
	e := New(Config{SpeakerAddr: func() string { return "" }})
	if h := e.StreamHost(); h != nil {
		t.Errorf("StreamHost() = %#v, want a nil interface", h)
	}
}

// The same, for a configuration that supplies no way to find a speaker at all.
func TestStreamHostIsNilWithNoSpeakerLookup(t *testing.T) {
	if h := New(Config{}).StreamHost(); h != nil {
		t.Errorf("StreamHost() = %#v, want a nil interface", h)
	}
}

// An explicit base URL is taken as given: it exists for the household whose
// speakers are not on our subnet, where resolving a route would find the
// wrong interface or none.
func TestExplicitBaseURLWins(t *testing.T) {
	e := New(Config{
		BaseURL:     "http://10.0.0.5:8080",
		SpeakerAddr: func() string { return "192.168.1.20" },
	})
	if got, want := e.BaseURL(), "http://10.0.0.5:8080"; got != want {
		t.Errorf("BaseURL() = %q, want %q", got, want)
	}
}

// The quality setting is baked into the decoder's command line, so a change
// has to produce a new decoder. Without this the setting is a preference that
// changes nothing.
func TestDecoderIsRebuiltWhenTheQualityChanges(t *testing.T) {
	quality := media.QualityBest
	e := New(Config{Quality: func() media.StreamQuality { return quality }})

	if got := e.DecoderBitrate(); got != 0 {
		t.Fatalf("DecoderBitrate() = %d before anything decoded, want 0", got)
	}

	_ = e.Decoder()
	if got, want := e.DecoderBitrate(), media.QualityBest.Bitrate(); got != want {
		t.Fatalf("decoder built at %d kbps, want %d", got, want)
	}

	quality = media.QualitySaver
	_ = e.Decoder()
	if got, want := e.DecoderBitrate(), media.QualitySaver.Bitrate(); got != want {
		t.Errorf("decoder still at %d kbps after the setting changed, want %d", got, want)
	}
}

// A quality that has not moved must reuse the decoder. Rebuilding would stop
// a subprocess that is mid-song to replace it with an identical one.
func TestDecoderIsReusedWhenTheQualityHoldsStill(t *testing.T) {
	e := New(Config{Quality: func() media.StreamQuality { return media.QualityBest }})
	first := e.Decoder()
	if second := e.Decoder(); second != first {
		t.Error("Decoder() built a second decoder for an unchanged setting")
	}
}

// No catalogue means no lossless decoder — and a nil interface, not an
// interface holding a nil pointer, or every "is Qobuz configured" check
// downstream would pass on its way to a panic.
func TestQobuzDecoderIsNilWithoutACatalogue(t *testing.T) {
	if d := New(Config{}).QobuzDecoder(); d != nil {
		t.Errorf("QobuzDecoder() = %#v, want a nil interface", d)
	}
}

// Closing an engine that never decoded anything must be safe: it is what a
// shutdown does in a house that only ever plays natively.
func TestCloseIsSafeWhenNothingWasBuilt(t *testing.T) {
	New(Config{}).Close()
}
