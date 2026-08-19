package control

import (
	"time"

	"homehub/internal/store"
)

// This file holds the two entry points the house uses on its own: a target
// that was chosen when a schedule or a timer was created, and an automation
// rule's list of actions. They exist here rather than in the scheduler for the
// reason the package doc gives — the staged flow is the rule that must not be
// written twice — and because the "Run" button in the automation editor and
// the trigger that fires the same rule at 07:00 have to be one code path. They
// were two, once, and only one of them saved the sockets it had switched.

// Run is one target firing: what to act on, and the bookkeeping that belongs
// in the same transaction as the state change.
//
// TargetType is "socket", "group", "room" or "scene" and TargetID names one;
// Action is "on", "off", "toggle" or, for a scene, "activate". A target that
// has been deleted since the schedule was written is not an error the caller
// has to pre-empt: it comes back as a failed send, so the run is still
// recorded and the household can see why nothing happened.
type Run struct {
	TargetType string
	TargetID   string
	Action     string
	Source     Source

	// Before runs under the write lock immediately before the target is
	// resolved, for state the run consumes: a one-shot timer removes itself
	// here, so that a device refusing cannot leave it to fire again on the
	// next tick. Optional.
	Before func()

	// After runs under the write lock once the results have been folded in
	// and before the save, for "last fired" bookkeeping. Optional.
	After func(Result)
}

// Target applies an action to one target named by type and id.
//
// This is what a due schedule or a one-shot timer needs: the target was picked
// when the schedule was written, so what arrives here is a type and an id
// rather than a particular kind of thing. Caller must NOT hold the lock.
func (a *Actions) Target(r Run) (Result, error) {
	st := a.cfg.Store
	return a.staged(staged{
		kind: r.TargetType, action: r.Action, source: r.Source,
		stage: func() (string, []store.StagedSend, bool) {
			if r.Before != nil {
				r.Before()
			}
			label := targetLabel(st, r.TargetType, r.TargetID)
			sends, err := st.StageAction(r.TargetType, r.TargetID, r.Action)
			if err != nil {
				// Carried as a failed send rather than a "not found": the
				// household asked for this at some point and deserves the
				// activity entry saying it could not be done.
				return label, []store.StagedSend{{Err: err}}, true
			}
			return label, sends, true
		},
		afterApply: r.After,
		// A scene target queues brightness, colour and its own music while it
		// stages; every other target queues nothing and drains an empty
		// buffer, which costs nothing.
		flushLights: true,
	})
}

// AutomationRun is one automation firing: a whole automation's actions or a
// single rule's, plus the music that goes with them.
type AutomationRun struct {
	// ID is what the run is recorded against; Name is what the activity log
	// shows.
	ID   string
	Name string

	// Actions and Music are the rule's, already read out of the store by the
	// caller — every rule's when a person presses "Run" on the automation,
	// one rule's when its trigger matches or its own "Run" is pressed.
	Actions []store.AutomationAction
	Music   []store.MusicAction

	Source Source

	// Recorded runs under the write lock once the run has been folded in and
	// the automation's counters updated, with the automation as it then
	// stands — nil when it was deleted while the sends were in flight. For a
	// caller that has to answer with the new state. Do not retain the
	// pointer: it belongs to the store and is only safe inside the call.
	// Optional.
	Recorded func(*store.Automation, Result)
}

// Automation runs a set of automation actions as one unit and records the run
// against the automation.
//
// LastFiredAt and RunCount are updated whether or not every device took the
// command, because the automation did run — a lamp that was unplugged is in
// the result, not in the question of whether the rule fired. Caller must NOT
// hold the lock.
func (a *Actions) Automation(r AutomationRun) (Result, error) {
	st := a.cfg.Store
	kind := "bulk"
	if len(r.Actions) == 1 {
		kind = r.Actions[0].TargetType
	}
	return a.staged(staged{
		kind: kind, action: "run", source: r.Source,
		stage: func() (string, []store.StagedSend, bool) {
			sends := st.StageAutomationActions(r.Actions)
			// The rule's own music, queued beside whatever a scene among its
			// targets may have added; both come out at the flush below.
			st.QueueMusic(r.Music)
			// Always found: the caller resolved the automation before calling,
			// and an automation with no actions is an empty run, not a 404.
			return r.Name, sends, true
		},
		afterApply: func(res Result) {
			// Re-fetched rather than captured: the automation may have been
			// deleted while the sends were in flight.
			cur := st.Automations[r.ID]
			if cur != nil {
				cur.LastFiredAt = time.Now().UTC()
				cur.RunCount++
			}
			if r.Recorded != nil {
				r.Recorded(cur, res)
			}
		},
		flushLights: true,
	})
}

// targetLabel is what a target is called, for the activity log and the push.
// Falls back to the id: a schedule pointing at something deleted should still
// say which thing it was. Caller must hold the lock.
func targetLabel(st *store.Store, kind, id string) string {
	switch kind {
	case "socket":
		if v, ok := st.Sockets[id]; ok {
			return v.Name
		}
	case "group":
		if v, ok := st.Groups[id]; ok {
			return v.Name
		}
	case "room":
		if v, ok := st.Rooms[id]; ok {
			return v.Name
		}
	case "scene":
		if v, ok := st.Scenes[id]; ok {
			return v.Name
		}
	}
	return id
}
