package stream

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"homehub/internal/media"
)

// blockingReader hands out a fixed chunk each time it is read, until it is
// closed. It stands in for a decoder producing audio forever.
type blockingReader struct {
	chunk []byte
	mu    sync.Mutex
	done  chan struct{}
	once  sync.Once
	reads int
}

func newBlockingReader(chunk []byte) *blockingReader {
	return &blockingReader{chunk: chunk, done: make(chan struct{})}
}

func (b *blockingReader) Read(p []byte) (int, error) {
	select {
	case <-b.done:
		return 0, io.EOF
	default:
	}
	b.mu.Lock()
	b.reads++
	b.mu.Unlock()
	n := copy(p, b.chunk)
	// Pace it slightly so a test can observe the stream mid-flight rather
	// than racing a tight loop.
	time.Sleep(time.Millisecond)
	return n, nil
}

func (b *blockingReader) Close() error {
	b.once.Do(func() { close(b.done) })
	return nil
}

func (b *blockingReader) readCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reads
}

func newTestHost(t *testing.T) (*Host, *httptest.Server) {
	t.Helper()
	h := NewHost(Config{Logf: func(string, ...any) {}})
	srv := httptest.NewServer(h.Handler())
	h.cfg.BaseURL = srv.URL
	t.Cleanup(srv.Close)
	return h, srv
}

func TestPublishServesAudioToOneListener(t *testing.T) {
	h, _ := newTestHost(t)
	src := newBlockingReader([]byte("0123456789abcdef"))
	defer func() { _ = src.Close() }()

	url, stop, err := h.Publish(t.Context(), &media.Stream{
		Body: src, ContentType: ContentTypeWAV,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	defer stop()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get("Content-Type"); got != ContentTypeWAV {
		t.Errorf("content type = %q, want %q", got, ContentTypeWAV)
	}

	// The header has to arrive before any audio, or the speaker has no idea
	// what it is receiving.
	head := make([]byte, 44)
	if _, err := io.ReadFull(resp.Body, head); err != nil {
		t.Fatalf("reading WAV header: %v", err)
	}
	if string(head[0:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		t.Fatalf("did not receive a WAV header, got %q", head[:12])
	}

	audio := make([]byte, 32)
	if _, err := io.ReadFull(resp.Body, audio); err != nil {
		t.Fatalf("reading audio: %v", err)
	}
	if !bytes.Contains(audio, []byte("0123456789abcdef")) {
		t.Errorf("audio does not contain the source pattern, got %q", audio)
	}
}

// TestDecoderIsNotReadUntilAListenerConnects covers the lazy start. Reading
// the decoder before a speaker connects would discard the opening seconds of
// a track into an empty room.
func TestDecoderIsNotReadUntilAListenerConnects(t *testing.T) {
	h, _ := newTestHost(t)
	src := newBlockingReader(make([]byte, 1024))
	defer func() { _ = src.Close() }()

	url, stop, err := h.Publish(t.Context(), &media.Stream{Body: src, ContentType: ContentTypeWAV})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	defer stop()

	time.Sleep(50 * time.Millisecond)
	if n := src.readCount(); n != 0 {
		t.Fatalf("decoder was read %d times before any speaker connected", n)
	}

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 128)
	_, _ = io.ReadFull(resp.Body, buf)

	if src.readCount() == 0 {
		t.Error("decoder should be running once a speaker has connected")
	}
}

// TestFanOutToMultipleListeners is the case the whole package exists for: one
// decoder, several speakers, each getting the audio.
func TestFanOutToMultipleListeners(t *testing.T) {
	h, _ := newTestHost(t)
	pattern := []byte("SYNCSYNCSYNCSYNC")
	src := newBlockingReader(pattern)
	defer func() { _ = src.Close() }()

	url, stop, err := h.Publish(t.Context(), &media.Stream{Body: src, ContentType: ContentTypeWAV})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	defer stop()

	const listeners = 3
	var wg sync.WaitGroup
	results := make([][]byte, listeners)
	errs := make([]error, listeners)

	for i := range listeners {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := http.Get(url)
			if err != nil {
				errs[i] = err
				return
			}
			defer func() { _ = resp.Body.Close() }()
			buf := make([]byte, 44+64)
			if _, err := io.ReadFull(resp.Body, buf); err != nil {
				errs[i] = err
				return
			}
			results[i] = buf
		}(i)
	}
	wg.Wait()

	for i := range listeners {
		if errs[i] != nil {
			t.Fatalf("listener %d: %v", i, errs[i])
		}
		if string(results[i][0:4]) != "RIFF" {
			t.Errorf("listener %d did not get its own WAV header", i)
		}
		if !bytes.Contains(results[i][44:], pattern) {
			t.Errorf("listener %d did not receive the audio pattern", i)
		}
	}
	// One decoder, not one per listener — that is the entire point.
	if h.streamCount() != 1 {
		t.Errorf("host holds %d streams, want 1", h.streamCount())
	}
}

// TestSlowListenerDoesNotStallOthers is the reason broadcast drops instead of
// blocking. A speaker on bad Wi-Fi must not take the rest of the zone with it.
func TestSlowListenerDoesNotStallOthers(t *testing.T) {
	h, _ := newTestHost(t)
	src := newBlockingReader(make([]byte, chunkSize))
	defer func() { _ = src.Close() }()

	url, stop, err := h.Publish(t.Context(), &media.Stream{Body: src, ContentType: ContentTypeWAV})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	defer stop()

	// A listener that connects and then never reads.
	slow, err := http.Get(url)
	if err != nil {
		t.Fatalf("slow listener: %v", err)
	}
	defer func() { _ = slow.Body.Close() }()

	// Give the pump time to fill and overrun the slow listener's buffer.
	time.Sleep(100 * time.Millisecond)

	// A second listener must still be served promptly.
	fast, err := http.Get(url)
	if err != nil {
		t.Fatalf("fast listener: %v", err)
	}
	defer func() { _ = fast.Body.Close() }()

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 44+chunkSize)
		_, err := io.ReadFull(fast.Body, buf)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("fast listener failed to read: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a stalled listener blocked the stream for everyone else")
	}
}

