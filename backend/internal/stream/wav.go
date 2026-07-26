package stream

import "encoding/binary"

// Audio format constants. These are what librespot's pipe backend emits and
// are not configurable: the decoder writes raw S16LE at CD rate, so the
// header has to describe exactly that.
const (
	SampleRate    = 44100
	Channels      = 2
	BitsPerSample = 16

	// BytesPerSecond is the resulting bitrate, 176.4 kB/s. Worth having as a
	// number: it is what a listener's buffer is measured in, and it is the
	// reason serving PCM over a LAN is fine and over anything else is not.
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

// headerFor returns the bytes every listener receives before any audio.
// Content types other than WAV get nothing, since the decoder's output is
// already framed in that case.
func headerFor(contentType string) []byte {
	if contentType != ContentTypeWAV {
		return nil
	}
	return WAVHeader()
}

// WAVHeader builds a 44-byte RIFF/WAVE header describing an open-ended PCM
// stream.
//
// Each listener gets its own copy, because each one starts mid-stream: a
// speaker connecting second still needs a header before the audio, or it has
// no idea what it is receiving.
func WAVHeader() []byte {
	const headerLen = 44
	h := make([]byte, headerLen)

	byteRate := SampleRate * Channels * BitsPerSample / 8
	blockAlign := Channels * BitsPerSample / 8

	copy(h[0:4], "RIFF")
	// Total file size minus the 8 bytes of "RIFF" + this field.
	binary.LittleEndian.PutUint32(h[4:8], streamingDataSize+headerLen-8)
	copy(h[8:12], "WAVE")

	copy(h[12:16], "fmt ")
	binary.LittleEndian.PutUint32(h[16:20], 16) // PCM fmt chunk length
	binary.LittleEndian.PutUint16(h[20:22], 1)  // format 1 = PCM
	binary.LittleEndian.PutUint16(h[22:24], Channels)
	binary.LittleEndian.PutUint32(h[24:28], SampleRate)
	binary.LittleEndian.PutUint32(h[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(h[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(h[34:36], BitsPerSample)

	copy(h[36:40], "data")
	binary.LittleEndian.PutUint32(h[40:44], streamingDataSize)
	return h
}
