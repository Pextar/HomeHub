// Package sender provides a multi-protocol dispatcher that implements
// store.RFSender and routes each transmission to the right backend:
// Philips Hue (Wi-Fi) or 433 MHz RF (nexa/kaku/intertechno/raw).
package sender

import (
	"context"
	"strings"

	"rf-socket-controller/internal/hue"
	"rf-socket-controller/internal/rf"
	"rf-socket-controller/internal/store"
)

// Multi dispatches Send calls to the Hue HTTP API or the RF transmitter
// based on the protocol field. Settings is read on every call so that
// changes made via the Settings UI take effect immediately.
type Multi struct {
	RF       rf.Sender
	Settings *store.Settings
}

// Send implements store.RFSender.
func (m *Multi) Send(code, protocol string, state bool) error {
	if strings.EqualFold(protocol, "hue") {
		ctx, cancel := context.WithTimeout(context.Background(), hue.DefaultTimeout)
		defer cancel()
		return hue.Send(ctx, m.Settings.HueBridgeIP, m.Settings.HueUsername, code, state)
	}
	return m.RF.Send(code, protocol, state)
}
