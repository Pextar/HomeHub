// Package hue provides a minimal client for the Philips Hue local HTTP API (v1).
// It communicates with the bridge over the local network — no cloud dependency.
package hue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// LightState mirrors the fields we care about from a Hue light's state.
// Pointers are used so missing fields stay nil rather than defaulting to 0
// (lets the frontend tell "feature absent" from "value is zero").
type LightState struct {
	On        bool     `json:"on"`
	Bri       *int     `json:"bri,omitempty"` // 1..254
	Hue       *int     `json:"hue,omitempty"` // 0..65535
	Sat       *int     `json:"sat,omitempty"` // 0..254
	CT        *int     `json:"ct,omitempty"`  // 153..500 (mireds)
	ColorMode string   `json:"colormode,omitempty"`
	Reachable bool     `json:"reachable"`
}

// Light is the metadata + state returned by the bridge for one light.
type Light struct {
	Name  string     `json:"name"`
	Type  string     `json:"type,omitempty"` // e.g. "Extended color light"
	State LightState `json:"state"`
}

// Send turns a single Hue light on or off. Convenience wrapper around SetState.
func Send(ctx context.Context, bridgeIP, username, lightID string, on bool) error {
	return SetState(ctx, bridgeIP, username, lightID, map[string]any{"on": on})
}

// SetState applies a partial state update. Accepted keys mirror the Hue API:
// on (bool), bri (int), hue (int), sat (int), ct (int), transitiontime (int).
func SetState(ctx context.Context, bridgeIP, username, lightID string, state map[string]any) error {
	if bridgeIP == "" || username == "" {
		return fmt.Errorf("hue bridge not configured (set bridge IP and username in Settings)")
	}
	lightID = strings.TrimSpace(lightID)
	if lightID == "" {
		return fmt.Errorf("hue light ID is empty")
	}
	if len(state) == 0 {
		return fmt.Errorf("hue: empty state update")
	}

	url := fmt.Sprintf("http://%s/api/%s/lights/%s/state", bridgeIP, username, lightID)
	body, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("hue: encode state: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("hue: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("hue: PUT %s: %w", url, err)
	}
	defer resp.Body.Close()

	return parseHueResponse(resp)
}

// Pair registers this app with the Hue bridge and returns the API username.
// The user must press the physical link button on the bridge before calling this.
func Pair(ctx context.Context, bridgeIP string) (string, error) {
	if bridgeIP == "" {
		return "", fmt.Errorf("bridge IP is required")
	}

	url := fmt.Sprintf("http://%s/api", bridgeIP)
	body, _ := json.Marshal(map[string]string{"devicetype": "rf-socket-controller#pi"})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("hue: build pair request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("hue: POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Hue returns an array of success or error objects.
	var result []struct {
		Success *struct {
			Username string `json:"username"`
		} `json:"success"`
		Error *struct {
			Description string `json:"description"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("hue: decode pair response: %w", err)
	}
	if len(result) == 0 {
		return "", fmt.Errorf("hue: empty response from bridge")
	}
	if result[0].Error != nil {
		return "", fmt.Errorf("hue: %s", result[0].Error.Description)
	}
	if result[0].Success == nil || result[0].Success.Username == "" {
		return "", fmt.Errorf("hue: no username in pair response")
	}
	return result[0].Success.Username, nil
}

// ListLights returns the lights registered on the bridge, keyed by light ID.
func ListLights(ctx context.Context, bridgeIP, username string) (map[string]Light, error) {
	if bridgeIP == "" || username == "" {
		return nil, fmt.Errorf("hue bridge not configured")
	}

	url := fmt.Sprintf("http://%s/api/%s/lights", bridgeIP, username)
	return doGet[map[string]Light](ctx, url)
}

// GetLight returns full state for a single light.
func GetLight(ctx context.Context, bridgeIP, username, lightID string) (*Light, error) {
	if bridgeIP == "" || username == "" {
		return nil, fmt.Errorf("hue bridge not configured")
	}
	lightID = strings.TrimSpace(lightID)
	if lightID == "" {
		return nil, fmt.Errorf("hue light ID is empty")
	}
	url := fmt.Sprintf("http://%s/api/%s/lights/%s", bridgeIP, username, lightID)
	light, err := doGet[Light](ctx, url)
	if err != nil {
		return nil, err
	}
	return &light, nil
}

// DefaultTimeout is used by callers that don't supply their own context.
const DefaultTimeout = 5 * time.Second

func doGet[T any](ctx context.Context, url string) (T, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return zero, fmt.Errorf("hue: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("hue: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	// The bridge can return either an object or an error array for a single GET.
	// Decode into json.RawMessage first so we can distinguish.
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return zero, fmt.Errorf("hue: decode: %w", err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		// Error array shape.
		var errs []struct {
			Error *struct {
				Description string `json:"description"`
			} `json:"error"`
		}
		if err := json.Unmarshal(trimmed, &errs); err == nil {
			for _, e := range errs {
				if e.Error != nil {
					return zero, fmt.Errorf("hue: %s", e.Error.Description)
				}
			}
		}
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, fmt.Errorf("hue: decode body: %w", err)
	}
	return out, nil
}

// parseHueResponse reads the Hue API array response and returns any error.
func parseHueResponse(resp *http.Response) error {
	var result []struct {
		Error *struct {
			Description string `json:"description"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Non-JSON body or unexpected shape — treat HTTP status as the signal.
		if resp.StatusCode >= 300 {
			return fmt.Errorf("hue: HTTP %d", resp.StatusCode)
		}
		return nil
	}
	for _, r := range result {
		if r.Error != nil {
			return fmt.Errorf("hue: %s", r.Error.Description)
		}
	}
	return nil
}
