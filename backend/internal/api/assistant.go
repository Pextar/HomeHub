// Package api: assistant.go — the local LLM assistant. It exposes three
// routes (status, chat, confirm), runs the server-side agentic loop that turns
// the model's tool calls into store mutations, and streams the result back as
// Server-Sent Events over POST.
//
// Two invariants shape this file:
//   - The Ollama call is slow (seconds–minutes) and runs entirely OFF the
//     store lock. Tools take Mu only briefly, via the shared do* helpers.
//   - Bulk/destructive tools never execute without an explicit user
//     confirmation, carried by a stateless HMAC-signed token (no server-side
//     pending-action map).
package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"rf-socket-controller/internal/llm"
	"rf-socket-controller/internal/store"
)

const (
	// maxToolRounds bounds the agent loop so a confused model can't spin
	// forever on a slow Pi.
	maxToolRounds = 6
	// confirmationTTL bounds how long a confirmation token stays valid.
	confirmationTTL = 5 * time.Minute
)

// assistantClientMessage is one turn as the frontend sends it. Only user and
// assistant text are accepted; the system prompt and tool results are
// reconstructed server-side so a client can't inject either.
type assistantClientMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type assistantChatRequest struct {
	Messages []assistantClientMessage `json:"messages"`
}

type assistantConfirmRequest struct {
	Token    string                   `json:"token"`
	Messages []assistantClientMessage `json:"messages"`
}

// assistantStatus reports whether the assistant is usable so the frontend can
// show or hide its entrance and surface a clear reason when it's down.
func (s *Server) assistantStatus(w http.ResponseWriter, r *http.Request) {
	if !s.LLM.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	resp := map[string]any{"enabled": true, "model": s.LLM.Model, "reachable": true}
	if err := s.LLM.Health(ctx); err != nil {
		resp["reachable"] = false
		resp["last_error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// assistantChat runs a fresh turn: build the system prompt + history and drive
// the agent loop, streaming events back to the client.
func (s *Server) assistantChat(w http.ResponseWriter, r *http.Request) {
	if !s.LLM.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "assistant is disabled")
		return
	}
	var body assistantChatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	user := currentUser(r)
	messages := s.buildMessages(user, body.Messages)

	stream, ok := newEventStream(w)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	s.runLoop(r.Context(), user, messages, stream)
}

// assistantConfirm executes a previously-paused bulk/destructive tool after
// the user confirmed it, then re-enters the loop so the model can summarise.
func (s *Server) assistantConfirm(w http.ResponseWriter, r *http.Request) {
	if !s.LLM.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "assistant is disabled")
		return
	}
	var body assistantConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	user := currentUser(r)
	pending, err := s.verifyConfirmation(body.Token, user)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tools := s.assistantTools()
	tool, exists := tools[pending.Tool]
	if !exists {
		writeError(w, http.StatusBadRequest, "unknown tool")
		return
	}

	stream, ok := newEventStream(w)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	// Execute the confirmed tool, then feed the result back to the model as a
	// completed tool call so it can produce a natural closing message.
	result := tool.Execute(user, pending.Args)
	_ = stream.emit("tool", map[string]any{"name": pending.Tool, "args": pending.Args, "result": result})

	messages := s.buildMessages(user, body.Messages)
	messages = append(messages,
		llm.ChatMessage{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
			Function: llm.ToolCallFunction{Name: pending.Tool, Arguments: pending.Args},
		}}},
		llm.ChatMessage{Role: llm.RoleTool, ToolName: pending.Tool, Content: result},
	)
	s.runLoop(r.Context(), user, messages, stream)
}

