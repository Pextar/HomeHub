// Package control turns "turn the kitchen off" into device commands.
//
// Every way a household reaches its sockets — a tap in the app, an iOS
// shortcut, a sentence to the assistant — arrives here. The rules that make
// that safe belong with the actions rather than with any one caller, which is
// why this is a package and not a set of helpers on an HTTP server.
//
// The rule that matters is the staged flow. Turning on a group, a room, a
// scene or everything at once all follow the same three beats, and getting the
// order wrong is the most consequential mistake available in this codebase:
//
//  1. Under the write lock: resolve the target into the flat list of sends it
//     implies.
//  2. Off the lock: transmit. Device I/O must never hold the lock — one slow
//     or unreachable device would otherwise stall every other request.
//  3. Under the write lock again: fold the results back into socket state,
//     record one summary activity entry, and persist.
//
// A single socket is the exception: it transmits synchronously, because the
// caller is waiting on one device and can be told directly that it failed.
package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"homehub/internal/store"
)

// Source says who asked, for the activity log. It is not decoration: a
// household reading back "everything went off at 23:40" wants to know whether
// a person did that or something in the house decided to.
type Source string

const (
	// SourceManual is a person acting through the app or a shortcut.
	SourceManual Source = "manual"
	// SourceAssistant is the local LLM acting on a sentence.
	SourceAssistant Source = "assistant"
	// SourceSchedule is a schedule coming due on the clock.
	SourceSchedule Source = "schedule"
	// SourceTimer is a one-shot timer firing.
	SourceTimer Source = "timer"
	// SourceAutomation is an automation rule — whether a trigger matched or a
	// person pressed "Run" in its editor. The two are the same run, and the
	// log should not claim otherwise.
	SourceAutomation Source = "automation"
)

// Allow reports whether the caller may act on a socket. Passing a predicate
// rather than a user keeps this package out of the permissions model: it
// enforces the answer, it does not compute it.
//
// A nil Allow means everything is permitted, which is what an unauthenticated
// household is.
type Allow func(socketID string) bool

func (a Allow) permits(socketID string) bool { return a == nil || a(socketID) }

// Config is what the action layer needs from the rest of the application.
type Config struct {
	// Store holds the sockets, groups, scenes and the activity log.
	Store *store.Store

	// Notify sends the one summary push a multi-device action produces.
	// Per-socket notifications are suppressed while it runs, so this is the
	// only one. Optional.
	Notify func(title string, changed int)
}

// Actions is the entry point for everything that switches a device.
type Actions struct{ cfg Config }

// New returns the action layer. It holds no state of its own: the store is
// the only thing an action changes.
func New(cfg Config) *Actions { return &Actions{cfg: cfg} }

// Result is how a multi-device action went.
//
// OK counts the sends that succeeded and Failures lists the ones that did not
// — a device that cannot be reached is reported per-socket rather than failing
// the whole request, because switching nine lights and missing one is not the
// same as doing nothing.
type Result struct {
	// Label is what the target is called, for the response and the log.
	Label string
	// OK is how many sockets took the command.
	OK int
	// Failures is one entry per socket that refused, with its id and error.
	Failures []map[string]string
	// Found is false when nothing matched the target at all — a caller turns
	// that into a 404 rather than reporting a successful no-op.
	Found bool
}

// Err returns the first device failure, or nil when every send landed.
//
// It exists for the background engines, which have no one to report a partial
// result to and want a line in the log. A caller answering a request should
// use Failures instead: "one of nine lamps refused" is what the household
// needs to see, and an error flattens it.
func (r Result) Err() error {
	if len(r.Failures) == 0 {
		return nil
	}
	return errors.New(r.Failures[0]["error"])
}

