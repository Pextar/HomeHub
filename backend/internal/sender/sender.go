// Package sender provides a multi-protocol dispatcher that routes each
// command to the right backend: Tasmota (Wi-Fi, local HTTP), Matter (via
// matter-bridge sidecar), MQTT (publish to a broker), or 433 MHz RF
// (nexa/kaku/intertechno/raw).
//
// Multi implements both of the store's outbound interfaces — RFSender for
// on/off and LightController for brightness and colour — because both are the
// same decision made from the same field. Splitting them across two types
// meant two switches on socket.Protocol that had to be kept in step, and a
// protocol added to one of them silently did nothing in the other.
package sender

import (
	"context"
	"strings"
	"time"

	"homehub/internal/matter"
	"homehub/internal/mqtt"
	"homehub/internal/rf"
	"homehub/internal/store"
	"homehub/internal/tasmota"
)

// Multi dispatches commands based on the protocol field on the socket.
type Multi struct {
	RF     rf.Sender
	Matter *matter.Client // optional; nil disables the matter path
	MQTT   *mqtt.Client   // optional; nil disables the mqtt path
}

// Compile-time proof that Multi is everything the store sends through.
var (
	_ store.RFSender        = (*Multi)(nil)
	_ store.LightController = (*Multi)(nil)
)

// Send implements store.RFSender.
func (m *Multi) Send(code, protocol string, state bool) error {
	switch {
	case strings.EqualFold(protocol, "tasmota"):
		ctx, cancel := context.WithTimeout(context.Background(), tasmota.DefaultTimeout)
		defer cancel()
		return tasmota.Send(ctx, code, state)
	case strings.EqualFold(protocol, "matter"), strings.EqualFold(protocol, "matter-thread"):
		ctx, cancel := context.WithTimeout(context.Background(), matter.DefaultTimeout)
		defer cancel()
		return m.Matter.Send(ctx, code, state)
	case strings.EqualFold(protocol, "mqtt"):
		// Code is the command topic; publish ON/OFF. The paho client
		// applies its own publish timeout, so no context is needed here.
		return m.MQTT.Send(code, state)
	default:
		return m.RF.Send(code, protocol, state)
	}
}

// lightTimeout caps a brightness/colour call. Longer than a plain on/off
// because a bulb asked to fade to a level acknowledges when it gets there,
// not when it starts.
const lightTimeout = 6 * time.Second

// SetLight implements store.LightController: it applies a scene's brightness
// and colour to a smart light.
//
// Only the two protocols that have a concept of either are addressed. An RF
// socket is on/off and returns nil rather than an error, which is what lets a
// scene name a mixed set of lights without every caller having to filter them
// first — the store queues one command per socket and this decides which of
// them mean anything.
func (m *Multi) SetLight(socket store.Socket, level *int, color string) error {
	ctx, cancel := context.WithTimeout(context.Background(), lightTimeout)
	defer cancel()
	switch {
	case strings.EqualFold(socket.Protocol, "tasmota"):
		return tasmota.SetState(ctx, socket.Code, tasmota.StateUpdate{Dimmer: level, Color: color})
	case strings.EqualFold(socket.Protocol, "matter"), strings.EqualFold(socket.Protocol, "matter-thread"):
		// Unlike Send, an absent bridge is not an error here: a scene that
		// dims one Matter bulb among six RF sockets should still run.
		if m.Matter == nil || !m.Matter.Enabled() {
			return nil
		}
		return m.Matter.SetState(ctx, socket.Code, matter.StateUpdate{Level: level, Color: color})
	}
	return nil
}
