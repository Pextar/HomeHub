package stream

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"

	"homehub/internal/media"
	"homehub/internal/qobuz"
)

// blockSamples is the shortest block FLAC permits. Every fixture here is
// exactly one block long, which keeps the expected byte counts arithmetic
// rather than something to be looked up.
const blockSamples = 16

// encodeFLAC builds a real FLAC file from the given samples so the decoder is
// tested against the format rather than against a mock of it. Bit-exactness is
// the entire claim this provider makes; asserting it over a fake decoder would
// assert nothing.
//
// Channels shorter than a block are zero-padded, so a test can name the few
// samples it cares about without hand-writing sixteen of them.
func encodeFLAC(t *testing.T, samples [][]int32, rate, depth int) []byte {
	t.Helper()
	for i, ch := range samples {
		if len(ch) < blockSamples {
			padded := make([]int32, blockSamples)
			copy(padded, ch)
			samples[i] = padded
		}
	}
	n := len(samples[0])
	info := &meta.StreamInfo{
		BlockSizeMin: 16, BlockSizeMax: uint16(n),
		SampleRate: uint32(rate), NChannels: uint8(len(samples)),
		BitsPerSample: uint8(depth), NSamples: uint64(n),
	}
	var out bytes.Buffer
	enc, err := flac.NewEncoder(&out, info)
	if err != nil {
		t.Fatalf("new encoder: %v", err)
	}
	subs := make([]*frame.Subframe, len(samples))
	for i, ch := range samples {
		subs[i] = &frame.Subframe{
			SubHeader: frame.SubHeader{Pred: frame.PredVerbatim},
			Samples:   ch, NSamples: len(ch),
		}
	}
	channels := frame.ChannelsLR
	if len(samples) == 1 {
		channels = frame.ChannelsMono
	}
	f := &frame.Frame{
		Header: frame.Header{
			HasFixedBlockSize: true, BlockSize: uint16(n),
			SampleRate: uint32(rate), Channels: channels,
			BitsPerSample: uint8(depth),
		},
		Subframes: subs,
	}
	if err := enc.WriteFrame(f); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("close encoder: %v", err)
	}
	return out.Bytes()
}

// fakeCatalog serves a fixed track list and points every file at a test server.
type fakeCatalog struct {
	tracks []qobuz.Item
	files  map[string]*qobuz.File
	max    qobuz.FormatID
	err    error
}

func (c *fakeCatalog) Tracks(context.Context, string) ([]qobuz.Item, error) {
	return c.tracks, c.err
}
func (c *fakeCatalog) FileURL(_ context.Context, id string, _ qobuz.FormatID) (*qobuz.File, error) {
	f, ok := c.files[id]
	if !ok {
		return nil, qobuz.ErrNotStreamable
	}
	return f, nil
}
func (c *fakeCatalog) MaxFormat() qobuz.FormatID {
	if c.max == 0 {
		return qobuz.FormatHiRes192
	}
	return c.max
}