// runLoop is the agent loop: ask the model, run safe tools and feed results
// back, pause on the first confirm-required tool, stream the final answer.
// messages already includes the system prompt. Errors are streamed to the
// client (the HTTP status is already 200 once streaming has begun).
func (s *Server) runLoop(ctx context.Context, user *store.User, messages []llm.ChatMessage, stream *eventStream) {
	// Keep the SSE connection alive during the long, silent gaps where the model
	// cold-loads into RAM and evaluates the prompt (30s+ on a Pi) before the
	// first token. Without periodic bytes, iOS Safari (and proxies) drop the
	// fetch and the user sees "Load failed". The frontend ignores comment frames.
	stopBeat := stream.heartbeat(ctx, 10*time.Second)
	defer stopBeat()

	tools := s.assistantTools()
	specs := specsFor(tools)
	// Tuned for CPU inference on a Pi: a 2048-token context fits the compact
	// prompt + tool specs + a (trimmed) tool result with room to spare, while
	// halving the KV-cache and prompt-eval cost vs 4096. num_predict caps the
	// answer so the model can't ramble into minutes of generation; replies are
	// meant to be short. Bump num_ctx if a very large home overflows the prompt.
	options := map[string]any{"num_ctx": 2048, "temperature": 0.4, "num_predict": 512}

	for round := 0; round < maxToolRounds; round++ {
		roundCtx, cancel := context.WithTimeout(ctx, s.LLM.Timeout)
		msg, err := s.LLM.ChatStream(roundCtx, messages, specs, options, func(delta string) error {
			return stream.emit("token", delta)
		})
		cancel()
		if err != nil {
			_ = stream.emit("error", "the assistant failed: "+err.Error())
			_ = stream.emit("done", "1")
			return
		}
		messages = append(messages, msg)

		if len(msg.ToolCalls) == 0 {
			_ = stream.emit("done", "1") // final answer already streamed
			return
		}

		for _, tc := range msg.ToolCalls {
			name := tc.Function.Name
			tool, exists := tools[name]
			if !exists {
				messages = append(messages, llm.ChatMessage{
					Role: llm.RoleTool, ToolName: name,
					Content: "unknown tool " + quote(name),
				})
				continue
			}
			if tool.NeedsConfirm {
				token, err := s.signConfirmation(pendingAction{Tool: name, Args: tc.Function.Arguments, UserID: userID(user)})
				if err != nil {
					_ = stream.emit("error", "could not prepare confirmation")
					_ = stream.emit("done", "1")
					return
				}
				summary, affected := s.confirmationSummary(user, name, tc.Function.Arguments)
				_ = stream.emit("confirmation", map[string]any{
					"token":    token,
					"summary":  summary,
					"affected": affected,
					"tool":     name,
				})
				_ = stream.emit("done", "1")
				return
			}
			result := tool.Execute(user, tc.Function.Arguments)
			_ = stream.emit("tool", map[string]any{"name": name, "args": tc.Function.Arguments, "result": result})
			messages = append(messages, llm.ChatMessage{Role: llm.RoleTool, ToolName: name, Content: result})
		}
	}

	_ = stream.emit("error", "stopped after too many steps — try rephrasing")
	_ = stream.emit("done", "1")
}

// buildMessages prepends the system prompt (with the live state snapshot) to
// the sanitised client history.
func (s *Server) buildMessages(user *store.User, history []assistantClientMessage) []llm.ChatMessage {
	snap := s.buildSnapshot(user)
	out := []llm.ChatMessage{{Role: llm.RoleSystem, Content: systemPrompt(snap)}}
	for _, m := range history {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != llm.RoleUser && role != llm.RoleAssistant {
			continue // ignore client-supplied system/tool messages
		}
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		out = append(out, llm.ChatMessage{Role: role, Content: m.Content})
	}
	return out
}

// systemPrompt frames the assistant and embeds a compact text view of the
// current home state so the model can pass names straight to the tools.
func systemPrompt(snap stateSnapshot) string {
	state := snap.render()
	return strings.Join([]string{
		"You are the assistant for HomeHub, a home automation app. You help the user",
		"control their devices and answer questions about their home by calling tools.",
		"",
		"Rules:",
		"- The home state below is live and complete. Answer questions about current",
		"  device on/off status, rooms, scenes, groups, and latest sensor values",
		"  DIRECTLY from it — do NOT call a tool just to read what is already shown.",
		"- Call a tool only to DO something (control a device/room/group/scene) or to",
		"  fetch sensor history/trends over time (get_sensor_readings).",
		"- Use the tools to act; never claim you did something without calling the tool.",
		"- Prefer device/room/scene names from the state below; pass them straight to the tools.",
		"- If a name is ambiguous or missing, ask the user rather than guessing.",
		"- Bulk actions (whole room, group, or all devices) need confirmation — the app",
		"  handles that automatically when you call the tool; tell the user what you're about to do.",
		"- Keep replies short and concrete. Numbers and device names, not fluff.",
		"- You cannot create or delete schedules, scenes, or groups yet; point the user to the app for that.",
		"",
		"Current home state:",
		state,
	}, "\n")
}

