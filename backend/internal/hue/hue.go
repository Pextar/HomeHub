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

// Light holds the subset of Hue light metadata used for the picker UI.
type Light struct {
	Name string `json:"name"`
	On   bool   `json:"on"`
}

// Send turns a single Hue light on or off.
// lightID is the numeric string assigned by the bridge (e.g. "1", "3").
func Send(ctx context.Context, bridgeIP, username, lightID string, on bool) error {
	if bridgeIP == "" || username == "" {
		return fmt.Errorf("hue bridge not configured (set bridge IP and username in Settings)")
	}
	lightID = strings.TrimSpace(lightID)
	if lightID == "" {
		return fmt.Errorf("hue light ID is empty")
	}

	url := fmt.Sprintf("http://%s/api/%s/lights/%s/state", bridgeIP, username, lightID)
	body, _ := json.Marshal(map[string]bool{"on": on})

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("hue: build lights request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hue: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Raw response: map[lightID]{"name":..., "state":{"on":...}, ...}
	var raw map[string]struct {
		Name  string `json:"name"`
		State struct {
			On bool `json:"on"`
		} `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("hue: decode lights: %w", err)
	}

	out := make(map[string]Light, len(raw))
	for id, l := range raw {
		out[id] = Light{Name: l.Name, On: l.State.On}
	}
	return out, nil
}

// DefaultTimeout is used by callers that don't supply their own context.
const DefaultTimeout = 5 * time.Second

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
