// Package llm is the Go-side client for a local Ollama server. It speaks
// Ollama's small JSON HTTP API (/api/chat, /api/version) and exposes the
// tool-calling primitives the assistant layer needs.
//
// The client is deliberately thin: it knows how to run one chat round
// (deciding on tool calls) and how to stream a final answer token-by-token.
// The agentic loop, tool registry, and store mutations all live in the api
// package — this package never touches the store and never blocks on it.
//
// Modeled on internal/matter: a context-deadline-only HTTP client (no
// client-level timeout, so a slow first inference on a Pi can't be severed
// mid-stream), an Enabled() guard, and a FromEnv() constructor.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultModel is a small, fast tool-calling model — chosen so CPU inference
// on a Pi stays responsive across the agent's multi-round tool loop. A 3B is
// noticeably slower per round and can exceed LLM_TIMEOUT. Override with
// LLM_MODEL (e.g. llama3.2:3b) for stronger results on faster hardware.
const DefaultModel = "llama3.2:1b"

// DefaultBaseURL is Ollama's loopback address. Override with OLLAMA_URL.
const DefaultBaseURL = "http://127.0.0.1:11434"

// DefaultTimeout caps a single chat round. Generous because Ollama reloads
// the model into RAM on the first request after idle (10–30s on a Pi 4) and
// a 3B model generates at low single-digit tokens/sec. Override with LLM_TIMEOUT.
const DefaultTimeout = 120 * time.Second

// DefaultKeepAlive asks Ollama to hold the model in RAM between requests so
// back-to-back questions skip the multi-second cold reload. Sent on every chat
// request so it applies even if the server's OLLAMA_KEEP_ALIVE isn't set.
// Override with LLM_KEEP_ALIVE (e.g. "-1" to keep it resident indefinitely).
const DefaultKeepAlive = "30m"

// Role values for ChatMessage.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall is one function invocation the model requested. Ollama returns
// arguments as a decoded JSON object (not a string, unlike OpenAI).
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction names the tool and carries its arguments.
type ToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ChatMessage is one turn in the conversation. ToolCalls is populated on
// assistant messages that request tools; ToolName labels a tool result so
// the model can match it to the call it made.
type ChatMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
}

// Tool is a function definition advertised to the model. Parameters is a
// JSON Schema object describing the arguments.
type Tool struct {
	Type     string       `json:"type"` // always "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction is the function half of a Tool.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatRequest is the body for POST /api/chat.
type ChatRequest struct {
	Model     string         `json:"model"`
	Messages  []ChatMessage  `json:"messages"`
	Tools     []Tool         `json:"tools,omitempty"`
	Stream    bool           `json:"stream"`
	Options   map[string]any `json:"options,omitempty"` // num_ctx, temperature, …
	KeepAlive string         `json:"keep_alive,omitempty"`
}

// chatResponse is one /api/chat object. When streaming, the server emits a
// sequence of these (NDJSON); Message.Content holds the delta and Done marks
// the final object. When not streaming, a single object carries the full
// message.
type chatResponse struct {
	Model   string      `json:"model"`
	Message ChatMessage `json:"message"`
	Done    bool        `json:"done"`
	Error   string      `json:"error"`
}

// Client talks to a local Ollama server.
type Client struct {
	BaseURL   string
	Model     string
	Timeout   time.Duration
	KeepAlive string
	HTTP      *http.Client
}

// FromEnv builds a Client from the environment, or returns nil when the
// assistant is disabled (LLM_ENABLED is not "true", or OLLAMA_URL is the
// literal "disabled"). A nil client is safe: Enabled() reports false and the
// api layer returns 503 for assistant routes.
func FromEnv() *Client {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("LLM_ENABLED")), "true") {
		return nil
	}
	base := strings.TrimSpace(os.Getenv("OLLAMA_URL"))
	if strings.EqualFold(base, "disabled") {
		return nil
	}
	if base == "" {
		base = DefaultBaseURL
	}
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))
	if model == "" {
		model = DefaultModel
	}
	timeout := DefaultTimeout
	if v := strings.TrimSpace(os.Getenv("LLM_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			timeout = d
		}
	}
	keepAlive := DefaultKeepAlive
	if v := strings.TrimSpace(os.Getenv("LLM_KEEP_ALIVE")); v != "" {
		keepAlive = v
	}
	return &Client{
		BaseURL:   strings.TrimRight(base, "/"),
		Model:     model,
		Timeout:   timeout,
		KeepAlive: keepAlive,
		// No client-level timeout — each call carries its own context
		// deadline so streaming isn't cut off mid-answer (matter pattern).
		HTTP: &http.Client{},
	}
}

// Enabled reports whether the client is configured to call a server.
func (c *Client) Enabled() bool { return c != nil && c.BaseURL != "" }

// Health pings the server's /api/version. Returns nil when reachable.
func (c *Client) Health(ctx context.Context) error {
	if !c.Enabled() {
		return fmt.Errorf("llm is not configured (set LLM_ENABLED=true)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/version", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("llm: reach %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("llm: server returned %d", resp.StatusCode)
	}
	return nil
}

// Chat runs one non-streaming round and returns the assistant message,
// including any tool calls the model decided to make. Used when we need the
// complete tool_calls array before acting.
func (c *Client) Chat(ctx context.Context, messages []ChatMessage, tools []Tool, options map[string]any) (ChatMessage, error) {
	return c.ChatStream(ctx, messages, tools, options, nil)
}

// ChatStream runs one round with streaming enabled, forwarding each content
// delta to onToken as it arrives, and returns the fully-assembled assistant
// message (concatenated content plus any tool calls). onToken may be nil to
// ignore deltas (effectively non-streaming). A round either produces tool
// calls (to act on) or a final answer (to show) — the loop in the api layer
// decides which based on the returned message.
func (c *Client) ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool, options map[string]any, onToken func(string) error) (ChatMessage, error) {
	if !c.Enabled() {
		return ChatMessage{}, fmt.Errorf("llm is not configured (set LLM_ENABLED=true)")
	}

	body, err := json.Marshal(ChatRequest{
		Model:     c.Model,
		Messages:  messages,
		Tools:     tools,
		Stream:    true,
		Options:   options,
		KeepAlive: c.KeepAlive,
	})
	if err != nil {
		return ChatMessage{}, fmt.Errorf("llm: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return ChatMessage{}, fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("llm: POST /api/chat: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ChatMessage{}, fmt.Errorf("llm: server returned %d", resp.StatusCode)
	}

	// Ollama streams NDJSON: one JSON object per line. Accumulate content and
	// collect any tool calls; the final object has done=true.
	out := ChatMessage{Role: RoleAssistant}
	var content strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	// Lines are small, but tool-call arguments can be sizable; lift the
	// default 64 KiB token cap generously.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var chunk chatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			return ChatMessage{}, fmt.Errorf("llm: decode stream: %w", err)
		}
		if chunk.Error != "" {
			return ChatMessage{}, fmt.Errorf("llm: %s", chunk.Error)
		}
		if delta := chunk.Message.Content; delta != "" {
			content.WriteString(delta)
			if onToken != nil {
				if err := onToken(delta); err != nil {
					return ChatMessage{}, err
				}
			}
		}
		if len(chunk.Message.ToolCalls) > 0 {
			out.ToolCalls = append(out.ToolCalls, chunk.Message.ToolCalls...)
		}
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatMessage{}, fmt.Errorf("llm: read stream: %w", err)
	}
	out.Content = content.String()
	return out, nil
}