// Socket applies on/off/toggle to one socket by id.
//
// Unlike the multi-device actions this transmits synchronously, so a device
// error reaches the caller directly instead of arriving as a failure count.
// found is false when no socket has the id. Caller must NOT hold the lock.
func (a *Actions) Socket(id, action string, source Source) (sock store.Socket, found bool, err error) {
	var target *bool
	switch action {
	case "on":
		t := true
		target = &t
	case "off":
		t := false
		target = &t
	case "toggle":
		target = nil
	default:
		return store.Socket{}, true, fmt.Errorf("unsupported action %q (use on, off, or toggle)", action)
	}

	st := a.cfg.Store
	var applyErr error
	saveErr := st.Update(func() error {
		socket, ok := st.Sockets[id]
		if !ok {
			return store.ErrNoChange
		}
		found = true
		applyErr = st.ApplyState(socket, target)
		entry := store.ActivityEntry{
			Kind: "socket", Source: string(source), Action: action, Label: socket.Name,
		}
		if applyErr != nil {
			entry.Status = "error"
			entry.Error = applyErr.Error()
		}
		st.Activity.Add(entry)
		sock = *socket
		if applyErr != nil {
			// The transmit failed, so there is no new state to persist. The
			// activity entry above still records the attempt.
			return store.ErrNoChange
		}
		return nil
	})
	if !found {
		return store.Socket{}, false, nil
	}
	if applyErr != nil {
		return sock, true, applyErr
	}
	return sock, true, saveErr
}

// All switches every socket the caller may reach on or off.
func (a *Actions) All(allow Allow, target bool, source Source) (Result, error) {
	action := onOff(target)
	st := a.cfg.Store
	return a.staged(staged{
		kind: "bulk", action: action, source: source,
		stage: func() (string, []store.StagedSend, bool) {
			sends := make([]store.StagedSend, 0, len(st.Sockets))
			for _, sock := range st.Sockets {
				if !allow.permits(sock.ID) {
					continue
				}
				sends = append(sends, st.StageSocketSend(sock.ID, action))
			}
			// Always found: switching an empty set is a success, not a 404.
			return "All sockets", sends, true
		},
		notify: func(string, int) string { return "All devices turned " + action },
	})
}

// Room switches every socket the caller may reach in a named room.
func (a *Actions) Room(room string, allow Allow, target bool, source Source) (Result, error) {
	action := onOff(target)
	st := a.cfg.Store
	return a.staged(staged{
		kind: "room", action: action, source: source,
		stage: func() (string, []store.StagedSend, bool) {
			var sends []store.StagedSend
			for _, sock := range st.Sockets {
				if !strings.EqualFold(sock.Room, room) || !allow.permits(sock.ID) {
					continue
				}
				sends = append(sends, st.StageSocketSend(sock.ID, action))
			}
			// An empty room is a 404 rather than a no-op success: the caller
			// named something, and nothing by that name has any devices.
			return room, sends, len(sends) > 0
		},
		notify: func(label string, _ int) string { return label + " turned " + action },
	})
}

// Group applies an action to every member of a group.
func (a *Actions) Group(id, action string, source Source) (Result, error) {
	st := a.cfg.Store
	return a.staged(staged{
		kind: "group", action: action, source: source,
		stage: func() (string, []store.StagedSend, bool) {
			g, exists := st.Groups[id]
			if !exists {
				return "", nil, false
			}
			sends, _ := st.StageAction("group", id, action)
			return g.Name, sends, true
		},
		notify: func(label string, _ int) string { return label + " turned " + action },
	})
}

// Scene runs a scene's immediate step and records activation telemetry.
func (a *Actions) Scene(id string, source Source) (Result, error) {
	st := a.cfg.Store
	// Music needs nothing extra here: staging the scene queues the immediate
	// step's actions in the same buffer the lights use, and the staged flow
	// drains both once the lock is released. Later steps carry their own and
	// go out with them when they fire (store.ScheduleStep).
	return a.staged(staged{
		kind: "scene", action: "activate", source: source,
		stage: func() (string, []store.StagedSend, bool) {
			scene, ok := st.Scenes[id]
			if !ok {
				return "", nil, false
			}
			// Staging also queues smart-light brightness/colour and schedules
			// any delayed steps; flushLights below drains the queue.
			sends, _ := st.StageAction("scene", id, "activate")
			return scene.Name, sends, true
		},
		afterApply: func(Result) {
			// Telemetry for the UI's "ran N× · 2h ago". Re-fetched rather
			// than captured: the scene may have been deleted while the sends
			// were in flight.
			if sc, still := st.Scenes[id]; still {
				sc.LastActivatedAt = time.Now().UTC()
				sc.ActivateCount++
			}
		},
		flushLights: true,
		notify:      func(label string, _ int) string { return "Scene activated: " + label },
	})
}

