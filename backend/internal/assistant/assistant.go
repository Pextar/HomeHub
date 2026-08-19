// Package assistant is HomeHub's local LLM assistant: the agent loop that
// turns a sentence into device commands, and the machinery that makes that
// safe.
//
// Three things shape it, and all three are why it is a package rather than a
// handler:
//
//   - The model call is slow — seconds to minutes on a Pi — and must run
//     entirely off the store lock. Tools take the lock briefly, through
//     internal/control, which is the same path the app's own buttons use.
//   - A bulk or destructive tool never executes without an explicit
//     confirmation. The pending action rides in an HMAC-signed token rather
//     than a server-side map, so nothing has to be remembered between
//     requests and a restart cannot strand one.
//   - The model is told what the house looks like and then asked to act on
//     it. Both halves — the state snapshot and the entity resolution that
//     turns "the kitchen lamp" into a socket id — are the assistant's own
//     work, not the HTTP layer's.
//
// What is left in the HTTP layer is the three routes and the Server-Sent
// Events writer they stream through.
package assistant

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"homehub/internal/control"
	"homehub/internal/llm"
	"homehub/internal/store"
)

// Stream is where an agent turn's progress goes: tokens as the model produces
// them, one event per tool call, a confirmation request, and a final "done".
//
// An interface because the agent should not know it is talking to a browser.
// The HTTP layer implements it over Server-Sent Events; a test implements it
// with a slice.
type Stream interface {
	Emit(event string, payload any) error
}

// Config is what the agent needs from the rest of the application.
type Config struct {
	// Store is the house it answers questions about.
	Store *store.Store
	// LLM is the model. Nil-safe via Enabled: a house without one simply has
	// no assistant.
	LLM *llm.Client
	// Control is how it acts, the same layer the REST handlers use, so the
	// lock and staged-flow discipline is never re-implemented here.
	Control *control.Actions
	// Secret signs confirmation tokens. The session secret, so a token
	// cannot outlive the installation that minted it.
	Secret []byte
}

// Agent runs turns. It holds no conversation state: the history arrives with
// each request, and a paused action rides in its token.
type Agent struct{ cfg Config }

// New returns an agent.
func New(cfg Config) *Agent { return &Agent{cfg: cfg} }

// Enabled reports whether there is a model to talk to.
func (a *Agent) Enabled() bool { return a.cfg.LLM.Enabled() }

