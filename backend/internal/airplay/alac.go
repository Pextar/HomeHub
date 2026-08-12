package airplay

// Turning the decoder's PCM into what a receiver wants.
//
// Two shapes, neither of which loses a sample:
//
//	PCM   the same 16-bit samples, byte-swapped to network order. RTP audio
//	      formats are big-endian; librespot's pipe backend writes S16LE.
//	ALAC  the same samples wrapped in an Apple Lossless frame that declares
//	      itself uncompressed. ALAC has an escape hatch for exactly this —
//	      a bit that says "the rest is verbatim" — so a bit-exact ALAC frame
//	      needs a bit packer and no encoder, which is the same trade
//	      internal/stream makes when it serves WAV rather than MP3.
//
// The alternative was linking a real ALAC encoder for the compression, which
// would buy bandwidth this does not need: 1.4 Mbit/s per receiver on a LAN is
// nothing, and it is what AirPlay senders have always used.

import "encoding/binary"

// The audio format AirPlay 1 carries. Fixed, not configurable: the receiver's
// advertisement is checked against these in Device.Supported rather than the
// sender adapting, because resampling would be the one lossy step in an
// otherwise bit-exact path.
const (
	SampleRate    = 44100
	Channels      = 2
	BitsPerSample = 16

	// FramesPerPacket is the RAOP packet size in sample frames — 352, which
	// is the ALAC frame length every receiver expects and which works out
	// at just under 8 ms of audio.
	FramesPerPacket = 352

	// BytesPerFrame is one sample frame of interleaved stereo.
	BytesPerFrame = Channels * BitsPerSample / 8
	// PacketBytes is one packet's worth of PCM.
	PacketBytes = FramesPerPacket * BytesPerFrame
)

// pcmPayload byte-swaps interleaved little-endian samples into the big-endian
// order the wire uses. A short final buffer is handled rather than rejected:
// the end of a stream rarely lands on a packet boundary.
func pcmPayload(src []byte) []byte {
	out := make([]byte, len(src)/2*2)
	for i := 0; i+1 < len(src); i += 2 {
		out[i], out[i+1] = src[i+1], src[i]
	}
	return out
}

// alacPayload wraps samples in an uncompressed ALAC frame.
//
// The bitstream, in order, is: a 3-bit element tag (1 = a stereo channel
// pair), 16 bits of unused header, a "has size" bit (0 — the frame is the
// declared 352 frames), 2 bits of "wasted bytes" (none), and the bit that
// makes this cheap: "not compressed" set to 1. Everything after it is raw
// samples, most significant bit first, interleaved left then right. A 3-bit
// end-of-elements tag closes the frame.
//
// That leaves the samples starting at bit 23, which is why this needs a bit
// packer at all rather than a memcpy.
func alacPayload(src []byte) []byte {
	frames := len(src) / BytesPerFrame
	w := &bitWriter{buf: make([]byte, 0, len(src)+8)}

	w.write(1, 3)  // element tag: stereo channel pair
	w.write(0, 4)  // unused
	w.write(0, 12) // unused
	w.write(0, 1)  // has-size: no, the frame is the declared length
	w.write(0, 2)  // wasted bytes: none
	w.write(1, 1)  // not compressed: the rest is verbatim

	for i := 0; i < frames; i++ {
		off := i * BytesPerFrame
		left := binary.LittleEndian.Uint16(src[off : off+2])
		right := binary.LittleEndian.Uint16(src[off+2 : off+4])
		w.write(uint32(left), BitsPerSample)
		w.write(uint32(right), BitsPerSample)
	}

	w.write(7, 3) // end of elements
	return w.bytes()
}

// bitWriter packs values MSB-first into a byte slice. Small and local: the
// only bitstream in the project is the one above.
type bitWriter struct {
	buf  []byte
	acc  uint64 // pending bits, left-aligned in the low `n` positions
	n    uint   // how many bits are pending
	spun bool   // set once anything has been written, so bytes() knows to flush
}

// write appends the low `bits` bits of v.
func (w *bitWriter) write(v uint32, bits uint) {
	w.spun = true
	w.acc = w.acc<<bits | uint64(v)&(1<<bits-1)
	w.n += bits
	for w.n >= 8 {
		w.n -= 8
		w.buf = append(w.buf, byte(w.acc>>w.n))
	}
}

// bytes returns the packed bytes, padding the last one with zeros. Padding to
// a byte boundary is part of the frame format, not an artefact.
func (w *bitWriter) bytes() []byte {
	if w.n > 0 {
		w.buf = append(w.buf, byte(w.acc<<(8-w.n)))
		w.acc, w.n = 0, 0
	}
	return w.buf
}