// ── The staged flow ──────────────────────────────────────────────────────

// staged describes one multi-device action: what to send, and how to label the
// result. Only the parts that differ between actions; the sequence itself is
// owned by run, so it exists once rather than in four near-identical copies.
type staged struct {
	// kind and action land on the activity entry: "group"/"room"/"scene"/
	// "bulk", and "on"/"off"/"toggle"/"activate".
	kind   string
	action string
	source Source

	// stage resolves the target into the sends it implies. It runs while the
	// write lock is held and must not perform I/O. Returning found=false
	// means no such target.
	stage func() (label string, sends []store.StagedSend, found bool)

	// afterApply runs under the write lock once the send results have been
	// folded in and before the save, for bookkeeping that belongs in the same
	// transaction as the state change. It is handed the result so that
	// bookkeeping which only counts on success — a schedule's "last fired" —
	// can tell the difference. Optional.
	afterApply func(Result)

	// flushLights drains the smart-light queue after the lock is released.
	// Only staging that queues light changes (scenes) needs it.
	flushLights bool

	// notify builds the single summary push sent when nothing failed to
	// persist. Returning "" skips it. Optional.
	notify func(label string, ok int) string
}

// staged executes one staged action. err is only non-nil when persisting
// failed — a device that refused is in Result.Failures, not here.
// Caller must NOT hold the lock.
func (a *Actions) staged(s staged) (Result, error) {
	st := a.cfg.Store

	st.Mu.Lock()
	label, sends, found := s.stage()
	st.Mu.Unlock()
	if !found {
		return Result{}, nil
	}

	st.SendStaged(sends)

	st.Mu.Lock()
	// Per-socket notifications are suppressed so the one summary below is the
	// only push a multi-device action produces.
	st.SuppressStateChange = true
	_ = st.ApplyStaged(sends)
	st.SuppressStateChange = false
	res := Result{Label: label, Found: true}
	res.OK, res.Failures = tally(sends)
	if s.afterApply != nil {
		s.afterApply(res)
	}
	entry := store.ActivityEntry{
		Kind: s.kind, Source: string(s.source), Action: s.action, Label: label,
	}
	if len(res.Failures) > 0 {
		entry.Status = "error"
		// The count and one of the reasons. A household looking at "2 of 9
		// failed" can see something is wrong; "no route to device" is what
		// tells them which kind of wrong, and a schedule pointed at a group
		// that has since been deleted says exactly that here.
		entry.Error = fmt.Sprintf("%d of %d failed: %s",
			len(res.Failures), res.OK+len(res.Failures), res.Failures[0]["error"])
	}
	st.Activity.Add(entry)
	err := st.Save()
	st.Mu.Unlock()

	if s.flushLights {
		st.FlushLights()
		// And the speakers, on the same terms — a scene's music is queued
		// while it is staged (store.QueueMusic).
		st.FlushMusic()
	}
	if err != nil {
		return res, err
	}
	if s.notify != nil && a.cfg.Notify != nil {
		if msg := s.notify(label, res.OK); msg != "" {
			a.cfg.Notify(msg, res.OK)
		}
	}
	return res, nil
}

// tally splits staged results into a success count and the per-socket failure
// list every bulk response shares.
func tally(sends []store.StagedSend) (ok int, failures []map[string]string) {
	failures = make([]map[string]string, 0)
	for _, c := range sends {
		if c.Err != nil {
			failures = append(failures, map[string]string{
				"socket_id": c.SocketID,
				"error":     c.Err.Error(),
			})
			continue
		}
		ok++
	}
	return ok, failures
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