// --- confirmation token (stateless, HMAC-signed with the session secret) ---

type pendingAction struct {
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	UserID string         `json:"user"`
}

// signConfirmation encodes a pending action as "base64(json):expiry:hmac".
func (s *Server) signConfirmation(p pendingAction) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	data := base64.RawURLEncoding.EncodeToString(raw)
	exp := strconv.FormatInt(time.Now().Add(confirmationTTL).Unix(), 10)
	payload := data + ":" + exp
	return payload + ":" + confirmationSig(s.SessionSecret, payload), nil
}

// verifyConfirmation checks the signature, expiry, and that the token belongs
// to the requesting user, then returns the decoded action.
func (s *Server) verifyConfirmation(token string, user *store.User) (pendingAction, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		return pendingAction{}, fmt.Errorf("malformed confirmation token")
	}
	data, expStr, sig := parts[0], parts[1], parts[2]
	payload := data + ":" + expStr
	want := confirmationSig(s.SessionSecret, payload)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return pendingAction{}, fmt.Errorf("invalid confirmation token")
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return pendingAction{}, fmt.Errorf("confirmation expired — ask again")
	}
	raw, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return pendingAction{}, fmt.Errorf("malformed confirmation token")
	}
	var p pendingAction
	if err := json.Unmarshal(raw, &p); err != nil {
		return pendingAction{}, fmt.Errorf("malformed confirmation token")
	}
	if p.UserID != userID(user) {
		return pendingAction{}, fmt.Errorf("confirmation does not match this session")
	}
	return p, nil
}

func confirmationSig(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("assistant-confirm:"))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// confirmationSummary produces a human sentence and the list of affected
// device names for the confirmation card.
func (s *Server) confirmationSummary(user *store.User, tool string, args map[string]any) (string, []string) {
	action := normalizeAction(argString(args, "action"))
	switch tool {
	case "all_devices":
		names := s.accessibleDeviceNames(user, "")
		return fmt.Sprintf("Turn %s all %d devices?", action, len(names)), names
	case "control_room":
		room, ok, _ := s.resolveRoom(argString(args, "room"))
		if !ok {
			room = argString(args, "room")
		}
		names := s.accessibleDeviceNames(user, room)
		return fmt.Sprintf("Turn %s all %d devices in %s?", action, len(names), room), names
	case "control_group":
		_, name, ok, _ := s.resolveGroup(argString(args, "group"))
		if !ok {
			name = argString(args, "group")
		}
		names := s.groupDeviceNames(argString(args, "group"))
		return fmt.Sprintf("Turn %s the %d devices in %s?", action, len(names), name), names
	default:
		return "Confirm this action?", nil
	}
}

// accessibleDeviceNames lists the names of devices the user can access,
// optionally filtered to a room (case-insensitive). Caller must NOT hold Mu.
func (s *Server) accessibleDeviceNames(user *store.User, room string) []string {
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	var names []string
	for _, sock := range s.Store.Sockets {
		if !canAccess(user, sock.ID) {
			continue
		}
		if room != "" && !strings.EqualFold(sock.Room, room) {
			continue
		}
		names = append(names, sock.Name)
	}
	return names
}

// groupDeviceNames lists the member device names of a group. Caller must NOT hold Mu.
func (s *Server) groupDeviceNames(ref string) []string {
	id, _, ok, _ := s.resolveGroup(ref)
	if !ok {
		return nil
	}
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	g, found := s.Store.Groups[id]
	if !found {
		return nil
	}
	var names []string
	for _, sid := range g.SocketIDs {
		if sock, ok := s.Store.Sockets[sid]; ok {
			names = append(names, sock.Name)
		}
	}
	return names
}

func userID(u *store.User) string {
	if u == nil {
		return "" // auth disabled — single implicit admin
	}
	return u.ID
}

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
func (e *eventStream) emit(event string, payload any) error {
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
