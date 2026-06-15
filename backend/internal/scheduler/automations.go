package scheduler

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"rf-socket-controller/internal/push"
	"rf-socket-controller/internal/store"
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
//
// primed guards against firing device triggers on the very first tick (before
// we have a baseline snapshot), which would spuriously fire for every socket
// already in the wanted state at startup.
type autoEngine struct {
	lastFired  map[string]string
	sensorEdge map[string]bool
	prevSocket map[string]bool
	primed     bool
}

func newAutoEngine() *autoEngine {
	return &autoEngine{
		lastFired:  make(map[string]string),
		sensorEdge: make(map[string]bool),
		prevSocket: make(map[string]bool),
	}
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
	st.Mu.RLock()
	automations := make([]store.Automation, 0, len(st.Automations))
	for _, a := range st.Automations {
		automations = append(automations, *a)
	}
	curSocket := make(map[string]bool, len(st.Sockets))
	for id, s := range st.Sockets {
		curSocket[id] = s.State
	}
	sensorVal := make(map[string]float64)
	for id, s := range st.Sensors {
		if s.LastValue != nil {
			sensorVal[id] = *s.LastValue
		}
	}
	settings := *st.Settings
	st.Mu.RUnlock()

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
			if e.triggerFired(key, rule.Trigger, prev, now, stamp, curSocket, sensorVal, &settings) &&
				e.conditionsHold(rule.Conditions, curSocket, now) {
				due = append(due, dueRule{a: a, ruleIdx: ri})
			}
		}
	}

	// Refresh the socket baseline for next tick's device-trigger edges.
	e.prevSocket = curSocket
	e.primed = true

	for _, d := range due {
		if err := e.execute(st, d.a, d.ruleIdx, now, pushSvc); err != nil {
			log.Printf("automation %s (%s) rule %d failed: %v", d.a.ID, d.a.Name, d.ruleIdx, err)
		}
	}
}

func (e *autoEngine) triggerFired(
	key string, t store.AutomationTrigger, prev, now time.Time, stamp string,
	curSocket map[string]bool, sensorVal map[string]float64, settings *store.Settings,
) bool {
	switch t.Type {
	case "time":
		// Reuse the Schedule solar/fixed time resolution, matched against
		// the (prev, now] window so DST gaps don't swallow the trigger.
		sched := store.Schedule{TimeMode: t.TimeMode, Time: t.Time, SolarOffsetMinutes: t.SolarOffsetMinutes}
		eff, ok := sched.EffectiveHHMM(now, settings)
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
		cur := curSocket[t.SocketID]
		prevState, had := e.prevSocket[t.SocketID]
		want := t.ToState == "on"
		return e.primed && had && prevState != cur && cur == want

	case "sensor":
		v, ok := sensorVal[t.SensorID]
		truth := ok && ((t.Op == "above" && v > t.Value) || (t.Op == "below" && v < t.Value))
		prevTruth := e.sensorEdge[key]
		e.sensorEdge[key] = truth
		return truth && !prevTruth
	}
	return false
}

func (e *autoEngine) conditionsHold(conds []store.AutomationCondition, curSocket map[string]bool, now time.Time) bool {
	nowMin := now.Hour()*60 + now.Minute()
	for _, c := range conds {
		switch c.Type {
		case "device":
			if curSocket[c.SocketID] != (c.State == "on") {
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