func TestStopReleasesStream(t *testing.T) {
	h, _ := newTestHost(t)
	src := newBlockingReader(make([]byte, 256))

	url, stop, err := h.Publish(t.Context(), &media.Stream{Body: src, ContentType: ContentTypeWAV})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if h.streamCount() != 1 {
		t.Fatalf("stream not registered")
	}

	stop()

	if h.streamCount() != 0 {
		t.Error("stop should unregister the stream")
	}
	// The decoder must be closed, or librespot keeps holding the account's
	// Spotify session after playback ended.
	select {
	case <-src.done:
	default:
		t.Error("stop should close the decoder")
	}
	// And the URL must stop resolving straight away.
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET after stop: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("released stream answered %d, want 404", resp.StatusCode)
	}

	// Repeated stops must be harmless — the executor calls stop on failure
	// paths that can overlap with a normal teardown.
	stop()
	stop()
}

func TestUnknownStreamIs404(t *testing.T) {
	_, srv := newTestHost(t)
	resp, err := http.Get(srv.URL + "/stream/deadbeef")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got %d, want 404", resp.StatusCode)
	}
}

// TestHeadRequestIsAnswered matters because speakers probe with HEAD before
// committing to play, and a 404 or a hang there means they never start.
func TestHeadRequestIsAnswered(t *testing.T) {
	h, _ := newTestHost(t)
	src := newBlockingReader(make([]byte, 256))
	defer func() { _ = src.Close() }()

	url, stop, err := h.Publish(t.Context(), &media.Stream{Body: src, ContentType: ContentTypeWAV})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	defer stop()

	resp, err := http.Head(url)
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("HEAD answered %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != ContentTypeWAV {
		t.Errorf("HEAD content type = %q, want %q", got, ContentTypeWAV)
	}
	// DLNA renderers key off this header to treat the source as a live
	// stream rather than a file.
	if got := resp.Header.Get("transferMode.dlna.org"); got != "Streaming" {
		t.Errorf("transferMode = %q, want Streaming", got)
	}
	// A HEAD must not start the decoder — otherwise a probe would consume
	// the opening of the track before the real GET arrives.
	if src.readCount() != 0 {
		t.Error("a HEAD probe should not start the decoder")
	}
}

func TestPublishRejectsMissingConfig(t *testing.T) {
	t.Run("no body", func(t *testing.T) {
		h := NewHost(Config{BaseURL: "http://h"})
		if _, _, err := h.Publish(t.Context(), &media.Stream{}); err == nil {
			t.Error("expected an error with no audio to publish")
		}
	})
	t.Run("no base URL", func(t *testing.T) {
		h := NewHost(Config{})
		src := newBlockingReader(nil)
		defer func() { _ = src.Close() }()
		if _, _, err := h.Publish(t.Context(), &media.Stream{Body: src}); err == nil {
			t.Error("expected an error when no reachable address is configured")
		}
	})
}

