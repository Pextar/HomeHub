package scheduler

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"homehub/internal/push"
	"homehub/internal/store"
)

// autoEngine evaluates automations on every scheduler tick. It keeps the
// small amount of cross-tick state needed for edge detection:
//   - lastFired:  per-automation "YYYY-MM-DD HH:MM" so a time trigger fires
//     at most once per matching minute.
//   - sensorEdge: per-automation truthiness of a sensor trigger last tick, so
//     it fires on the rising edge (crossing) rather than every
//     tick the value stays past the threshold.
//   - prevSocket: last-seen socket states, so a device trigger fires only on
//     the transition into the wanted state.
//   - prevMusic:  the same, for the media rooms any rule watches. Only rooms
//     something could answer for are in it, so a speaker that
//     drops off the network leaves a gap rather than a "stopped".
//
// primed guards against firing device triggers on the very first tick (before
// we have a baseline snapshot), which would spuriously fire for every socket
// already in the wanted state at startup.
type autoEngine struct {
	lastFired  map[string]string
	sensorEdge map[string]bool
	prevSocket map[string]bool
	prevMusic  map[string]bool
	primed     bool
}

func newAutoEngine() *autoEngine {
	return &autoEngine{
		lastFired:  make(map[string]string),
		sensorEdge: make(map[string]bool),
		prevSocket: make(map[string]bool),
		prevMusic:  make(map[string]bool),
	}
}

// snap is everything one tick reads about the house, gathered once and then
// evaluated against every rule. Passed around rather than re-read per rule
// because half of it costs a lock and the other half costs a cache lookup per
// room, and a house with twenty rules would pay both twenty times over.
type snap struct {
	socket map[string]bool
	sensor map[string]float64
	// music is playing-or-not per media room, and holds an entry *only* for
	// a room something could answer for. A missing key means "no reading",
	// which is a different answer from "quiet" and is why this is not a
	// plain bool map read with the zero value.
	music    map[string]bool
	settings *store.Settings
}

// ruleKey identifies one rule within an automation for edge-tracking maps.
func ruleKey(id string, idx int) string { return id + "#" + strconv.Itoa(idx) }

// tick evaluates every rule of every enabled automation against the current
// state and fires those whose trigger edge occurred and whose conditions all
// hold. prev is the previous tick's time, anchoring the (prev, now] window for
// time triggers (see timeWindowMatches).
func (e *autoEngine) tick(st *store.Store, prev, now time.Time, pushSvc *push.Service) {
	stamp := now.Format("2006-01-02 15:04")

	// Snapshot the state we need under a read lock, then evaluate without it.
	var automations []store.Automation
	cur := snap{}
	var settings store.Settings
	st.View(func() {
		automations = make([]store.Automation, 0, len(st.Automations))
		for _, a := range st.Automations {
			automations = append(automations, *a)
		}
		cur.socket = make(map[string]bool, len(st.Sockets))
		for id, s := range st.Sockets {
			cur.socket[id] = s.State
		}
		cur.sensor = make(map[string]float64)
		for id, s := range st.Sensors {
			if s.LastValue != nil {
				cur.sensor[id] = *s.LastValue
			}
		}
		settings = *st.Settings
	})
	cur.settings = &settings

	// The speakers, read *outside* the lock: RoomPlaying goes through the
	// api layer, which takes its own read lock to resolve a zone, and a
	// second RLock from inside View deadlocks the moment a writer is queued.
	// Only rooms some rule actually watches are asked about — a house with no
	// music rules costs nothing here.
	cur.music = readMusic(st, automations)

	// Drop edge-tracking state for rules that no longer exist so the maps
	// don't grow forever on a long-running install. Keys are per rule.
	alive := make(map[string]bool)
	for _, a := range automations {
		for ri := range a.Rules {
			alive[ruleKey(a.ID, ri)] = true
		}
	}
	for id := range e.lastFired {
		if !alive[id] {
			delete(e.lastFired, id)
		}
	}
	for id := range e.sensorEdge {
		if !alive[id] {
			delete(e.sensorEdge, id)
		}
	}
	// prevMusic is keyed by room rather than by rule, and readMusic already
	// builds it from the rules that exist, so it prunes itself.

	type dueRule struct {
		a       store.Automation
		ruleIdx int
	}
	var due []dueRule
	for _, a := range automations {
		if !a.Enabled {
			continue
		}
		for ri := range a.Rules {
			rule := a.Rules[ri]
			key := ruleKey(a.ID, ri)
			if e.triggerFired(key, rule.Trigger, prev, now, stamp, &cur) &&
				e.conditionsHold(rule.Conditions, &cur, now) {
				due = append(due, dueRule{a: a, ruleIdx: ri})
			}
		}
	}

	// Refresh the baselines for next tick's device and music edges.
	e.prevSocket = cur.socket
	e.prevMusic = cur.music
	e.primed = true

	for _, d := range due {
		if err := e.execute(st, d.a, d.ruleIdx, now, pushSvc); err != nil {
			log.Printf("automation %s (%s) rule %d failed: %v", d.a.ID, d.a.Name, d.ruleIdx, err)
		}
	}
}

