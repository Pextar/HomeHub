package scheduler

import (
	"testing"
	"time"

	"homehub/internal/store"
)

func deviceTrigger(socketID, to string) store.AutomationTrigger {
	return store.AutomationTrigger{Type: "device", SocketID: socketID, ToState: to}
}

// sockets/sensors/music build the one-field snapshots most of these tests
// want, so a test says what it is about rather than spelling out three empty
// maps beside the one it cares about.
func sockets(m map[string]bool) *snap {
	return &snap{socket: m, settings: &store.Settings{}}
}
func sensors(m map[string]float64) *snap {
	return &snap{sensor: m, settings: &store.Settings{}}
}
func music(m map[string]bool) *snap {
	return &snap{music: m, settings: &store.Settings{}}
}

func TestDeviceTriggerFiresOnEdgeOnly(t *testing.T) {
	e := newAutoEngine()
	tr := deviceTrigger("s1", "on")
	now := time.Now()

	// First tick: not primed yet — must not fire even if already on.
	if e.triggerFired("a1#0", tr, now.Add(-5*time.Second), now, "", sockets(map[string]bool{"s1": true})) {
		t.Fatal("device trigger fired before engine was primed")
	}
	e.prevSocket = map[string]bool{"s1": false}
	e.primed = true

	// Transition off -> on fires.
	if !e.triggerFired("a1#0", tr, now.Add(-5*time.Second), now, "", sockets(map[string]bool{"s1": true})) {
		t.Fatal("device trigger did not fire on off->on edge")
	}
	// Staying on does not fire again (prevSocket updated by tick(), simulate it).
	e.prevSocket = map[string]bool{"s1": true}
	if e.triggerFired("a1#0", tr, now.Add(-5*time.Second), now, "", sockets(map[string]bool{"s1": true})) {
		t.Fatal("device trigger fired while state held on")
	}
}

func TestSensorTriggerFiresOnRisingEdge(t *testing.T) {
	e := newAutoEngine()
	tr := store.AutomationTrigger{Type: "sensor", SensorID: "temp", Op: "above", Value: 25}
	now := time.Now()

	// Below threshold: no fire.
	if e.triggerFired("a2#0", tr, now.Add(-5*time.Second), now, "", sensors(map[string]float64{"temp": 20})) {
		t.Fatal("sensor trigger fired below threshold")
	}
	// Crossing above: fires once.
	if !e.triggerFired("a2#0", tr, now.Add(-5*time.Second), now, "", sensors(map[string]float64{"temp": 30})) {
		t.Fatal("sensor trigger did not fire on crossing")
	}
	// Still above: does not re-fire.
	if e.triggerFired("a2#0", tr, now.Add(-5*time.Second), now, "", sensors(map[string]float64{"temp": 31})) {
		t.Fatal("sensor trigger re-fired while held above threshold")
	}
}

func TestTimeTriggerMatchesMinuteOnce(t *testing.T) {
	e := newAutoEngine()
	now := time.Date(2026, 1, 5, 7, 30, 0, 0, time.Local) // a Monday
	stamp := now.Format("2006-01-02 15:04")
	tr := store.AutomationTrigger{Type: "time", TimeMode: "fixed", Time: "07:30"}
	empty := &snap{settings: &store.Settings{}}

	if !e.triggerFired("a3#0", tr, now.Add(-5*time.Second), now, stamp, empty) {
		t.Fatal("time trigger did not fire at matching minute")
	}
	if e.triggerFired("a3#0", tr, now.Add(-5*time.Second), now, stamp, empty) {
		t.Fatal("time trigger fired twice in the same minute")
	}
}

// The living room going quiet is the case this whole trigger exists for:
// "when the music or the TV stops in the living room, take the bedroom lamp
// down to 2%".
func TestMusicTriggerFiresOnStopEdge(t *testing.T) {
	e := newAutoEngine()
	tr := store.AutomationTrigger{Type: "music", Room: "kef:living", ToState: store.MusicStopped}
	now := time.Now()
	prev := now.Add(-5 * time.Second)

	// Not primed: a house that starts up with a quiet living room has not
	// just watched it go quiet.
	if e.triggerFired("a4#0", tr, prev, now, "", music(map[string]bool{"kef:living": false})) {
		t.Fatal("music trigger fired before the engine was primed")
	}
	e.primed = true
	e.prevMusic = map[string]bool{"kef:living": true}

	if !e.triggerFired("a4#0", tr, prev, now, "", music(map[string]bool{"kef:living": false})) {
		t.Fatal("music trigger did not fire when the room stopped")
	}
	// Still quiet on the next tick: the room stopped once.
	e.prevMusic = map[string]bool{"kef:living": false}
	if e.triggerFired("a4#0", tr, prev, now, "", music(map[string]bool{"kef:living": false})) {
		t.Fatal("music trigger re-fired while the room stayed quiet")
	}
	// And the opposite edge is not this rule's.
	e.prevMusic = map[string]bool{"kef:living": false}
	if e.triggerFired("a4#0", tr, prev, now, "", music(map[string]bool{"kef:living": true})) {
		t.Fatal("a stopped-trigger fired when the room started playing")
	}
}

