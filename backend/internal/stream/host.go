// Package stream turns one decoded audio source into something several
// speakers can play at once.
//
// This is the machinery behind the media protocol's stream route, and it
// exists for one reason: a Spotify account has a single active playback
// session, so a KEF and a Sonos cannot each be told to play the same track.
// The only way to get one service onto speakers of different makes is for
// HomeHub to hold that single session itself, decode once, and re-serve the
// audio over the LAN — the role Roon Core plays for the services it supports.
//
// Two pieces:
//
//	Host      serves a decoded stream over HTTP and fans it out to however
//	          many speakers connect, each with its own buffer.
//	Librespot runs the decoder (see librespot.go).
//
// What this is not: a clock. Speakers each fill their own jitter buffer and
// start when ready, so they land within a few hundred milliseconds of each
// other rather than sample-locked. docs/MEDIA-PROTOCOL.md states that plainly
// and this package must not imply better.
package stream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"homehub/internal/media"
)

const (
	// chunkSize is how much is read from the decoder at a time. At CD-quality
	// PCM (176.4 kB/s) this is a little under 100 ms — small enough that a
	// speaker joining mid-stream starts quickly, large enough that the
	// per-chunk overhead stays irrelevant.
	chunkSize = 16 << 10

	// bufferChunks is how much a slow listener may fall behind before its
	// oldest audio is dropped, about six seconds. Speakers buffer a few
	// seconds themselves, so this absorbs a stall without letting a wedged
	// listener pin the whole stream in memory.
	bufferChunks = 64

	// idleTimeout is how long a published stream waits for its first
	// listener before giving up. A speaker that was told to play and never
	// connected is a failure, not something to hold a decoder open for.
	idleTimeout = 30 * time.Second
)

// ErrNoListeners is reported when nothing ever connected to a published
// stream — usually a speaker that accepted the URI and then couldn't reach us,
// which is worth distinguishing from a decoder that died.
var ErrNoListeners = errors.New("stream: no speaker connected")

// Config wires a Host to its surroundings.
type Config struct {
	// BaseURL is the address speakers should fetch from, e.g.
	// "http://192.168.1.10:8080". It must be an address the *speakers* can
	// reach, which on a multi-homed host is not just any local address —
	// the same problem the Sonos event callback already solves.
	BaseURL string
	// PathPrefix is where Handler is mounted. Defaults to "/stream".
	PathPrefix string
	// StartDelays compensates for speakers filling their buffers at
	// different rates, by spacing out when each is told to play.
	//
	// Defaults to nothing. Real values depend on the speakers, the network
	// and the firmware, so inventing them would be worse than leaving the
	// zone a few hundred milliseconds apart and letting someone who can
	// actually hear it tune the numbers.
	StartDelays map[media.Vendor]time.Duration
	Logf        func(format string, args ...any)
}

// Host serves decoded streams to speakers.
type Host struct {
	cfg Config

	mu      sync.RWMutex
	streams map[string]*published
}

// NewHost creates a Host. It serves nothing until Publish is called.
func NewHost(cfg Config) *Host {
	if cfg.PathPrefix == "" {
		cfg.PathPrefix = "/stream"
	}
	cfg.PathPrefix = "/" + strings.Trim(cfg.PathPrefix, "/")
	return &Host{cfg: cfg, streams: map[string]*published{}}
}

func (h *Host) logf(format string, args ...any) {
	if h.cfg.Logf != nil {
		h.cfg.Logf(format, args...)
	}
}

// published is one live stream and its listeners.
type published struct {
	id     string
	source io.ReadCloser
	// header is written to each listener before any audio — the WAV header,
	// which every listener needs its own copy of since each begins mid-stream.
	header      []byte
	contentType string

	mu        sync.Mutex
	listeners map[*listener]struct{}
	closed    bool
	// started guards the lazy read: the decoder is only pulled once a
	// speaker actually connects, so the opening seconds of a track aren't
	// discarded into an empty room.
	started bool

	cancel context.CancelFunc
	done   chan struct{}
}

// listener is one connected speaker.
type listener struct {
	ch chan []byte
	// dropped counts chunks discarded because this listener fell behind,
	// which is the signal that a speaker is struggling rather than idle.
	dropped int
}

// Publish begins serving s and returns the URL speakers should play.
//
// The decoder is not read until the first speaker connects, so nothing is
// lost in the gap between handing out the URL and a speaker acting on it.
// The returned stop function releases the stream, disconnects listeners and
// closes the source; it is safe to call more than once.
func (h *Host) Publish(ctx context.Context, s *media.Stream) (string, func(), error) {
	if s == nil || s.Body == nil {
		return "", nil, errors.New("stream: nothing to publish")
	}
	if h.cfg.BaseURL == "" {
		return "", nil, errors.New("stream: no reachable address is configured for this server")
	}
	id, err := randomID()
	if err != nil {
		return "", nil, err
	}

	contentType := s.ContentType
	if contentType == "" {
		contentType = ContentTypeWAV
	}

	// The stream outlives the request that created it: a speaker connects
	// after the HTTP response has gone back, so tying its lifetime to the
	// request context would tear it down immediately.
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	p := &published{
		id:          id,
		source:      s.Body,
		header:      headerFor(contentType),
		contentType: contentType,
		listeners:   map[*listener]struct{}{},
		cancel:      cancel,
		done:        make(chan struct{}),
	}

	h.mu.Lock()
	h.streams[id] = p
	h.mu.Unlock()

	// Give up if nothing ever connects, rather than holding a decoder open
	// for a speaker that silently failed to reach us.
	go h.reapIfIdle(streamCtx, p)

	url := strings.TrimRight(h.cfg.BaseURL, "/") + h.cfg.PathPrefix + "/" + id
	h.logf("stream: published %s (%s)", url, contentType)

	stop := func() { h.release(p) }
	return url, stop, nil
}