// readMusic asks the store what every media room named by a rule is doing.
// Rooms are collected first and asked about once each, so two rules watching
// the living room cost one lookup. Rooms nothing can answer for are left out
// of the map entirely — see snap.music. Caller must NOT hold Mu.
func readMusic(st *store.Store, automations []store.Automation) map[string]bool {
	var want map[string]struct{}
	add := func(room string) {
		if room == "" {
			return
		}
		if want == nil {
			want = make(map[string]struct{}, 4)
		}
		want[room] = struct{}{}
	}
	for _, a := range automations {
		if !a.Enabled {
			continue
		}
		for _, r := range a.Rules {
			if r.Trigger.Type == "music" {
				add(r.Trigger.Room)
			}
			for _, c := range r.Conditions {
				if c.Type == "music" {
					add(c.Room)
				}
			}
		}
	}
	if len(want) == 0 {
		return nil
	}
	out := make(map[string]bool, len(want))
	for room := range want {
		if playing, known := st.RoomPlaying(room); known {
			out[room] = playing
		}
	}
	return out
}

func (e *autoEngine) triggerFired(
	key string, t store.AutomationTrigger, prev, now time.Time, stamp string, cur *snap,
) bool {
	switch t.Type {
	case "time":
		// Reuse the Schedule solar/fixed time resolution, matched against
		// the (prev, now] window so DST gaps don't swallow the trigger.
		sched := store.Schedule{TimeMode: t.TimeMode, Time: t.Time, SolarOffsetMinutes: t.SolarOffsetMinutes}
		eff, ok := sched.EffectiveHHMM(now, cur.settings)
		if !ok || !timeWindowMatches(eff, prev, now) {
			return false
		}
		if !dayMatches(t.Days, fireWeekday(eff, prev, now)) {
			return false
		}
		if e.lastFired[key] == stamp {
			return false
		}
		e.lastFired[key] = stamp
		return true

	case "device":
		state := cur.socket[t.SocketID]
		prevState, had := e.prevSocket[t.SocketID]
		want := t.ToState == "on"
		return e.primed && had && prevState != state && state == want

	case "music":
		// Same edge as a device, with one extra guard: a room only counts as
		// having changed when *both* readings exist. A speaker that dropped
		// off the network and came back quiet has not stopped playing while
		// we were watching, and a rule that dimmed the bedroom every time
		// the Wi-Fi hiccuped is the failure this prevents.
		state, known := cur.music[t.Room]
		prevState, had := e.prevMusic[t.Room]
		want := t.ToState == store.MusicPlaying
		return e.primed && known && had && prevState != state && state == want

	case "sensor":
		v, ok := cur.sensor[t.SensorID]
		truth := ok && ((t.Op == "above" && v > t.Value) || (t.Op == "below" && v < t.Value))
		prevTruth := e.sensorEdge[key]
		e.sensorEdge[key] = truth
		return truth && !prevTruth
	}
	return false
}

func (e *autoEngine) conditionsHold(conds []store.AutomationCondition, cur *snap, now time.Time) bool {
	nowMin := now.Hour()*60 + now.Minute()
	for _, c := range conds {
		switch c.Type {
		case "device":
			if cur.socket[c.SocketID] != (c.State == "on") {
				return false
			}
		case "music":
			// Fails closed on a room nothing can answer for. A gate that
			// opens because we couldn't read the speaker is a gate that
			// isn't one.
			state, known := cur.music[c.Room]
			if !known || state != (c.State == store.MusicPlaying) {
				return false
			}
		case "time_range":
			after := hhmmToMin(c.After)
			before := hhmmToMin(c.Before)
			if after < 0 || before < 0 {
				return false
			}
			var inRange bool
			if after <= before {
				inRange = nowMin >= after && nowMin <= before
			} else { // window wraps past midnight
				inRange = nowMin >= after || nowMin <= before
			}
			if !inRange {
				return false
			}
		case "time_before":
			before := hhmmToMin(c.Before)
			if before < 0 || nowMin >= before {
				return false
			}
		case "time_after":
			after := hhmmToMin(c.After)
			if after < 0 || nowMin < after {
				return false
			}
		}
	}
	return true
}

func (e *autoEngine) execute(st *store.Store, a store.Automation, ruleIdx int, now time.Time, pushSvc *push.Service) error {
	actions := a.Rules[ruleIdx].Actions
	// Stage under the lock (this also queues smart-light brightness/colour),
	// transmit off-lock, then fold the results back in — a slow device can't
	// stall the scheduler tick or the API.
	st.Mu.Lock()
	staged := st.StageAutomationActions(actions)
	st.QueueMusic(a.Rules[ruleIdx].Music)
	st.Mu.Unlock()

	st.SendStaged(staged)

	st.Mu.Lock()
	st.SuppressStateChange = true
	firstErr := st.ApplyStaged(staged)
	st.SuppressStateChange = false

	kind := "bulk"
	if len(actions) == 1 {
		kind = actions[0].TargetType
	}
	entry := store.ActivityEntry{Kind: kind, Source: "automation", Action: "run", Label: a.Name}
	if firstErr != nil {
		entry.Status = "error"
		entry.Error = firstErr.Error()
	}
	st.Activity.Add(entry)

	if existing, ok := st.Automations[a.ID]; ok {
		existing.LastFiredAt = now.UTC()
		existing.RunCount++
	}
	if err := st.Save(); err != nil && firstErr == nil {
		firstErr = err
	}
	st.Mu.Unlock()
	st.FlushLights() // off-lock bridge calls for scene brightness/colour
	st.FlushMusic()  // and the speakers, on the same terms

	if firstErr == nil {
		log.Printf("automation fired: %s (%s)", a.Name, a.ID)
		if pushSvc != nil {
			go pushSvc.NotifyEvent(push.CategoryScheduleFired, "", push.PushPayload{
				Title: fmt.Sprintf("⚙️ Automation: %s", a.Name),
				URL:   "/#/automations",
				Tag:   "automation-" + a.ID,
			})
		}
	}
	return firstErr
}

func hhmmToMin(s string) int {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return -1
	}
	return t.Hour()*60 + t.Minute()
}
