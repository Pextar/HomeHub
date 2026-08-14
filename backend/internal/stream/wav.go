package stream

import (
	"encoding/binary"

	"homehub/internal/media"
)

// Audio format constants for the decoder this host was first written around:
// librespot's pipe backend, which emits raw S16LE at CD rate.
//
// They are a default, not a truth. The header is built from the format the
// stream actually declares, because the alternative is describing a 24-bit
// source with these numbers and handing a speaker samples it will read at the
// wrong word length — noise, at whatever volume the room was left on. A
// decoder that produces something else must say so and be believed.
const (
	SampleRate    = 44100
	Channels      = 2
	BitsPerSample = 16

	// BytesPerSecond is CD rate, 176.4 kB/s. Kept because it is the number
	// that makes serving PCM over a LAN reasonable and over anything else
	// not; for sizing anything against a real stream, ask that stream's
	// format via media.PCMFormat.BytesPerSecond.
	BytesPerSecond = SampleRate * Channels * BitsPerSample / 8
)

// ContentTypeWAV is what the stream advertises. WAV rather than FLAC or MP3
// because it needs no encoder: librespot already hands us PCM, and prepending
// a 44-byte header is the whole conversion. An MP3 stream would mean a
// dependency on lame or ffmpeg and a second transcode of already-lossy audio,
// for a bandwidth saving that does not matter on a wired LAN.
const ContentTypeWAV = "audio/wav"

// streamingDataSize is the size written into the header's length fields for a
// stream with no known end.
//
// A WAV header has to declare how many bytes follow, which an endless stream
// cannot know. The convention players expect is a maximum value: they read
// until the connection closes and treat the length as advisory. Using the
// literal maximum (0xFFFFFFFF) trips overflow checks in some renderers, so
// this is the next size down that still means "effectively forever" — about
// 3.4 hours short of 4 GiB, which at CD rate is roughly six and a half hours
// of audio.
const streamingDataSize = 0xFFFFFFFF - 128

// DefaultFormat is what a stream is assumed to carry when it declares nothing.
// Only a decoder that predates PCMFormat should be relying on this.
var DefaultFormat = media.PCMFormat{
	SampleRate: SampleRate, BitDepth: BitsPerSample, Channels: Channels, LittleEndian: true,
}

// headerFor returns the bytes every listener receives before any audio, framed
// to the format the stream actually carries. Content types other than WAV get
// nothing, since the decoder's output is already framed in that case.
func headerFor(contentType string, f *media.PCMFormat) []byte {
	if contentType != ContentTypeWAV {
		return nil
	}
	if !f.Valid() {
		return WAVHeader(DefaultFormat)
	}
	return WAVHeader(*f)
}

// WAVHeader builds a 44-byte RIFF/WAVE header describing an open-ended PCM
// stream in format f.
//
// Each listener gets its own copy, because each one starts mid-stream: a
// speaker connecting second still needs a header before the audio, or it has
// no idea what it is receiving.
//
// The header is the entire conversion this route performs, which is why it
// takes a format rather than reading constants. HomeHub does not resample and
// does not requantise: whatever the decoder handed over is what goes on the
// wire, and this header's job is to describe it accurately rather than to
// describe what the host would have preferred.
func WAVHeader(f media.PCMFormat) []byte {
	const headerLen = 44
	h := make([]byte, headerLen)

	byteRate := f.BytesPerSecond()
	blockAlign := f.Channels * f.BitDepth / 8

	copy(h[0:4], "RIFF")
	// Total file size minus the 8 bytes of "RIFF" + this field.
	binary.LittleEndian.PutUint32(h[4:8], streamingDataSize+headerLen-8)
	copy(h[8:12], "WAVE")

	copy(h[12:16], "fmt ")
	binary.LittleEndian.PutUint32(h[16:20], 16) // PCM fmt chunk length
	binary.LittleEndian.PutUint16(h[20:22], 1)  // format 1 = PCM
	binary.LittleEndian.PutUint16(h[22:24], uint16(f.Channels))
	binary.LittleEndian.PutUint32(h[24:28], uint32(f.SampleRate))
	binary.LittleEndian.PutUint32(h[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(h[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(h[34:36], uint16(f.BitDepth))

	copy(h[36:40], "data")
	binary.LittleEndian.PutUint32(h[40:44], streamingDataSize)
	return h
}
