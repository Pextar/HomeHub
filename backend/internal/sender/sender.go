// Package sender provides a multi-protocol dispatcher that implements
// store.RFSender and routes each transmission to the right backend:
// Tasmota (Wi-Fi, local HTTP) or 433 MHz RF (nexa/kaku/intertechno/raw).
package sender

import (
	"context"
	"strings"

	"rf-socket-controller/internal/rf"
	"rf-socket-controller/internal/tasmota"
)

// Multi dispatches Send calls to the Tasmota HTTP API or the RF transmitter
// based on the protocol field stored on the socket.
type Multi struct {
	RF rf.Sender
}

// Send implements store.RFSender.
func (m *Multi) Send(code, protocol string, state bool) error {
	if strings.EqualFold(protocol, "tasmota") {
		ctx, cancel := context.WithTimeout(context.Background(), tasmota.DefaultTimeout)
		defer cancel()
		return tasmota.Send(ctx, code, state)
	}
	return m.RF.Send(code, protocol, state)
}
