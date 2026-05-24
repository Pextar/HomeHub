// Package scheduler ticks every 5 seconds, fires due one-shot timers,
// and (per-minute) any enabled schedules whose HH:MM + weekday match
// the current local time. It owns no state of its own — everything
// runs against an injected *store.Store.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"rf-socket-controller/internal/push"
	"rf-socket-controller/internal/store"
)

// pendingFire holds a randomly-delayed fire time for a schedule that has
// random_offset_minutes set. enqueued is when the base time matched, used
// to expire stale entries.
type pendingFire struct {
	fireAt   time.Time
	enqueued time.Time
}

// Run blocks until ctx is cancelled. Spawn it in a goroutine.
// pushSvc is optional — pass nil to disable push notifications from the scheduler.
func Run(ctx context.Context, st *store.Store, pushSvc *push.Service) {
	lastFired := make(map[string]string)
	// pending holds schedules that are waiting for their random offset to elapse.
	pending := make(map[string]pendingFire)
	// automations are evaluated on the same tick via their own edge-tracking engine.
	autos := newAutoEngine()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		now := time.Now()
		stamp := now.Format("2006-01-02 15:04")

		// Collect due schedules and timers under a read lock.
		var dueSchedules []store.Schedule
		var toEnqueue []store.Schedule
		var dueTimers []store.Timer
		st.Mu.RLock()
		for _, s := range st.Schedules {
			if !s.Enabled {
				continue
			}
			// If this schedule has a pending random-offset fire, check it.
			if pf, ok := pending[s.ID]; ok {
				maxAge := time.Duration(s.RandomOffsetMinutes)*time.Minute + 30*time.Second
				if time.Since(pf.enqueued) > maxAge {
					// Stale entry (e.g. schedule updated, or carried over from
					// a previous day). Drop it and fall through to re-check.
					delete(pending, s.ID)
				} else if !now.Before(pf.fireAt) {
					dueSchedules = append(dueSchedules, *s)
				}
				// Either way, skip the base-time check this tick.
				continue
			}
			if !scheduleMatchesNow(s, now, st.Settings, lastFired[s.ID], stamp) {
				continue
			}
			if s.RandomOffsetMinutes > 0 {
				toEnqueue = append(toEnqueue, *s)
			} else {
				dueSchedules = append(dueSchedules, *s)
			}
		}
		for _, t := range st.Timers {
			if !now.Before(t.FiresAt) {
				dueTimers = append(dueTimers, *t)
			}
		}
		st.Mu.RUnlock()

		// Register random-offset schedules into the pending map.
		for _, s := range toEnqueue {
			offsetSec := rand.Intn(s.RandomOffsetMinutes*60 + 1)
			fireAt := now.Add(time.Duration(offsetSec) * time.Second)
			pending[s.ID] = pendingFire{fireAt: fireAt, enqueued: now}
			lastFired[s.ID] = stamp
			log.Printf("scheduler: schedule %s queued with +%ds random offset", s.ID, offsetSec)
		}

		for _, s := range dueSchedules {
			delete(pending, s.ID)
			lastFired[s.ID] = stamp
			if err := executeSchedule(st, s, pushSvc); err != nil {
				log.Printf("scheduler: schedule %s failed: %v", s.ID, err)
			}
		}
		for _, t := range dueTimers {
			if err := executeTimer(st, t, pushSvc); err != nil {
				log.Printf("scheduler: timer %s failed: %v", t.ID, err)
			}
		}

		// Automations run off the same tick: time triggers match the minute,
		// while sensor/device triggers fire on edges detected against the
		// previous tick's snapshot.
		autos.tick(st, now, pushSvc)
	}
}

// executeTimer fires a one-shot timer and removes it from the persistent
// store regardless of success — the user already saw it scheduled and
// will see the resulting state on the next refresh.
func executeTimer(st *store.Store, t store.Timer, pushSvc *push.Service) error {
	st.Mu.Lock()
	defer st.FlushLights() // off-lock bridge calls for scene brightness/colour
	defer st.Mu.Unlock()

	delete(st.Timers, t.ID)
	label := targetLabel(st, t.TargetType, t.TargetID)
	// Suppress per-socket state-change pushes; the timer summary below covers it.
	st.SuppressStateChange = true
	err := st.ExecuteAction(t.TargetType, t.TargetID, t.Action)
	st.SuppressStateChange = false
	entry := store.ActivityEntry{Kind: t.TargetType, Source: "timer", Action: t.Action, Label: label}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
	}
	st.Activity.Add(entry)
	if saveErr := st.Save(); err == nil && saveErr != nil {
		err = saveErr
	}
	if err == nil {
		log.Printf("timer fired: %s on %s/%s", t.Action, t.TargetType, t.TargetID)
		if pushSvc != nil {
			go pushSvc.NotifyEvent(push.CategoryScheduleFired, "", push.PushPayload{
				Title: fmt.Sprintf("⏰ Timer: %s %s", label, t.Action),
				URL:   "/#/sockets",
				Tag:   "timer-" + t.ID,
			})
		}
	}
	return err
}

func executeSchedule(st *store.Store, s store.Schedule, pushSvc *push.Service) error {
	st.Mu.Lock()
	defer st.FlushLights() // off-lock bridge calls for scene brightness/colour
	defer st.Mu.Unlock()

	tt, tid, action := s.TargetType, s.TargetID, s.Action
	if tt == "" && s.SocketID != "" {
		tt, tid = "socket", s.SocketID
	}
	label := targetLabel(st, tt, tid)
	// Suppress per-socket state-change pushes; the schedule summary below covers it.
	st.SuppressStateChange = true
	err := st.ExecuteAction(tt, tid, action)
	st.SuppressStateChange = false
	entry := store.ActivityEntry{Kind: tt, Source: "schedule", Action: action, Label: label}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
	}
	st.Activity.Add(entry)
	if err != nil {
		return err
	}

	if existing, ok := st.Schedules[s.ID]; ok {
		existing.LastFiredAt = time.Now().UTC()
	}
	if err := st.Save(); err != nil {
		return err
	}
	log.Printf("scheduler: %s %s (%s/%s)", action, s.ID, tt, tid)
	if pushSvc != nil {
		go pushSvc.NotifyEvent(push.CategoryScheduleFired, "", push.PushPayload{
			Title: fmt.Sprintf("⏰ Schedule: %s %s", label, action),
			URL:   "/#/schedules",
			Tag:   "schedule-" + s.ID,
		})
	}
	return nil
}

// targetLabel resolves a (kind, id) pair to a human-readable name for
// the activity log. Falls back to the id if the target was deleted.
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
	case "scene":
		if v, ok := st.Scenes[id]; ok {
			return v.Name
		}
	}
	return id
}

// scheduleMatchesNow reports whether s's trigger time falls in the current
// minute on a matching weekday and it hasn't already fired this minute. It
// does not consider the random offset or pending state — the caller layers
// those on top. lastStamp is the "YYYY-MM-DD HH:MM" the schedule last fired
// at; nowStamp is the same format for now.
func scheduleMatchesNow(s *store.Schedule, now time.Time, settings *store.Settings, lastStamp, nowStamp string) bool {
	triggerHHMM, ok := s.EffectiveHHMM(now, settings)
	if !ok || triggerHHMM != now.Format("15:04") {
		return false
	}
	if !dayMatches(s.Days, int(now.Weekday())) {
		return false
	}
	return lastStamp != nowStamp
}

func dayMatches(days []int, weekday int) bool {
	if len(days) == 0 {
		return true
	}
	for _, d := range days {
		if d == weekday {
			return true
		}
	}
	return false
}
