package sonos

// Battery state for the portable models — Roam, Roam 2, Move, Move 2.
//
// This is the one thing in the whole bridge that isn't UPnP: there is no
// SOAP action for it, and the speakers answer a plain HTTP document at
// /status/batterystatus instead. Every other Sonos ever made returns an
// error or an empty block there, which is exactly the capability probe the
// EQ settings already use — ask, and let the answer decide whether the
// control exists (settings.go).
//
// It lives with the settings snapshot rather than in the status poll, and
// that is a deliberate exception to "only state belongs in the poll": a
// battery *is* state, but it is state that moves over hours. Polling every
// speaker in the house every five seconds to watch a number go from 84 to
// 83 is the cost that rule exists to prevent, and the answer is stale by
// minutes at worst on a surface someone had to open.

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Battery is a portable speaker's power state. Nil on every model without
// one — absent rather than zeroed, so "no battery" can't be mistaken for
// "flat battery".
type Battery struct {
	// Level is the charge percentage, 0–100.
	Level int `json:"level"`
	// Charging is true whenever the speaker is drawing power rather than
	// running the battery down — on its ring, on USB, or on a dock. Which of
	// those it is doesn't change what anyone would do about it.
	Charging bool `json:"charging"`
	// Health and Temperature are the speaker's own words, and are worth
	// showing only when they are *not* the normal ones: a four-year-old Roam
	// reporting anything but GREEN/NORMAL is the reason someone opened this
	// screen. Empty when the speaker didn't say.
	Health      string `json:"health,omitempty"`
	Temperature string `json:"temperature,omitempty"`
}

// OK reports whether the speaker is happy with its own battery. False is
// what earns a warning in the UI; an unstated field is not a complaint.
func (b *Battery) OK() bool {
	if b == nil {
		return true
	}
	healthy := b.Health == "" || strings.EqualFold(b.Health, "GREEN")
	cool := b.Temperature == "" || strings.EqualFold(b.Temperature, "NORMAL")
	return healthy && cool
}

// batteryStatusXML mirrors /status/batterystatus, which is a bag of named
// <Data> elements rather than a typed document.
type batteryStatusXML struct {
	Local struct {
		Data []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"LocalBatteryStatus"`
}

// GetBattery reads the battery block from a speaker. It returns (nil, nil)
// for a speaker that has no battery to report, which is most of them — the
// absence is an answer, not a failure, and callers fold it into a settings
// snapshot rather than treating it as an outage.
func GetBattery(ctx context.Context, ip string) (*Battery, error) {
	if err := ValidateHost(ip); err != nil {
		return nil, fmt.Errorf("sonos: %w", err)
	}
	u := fmt.Sprintf("http://%s:%d/status/batterystatus", ip, Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("sonos: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sonos at %s: %w", ip, err)
	}
	defer resp.Body.Close()
	// A mains-only model answers 404 (or, on some firmwares, 200 with an
	// empty block). Both mean the same thing and neither is an error worth
	// showing anyone.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("sonos at %s returned HTTP %d", ip, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("sonos: read battery status: %w", err)
	}
	return ParseBattery(string(raw)), nil
}

// ParseBattery reads the status document. Nil whenever there is no battery
// being described — an empty block, a document without a level, or anything
// that doesn't parse. Split out for testability, like every other parser
// here.
func ParseBattery(body string) *Battery {
	var doc batteryStatusXML
	if err := xml.Unmarshal([]byte(body), &doc); err != nil {
		return nil
	}
	var b Battery
	var haveLevel bool
	for _, d := range doc.Local.Data {
		v := strings.TrimSpace(d.Value)
		switch strings.ToLower(d.Name) {
		case "level":
			n, err := strconv.Atoi(v)
			if err != nil {
				continue
			}
			b.Level = clamp(n, 0, 100)
			haveLevel = true
		case "health":
			b.Health = v
		case "temperature":
			b.Temperature = v
		case "powersource":
			// Anything that isn't the battery itself is a supply: the ring,
			// USB, a dock. The distinction the UI needs is "draining or
			// not", and the speaker names more sources than that.
			b.Charging = v != "" && !strings.EqualFold(v, "BATTERY")
		}
	}
	// A level is the one field that makes this worth reporting: without it
	// there is nothing to show, and a zeroed struct would read as flat.
	if !haveLevel {
		return nil
	}
	return &b
}
