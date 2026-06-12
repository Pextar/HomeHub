package scheduler

import (
	"testing"
	"time"

	"rf-socket-controller/internal/store"
)

func deviceTrigger(socketID, to string) store.Automation {
	return store.Automation{
		ID:      "a1",
		Trigger: store.AutomationTrigger{Type: "device", SocketID: socketID, ToState: to},
	}
}

func TestDeviceTriggerFiresOnEdgeOnly(t *testing.T) {
	e := newAutoEngine()
	a := deviceTrigger("s1", "on")
	now := time.Now()
	settings := &store.Settings{}

	// First tick: not primed yet — must not fire even if already on.
	if e.triggerFired(a, now.Add(-5*time.Second), now, "", map[string]bool{"s1": true}, nil, settings) {
		t.Fatal("device trigger fired before engine was primed")
	}
	e.prevSocket = map[string]bool{"s1": false}
	e.primed = true

	// Transition off -> on fires.
	if !e.triggerFired(a, now.Add(-5*time.Second), now, "", map[string]bool{"s1": true}, nil, settings) {
		t.Fatal("device trigger did not fire on off->on edge")
	}
	// Staying on does not fire again (prevSocket updated by tick(), simulate it).
	e.prevSocket = map[string]bool{"s1": true}
	if e.triggerFired(a, now.Add(-5*time.Second), now, "", map[string]bool{"s1": true}, nil, settings) {
		t.Fatal("device trigger fired while state held on")
	}
}

func TestSensorTriggerFiresOnRisingEdge(t *testing.T) {
	e := newAutoEngine()
	a := store.Automation{
		ID:      "a2",
		Trigger: store.AutomationTrigger{Type: "sensor", SensorID: "temp", Op: "above", Value: 25},
	}
	now := time.Now()
	settings := &store.Settings{}

	// Below threshold: no fire.
	if e.triggerFired(a, now.Add(-5*time.Second), now, "", nil, map[string]float64{"temp": 20}, settings) {
		t.Fatal("sensor trigger fired below threshold")
	}
	// Crossing above: fires once.
	if !e.triggerFired(a, now.Add(-5*time.Second), now, "", nil, map[string]float64{"temp": 30}, settings) {
		t.Fatal("sensor trigger did not fire on crossing")
	}
	// Still above: does not re-fire.
	if e.triggerFired(a, now.Add(-5*time.Second), now, "", nil, map[string]float64{"temp": 31}, settings) {
		t.Fatal("sensor trigger re-fired while held above threshold")
	}
}

func TestTimeTriggerMatchesMinuteOnce(t *testing.T) {
	e := newAutoEngine()
	now := time.Date(2026, 1, 5, 7, 30, 0, 0, time.Local) // a Monday
	stamp := now.Format("2006-01-02 15:04")
	a := store.Automation{
		ID:      "a3",
		Trigger: store.AutomationTrigger{Type: "time", TimeMode: "fixed", Time: "07:30"},
	}
	settings := &store.Settings{}

	if !e.triggerFired(a, now.Add(-5*time.Second), now, stamp, nil, nil, settings) {
		t.Fatal("time trigger did not fire at matching minute")
	}
	if e.triggerFired(a, now.Add(-5*time.Second), now, stamp, nil, nil, settings) {
		t.Fatal("time trigger fired twice in the same minute")
	}
}

func TestConditionsHold(t *testing.T) {
	e := newAutoEngine()
	now := time.Date(2026, 1, 5, 20, 0, 0, 0, time.Local)

	// Device condition.
	devCond := []store.AutomationCondition{{Type: "device", SocketID: "s1", State: "on"}}
	if !e.conditionsHold(devCond, map[string]bool{"s1": true}, now) {
		t.Fatal("device condition should hold when socket is on")
	}
	if e.conditionsHold(devCond, map[string]bool{"s1": false}, now) {
		t.Fatal("device condition should fail when socket is off")
	}

	// Time range that wraps past midnight (22:00–07:00) — 20:00 is outside.
	wrap := []store.AutomationCondition{{Type: "time_range", After: "22:00", Before: "07:00"}}
	if e.conditionsHold(wrap, nil, now) {
		t.Fatal("20:00 should be outside a 22:00–07:00 window")
	}
	// 23:30 is inside the wrapping window.
	if !e.conditionsHold(wrap, nil, time.Date(2026, 1, 5, 23, 30, 0, 0, time.Local)) {
		t.Fatal("23:30 should be inside a 22:00–07:00 window")
	}
}
