package scheduler

import (
	"errors"
	"sync"
	"testing"

	"homehub/internal/control"
	"homehub/internal/store"
)

// These cover the seam between deciding *when* and doing it: the scheduler
// hands a due schedule or timer to internal/control and reads the answer back.
// Everything about the staged flow itself is control's own test.

type fakeRF struct {
	mu     sync.Mutex
	sends  []string
	refuse bool
}

func (f *fakeRF) Send(code, protocol string, state bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse {
		return errors.New("no route to device")
	}
	f.sends = append(f.sends, code)
	return nil
}

func (f *fakeRF) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends)
}

func testConfig(t *testing.T) (Config, *store.Store, *fakeRF) {
	t.Helper()
	rf := &fakeRF{}
	st := store.New(t.TempDir(), rf)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	st.Sockets["s1"] = &store.Socket{ID: "s1", Name: "Porch", Code: "1001", Protocol: "nexa"}
	return Config{Store: st, Control: control.New(control.Config{Store: st})}, st, rf
}

// The whole point of the wiring: a schedule coming due switches a real device,
// through the same action layer a tap in the app uses.
func TestScheduleFireReachesTheDevice(t *testing.T) {
	cfg, st, rf := testConfig(t)
	st.Schedules["sch1"] = &store.Schedule{ID: "sch1", TargetType: "socket", TargetID: "s1", Action: "on", Enabled: true}

	if err := executeSchedule(cfg, *st.Schedules["sch1"]); err != nil {
		t.Fatalf("executeSchedule = %v", err)
	}
	if rf.count() != 1 {
		t.Errorf("%d transmissions, want 1", rf.count())
	}
	var on, fired bool
	st.View(func() {
		on = st.Sockets["s1"].State
		fired = !st.Schedules["sch1"].LastFiredAt.IsZero()
	})
	if !on {
		t.Error("the socket is still off after its schedule fired")
	}
	if !fired {
		t.Error("LastFiredAt was not stamped")
	}
}

// A schedule that could not reach its device has not fired. Stamping it anyway
// would make the log say the porch light came on when the porch is dark.
func TestScheduleThatFailedIsNotStampedAsFired(t *testing.T) {
	cfg, st, rf := testConfig(t)
	rf.refuse = true
	st.Schedules["sch1"] = &store.Schedule{ID: "sch1", TargetType: "socket", TargetID: "s1", Action: "on", Enabled: true}

	if err := executeSchedule(cfg, *st.Schedules["sch1"]); err == nil {
		t.Fatal("a refusing device reported success")
	}
	var fired bool
	st.View(func() { fired = !st.Schedules["sch1"].LastFiredAt.IsZero() })
	if fired {
		t.Error("LastFiredAt was stamped for a schedule that never reached anything")
	}
}

// A one-shot timer is spent whether or not the device took the command. It
// must not come back on the next tick, five seconds later, and every tick
// after that for as long as the device stays unreachable.
func TestTimerIsConsumedEvenWhenTheDeviceRefuses(t *testing.T) {
	cfg, st, rf := testConfig(t)
	rf.refuse = true
	timer := store.Timer{ID: "t1", TargetType: "socket", TargetID: "s1", Action: "on"}
	st.Timers["t1"] = &timer

	if err := executeTimer(cfg, timer); err == nil {
		t.Fatal("a refusing device reported success")
	}
	var still bool
	st.View(func() { _, still = st.Timers["t1"] })
	if still {
		t.Error("the timer survived a failed fire and will fire again")
	}
}

// An automation rule fires through control.Automation — the same call the
// "Run" button in its editor makes — and the run is counted.
func TestAutomationRuleFiresThroughControl(t *testing.T) {
	cfg, st, rf := testConfig(t)
	st.Automations["a1"] = &store.Automation{
		ID: "a1", Name: "Evening", Enabled: true,
		Rules: []store.AutomationRule{{
			Actions: []store.AutomationAction{{TargetType: "socket", TargetID: "s1", Action: "on"}},
		}},
	}

	e := newAutoEngine()
	if err := e.execute(cfg, *st.Automations["a1"], 0); err != nil {
		t.Fatalf("execute = %v", err)
	}
	if rf.count() != 1 {
		t.Errorf("%d transmissions, want 1", rf.count())
	}
	var runs int
	st.View(func() { runs = st.Automations["a1"].RunCount })
	if runs != 1 {
		t.Errorf("RunCount = %d after one run, want 1", runs)
	}
}
