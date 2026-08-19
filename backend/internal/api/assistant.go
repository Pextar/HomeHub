package api

// The assistant's three routes. The agent behind them is internal/assistant;
// what is here is the transport: reading a turn off the wire, and streaming
// the answer back as Server-Sent Events over POST.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"homehub/internal/assistant"
)

// assistantChatRequest is one turn as the frontend sends it. Only user and
// assistant text is accepted; the system prompt and tool results are
// reconstructed server-side so a client can't inject either.
type assistantChatRequest struct {
	Messages []assistant.Message `json:"messages"`
}

type assistantConfirmRequest struct {
	Token    string              `json:"token"`
	Messages []assistant.Message `json:"messages"`
}

// assistantStatus reports whether the assistant is usable so the frontend can
// show or hide its entrance and surface a clear reason when it's down.
func (s *Server) assistantStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.Assistant.Status(ctx))
}

// assistantChat runs a fresh turn, streaming events back to the client.
func (s *Server) assistantChat(w http.ResponseWriter, r *http.Request) {
	var body assistantChatRequest
	stream, ok := s.beginAssistantTurn(w, r, &body)
	if !ok {
		return
	}
	defer stream.heartbeat(r.Context(), assistantHeartbeat)()
	s.Assistant.Chat(r.Context(), currentUser(r), body.Messages, stream)
}

// assistantConfirm executes a previously-paused bulk or destructive tool after
// the user confirmed it, then lets the model summarise.
//
// The confirmation is checked before streaming begins, so a token that no
// longer means anything gets a status code rather than an error event inside
// an otherwise successful response.
func (s *Server) assistantConfirm(w http.ResponseWriter, r *http.Request) {
	var body assistantConfirmRequest
	stream, ok := s.beginAssistantTurn(w, r, &body)
	if !ok {
		return
	}
	defer stream.heartbeat(r.Context(), assistantHeartbeat)()

	err := s.Assistant.Confirm(r.Context(), currentUser(r), body.Token, body.Messages, stream)
	if errors.Is(err, assistant.ErrBadConfirmation) {
		// Streaming has not begun — nothing has been written but headers, so
		// an error event is the honest way to say so on this connection.
		_ = stream.Emit("error", err.Error())
		_ = stream.Emit("done", "1")
	}
}

// beginAssistantTurn does what both routes open with: refuse when there is no
// model, decode the body, and take over the response for streaming.
func (s *Server) beginAssistantTurn(w http.ResponseWriter, r *http.Request, body any) (*eventStream, bool) {
	if !s.Assistant.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "assistant is disabled")
		return nil, false
	}
	if !decodeBody(w, r, body) {
		return nil, false
	}
	stream, ok := newEventStream(w)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return nil, false
	}
	return stream, true
}

// assistantHeartbeat keeps the connection alive during the long, silent gaps
// where the model cold-loads into RAM and evaluates the prompt (30s+ on a Pi)
// before the first token. Without periodic bytes, iOS Safari and proxies drop
// the fetch and the user sees "Load failed". The frontend ignores comment
// frames.
const assistantHeartbeat = 10 * time.Second

// --- SSE-over-POST stream ---

// eventStream writes Server-Sent Events on a streaming POST response. Each
// event's data is JSON so deltas with newlines stay single-line. Modeled on
// handleEvents: the global WriteTimeout is lifted and each write gets its own
// bounded deadline.
type eventStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
	rc      *http.ResponseController
	// mu serializes writes: the heartbeat goroutine and the request goroutine
	// (tokens, tools, done) both write to w, which is not concurrent-safe.
	mu sync.Mutex
}

func newEventStream(w http.ResponseWriter) (*eventStream, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	rc := http.NewResponseController(w)
	// Lift BOTH connection deadlines for the life of the stream. The write
	// deadline would otherwise sever a slow answer mid-token; the read deadline
	// (from the server's ReadTimeout) is the subtler killer — set when the POST
	// body is read, it fires ~ReadTimeout later and net/http cancels the request,
	// dropping the stream before a cold-loading model emits its first token. The
	// /events SSE survives this only because its EventSource client silently
	// reconnects; this fetch-based stream does not, so the drop is fatal.
	_ = rc.SetWriteDeadline(time.Time{})
	_ = rc.SetReadDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return &eventStream{w: w, flusher: flusher, rc: rc}, true
}

// emit sends one event. payload is JSON-encoded; a plain string payload is
// encoded as a JSON string so the client parses every event uniformly.
// Emit implements assistant.Stream.
func (e *eventStream) Emit(event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.rc.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if _, err := fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	e.flusher.Flush()
	return nil
}

// comment writes an SSE comment frame (a line beginning with ":"). Per the SSE
// spec these carry no event and the frontend parser discards them — they exist
// only to push bytes down an otherwise-idle stream so it isn't dropped.
func (e *eventStream) comment(text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.rc.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if _, err := fmt.Fprintf(e.w, ": %s\n\n", text); err != nil {
		return err
	}
	e.flusher.Flush()
	return nil
}

// heartbeat sends a keepalive comment every interval until the returned stop
// func is called or ctx is cancelled. It guards against the stream sitting
// silent long enough (cold model load + prompt eval) for the client or a proxy
// to give up. The returned stop is safe to call multiple times.
func (e *eventStream) heartbeat(ctx context.Context, interval time.Duration) func() {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-t.C:
				if err := e.comment("keepalive"); err != nil {
					return // client gone; the request goroutine will notice too
				}
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}
