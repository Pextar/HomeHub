// Package reachability polls Wi-Fi / Matter devices on a fixed interval and
// fires push notifications when a device drops offline or comes back. RF
// sockets are fire-and-forget (no return channel) so they're skipped — only
// Tasmota (HTTP probe) and Matter (bridge reachability) devices are checked.
package reachability

import (
	"context"
	"log"
	"strings"
	"time"

	"rf-socket-controller/internal/matter"
	"rf-socket-controller/internal/push"
	"rf-socket-controller/internal/store"
	"rf-socket-controller/internal/tasmota"
)

const (
	// interval between full sweeps.
	interval = 90 * time.Second
	// failuresBeforeOffline debounces flaky devices: a device must miss this
	// many consecutive checks before we declare it offline.
	failuresBeforeOffline = 2
	// checkTimeout caps a single device probe.
	checkTimeout = 8 * time.Second
)

// Run blocks until ctx is cancelled. Spawn it in a goroutine. matterClient
// may be nil (Matter disabled); pushSvc may be nil (push disabled — then this
// is effectively a no-op and returns immediately).
func Run(ctx context.Context, st *store.Store, matterClient *matter.Client, pushSvc *push.Service) {
	if pushSvc == nil {
		return
	}
	// Per-socket health tracking, keyed by socket ID.
	failures := make(map[string]int)
	offline := make(map[string]bool)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		sweep(ctx, st, matterClient, pushSvc, failures, offline)
	}
}

// checkable is a snapshot of the fields needed to probe one device, taken
// under the store lock so the network calls below run lock-free.
type checkable struct {
	id       string
	name     string
	code     string
	protocol string
}

func sweep(
	ctx context.Context,
	st *store.Store,
	matterClient *matter.Client,
	pushSvc *push.Service,
	failures map[string]int,
	offline map[string]bool,
) {
	st.Mu.RLock()
	var devices []checkable
	for _, sock := range st.Sockets {
		if isCheckable(sock.Protocol, matterClient) {
			devices = append(devices, checkable{sock.ID, sock.Name, sock.Code, sock.Protocol})
		}
	}
	st.Mu.RUnlock()

	// Drop tracking state for devices that no longer exist or stopped being
	// checkable, so a re-added device starts clean.
	live := make(map[string]bool, len(devices))
	for _, d := range devices {
		live[d.id] = true
	}
	for id := range failures {
		if !live[id] {
			delete(failures, id)
			delete(offline, id)
		}
	}

	for _, d := range devices {
		reachable := probe(ctx, d, matterClient)
		if reachable {
			if offline[d.id] {
				offline[d.id] = false
				pushSvc.NotifyEvent(push.CategoryDeviceOffline, d.id, push.PushPayload{
					Title: "✅ " + d.name + " is back online",
					URL:   "/#/sockets",
					Tag:   "offline-" + d.id,
				})
			}
			failures[d.id] = 0
			continue
		}
		failures[d.id]++
		if failures[d.id] >= failuresBeforeOffline && !offline[d.id] {
			offline[d.id] = true
			log.Printf("reachability: %s (%s) is offline", d.name, d.id)
			pushSvc.NotifyEvent(push.CategoryDeviceOffline, d.id, push.PushPayload{
				Title: "🔌 " + d.name + " is offline",
				Body:  "The device stopped responding.",
				URL:   "/#/sockets",
				Tag:   "offline-" + d.id,
			})
		}
	}
}

// isCheckable reports whether a protocol exposes a reachability signal we can
// poll. Matter is only checkable when the bridge client is enabled.
func isCheckable(protocol string, matterClient *matter.Client) bool {
	switch strings.ToLower(protocol) {
	case "tasmota":
		return true
	case "matter", "matter-thread":
		return matterClient.Enabled()
	}
	return false
}

// probe returns true if the device responds / reports reachable.
func probe(ctx context.Context, d checkable, matterClient *matter.Client) bool {
	cctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	switch strings.ToLower(d.protocol) {
	case "tasmota":
		return tasmota.Probe(cctx, d.code) == nil
	case "matter", "matter-thread":
		if !matterClient.Enabled() {
			return true // can't check; assume fine
		}
		state, err := matterClient.GetState(cctx, d.code)
		return err == nil && state.Reachable
	}
	return true
}