// serveFiles maps track id to FLAC bytes over HTTP.
func serveFiles(t *testing.T, byID map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		body, ok := byID[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func track(id, name string) qobuz.Item {
	return qobuz.Item{Kind: "track", ID: id, URI: "qobuz:track:" + id, Name: name, Streamable: true}
}

func file(srv *httptest.Server, id string, format qobuz.FormatID, rate, depth int) *qobuz.File {
	return &qobuz.File{
		URL: srv.URL + "/" + id, FormatID: format, MimeType: "audio/flac",
		SampleRate: rate, BitDepth: depth,
	}
}

// The central claim: what comes out of the decoder is bit-for-bit what went
// into the encoder. If this ever fails, "lossless" is a marketing word in this
// codebase rather than a property of it.
func TestFLACDecodesBitExactAt24Bit(t *testing.T) {
	// Values chosen to exercise the sign boundary and the full 24-bit range,
	// because truncation bugs hide in the negatives.
	left := []int32{0, 1, -1, 8388607, -8388608, 12345, -12345}
	right := []int32{-1, 0, 1, -8388608, 8388607, -999, 999}
	flacBytes := encodeFLAC(t, [][]int32{left, right}, 96000, 24)

	srv := serveFiles(t, map[string][]byte{"1": flacBytes})
	cat := &fakeCatalog{
		tracks: []qobuz.Item{track("1", "Test")},
		files:  map[string]*qobuz.File{"1": file(srv, "1", qobuz.FormatHiRes96, 96000, 24)},
	}
	d := NewQobuz(QobuzConfig{Catalog: cat})

	s, err := d.Open(context.Background(), "qobuz:track:1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = d.Close() }()

	// The stream must declare the file's own format, not a house default.
	want := media.PCMFormat{SampleRate: 96000, BitDepth: 24, Channels: 2, LittleEndian: true}
	if s.PCM == nil || *s.PCM != want {
		t.Fatalf("declared format = %+v, want %+v", s.PCM, want)
	}

	got, err := io.ReadAll(s.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Rebuild what interleaved 24-bit little-endian should look like and
	// compare sample by sample, so a failure names the sample rather than
	// reporting that two long byte slices differ.
	if want := blockSamples * 2 * 3; len(got) != want {
		t.Fatalf("got %d bytes, want %d", len(got), want)
	}
	for i := range left {
		for ch, want := range map[int]int32{0: left[i], 1: right[i]} {
			off := (i*2 + ch) * 3
			if g := int24(got[off : off+3]); g != want {
				t.Errorf("sample %d channel %d = %d, want %d", i, ch, g, want)
			}
		}
	}
}

// 16-bit takes a different packing path and gets its own bit-exact check.
func TestFLACDecodesBitExactAt16Bit(t *testing.T) {
	left := []int32{0, 32767, -32768, 1234, -1234}
	right := []int32{-32768, 32767, 0, -1, 1}
	flacBytes := encodeFLAC(t, [][]int32{left, right}, 44100, 16)

	srv := serveFiles(t, map[string][]byte{"1": flacBytes})
	cat := &fakeCatalog{
		tracks: []qobuz.Item{track("1", "CD")},
		files:  map[string]*qobuz.File{"1": file(srv, "1", qobuz.FormatCD, 44100, 16)},
	}
	s, err := NewQobuz(QobuzConfig{Catalog: cat}).Open(context.Background(), "qobuz:track:1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := io.ReadAll(s.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for i := range left {
		for ch, want := range map[int]int32{0: left[i], 1: right[i]} {
			off := (i*2 + ch) * 2
			if g := int32(int16(binary.LittleEndian.Uint16(got[off:]))); g != want {
				t.Errorf("sample %d channel %d = %d, want %d", i, ch, g, want)
			}
		}
	}
}

// An album plays through as one stream. This is the thing librespot did for
// Spotify and that HomeHub has to do itself here.
func TestAlbumPlaysStraightThrough(t *testing.T) {
	a := encodeFLAC(t, [][]int32{{1, 2, 3}, {4, 5, 6}}, 44100, 16)
	b := encodeFLAC(t, [][]int32{{7, 8, 9}, {10, 11, 12}}, 44100, 16)
	srv := serveFiles(t, map[string][]byte{"1": a, "2": b})
	cat := &fakeCatalog{
		tracks: []qobuz.Item{track("1", "One"), track("2", "Two")},
		files: map[string]*qobuz.File{
			"1": file(srv, "1", qobuz.FormatCD, 44100, 16),
			"2": file(srv, "2", qobuz.FormatCD, 44100, 16),
		},
	}
	s, err := NewQobuz(QobuzConfig{Catalog: cat}).Open(context.Background(), "qobuz:album:x")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := io.ReadAll(s.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Six frames of stereo 16-bit across two tracks.
	if want := 2 * blockSamples * 2 * 2; len(got) != want {
		t.Errorf("got %d bytes, want %d — the second track didn't play", len(got), want)
	}
}

// The seam rule. A WAV stream carries one format to the end, so a track in a
// different format cannot be appended without converting it — and this system
// does not convert. The stream stops cleanly instead.
func TestAFormatChangeEndsTheStreamRatherThanConverting(t *testing.T) {
	cd := encodeFLAC(t, [][]int32{{1, 2, 3}, {4, 5, 6}}, 44100, 16)
	hi := encodeFLAC(t, [][]int32{{7, 8, 9}, {10, 11, 12}}, 96000, 24)
	srv := serveFiles(t, map[string][]byte{"1": cd, "2": hi})
	cat := &fakeCatalog{
		tracks: []qobuz.Item{track("1", "CD track"), track("2", "Hi-res track")},
		files: map[string]*qobuz.File{
			"1": file(srv, "1", qobuz.FormatCD, 44100, 16),
			"2": file(srv, "2", qobuz.FormatHiRes96, 96000, 24),
		},
	}
	var logged []string
	d := NewQobuz(QobuzConfig{Catalog: cat, Logf: func(f string, a ...any) {
		logged = append(logged, fmt.Sprintf(f, a...))
	}})
	s, err := d.Open(context.Background(), "qobuz:album:mixed")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := io.ReadAll(s.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := blockSamples * 2 * 2; len(got) != want {
		t.Errorf("got %d bytes, want %d — only the first track should have played", len(got), want)
	}
	if !strings.Contains(strings.Join(logged, "\n"), "won't convert") {
		t.Errorf("the stop should be explained in the log, got %q", logged)
	}
}

// A track the account can't stream is skipped, not fatal. A purchase-only
// bonus track at the end of an album is common, and ending the album on it
// would be a worse answer than playing the rest.
func TestAnUnplayableTrackIsSkipped(t *testing.T) {
	a := encodeFLAC(t, [][]int32{{1, 2}, {3, 4}}, 44100, 16)
	c := encodeFLAC(t, [][]int32{{5, 6}, {7, 8}}, 44100, 16)
	srv := serveFiles(t, map[string][]byte{"1": a, "3": c})
	cat := &fakeCatalog{
		tracks: []qobuz.Item{track("1", "One"), track("2", "Purchase only"), track("3", "Three")},
		files: map[string]*qobuz.File{
			"1": file(srv, "1", qobuz.FormatCD, 44100, 16),
			"3": file(srv, "3", qobuz.FormatCD, 44100, 16),
		},
	}
	s, err := NewQobuz(QobuzConfig{Catalog: cat}).Open(context.Background(), "qobuz:album:x")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := io.ReadAll(s.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := 2 * blockSamples * 2 * 2; len(got) != want {
		t.Errorf("got %d bytes, want %d — tracks 1 and 3 should both have played", len(got), want)
	}
}

// If Qobuz hands back MP3 — a lapsed subscription, or a track that only exists
// compressed — this path refuses rather than quietly serving lossy audio down
// a route the UI is calling lossless.
func TestALossyOfferIsRefused(t *testing.T) {
	srv := serveFiles(t, map[string][]byte{})
	cat := &fakeCatalog{
		tracks: []qobuz.Item{track("1", "Compressed")},
		files:  map[string]*qobuz.File{"1": file(srv, "1", qobuz.FormatMP3320, 44100, 16)},
	}
	_, err := NewQobuz(QobuzConfig{Catalog: cat}).Open(context.Background(), "qobuz:track:1")
	if err == nil || !strings.Contains(err.Error(), "not FLAC") {
		t.Errorf("error = %v, want a refusal naming the format", err)
	}
}

// An unopenable first track fails the tap rather than producing a stream that
// plays silence and never explains itself.
func TestAnUnplayableFirstTrackFailsTheTap(t *testing.T) {
	cat := &fakeCatalog{tracks: []qobuz.Item{track("9", "Missing")}, files: map[string]*qobuz.File{}}
	if _, err := NewQobuz(QobuzConfig{Catalog: cat}).Open(context.Background(), "qobuz:track:9"); err == nil {
		t.Error("opening an unplayable track should fail")
	}
}

// An unconfigured decoder says so instead of panicking on a nil catalogue.
func TestUnconfiguredQobuzReportsItself(t *testing.T) {
	d := NewQobuz(QobuzConfig{})
	if av := d.Available(); av.OK {
		t.Error("a decoder with no catalogue is not available")
	}
	if _, err := d.Open(context.Background(), "qobuz:track:1"); err == nil {
		t.Error("open should fail without a catalogue")
	}
}

// int24 reads a signed 24-bit little-endian sample.
func int24(b []byte) int32 {
	v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
	if v&0x800000 != 0 {
		v |= ^0xFFFFFF // sign-extend
	}
	return v
}
