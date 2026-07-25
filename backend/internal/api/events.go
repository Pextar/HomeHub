package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSE topics. Clients listen for the ones they care about, so a change in
// one corner of the house doesn't make every open tab refetch everything.
const (
	// topicChanged is the general store signal: a socket toggled, a
	// schedule fired, a scene ran.
	topicChanged = "changed"
	// topicMusic is speaker state — driven by the Sonos event monitor,
	// which is far chattier than the store (every volume nudge) and feeds
	// exactly one view plus the dashboard's glance card.
	topicMusic = "music"
)

// sseHub fans "something changed" signals out to every connected
// Server-Sent Events client. Clients use it to refresh immediately instead
// of waiting for their polling interval — e.g. when a schedule fires or a
// physical remote toggles a socket.
type sseHub struct {
	mu      sync.Mutex
	clients map[*sseClient]struct{}
}

// sseClient is one connected stream. Topics are collected in pending and
// drained by the writer, so a burst on one topic collapses into a single
// frame without hiding a different topic that arrived alongside it.
type sseClient struct {
	wake chan struct{}

	mu      sync.Mutex
	pending map[string]bool
}

func newSSEHub() *sseHub {
	return &sseHub{clients: make(map[*sseClient]struct{})}
}

// broadcast signals the general topic. Kept as a niladic func because it is
// installed directly as store.OnChange.
func (h *sseHub) broadcast() { h.broadcastTopic(topicChanged) }

// broadcastTopic signals every client that topic changed. Sends are
// non-blocking: a client whose wake-up is already pending coalesces into it,
// which is exactly what we want (one refresh covers many rapid changes).
// Safe to call while the store lock is held.
func (h *sseHub) broadcastTopic(topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		c.mu.Lock()
		c.pending[topic] = true
		c.mu.Unlock()
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}
}

// take returns the topics that have accumulated since the last call.
func (c *sseClient) take() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) == 0 {
		return nil
	}
	out := make([]string, 0, len(c.pending))
	for t := range c.pending {
		out = append(out, t)
	}
	clear(c.pending)
	return out
}

func (h *sseHub) add() *sseClient {
	c := &sseClient{wake: make(chan struct{}, 1), pending: make(map[string]bool, 2)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *sseHub) remove(c *sseClient) {
	h.mu.Lock()
	delete(h.clients, c)
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

	client := s.events.add()
	defer s.events.remove(client)

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
		case <-client.wake:
			for _, topic := range client.take() {
				if !write("event: " + topic + "\ndata: 1\n\n") {
					return
				}
			}
		}
	}
}
