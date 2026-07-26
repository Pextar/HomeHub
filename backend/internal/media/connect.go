package media

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoConnectDevice marks "this speaker isn't a playable Connect device right
// now". It is a state the user can fix — wake the speaker, pick it once in the
// service's own app — so the API answers 409 rather than 502.
var ErrNoConnectDevice = errors.New("connect")

// MatchConnectDevice picks the device for an endpoint out of a service's
// device list, using the hints the endpoint supplies.
//
// A pinned id wins outright. Otherwise the endpoint is matched by name — its
// pinned name first (a pin whose id rotated, which Spotify does when a device
// re-registers), then the speaker's own name. Nothing is guessed beyond that:
// starting music in the wrong room is worse than saying which speaker to pick,
// so an endpoint with no usable hint is an error rather than a fallback to
// whichever device happens to be first.
func MatchConnectDevice(t ConnectTarget, name string, devices []ConnectDevice) (ConnectDevice, error) {
	pinned, names := t.ConnectHint()
	if pinned != "" {
		for _, d := range devices {
			if d.ID == pinned {
				return usableDevice(d)
			}
		}
	}
	for _, want := range names {
		if normalizeDeviceName(want) == "" {
			continue
		}
		for _, d := range devices {
			if normalizeDeviceName(d.Name) == normalizeDeviceName(want) {
				return usableDevice(d)
			}
		}
	}
	// Which of the two failures this is changes what the user should do
	// about it, so they get different sentences.
	if pinned != "" {
		return ConnectDevice{}, fmt.Errorf(
			"%w: %q isn't visible right now — wake the speaker, or pick it again under its settings",
			ErrNoConnectDevice, name)
	}
	return ConnectDevice{}, fmt.Errorf(
		"%w: no Connect speaker is called %q — play to it once from the service's own app, then pick it under the speaker's settings",
		ErrNoConnectDevice, name)
}

// usableDevice rejects a matched device that would refuse the command anyway,
// so the failure names the reason instead of arriving as a silent no-op.
func usableDevice(d ConnectDevice) (ConnectDevice, error) {
	if d.Restricted {
		return ConnectDevice{}, fmt.Errorf("%w: the service won't let other apps control %q",
			ErrNoConnectDevice, d.Name)
	}
	if d.ID == "" {
		return ConnectDevice{}, fmt.Errorf("%w: %q has no device id", ErrNoConnectDevice, d.Name)
	}
	return d, nil
}

// normalizeDeviceName folds the differences between what a speaker calls
// itself and what it registered with the service: case, surrounding space, and
// runs of whitespace ("Living  Room" vs "Living Room").
func normalizeDeviceName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
