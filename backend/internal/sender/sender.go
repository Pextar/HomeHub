// Package sender provides a multi-protocol dispatcher that implements
// store.RFSender and routes each transmission to the right backend:
// Tasmota (Wi-Fi, local HTTP), Matter (via matter-bridge sidecar), or
// 433 MHz RF (nexa/kaku/intertechno/raw).
package sender

import (
	"context"
	"strings"

	"rf-socket-controller/internal/matter"
	"rf-socket-controller/internal/rf"
	"rf-socket-controller/internal/tasmota"
)

// Multi dispatches Send calls based on the protocol field on the socket.
type Multi struct {
	RF     rf.Sender
	Matter *matter.Client // optional; nil disables the matter path
}

// Send implements store.RFSender.
func (m *Multi) Send(code, protocol string, state bool) error {
	switch {
	case strings.EqualFold(protocol, "tasmota"):
		ctx, cancel := context.WithTimeout(context.Background(), tasmota.DefaultTimeout)
		defer cancel()
		return tasmota.Send(ctx, code, state)
	case strings.EqualFold(protocol, "matter"):
		ctx, cancel := context.WithTimeout(context.Background(), matter.DefaultTimeout)
		defer cancel()
		return m.Matter.Send(ctx, code, state)
	default:
		return m.RF.Send(code, protocol, state)
	}
}
