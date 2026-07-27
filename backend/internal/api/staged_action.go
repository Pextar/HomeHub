package api

import (
	"fmt"

	"homehub/internal/store"
)

// The staged flow for actions that touch more than one device.
//
// Turning on a group, a room, a scene or everything at once all follow the
// same three beats, and getting the order wrong is the most consequential
// mistake available in this package:
//
//  1. Under Mu: resolve the target into the flat list of sends it implies.
//  2. Off the lock: transmit. Device I/O must never hold Mu — one slow or
//     unreachable device would otherwise stall every other request.
//  3. Under Mu again: fold the results back into socket state, record one
//     summary activity entry, and persist.
//
// Each caller used to spell all three out, so the sequence existed in four
// near-identical copies. stagedAction describes only the parts that differ
// and runStaged owns the sequence.

// stagedAction is one multi-device action: what to send, and how to label
// the result.
type stagedAction struct {
	// Kind and Action land on the activity entry: "group"/"room"/"scene"/
	// "bulk", and "on"/"off"/"toggle"/"activate".
	Kind   string
	Action string
	// Source distinguishes a REST call ("manual") from the assistant.
	Source string

	// Stage resolves the target into the sends it implies. It runs while the
	// write lock is held and must not perform I/O. Returning found=false
	// means no such target, which the caller turns into a 404.
	Stage func() (label string, staged []store.StagedSend, found bool)

	// AfterApply runs under the write lock once the send results have been
	// folded in, for bookkeeping that belongs in the same transaction as the
	// state change. Optional.
	AfterApply func(label string)

	// FlushLights drains the smart-light queue after the lock is released.
	// Only staging that queues light changes (scenes) needs it.
	FlushLights bool

	// Notify builds the single summary push sent when nothing failed to
	// persist. Returning "" skips the notification. Optional.
	Notify func(label string, ok int) string
}

// runStaged executes a stagedAction and reports how it went.
//
// ok counts the sends that succeeded and failures lists the ones that did
// not — a device that cannot be reached is reported per-socket rather than
// failing the whole request. err is only non-nil when persisting failed.
// Caller must NOT hold Mu.
func (s *Server) runStaged(a stagedAction) (label string, ok int, failures []map[string]string, found bool, err error) {
	s.Store.Mu.Lock()
	label, staged, found := a.Stage()
	s.Store.Mu.Unlock()
	if !found {
		return "", 0, nil, false, nil
	}

	s.Store.SendStaged(staged)

	s.Store.Mu.Lock()
	// Per-socket notifications are suppressed so the one summary below is
	// the only push a bulk action produces.
	s.Store.SuppressStateChange = true
	_ = s.Store.ApplyStaged(staged)
	s.Store.SuppressStateChange = false
	ok, failures = stagedFailures(staged)
	if a.AfterApply != nil {
		a.AfterApply(label)
	}
	entry := store.ActivityEntry{Kind: a.Kind, Source: a.Source, Action: a.Action, Label: label}
	if len(failures) > 0 {
		entry.Status = "error"
		entry.Error = fmt.Sprintf("%d of %d failed", len(failures), ok+len(failures))
	}
	s.Store.Activity.Add(entry)
	err = s.Store.Save()
	s.Store.Mu.Unlock()

	if a.FlushLights {
		s.Store.FlushLights()
	}
	if err != nil {
		return label, ok, failures, true, err
	}
	if a.Notify != nil {
		if msg := a.Notify(label, ok); msg != "" {
			s.notifyBulkState(msg, ok)
		}
	}
	return label, ok, failures, true, nil
}
