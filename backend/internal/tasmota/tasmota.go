// Package tasmota provides a minimal client for the Tasmota local HTTP API.
// Tasmota devices expose a simple GET /cm?cmnd=<command> interface — no hub,
// no cloud, no pairing. The device IP is stored in Socket.Code.
package tasmota

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout caps how long we wait for a device to respond.
const DefaultTimeout = 5 * time.Second

// State is the current device state parsed from cmnd=State.
// Fields are nil/empty when the device doesn't support that capability,
// so the frontend can tell "not a dimmer" from "dimmer at 0".
type State struct {
	On     bool   `json:"on"`
	Dimmer *int   `json:"dimmer,omitempty"` // 1-100
	Color  string `json:"color,omitempty"`  // RRGGBB hex
	CT     *int   `json:"ct,omitempty"`     // 153-500 mired (warm=500, cool=153)
}

// StateUpdate is a partial change sent via SetState.
type StateUpdate struct {
	On     *bool  `json:"on,omitempty"`
	Dimmer *int   `json:"dimmer,omitempty"` // 1-100
	Color  string `json:"color,omitempty"`  // RRGGBB hex
	CT     *int   `json:"ct,omitempty"`     // 153-500 mired
}

// Send turns a Tasmota device on or off. Used by the multi-protocol sender
// in the normal on/off socket control path.
func Send(ctx context.Context, ip string, on bool) error {
	cmd := "Power Off"
	if on {
		cmd = "Power On"
	}
	return runCmd(ctx, ip, cmd)
}

// GetState fetches the full device state.
func GetState(ctx context.Context, ip string) (*State, error) {
	u := fmt.Sprintf("http://%s/cm?cmnd=State", ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("tasmota: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tasmota: GET %s: %w", u, err)
	}
	defer resp.Body.Close()

	// Tasmota State response — only the fields we care about.
	var raw struct {
		Power  string `json:"POWER"`
		Dimmer *int   `json:"Dimmer"`
		Color  string `json:"Color"`
		CT     *int   `json:"CT"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("tasmota: decode state: %w", err)
	}

	s := &State{
		On:     strings.EqualFold(raw.Power, "ON"),
		Dimmer: raw.Dimmer,
		CT:     raw.CT,
	}
	if len(raw.Color) >= 6 {
		s.Color = raw.Color[:6] // trim W/WW channel suffix present on RGBW/RGBWW devices
	}
	return s, nil
}

// SetState applies a partial state update. Multiple fields are combined into
// a single Backlog command so the device gets one HTTP round-trip.
func SetState(ctx context.Context, ip string, update StateUpdate) error {
	var cmds []string

	if update.On != nil {
		if *update.On {
			cmds = append(cmds, "Power On")
		} else {
			cmds = append(cmds, "Power Off")
		}
	}
	if update.Dimmer != nil {
		d := clamp(*update.Dimmer, 1, 100)
		cmds = append(cmds, fmt.Sprintf("Dimmer %d", d))
	}
	if update.Color != "" {
		cmds = append(cmds, fmt.Sprintf("Color %s", update.Color))
	}
	if update.CT != nil {
		ct := clamp(*update.CT, 153, 500)
		cmds = append(cmds, fmt.Sprintf("CT %d", ct))
	}
	if len(cmds) == 0 {
		return fmt.Errorf("tasmota: empty state update")
	}
	if len(cmds) == 1 {
		return runCmd(ctx, ip, cmds[0])
	}
	return runCmd(ctx, ip, "Backlog "+strings.Join(cmds, "; "))
}

// Probe checks whether a Tasmota device is reachable at ip.
// Used by the "Test connection" button in the socket editor.
func Probe(ctx context.Context, ip string) error {
	u := fmt.Sprintf("http://%s/cm?cmnd=Power", ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("tasmota: build probe: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("no Tasmota device found at %s: %w", ip, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("tasmota at %s returned HTTP %d", ip, resp.StatusCode)
	}
	return nil
}

func runCmd(ctx context.Context, ip, cmd string) error {
	u := fmt.Sprintf("http://%s/cm?cmnd=%s", ip, url.QueryEscape(cmd))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("tasmota: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("tasmota: %s: %w", ip, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("tasmota: HTTP %d from %s", resp.StatusCode, ip)
	}
	return nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
