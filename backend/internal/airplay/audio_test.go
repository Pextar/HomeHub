package airplay

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// samples builds n frames of interleaved little-endian stereo, with each
// channel counting so a byte-order mistake is visible rather than plausible.
func samples(n int) []byte {
	out := make([]byte, n*BytesPerFrame)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(out[i*4:], uint16(0x1000+i))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(0x2000+i))
	}
	return out
}

// PCM on the wire is big-endian; the decoder writes little-endian. Getting
// this backwards produces audio that is loud, continuous and completely
// wrong, which is exactly the kind of bug that survives a listening test on
// somebody else's hardware.
func TestPCMPayloadIsBigEndian(t *testing.T) {
	got := pcmPayload(samples(2))
	want := []byte{0x10, 0x00, 0x20, 0x00, 0x10, 0x01, 0x20, 0x01}
	if !bytes.Equal(got, want) {
		t.Errorf("payload = % x, want % x", got, want)
	}
}

// A buffer that doesn't end on a sample boundary loses the stray byte rather
// than shifting every sample after it.
func TestPCMPayloadDropsAStrayByte(t *testing.T) {
	if got := pcmPayload([]byte{1, 2, 3}); len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

// The ALAC frame header is 23 bits of fixed prefix followed by verbatim
// samples, so every sample is bit-shifted. This asserts the exact bytes for a
// one-frame packet: it is the only way to catch an off-by-one in the bit
// packer, which would otherwise present as noise on hardware nobody is
// standing next to.
func TestALACPayloadPacksAnUncompressedFrame(t *testing.T) {
	pcm := make([]byte, BytesPerFrame)
	binary.LittleEndian.PutUint16(pcm[0:], 0xABCD) // left
	binary.LittleEndian.PutUint16(pcm[2:], 0x1234) // right

	got := alacPayload(pcm)

	// Expected bitstream, MSB first:
	//   001            element tag: stereo pair
	//   0000 0000_0000 0000  unused header
	//   0              has-size: no
	//   00             wasted bytes: none
	//   1              not compressed
	//   1010_1011_1100_1101  left  = 0xABCD
	//   0001_0010_0011_0100  right = 0x1234
	//   111            end of elements
	// then zero padding to a byte boundary.
	want := bitsToBytes(t,
		"001", "0000", "000000000000", "0", "00", "1",
		"1010101111001101", "0001001000110100", "111")
	if !bytes.Equal(got, want) {
		t.Errorf("frame = % 08b\nwant     % 08b", got, want)
	}
}

func TestALACPayloadLengthMatchesAFullPacket(t *testing.T) {
	got := alacPayload(samples(FramesPerPacket))
	// 23 bits of header + 352 frames × 32 bits + 3 bits of terminator,
	// rounded up to whole bytes.
	wantBits := 23 + FramesPerPacket*32 + 3
	if want := (wantBits + 7) / 8; len(got) != want {
		t.Errorf("len = %d, want %d", len(got), want)
	}
}

// bitsToBytes assembles an expected byte slice from bit-string fragments,
// padding the tail with zeros — the same rule the packer follows.
func bitsToBytes(t *testing.T, groups ...string) []byte {
	t.Helper()
	var bits string
	for _, g := range groups {
		bits += g
	}
	for len(bits)%8 != 0 {
		bits += "0"
	}
	out := make([]byte, len(bits)/8)
	for i := range out {
		var b byte
		for j := 0; j < 8; j++ {
			b <<= 1
			if bits[i*8+j] == '1' {
				b |= 1
			}
		}
		out[i] = b
	}
	return out
}

// Encryption covers whole blocks and leaves the remainder in the clear. That
// looks like a bug and is what the protocol specifies; a test says so, so the
// next reader doesn't "fix" it.
func TestEncryptLeavesTheTrailingPartialBlockAlone(t *testing.T) {
	key, err := newCipherKey()
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 37) // two whole blocks plus five bytes
	for i := range payload {
		payload[i] = byte(i)
	}
	got := key.encrypt(payload)

	if len(got) != len(payload) {
		t.Fatalf("length changed: %d → %d", len(payload), len(got))
	}
	if bytes.Equal(got[:32], payload[:32]) {
		t.Error("the whole blocks should be enciphered")
	}
	if !bytes.Equal(got[32:], payload[32:]) {
		t.Error("the trailing partial block must be left in the clear")
	}
}

// Each packet restarts from the session IV rather than chaining, because
// packets are UDP and a lost one must not corrupt everything after it.
func TestEncryptDoesNotChainBetweenPackets(t *testing.T) {
	key, err := newCipherKey()
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{7}, 32)
	if !bytes.Equal(key.encrypt(payload), key.encrypt(payload)) {
		t.Error("the same payload must encipher identically in two packets")
	}
}

func TestWrappedKeyIsUnpaddedBase64(t *testing.T) {
	key, err := newCipherKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{key.WrappedKey, key.IVBase64} {
		if s == "" {
			t.Fatal("empty key material")
		}
		if s[len(s)-1] == '=' {
			t.Errorf("%q should have its padding stripped", s)
		}
	}
}

// NTP time is seconds since 1900 in the high half. A wrong epoch here would
// put every receiver's clock 70 years out and nothing would ever play.
func TestNTPTimeUsesThe1900Epoch(t *testing.T) {
	// 1970-01-01T00:00:00Z
	if got := ntpTime(0) >> 32; got != 2208988800 {
		t.Errorf("epoch = %d, want 2208988800", got)
	}
	// Half a second past it, as a binary fraction.
	if got := ntpTime(5e8) & 0xFFFFFFFF; got != 1<<31 {
		t.Errorf("fraction = %d, want %d", got, uint64(1)<<31)
	}
}

func TestDAAPMetadataNestsAndOmits(t *testing.T) {
	got := daapMetadata("Song", "", "Album")
	if !bytes.HasPrefix(got, []byte("mlit")) {
		t.Fatalf("should be an mlit container: % x", got[:8])
	}
	if !bytes.Contains(got, []byte("minm")) || !bytes.Contains(got, []byte("asal")) {
		t.Error("title and album should be present")
	}
	if bytes.Contains(got, []byte("asar")) {
		t.Error("an empty artist should be omitted, not sent blank")
	}
	// The container's declared length must match what follows it, or the
	// receiver reads past the end and shows nothing.
	body := got[8:]
	if n := binary.BigEndian.Uint32(got[4:8]); int(n) != len(body) {
		t.Errorf("declared %d bytes, carried %d", n, len(body))
	}
}

func TestDAAPMetadataIsNilWhenThereIsNothingToSay(t *testing.T) {
	if got := daapMetadata("", "", ""); got != nil {
		t.Errorf("got % x, want nil", got)
	}
}