// TestStreamIDsAreUnguessable checks the capability model: the id is the only
// thing protecting the stream, since speakers cannot authenticate.
func TestStreamIDsAreUnguessable(t *testing.T) {
	h := NewHost(Config{BaseURL: "http://h"})
	seen := map[string]bool{}
	for range 50 {
		src := newBlockingReader(nil)
		url, stop, err := h.Publish(t.Context(), &media.Stream{Body: src})
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}
		id := url[strings.LastIndex(url, "/")+1:]
		if len(id) != 32 {
			t.Fatalf("id %q is %d chars, want 32 hex chars (128 bits)", id, len(id))
		}
		if seen[id] {
			t.Fatalf("duplicate stream id %q", id)
		}
		seen[id] = true
		stop()
	}
}

func TestStartDelay(t *testing.T) {
	h := NewHost(Config{StartDelays: map[media.Vendor]time.Duration{
		media.VendorKEF: 250 * time.Millisecond,
	}})
	if got := h.StartDelay(media.VendorKEF); got != 250*time.Millisecond {
		t.Errorf("KEF delay = %v, want 250ms", got)
	}
	// An unconfigured vendor must be zero, not a guess.
	if got := h.StartDelay(media.VendorSonos); got != 0 {
		t.Errorf("unconfigured vendor delay = %v, want 0", got)
	}
}

func TestWAVHeader(t *testing.T) {
	h := WAVHeader()
	if len(h) != 44 {
		t.Fatalf("header is %d bytes, want 44", len(h))
	}
	if string(h[0:4]) != "RIFF" || string(h[8:12]) != "WAVE" ||
		string(h[12:16]) != "fmt " || string(h[36:40]) != "data" {
		t.Fatalf("malformed chunk identifiers: %q", h)
	}
	if got := binary.LittleEndian.Uint16(h[20:22]); got != 1 {
		t.Errorf("format = %d, want 1 (PCM)", got)
	}
	if got := binary.LittleEndian.Uint16(h[22:24]); got != Channels {
		t.Errorf("channels = %d, want %d", got, Channels)
	}
	if got := binary.LittleEndian.Uint32(h[24:28]); got != SampleRate {
		t.Errorf("sample rate = %d, want %d", got, SampleRate)
	}
	if got := binary.LittleEndian.Uint32(h[28:32]); got != BytesPerSecond {
		t.Errorf("byte rate = %d, want %d", got, BytesPerSecond)
	}
	if got := binary.LittleEndian.Uint16(h[32:34]); got != Channels*BitsPerSample/8 {
		t.Errorf("block align = %d, want %d", got, Channels*BitsPerSample/8)
	}
	if got := binary.LittleEndian.Uint16(h[34:36]); got != BitsPerSample {
		t.Errorf("bits per sample = %d, want %d", got, BitsPerSample)
	}
	// The declared length must be the streaming sentinel rather than the
	// literal maximum, which some renderers reject.
	if got := binary.LittleEndian.Uint32(h[40:44]); got != streamingDataSize {
		t.Errorf("data size = %d, want %d", got, streamingDataSize)
	}
	if got := binary.LittleEndian.Uint32(h[40:44]); got == 0xFFFFFFFF {
		t.Error("data size must not be the literal maximum")
	}
}

// TestLibrespotUnavailable covers the graceful-degradation promise: with no
// binary installed, the decoder reports why rather than failing obscurely at
// play time.
func TestLibrespotUnavailable(t *testing.T) {
	l := NewLibrespot(LibrespotConfig{Binary: "definitely-not-a-real-binary-xyz"})
	av := l.Available()
	if av.OK {
		t.Fatal("a missing binary must not report available")
	}
	if !strings.Contains(av.Reason, "librespot") {
		t.Errorf("reason should name what's missing: %q", av.Reason)
	}
	if _, err := l.Open(t.Context(), "spotify:track:x"); err == nil {
		t.Error("Open should fail when the decoder isn't installed")
	}
	// Closing a decoder that never ran must be harmless.
	if err := l.Close(); err != nil {
		t.Errorf("Close on an unstarted decoder: %v", err)
	}
}

func TestLibrespotDefaults(t *testing.T) {
	l := NewLibrespot(LibrespotConfig{})
	if l.DeviceName() != DefaultDeviceName {
		t.Errorf("device name = %q, want %q", l.DeviceName(), DefaultDeviceName)
	}
	args := strings.Join(l.args(), " ")
	// --backend pipe is what makes the whole approach work on a host with
	// no sound card; losing it would break headless setups silently.
	if !strings.Contains(args, "--backend pipe") {
		t.Errorf("args must use the pipe backend, got: %s", args)
	}
	if !strings.Contains(args, "--bitrate 320") {
		t.Errorf("default bitrate should be 320, got: %s", args)
	}
	if !strings.Contains(args, DefaultDeviceName) {
		t.Errorf("device name missing from args: %s", args)
	}
}

// streamCount is a test helper for asserting on registry bookkeeping.
func (h *Host) streamCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.streams)
}