// A speaker that drops off the network and comes back must not read as a
// room that stopped playing while we were watching.
func TestMusicTriggerIgnoresMissingReadings(t *testing.T) {
	e := newAutoEngine()
	tr := store.AutomationTrigger{Type: "music", Room: "sonos:living", ToState: store.MusicStopped}
	now := time.Now()
	prev := now.Add(-5 * time.Second)
	e.primed = true

	// Was playing, now unreadable: no reading is not a stop.
	e.prevMusic = map[string]bool{"sonos:living": true}
	if e.triggerFired("a5#0", tr, prev, now, "", music(map[string]bool{})) {
		t.Fatal("music trigger fired on a room with no current reading")
	}
	// Unreadable, now quiet: we never saw it playing, so we never saw it stop.
	e.prevMusic = map[string]bool{}
	if e.triggerFired("a5#0", tr, prev, now, "", music(map[string]bool{"sonos:living": false})) {
		t.Fatal("music trigger fired without a previous reading to change from")
	}
}

func TestConditionsHold(t *testing.T) {
	e := newAutoEngine()
	now := time.Date(2026, 1, 5, 20, 0, 0, 0, time.Local)

	// Device condition.
	devCond := []store.AutomationCondition{{Type: "device", SocketID: "s1", State: "on"}}
	if !e.conditionsHold(devCond, sockets(map[string]bool{"s1": true}), now) {
		t.Fatal("device condition should hold when socket is on")
	}
	if e.conditionsHold(devCond, sockets(map[string]bool{"s1": false}), now) {
		t.Fatal("device condition should fail when socket is off")
	}

	// Time range that wraps past midnight (22:00–07:00) — 20:00 is outside.
	wrap := []store.AutomationCondition{{Type: "time_range", After: "22:00", Before: "07:00"}}
	if e.conditionsHold(wrap, &snap{}, now) {
		t.Fatal("20:00 should be outside a 22:00–07:00 window")
	}
	// 23:30 is inside the wrapping window.
	if !e.conditionsHold(wrap, &snap{}, time.Date(2026, 1, 5, 23, 30, 0, 0, time.Local)) {
		t.Fatal("23:30 should be inside a 22:00–07:00 window")
	}
}

func TestMusicConditionFailsClosedWithoutAReading(t *testing.T) {
	e := newAutoEngine()
	now := time.Date(2026, 1, 5, 20, 0, 0, 0, time.Local)
	cond := []store.AutomationCondition{
		{Type: "music", Room: "zone:downstairs", State: store.MusicStopped},
	}

	if !e.conditionsHold(cond, music(map[string]bool{"zone:downstairs": false}), now) {
		t.Fatal("a quiet room should satisfy a 'stopped' condition")
	}
	if e.conditionsHold(cond, music(map[string]bool{"zone:downstairs": true}), now) {
		t.Fatal("a playing room should fail a 'stopped' condition")
	}
	// No reading at all: the gate stays shut rather than opening on a guess.
	if e.conditionsHold(cond, music(map[string]bool{}), now) {
		t.Fatal("a room with no reading should fail the condition")
	}
}

// readMusic asks about every room a rule watches, once each, and skips rooms
// nothing can answer for.
func TestReadMusicCollectsWatchedRooms(t *testing.T) {
	asked := map[string]int{}
	st := &store.Store{
		MusicPlaying: func(room string) (bool, bool) {
			asked[room]++
			switch room {
			case "kef:living":
				return true, true
			case "zone:downstairs":
				return false, true
			}
			return false, false // "sonos:gone" — nothing can answer
		},
	}
	autos := []store.Automation{
		{
			Enabled: true,
			Rules: []store.AutomationRule{
				{
					Trigger: store.AutomationTrigger{Type: "music", Room: "kef:living", ToState: store.MusicStopped},
					Conditions: []store.AutomationCondition{
						{Type: "music", Room: "zone:downstairs", State: store.MusicStopped},
						{Type: "device", SocketID: "s1", State: "on"},
					},
				},
				// Same room again: still one lookup.
				{Trigger: store.AutomationTrigger{Type: "music", Room: "kef:living", ToState: store.MusicPlaying}},
				{Trigger: store.AutomationTrigger{Type: "music", Room: "sonos:gone", ToState: store.MusicStopped}},
				{Trigger: store.AutomationTrigger{Type: "time", Time: "07:00"}},
			},
		},
		// A disabled automation's rooms are never asked about.
		{
			Rules: []store.AutomationRule{
				{Trigger: store.AutomationTrigger{Type: "music", Room: "kef:kitchen", ToState: store.MusicStopped}},
			},
		},
	}

	got := readMusic(st, autos)
	if len(got) != 2 {
		t.Fatalf("expected readings for 2 rooms, got %v", got)
	}
	if !got["kef:living"] {
		t.Error("living room should read as playing")
	}
	if playing, known := got["zone:downstairs"]; !known || playing {
		t.Error("downstairs zone should read as known and quiet")
	}
	if _, known := got["sonos:gone"]; known {
		t.Error("a room nothing can answer for must not be in the map")
	}
	if asked["kef:living"] != 1 {
		t.Errorf("living room asked %d times, want 1", asked["kef:living"])
	}
	if asked["kef:kitchen"] != 0 {
		t.Error("a disabled automation's room was asked about")
	}
}

func TestReadMusicSkipsHouseWithNoMusicRules(t *testing.T) {
	st := &store.Store{MusicPlaying: func(string) (bool, bool) {
		t.Fatal("no rule watches a room; nothing should have been asked")
		return false, false
	}}
	autos := []store.Automation{{
		Enabled: true,
		Rules:   []store.AutomationRule{{Trigger: deviceTrigger("s1", "on")}},
	}}
	if got := readMusic(st, autos); got != nil {
		t.Errorf("expected no readings, got %v", got)
	}
}