// StartDelay implements media.StreamHost.
func (h *Host) StartDelay(v media.Vendor) time.Duration { return h.cfg.StartDelays[v] }

// release tears a stream down: listeners are disconnected, the decoder is
// closed, and the id stops resolving. Safe to call repeatedly.
func (h *Host) release(p *published) {
	h.mu.Lock()
	delete(h.streams, p.id)
	h.mu.Unlock()

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	for l := range p.listeners {
		close(l.ch)
	}
	p.listeners = map[*listener]struct{}{}
	p.mu.Unlock()

	p.cancel()
	_ = p.source.Close()
	h.logf("stream: released %s", p.id)
}

// reapIfIdle releases a stream nothing connected to.
func (h *Host) reapIfIdle(ctx context.Context, p *published) {
	select {
	case <-time.After(idleTimeout):
	case <-ctx.Done():
		return
	case <-p.done:
		return // a listener arrived and the pump finished on its own terms
	}
	p.mu.Lock()
	started := p.started
	p.mu.Unlock()
	if !started {
		h.logf("stream: %s — %v", p.id, ErrNoListeners)
		h.release(p)
	}
}

// Handler serves the published streams. Mount it at Config.PathPrefix.
//
// Deliberately not behind the admin session middleware: the clients are
// speakers, which have no cookies and no way to authenticate. The stream id is
// the capability — a fresh 128-bit random per playback, unguessable and
// invalid the moment playback stops.
func (h *Host) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.Trim(strings.TrimPrefix(r.URL.Path, h.cfg.PathPrefix), "/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		h.mu.RLock()
		p := h.streams[id]
		h.mu.RUnlock()
		if p == nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", p.contentType)
		// Speakers probe with HEAD before playing, and some refuse a source
		// that doesn't declare it can't be seeked.
		w.Header().Set("Accept-Ranges", "none")
		w.Header().Set("Cache-Control", "no-store")
		// transferMode is what DLNA renderers look for to know this is
		// streaming audio rather than a file to download.
		w.Header().Set("transferMode.dlna.org", "Streaming")
		w.Header().Set("contentFeatures.dlna.org",
			"DLNA.ORG_OP=00;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000")

		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}

		h.serveListener(w, r, p)
	})
}

// serveListener attaches one speaker to a stream and writes to it until it
// disconnects or the stream ends.
func (h *Host) serveListener(w http.ResponseWriter, r *http.Request, p *published) {
	l := &listener{ch: make(chan []byte, bufferChunks)}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		http.Error(w, "stream ended", http.StatusGone)
		return
	}
	p.listeners[l] = struct{}{}
	first := !p.started
	p.started = true
	p.mu.Unlock()

	// The first listener starts the decoder pump. Later ones join live,
	// which is the correct behaviour for a shared source: a speaker that
	// connects second should hear what the first is hearing now, not replay
	// the opening from the start and sit permanently behind.
	if first {
		go h.pump(p)
	}

	defer func() {
		p.mu.Lock()
		if _, ok := p.listeners[l]; ok {
			delete(p.listeners, l)
		}
		p.mu.Unlock()
	}()

	flusher, _ := w.(http.Flusher)
	if _, err := w.Write(p.header); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}

	h.logf("stream: %s — listener connected from %s", p.id, r.RemoteAddr)
	for {
		select {
		case chunk, ok := <-l.ch:
			if !ok {
				return // stream ended
			}
			if _, err := w.Write(chunk); err != nil {
				h.logf("stream: %s — listener %s went away: %v", p.id, r.RemoteAddr, err)
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

// pump reads the decoder and fans each chunk out to every listener. One
// goroutine per stream, started by the first listener.
func (h *Host) pump(p *published) {
	defer close(p.done)
	buf := make([]byte, chunkSize)
	for {
		n, err := p.source.Read(buf)
		if n > 0 {
			// Each listener gets its own copy: they consume at their own
			// pace and buf is reused on the next read.
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if !p.broadcast(chunk) {
				return // released
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				h.logf("stream: %s — decoder ended: %v", p.id, err)
			}
			h.release(p)
			return
		}
	}
}

// broadcast delivers a chunk to every listener, dropping the oldest audio for
// any that has fallen behind. Reports false once the stream is released.
//
// Dropping rather than blocking is the important choice: one speaker that
// stops reading — wedged, or on a bad bit of Wi-Fi — must not stall the
// others. The cost is a glitch on the slow speaker, which is recoverable;
// blocking would take down the whole zone, which is not.
func (p *published) broadcast(chunk []byte) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	for l := range p.listeners {
		select {
		case l.ch <- chunk:
		default:
			// Full: discard this listener's oldest chunk to make room, so
			// it resumes at the live edge instead of drifting further back.
			select {
			case <-l.ch:
				l.dropped++
			default:
			}
			select {
			case l.ch <- chunk:
			default:
			}
		}
	}
	return true
}

// randomID is the unguessable part of a stream URL. 16 bytes, because the id
// is the only thing standing between the stream and anything else on the LAN.
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("stream: %w", err)
	}
	return hex.EncodeToString(b), nil
}
