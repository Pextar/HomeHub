package control

import (
	"errors"
	"testing"

	"homehub/internal/store"
)

// refusingRF is the speaker that is unplugged, the socket behind the sofa, the
// 433 MHz receiver out of range: every send fails.
type refusingRF struct{}

func (r *refusingRF) Send(code, protocol string, state bool) error {
	return errors.New("no route to device")
}

func refusingActions(t *testing.T) (*Actions, *store.Store) {
	t.Helper()
	st := store.New(t.TempDir(), &refusingRF{})
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	st.Sockets["s1"] = &store.Socket{ID: "s1", Name: "Lamp", Code: "1001", Protocol: "nexa", Room: "Lounge"}
	return New(Config{Store: st}), st
}

// ── Target: what a schedule or a timer fires ─────────────────────────────

func TestTargetSwitchesAndLabelsFromTheStore(t *testing.T) {
	a, _, rf := testActions(t)

	res, err := a.Target(Run{TargetType: "socket", TargetID: "s1", Action: "on", Source: SourceSchedule})
	if err != nil {
		t.Fatalf("Target = %v", err)
	}
	if res.Label != "Lamp" {
		t.Errorf("label = %q, want the socket's name", res.Label)
	}
	if res.OK != 1 || rf.count() != 1 {
		t.Errorf("OK = %d after %d sends, want one of each", res.OK, rf.count())
	}
}

// A schedule outlives the thing it points at. When the group is gone the run
// still has to be recorded — a household looking for why the lights did not
// come on needs to find the answer in the log, not an absence.
func TestTargetRecordsARunAtSomethingDeleted(t *testing.T) {
	a, st := refusingActions(t)

	res, err := a.Target(Run{TargetType: "group", TargetID: "gone", Action: "on", Source: SourceSchedule})
	if err != nil {
		t.Fatalf("Target = %v", err)
	}
	if res.Err() == nil {
		t.Error("a target that no longer exists reported success")
	}
	entries := st.Activity.Recent(1)
	if len(entries) != 1 {
		t.Fatalf("%d activity entries, want the run to have been recorded", len(entries))
	}
	if entries[0].Status != "error" || entries[0].Source != string(SourceSchedule) {
		t.Errorf("entry = %+v, want an errored schedule entry", entries[0])
	}
}

// A one-shot timer removes itself before the send, not after it. A device that
// refuses must not leave the timer to fire again on the next tick, five
// seconds later, and again after that.
func TestTargetRunsBeforeEvenWhenTheDeviceRefuses(t *testing.T) {
	a, st := refusingActions(t)
	st.Timers["t1"] = &store.Timer{ID: "t1", TargetType: "socket", TargetID: "s1", Action: "on"}

	res, err := a.Target(Run{
		TargetType: "socket", TargetID: "s1", Action: "on", Source: SourceTimer,
		Before: func() { delete(st.Timers, "t1") },
	})
	if err != nil {
		t.Fatalf("Target = %v", err)
	}
	if res.Err() == nil {
		t.Fatal("a refusing device reported success")
	}
	var still bool
	st.View(func() { _, still = st.Timers["t1"] })
	if still {
		t.Error("the timer survived a failed fire and will fire again")
	}
}

// After lands in the same transaction as the state change, so "last fired" and
// what was switched are persisted together or not at all.
func TestTargetAfterSeesTheResult(t *testing.T) {
	a, _, _ := testActions(t)
	var got Result
	if _, err := a.Target(Run{
		TargetType: "socket", TargetID: "s1", Action: "on", Source: SourceSchedule,
		After: func(r Result) { got = r },
	}); err != nil {
		t.Fatalf("Target = %v", err)
	}
	if got.Label != "Lamp" || got.OK != 1 {
		t.Errorf("After saw %+v, want the finished result", got)
	}
}

// ── Automation: the trigger and the "Run" button ─────────────────────────

func TestAutomationRunsEveryActionAndRecordsTheRun(t *testing.T) {
	a, st, rf := testActions(t)
	st.Automations["a1"] = &store.Automation{ID: "a1", Name: "Evening", Enabled: true}

	res, err := a.Automation(AutomationRun{
		ID: "a1", Name: "Evening", Source: SourceAutomation,
		Actions: []store.AutomationAction{
			{TargetType: "socket", TargetID: "s1", Action: "on"},
			{TargetType: "socket", TargetID: "s3", Action: "on"},
		},
	})
	if err != nil {
		t.Fatalf("Automation = %v", err)
	}
	if res.OK != 2 || rf.count() != 2 {
		t.Errorf("OK = %d after %d sends, want two of each", res.OK, rf.count())
	}
	var runs int
	st.View(func() { runs = st.Automations["a1"].RunCount })
	if runs != 1 {
		t.Errorf("RunCount = %d after one run, want 1", runs)
	}
}

// The automation ran. A lamp that was unplugged is in the result, not in the
// question of whether the rule fired — otherwise a rule with one dead device
// would look, in the UI, like a rule that never fires.
func TestAutomationCountsARunThatADeviceRefused(t *testing.T) {
	a, st := refusingActions(t)
	st.Automations["a1"] = &store.Automation{ID: "a1", Name: "Evening", Enabled: true}

	res, err := a.Automation(AutomationRun{
		ID: "a1", Name: "Evening", Source: SourceAutomation,
		Actions: []store.AutomationAction{{TargetType: "socket", TargetID: "s1", Action: "on"}},
	})
	if err != nil {
		t.Fatalf("Automation = %v", err)
	}
	if res.Err() == nil {
		t.Error("a refusing device reported success")
	}
	var runs int
	var fired bool
	st.View(func() {
		runs = st.Automations["a1"].RunCount
		fired = !st.Automations["a1"].LastFiredAt.IsZero()
	})
	if runs != 1 || !fired {
		t.Errorf("RunCount = %d, fired = %v; a run with a failure is still a run", runs, fired)
	}
}

// Recorded runs inside the transaction that updated the counters, which is
// what lets an HTTP caller answer with the count this run produced rather than
// re-reading and racing another request.
func TestAutomationRecordedSeesTheUpdatedCounters(t *testing.T) {
	a, st, _ := testActions(t)
	st.Automations["a1"] = &store.Automation{ID: "a1", Name: "Evening", Enabled: true}

	var seen int
	if _, err := a.Automation(AutomationRun{
		ID: "a1", Name: "Evening", Source: SourceAutomation,
		Actions:  []store.AutomationAction{{TargetType: "socket", TargetID: "s1", Action: "on"}},
		Recorded: func(cur *store.Automation, _ Result) { seen = cur.RunCount },
	}); err != nil {
		t.Fatalf("Automation = %v", err)
	}
	if seen != 1 {
		t.Errorf("Recorded saw RunCount %d, want the run it was called for", seen)
	}
}

// An automation deleted while its sends were in flight leaves nothing to
// record against, and the caller is told so rather than handed a zero value.
func TestAutomationRecordedIsNilWhenItWasDeleted(t *testing.T) {
	a, _, _ := testActions(t)

	called, gotNil := false, false
	if _, err := a.Automation(AutomationRun{
		ID: "gone", Name: "Evening", Source: SourceAutomation,
		Actions:  []store.AutomationAction{{TargetType: "socket", TargetID: "s1", Action: "on"}},
		Recorded: func(cur *store.Automation, _ Result) { called, gotNil = true, cur == nil },
	}); err != nil {
		t.Fatalf("Automation = %v", err)
	}
	if !called || !gotNil {
		t.Errorf("Recorded called = %v with nil automation = %v, want both", called, gotNil)
	}
}
