package app

// What the store calls back into.
//
// The store holds the house's state and deliberately knows nothing about how a
// speaker is reached, how a phone is notified, or that an HTTP connection
// exists. It offers hooks instead, and this is the one file that fills them
// in. Keeping them together is the point: "what happens when a socket
// changes" should be answerable by reading one page, not by finding five
// assignments scattered through the layers that happen to be able to make
// them.
//
// Every hook here runs while the store lock is held unless its own
// documentation says otherwise, so every one of them either is cheap or hands
// the work to a goroutine.

import (
	"fmt"

	"homehub/internal/push"
	"homehub/internal/store"
)

// wireStore installs the callbacks. Called once, after every subsystem exists.
func (a *App) wireStore() {
	// A live signal to connected clients whenever a socket's state changes —
	// including the scheduler's and the timers', since those flow through
	// ApplyState too.
	a.store.OnChange = a.api.Changed

	// Let a scene or an automation reach the speakers, and let a rule watch a
	// room so it can fire when the living room goes quiet.
	a.store.OnMusic = a.music.RunActions
	a.store.MusicPlaying = a.music.Playing

	if a.push == nil {
		return
	}

	// One push per socket, for the changes a person did not initiate. Bulk
	// actions suppress these and send a single summary instead — see
	// internal/control.
	a.store.OnStateChange = func(socket store.Socket, newState bool) {
		action := "off"
		if newState {
			action = "on"
		}
		go a.push.NotifyEvent(push.CategoryStateChanges, socket.ID, push.PushPayload{
			Title: fmt.Sprintf("💡 %s turned %s", socket.Name, action),
			URL:   "/#/sockets",
			Tag:   "state-" + socket.ID,
		})
	}

	// Rising edge only: the store fires this when a reading first crosses a
	// threshold, not on every reading while it stays over.
	a.store.OnSensorAlert = func(sensor store.Sensor, value float64, direction string) {
		go a.push.NotifyEvent(push.CategorySensorAlerts, sensor.ID, push.PushPayload{
			Title: fmt.Sprintf("⚠️ %s alert", sensor.Name),
			Body:  fmt.Sprintf("%.1f%s (%s threshold)", value, sensor.Unit, direction),
			URL:   "/#/sensors",
			Tag:   "sensor-" + sensor.ID,
		})
	}
}
