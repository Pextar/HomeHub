package announce

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

// The audio half of an announcement: reading a WAV, writing a WAV, and
// synthesising the chime that precedes the words.
//
// Everything here is 16-bit PCM, because that is the one format that needs no
// encoder and that every speaker on the LAN plays. There is deliberately no
// resampler: when a voice is present the chime is *generated at the voice's
// own format*, which is a few lines of arithmetic, where converting between
// two rates properly is a filter design nobody should be writing here.

// Format is the shape of a PCM stream. Two clips can only be joined when
// their formats match exactly, which is why this is carried around rather
// than assumed.
type Format struct {
	SampleRate int
	Channels   int
}

// defaultFormat is what a chime-only announcement uses: CD rate, mono. Mono
// because a chime has nothing to place in a stereo field, and it halves what
// goes over the wire to a speaker that is about to interrupt someone.
var defaultFormat = Format{SampleRate: 44100, Channels: 1}

// Valid reports whether a format is one this package will build with. The
// bounds are sanity, not policy: a rate outside them means the header was
// misread rather than that the speaker wants something exotic.
func (f Format) Valid() bool {
	return f.SampleRate >= 8000 && f.SampleRate <= 192000 && f.Channels >= 1 && f.Channels <= 2
}

// bytesPerSecond is what a duration costs in this format.
func (f Format) bytesPerSecond() int { return f.SampleRate * f.Channels * 2 }

// Clip is decoded audio plus what it takes to play it.
type Clip struct {
	Format Format
	PCM    []byte
}

// Duration is how long the clip runs — which is how long a room stays
// interrupted, so it decides when the transport is handed back.
func (c Clip) Duration() time.Duration {
	if !c.Format.Valid() || len(c.PCM) == 0 {
		return 0
	}
	return time.Duration(float64(len(c.PCM)) / float64(c.Format.bytesPerSecond()) * float64(time.Second))
}

// WAV renders the clip as a self-contained RIFF/WAVE file.
func (c Clip) WAV() []byte {
	const headerLen = 44
	out := make([]byte, headerLen+len(c.PCM))
	byteRate := uint32(c.Format.bytesPerSecond())
	blockAlign := uint16(c.Format.Channels * 2)

	copy(out[0:], "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+len(c.PCM)))
	copy(out[8:], "WAVE")
	copy(out[12:], "fmt ")
	binary.LittleEndian.PutUint32(out[16:], 16) // PCM fmt chunk size
	binary.LittleEndian.PutUint16(out[20:], 1)  // PCM
	binary.LittleEndian.PutUint16(out[22:], uint16(c.Format.Channels))
	binary.LittleEndian.PutUint32(out[24:], uint32(c.Format.SampleRate))
	binary.LittleEndian.PutUint32(out[28:], byteRate)
	binary.LittleEndian.PutUint16(out[32:], blockAlign)
	binary.LittleEndian.PutUint16(out[34:], 16) // bits per sample
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(c.PCM)))
	copy(out[headerLen:], c.PCM)
	return out
}

// ErrUnsupportedAudio is what a voice that isn't 16-bit PCM WAV comes back
// as. It is reported rather than worked around: a TTS endpoint that was
// asked for WAV and answered with MP3 is misconfigured, and guessing at the
// format is how you get a speaker playing three seconds of noise at volume 40.
var ErrUnsupportedAudio = errors.New("announce: expected 16-bit PCM WAV audio")

