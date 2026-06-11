package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// sseHub fans a single "something changed" signal out to every connected
// Server-Sent Events client. Clients use it to refresh immediately instead
// of waiting for their polling interval — e.g. when a schedule fires or a
// physical remote toggles a socket.
type sseHub struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{clients: make(map[chan struct{}]struct{})}
}

// broadcast signals every client. Sends are non-blocking: a client whose
// buffer is already full will coalesce this signal into the pending one,
// which is exactly what we want (one refresh covers many rapid changes).
// Safe to call while the store lock is held.
func (h *sseHub) broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (h *sseHub) add() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *sseHub) remove(ch chan struct{}) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

// handleEvents streams change notifications to the client as SSE. The
// connection stays open; each "changed" event tells the SPA to refresh.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	// The server's global WriteTimeout (15s) would sever this long-lived
	// stream on its first slow moment; lift it for this connection and
	// instead bound each individual write below.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)

	ch := s.events.add()
	defer s.events.remove(ch)

	// write sends one frame with a bounded deadline so a stalled client
	// can't pin this goroutine forever; any error ends the stream and the
	// client's EventSource reconnects.
	write := func(frame string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprint(w, frame); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// Initial comment so the client's onopen fires promptly.
	if !write(": connected\n\n") {
		return
	}

	// Periodic keepalive: detects dead connections (deadline error above)
	// and stops idle proxies from timing out the stream.
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			if !write(": ping\n\n") {
				return
			}
		case <-ch:
			if !write("event: changed\ndata: 1\n\n") {
				return
			}
		}
	}
}
