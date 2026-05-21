// Package matter is the Go-side client for the matter-bridge Node.js
// sidecar (see matter-bridge/). The bridge owns the matter.js library and
// the IP/BLE conversation with each commissioned device; this package
// just speaks its small JSON HTTP API.
//
// Socket.Code stores the Matter node id assigned by the bridge.
package matter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultTimeout caps how long we wait for the bridge on read/write
// operations. Matter is slow on first reach (mDNS resolution, CASE
// session) so this is noticeably more generous than the Tasmota timeout.
//
// Commissioning needs much longer (BLE discovery + Wi-Fi onboarding can
// run 30–60s easily); callers there pass a wider context — the http.Client
// itself has no deadline so context is the single source of truth.
const DefaultTimeout = 15 * time.Second

// State mirrors the bridge's DeviceState. Fields are nil/empty when the
// device doesn't expose that capability, matching the Tasmota state shape
// so the frontend can reuse the same "smart light" modal.
type State struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Vendor    string `json:"vendor,omitempty"`
	Product   string `json:"product,omitempty"`
	Reachable bool   `json:"reachable"`
	On        *bool  `json:"on,omitempty"`
	Level     *int   `json:"level,omitempty"` // 0..100
	Color     string `json:"color,omitempty"` // RRGGBB
	CT        *int   `json:"ct,omitempty"`    // 153..500 mired
}

// StateUpdate is a partial change applied via SetState.
type StateUpdate struct {
	On    *bool  `json:"on,omitempty"`
	Level *int   `json:"level,omitempty"`
	Color string `json:"color,omitempty"`
	CT    *int   `json:"ct,omitempty"`
}

// Client talks to the matter-bridge sidecar.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// FromEnv returns a Client pointing at MATTER_BRIDGE_URL or the
// default loopback address. Returns nil when the env var is the literal
// "disabled", so deployments without Matter can opt out cleanly.
func FromEnv() *Client {
	u := strings.TrimSpace(os.Getenv("MATTER_BRIDGE_URL"))
	if strings.EqualFold(u, "disabled") {
		return nil
	}
	if u == "" {
		u = "http://127.0.0.1:8765"
	}
	// No client-level timeout — every call passes its own context deadline.
	// A client timeout would silently override the longer commissioning
	// deadline (90s) and abort BLE onboarding mid-handshake.
	return &Client{BaseURL: strings.TrimRight(u, "/"), HTTP: &http.Client{}}
}

// Enabled reports whether the client has a base URL to call. Callers can
// use this to skip Matter codepaths entirely when the bridge isn't
// configured (e.g. on a dev laptop).
func (c *Client) Enabled() bool { return c != nil && c.BaseURL != "" }

// Send turns a Matter device on or off. Used by the multi-protocol
// sender in the normal on/off socket control path.
func (c *Client) Send(ctx context.Context, nodeID string, on bool) error {
	return c.SetState(ctx, nodeID, StateUpdate{On: &on})
}

// List returns every commissioned device the bridge knows about.
func (c *Client) List(ctx context.Context) ([]State, error) {
	var out []State
	if err := c.do(ctx, http.MethodGet, "/devices", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetState fetches one device's live state.
func (c *Client) GetState(ctx context.Context, nodeID string) (*State, error) {
	var s State
	if err := c.do(ctx, http.MethodGet, "/devices/"+url.PathEscape(nodeID), nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SetState applies a partial state update to one device.
func (c *Client) SetState(ctx context.Context, nodeID string, update StateUpdate) error {
	return c.do(ctx, http.MethodPut, "/devices/"+url.PathEscape(nodeID)+"/state", update, nil)
}

// Commission asks the bridge to onboard a new device using the manual or
// "MT:" pairing code printed on it. Returns the assigned node id.
// transport selects the network type ("wifi" or "thread"); empty string means
// auto-detect (the bridge picks from what is configured, Thread-first).
func (c *Client) Commission(ctx context.Context, pairingCode, transport string) (string, error) {
	body := map[string]string{"pairing_code": pairingCode}
	if transport != "" {
		body["transport"] = transport
	}
	var out struct {
		NodeID string `json:"node_id"`
	}
	if err := c.do(ctx, http.MethodPost, "/commission", body, &out); err != nil {
		return "", err
	}
	return out.NodeID, nil
}

// Remove decommissions and forgets the device.
func (c *Client) Remove(ctx context.Context, nodeID string) error {
	return c.do(ctx, http.MethodDelete, "/devices/"+url.PathEscape(nodeID), nil, nil)
}

// Health pings the bridge. Returns nil if reachable.
func (c *Client) Health(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/health", nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	if !c.Enabled() {
		return fmt.Errorf("matter bridge is not configured (set MATTER_BRIDGE_URL)")
	}
	// Body must be passed as an untyped nil io.Reader when absent — passing
	// a typed-nil *bytes.Reader makes the body interface non-nil, and
	// http.NewRequestWithContext's *bytes.Reader fast-path then calls .Len()
	// on the nil pointer and panics.
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("matter: encode request: %w", err)
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("matter: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("matter: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Error == "" {
			apiErr.Error = resp.Status
		}
		return fmt.Errorf("matter: bridge returned %d: %s", resp.StatusCode, apiErr.Error)
	}
	if resp.StatusCode == http.StatusNoContent || out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("matter: decode response: %w", err)
	}
	return nil
}