// Status is what the frontend needs to decide whether to offer the assistant
// at all, and what to say when it cannot.
type Status struct {
	Enabled   bool   `json:"enabled"`
	Model     string `json:"model,omitempty"`
	Reachable bool   `json:"reachable,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

// Status reports whether the assistant is usable, reaching the model to find
// out. A disabled assistant answers without any round trip.
func (a *Agent) Status(ctx context.Context) Status {
	if !a.Enabled() {
		return Status{}
	}
	s := Status{Enabled: true, Model: a.cfg.LLM.Model, Reachable: true}
	if err := a.cfg.LLM.Health(ctx); err != nil {
		s.Reachable = false
		s.LastError = err.Error()
	}
	return s
}

// Chat runs a fresh turn from the client's history.
func (a *Agent) Chat(ctx context.Context, user *store.User, history []Message, stream Stream) {
	a.run(ctx, user, a.buildMessages(user, history), stream)
}

// ErrBadConfirmation marks a confirmation that cannot be honoured — expired,
// tampered with, or belonging to another session. It is returned rather than
// streamed because it happens before the turn begins, so the caller can still
// answer with a status code.
var ErrBadConfirmation = errors.New("confirmation cannot be honoured")

// Confirm executes a previously-paused tool after the user agreed to it, then
// re-enters the loop so the model can summarise what happened.
//
// The tool runs before anything is streamed, so a token that no longer means
// anything fails cleanly instead of half-way through a response.
func (a *Agent) Confirm(ctx context.Context, user *store.User, token string, history []Message, stream Stream) error {
	pending, err := a.verifyConfirmation(token, user)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBadConfirmation, err)
	}
	tool, exists := a.tools()[pending.Tool]
	if !exists {
		return fmt.Errorf("%w: unknown tool %s", ErrBadConfirmation, quote(pending.Tool))
	}

	// Feed the result back to the model as a completed tool call so it can
	// produce a natural closing message.
	result := tool.Execute(user, pending.Args)
	_ = stream.Emit("tool", map[string]any{"name": pending.Tool, "args": pending.Args, "result": result})

	messages := append(a.buildMessages(user, history),
		llm.ChatMessage{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
			Function: llm.ToolCallFunction{Name: pending.Tool, Arguments: pending.Args},
		}}},
		llm.ChatMessage{Role: llm.RoleTool, ToolName: pending.Tool, Content: result},
	)
	a.run(ctx, user, messages, stream)
	return nil
}

const (
	// maxToolRounds bounds the agent loop so a confused model can't spin
	// forever on a slow Pi.
	maxToolRounds = 6
	// confirmationTTL bounds how long a confirmation token stays valid.
	confirmationTTL = 5 * time.Minute
)

// Message is one turn as the frontend sends it. Only user and
// assistant text are accepted; the system prompt and tool results are
// reconstructed server-side so a client can't inject either.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// runLoop is the agent loop: ask the model, run safe tools and feed results
// back, pause on the first confirm-required tool, stream the final answer.
// messages already includes the system prompt. Errors are streamed to the
// client (the HTTP status is already 200 once streaming has begun).
func (a *Agent) run(ctx context.Context, user *store.User, messages []llm.ChatMessage, stream Stream) {
	tools := a.tools()
	specs := specsFor(tools)
	// Tuned for CPU inference on a Pi: a 2048-token context fits the compact
	// prompt + tool specs + a (trimmed) tool result with room to spare, while
	// halving the KV-cache and prompt-eval cost vs 4096. num_predict caps the
	// answer so the model can't ramble into minutes of generation; replies are
	// meant to be short. Bump num_ctx if a very large home overflows the prompt.
	options := map[string]any{"num_ctx": 2048, "temperature": 0.4, "num_predict": 512}

	for round := 0; round < maxToolRounds; round++ {
		roundCtx, cancel := context.WithTimeout(ctx, a.cfg.LLM.Timeout)
		msg, err := a.cfg.LLM.ChatStream(roundCtx, messages, specs, options, func(delta string) error {
			return stream.Emit("token", delta)
		})
		cancel()
		if err != nil {
			_ = stream.Emit("error", "the assistant failed: "+err.Error())
			_ = stream.Emit("done", "1")
			return
		}
		messages = append(messages, msg)

		if len(msg.ToolCalls) == 0 {
			_ = stream.Emit("done", "1") // final answer already streamed
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
				token, err := a.signConfirmation(pendingAction{Tool: name, Args: tc.Function.Arguments, UserID: userID(user)})
				if err != nil {
					_ = stream.Emit("error", "could not prepare confirmation")
					_ = stream.Emit("done", "1")
					return
				}
				summary, affected := a.confirmationSummary(user, name, tc.Function.Arguments)
				_ = stream.Emit("confirmation", map[string]any{
					"token":    token,
					"summary":  summary,
					"affected": affected,
					"tool":     name,
				})
				_ = stream.Emit("done", "1")
				return
			}
			result := tool.Execute(user, tc.Function.Arguments)
			_ = stream.Emit("tool", map[string]any{"name": name, "args": tc.Function.Arguments, "result": result})
			messages = append(messages, llm.ChatMessage{Role: llm.RoleTool, ToolName: name, Content: result})
		}
	}

	_ = stream.Emit("error", "stopped after too many steps — try rephrasing")
	_ = stream.Emit("done", "1")
}

// buildMessages prepends the system prompt (with the live state snapshot) to
// the sanitised client history.
func (a *Agent) buildMessages(user *store.User, history []Message) []llm.ChatMessage {
	snap := a.buildSnapshot(user)
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
func (a *Agent) signConfirmation(p pendingAction) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	data := base64.RawURLEncoding.EncodeToString(raw)
	exp := strconv.FormatInt(time.Now().Add(confirmationTTL).Unix(), 10)
	payload := data + ":" + exp
	return payload + ":" + confirmationSig(a.cfg.Secret, payload), nil
}

// verifyConfirmation checks the signature, expiry, and that the token belongs
// to the requesting user, then returns the decoded action.
func (a *Agent) verifyConfirmation(token string, user *store.User) (pendingAction, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		return pendingAction{}, fmt.Errorf("malformed confirmation token")
	}
	data, expStr, sig := parts[0], parts[1], parts[2]
	payload := data + ":" + expStr
	want := confirmationSig(a.cfg.Secret, payload)
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
func (a *Agent) confirmationSummary(user *store.User, tool string, args map[string]any) (string, []string) {
	action := normalizeAction(argString(args, "action"))
	switch tool {
	case "all_devices":
		names := a.accessibleDeviceNames(user, "")
		return fmt.Sprintf("Turn %s all %d devices?", action, len(names)), names
	case "control_room":
		room, ok, _ := a.resolveRoom(argString(args, "room"))
		if !ok {
			room = argString(args, "room")
		}
		names := a.accessibleDeviceNames(user, room)
		return fmt.Sprintf("Turn %s all %d devices in %s?", action, len(names), room), names
	case "control_group":
		_, name, ok, _ := a.resolveGroup(argString(args, "group"))
		if !ok {
			name = argString(args, "group")
		}
		names := a.groupDeviceNames(argString(args, "group"))
		return fmt.Sprintf("Turn %s the %d devices in %s?", action, len(names), name), names
	default:
		return "Confirm this action?", nil
	}
}

// accessibleDeviceNames lists the names of devices the user can access,
// optionally filtered to a room (case-insensitive). Caller must NOT hold Mu.
func (a *Agent) accessibleDeviceNames(user *store.User, room string) []string {
	return store.ViewValue(a.cfg.Store, func() []string {
		var names []string
		for _, sock := range a.cfg.Store.Sockets {
			if !user.MayAccess(sock.ID) {
				continue
			}
			if room != "" && !strings.EqualFold(sock.Room, room) {
				continue
			}
			names = append(names, sock.Name)
		}
		return names
	})
}

// groupDeviceNames lists the member device names of a group. Caller must NOT hold Mu.
func (a *Agent) groupDeviceNames(ref string) []string {
	id, _, ok, _ := a.resolveGroup(ref)
	if !ok {
		return nil
	}
	return store.ViewValue(a.cfg.Store, func() []string {
		g, found := a.cfg.Store.Groups[id]
		if !found {
			return nil
		}
		var names []string
		for _, sid := range g.SocketIDs {
			if sock, ok := a.cfg.Store.Sockets[sid]; ok {
				names = append(names, sock.Name)
			}
		}
		return names
	})
}

func userID(u *store.User) string {
	if u == nil {
		return "" // auth disabled — single implicit admin
	}
	return u.ID
}