// parseWAV pulls the format and the samples out of a RIFF/WAVE file.
//
// It walks the chunk list rather than assuming the canonical 44-byte header:
// real encoders emit LIST/fact chunks before the data, and a parser that
// trusts the offset plays those bytes as audio.
func parseWAV(b []byte) (Clip, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return Clip{}, ErrUnsupportedAudio
	}
	var c Clip
	var seenFmt bool
	for pos := 12; pos+8 <= len(b); {
		id := string(b[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		body := pos + 8
		// A WAV written to a stream cannot know its own length, and the
		// several conventions for saying so all have to be read as "the
		// rest of what arrived": a zero, a placeholder maximum (which is
		// also negative when int is 32 bits — a Pi), or simply more than
		// turned up. This is not a corner case; it is what an HTTP TTS
		// endpoint that streams its answer sends every time.
		//
		// The rule is only safe for the audio itself. An empty LIST or fact
		// chunk is legitimately zero-length, and letting one of those
		// swallow the remainder would eat the data chunk behind it.
		if size < 0 || body+size > len(b) || (size == 0 && id == "data") {
			size = len(b) - body
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return Clip{}, ErrUnsupportedAudio
			}
			if format := binary.LittleEndian.Uint16(b[body:]); format != 1 {
				return Clip{}, fmt.Errorf("%w (got format %d)", ErrUnsupportedAudio, format)
			}
			if bits := binary.LittleEndian.Uint16(b[body+14:]); bits != 16 {
				return Clip{}, fmt.Errorf("%w (got %d-bit)", ErrUnsupportedAudio, bits)
			}
			c.Format = Format{
				Channels:   int(binary.LittleEndian.Uint16(b[body+2:])),
				SampleRate: int(binary.LittleEndian.Uint32(b[body+4:])),
			}
			seenFmt = true
		case "data":
			c.PCM = b[body : body+size]
		}
		pos = body + size
		if size%2 == 1 {
			pos++ // RIFF chunks are word-aligned
		}
	}
	if !seenFmt || !c.Format.Valid() || len(c.PCM) == 0 {
		return Clip{}, ErrUnsupportedAudio
	}
	return c, nil
}

// chime synthesises the two notes that precede an announcement.
//
// It is generated rather than shipped as an asset for the same reason the
// stream host serves WAV: no encoder, no file to lose, and — the part that
// matters here — it can be produced at whatever format the voice turned out
// to be, which is what lets the two be joined without a resampler.
//
// Two notes a fifth apart with an exponential decay: recognisably a doorbell
// rather than an alert, because the job is "look up", not "something is
// wrong". The amplitude is deliberately below full scale so that the voice
// after it is the loud part.
func chime(f Format) Clip {
	const (
		noteA     = 587.33 // D5
		noteB     = 880.00 // A5
		noteLen   = 550 * time.Millisecond
		noteGap   = 170 * time.Millisecond
		amplitude = 0.32
		decay     = 6.0 // e-folds per note; a bell, not a beep
	)
	if !f.Valid() {
		f = defaultFormat
	}
	total := noteGap + noteLen
	samples := int(float64(f.SampleRate) * total.Seconds())
	pcm := make([]byte, 0, samples*f.Channels*2)

	gapSamples := int(float64(f.SampleRate) * noteGap.Seconds())
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(f.SampleRate)
		var v float64
		// First note from the start, second one note-gap later, overlapping
		// so the two ring together rather than sounding like two beeps.
		v += math.Sin(2*math.Pi*noteA*t) * math.Exp(-decay*t)
		if i >= gapSamples {
			t2 := float64(i-gapSamples) / float64(f.SampleRate)
			v += math.Sin(2*math.Pi*noteB*t2) * math.Exp(-decay*t2)
		}
		s := int16(math.Max(-1, math.Min(1, v*amplitude/2)) * math.MaxInt16)
		for ch := 0; ch < f.Channels; ch++ {
			pcm = append(pcm, byte(s), byte(uint16(s)>>8))
		}
	}
	return Clip{Format: f, PCM: pcm}
}

// silence is the pause between the chime and the words. Without it the
// speaker's own fade-in eats the first syllable.
func silence(f Format, d time.Duration) []byte {
	n := int(float64(f.bytesPerSecond()) * d.Seconds())
	n -= n % (f.Channels * 2) // never split a frame
	return make([]byte, n)
}

// join concatenates clips that share a format. Clips that don't are dropped
// rather than mangled — see the note at the top of this file.
func join(clips ...Clip) Clip {
	var out Clip
	for _, c := range clips {
		if len(c.PCM) == 0 {
			continue
		}
		if out.PCM == nil {
			out = Clip{Format: c.Format, PCM: append([]byte(nil), c.PCM...)}
			continue
		}
		if c.Format != out.Format {
			continue
		}
		out.PCM = append(out.PCM, c.PCM...)
	}
	return out
}
