// Package scheduler ticks every 5 seconds, fires due one-shot timers,
// and (per-minute) any enabled schedules whose HH:MM + weekday match
// the current local time. It owns no state of its own — everything
// runs against an injected *store.Store.
//
// It decides *when*, never *how*: reaching a device is internal/control's
// staged flow, the same one a tap in the app takes. A schedule turning the
// porch light off at 23:40 and a person turning it off from the sofa are one
// code path, which is what stops the two from drifting.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"homehub/internal/control"
	"homehub/internal/push"
	"homehub/internal/store"
)

// Config is what the tick needs from the rest of the application.
type Config struct {
	// Store holds the schedules, timers and automations to match against.
	Store *store.Store

	// Control is how anything due reaches a device.
	Control *control.Actions

	// Push is optional; nil disables the notifications a fire produces.
	Push *push.Service
}

// pendingFire holds a randomly-delayed fire time for a schedule that has
// random_offset_minutes set. enqueued is when the base time matched, used
// to expire stale entries.
type pendingFire struct {
	fireAt   time.Time
	enqueued time.Time
}

// Run blocks until ctx is cancelled. Spawn it in a goroutine.
func Run(ctx context.Context, cfg Config) {
	st := cfg.Store
	lastFired := make(map[string]string)
	// pending holds schedules that are waiting for their random offset to elapse.
	pending := make(map[string]pendingFire)
	// automations are evaluated on the same tick via their own edge-tracking engine.
	autos := newAutoEngine()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// prevTick anchors the (prev, now] matching window. Seeded just before
	// the current minute starts so schedules due in the startup minute
	// still fire on the first tick.
	prevTick := time.Now().Truncate(time.Minute).Add(-time.Second)

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
		st.View(func() {
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
				if !scheduleMatchesNow(s, prevTick, now, st.Settings, lastFired[s.ID], stamp) {
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
			// Drop bookkeeping for schedules that no longer exist — without this
			// the maps grow forever on a long-running install.
			for id := range lastFired {
				if _, ok := st.Schedules[id]; !ok {
					delete(lastFired, id)
				}
			}
			for id := range pending {
				if _, ok := st.Schedules[id]; !ok {
					delete(pending, id)
				}
			}
		})

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
			if err := executeSchedule(cfg, s); err != nil {
				log.Printf("scheduler: schedule %s failed: %v", s.ID, err)
			}
		}
		for _, t := range dueTimers {
			if err := executeTimer(cfg, t); err != nil {
				log.Printf("scheduler: timer %s failed: %v", t.ID, err)
			}
		}

		// Automations run off the same tick: time triggers match the minute,
		// while sensor/device triggers fire on edges detected against the
		// previous tick's snapshot.
		autos.tick(cfg, prevTick, now)
		prevTick = now
	}
}

// executeTimer fires a one-shot timer and removes it from the persistent
// store regardless of success — the user already saw it scheduled and
// will see the resulting state on the next refresh.
func executeTimer(cfg Config, t store.Timer) error {
	res, err := cfg.Control.Target(control.Run{
		TargetType: t.TargetType, TargetID: t.TargetID, Action: t.Action,
		Source: control.SourceTimer,
		// Removed before the target is resolved and in the same transaction:
		// a device that refuses must not leave the timer to fire again on the
		// next tick five seconds from now.
		Before: func() { delete(cfg.Store.Timers, t.ID) },
	})
	if err == nil {
		err = res.Err()
	}
	if err != nil {
		return err
	}
	log.Printf("timer fired: %s on %s/%s", t.Action, t.TargetType, t.TargetID)
	if cfg.Push != nil {
		go cfg.Push.NotifyEvent(push.CategoryScheduleFired, "", push.PushPayload{
			Title: fmt.Sprintf("⏰ Timer: %s %s", res.Label, t.Action),
			URL:   "/#/sockets",
			Tag:   "timer-" + t.ID,
		})
	}
	return nil
}

func executeSchedule(cfg Config, s store.Schedule) error {
	tt, tid, action := s.TargetType, s.TargetID, s.Action
	if tt == "" && s.SocketID != "" {
		tt, tid = "socket", s.SocketID
	}

	res, err := cfg.Control.Target(control.Run{
		TargetType: tt, TargetID: tid, Action: action,
		Source: control.SourceSchedule,
		After: func(r control.Result) {
			// Only a schedule that reached its devices counts as having
			// fired: the next run should not be told it already happened.
			if r.Err() != nil {
				return
			}
			if existing, ok := cfg.Store.Schedules[s.ID]; ok {
				existing.LastFiredAt = time.Now().UTC()
			}
		},
	})
	if err == nil {
		err = res.Err()
	}
	if err != nil {
		return err
	}
	log.Printf("scheduler: %s %s (%s/%s)", action, s.ID, tt, tid)
	if cfg.Push != nil {
		go cfg.Push.NotifyEvent(push.CategoryScheduleFired, "", push.PushPayload{
			Title: fmt.Sprintf("⏰ Schedule: %s %s", res.Label, action),
			URL:   "/#/schedules",
			Tag:   "schedule-" + s.ID,
		})
	}
	return nil
}

// scheduleMatchesNow reports whether s's trigger time falls inside the
// (prev, now] wall-clock window on a matching weekday and it hasn't already
// fired this minute. It does not consider the random offset or pending
// state — the caller layers those on top. lastStamp is the
// "YYYY-MM-DD HH:MM" the schedule last fired at; nowStamp is the same
// format for now.
func scheduleMatchesNow(s *store.Schedule, prev, now time.Time, settings *store.Settings, lastStamp, nowStamp string) bool {
	triggerHHMM, ok := s.EffectiveHHMM(now, settings)
	if !ok || !timeWindowMatches(triggerHHMM, prev, now) {
		return false
	}
	if !dayMatches(s.Days, fireWeekday(triggerHHMM, prev, now)) {
		return false
	}
	return lastStamp != nowStamp
}

// timeWindowMatches reports whether an "HH:MM" trigger falls inside the
// half-open wall-clock window (prev, now]. The window normally spans one
// 5s tick — exactly one tick crosses each minute boundary, so this behaves
// like the old exact-minute match — but it widens across clock
// discontinuities: a DST spring-forward (02:00 → 03:00) or a suspend/
// resume still fires the schedules whose trigger time the clock skipped.
// A backwards step (DST fall-back, NTP) degrades to exact-minute matching;
// the per-minute lastFired stamp already dedupes the repeated hour.
func timeWindowMatches(triggerHHMM string, prev, now time.Time) bool {
	t, err := time.Parse("15:04", triggerHHMM)
	if err != nil {
		return false
	}
	trigger := t.Hour()*60 + t.Minute()
	prevMin := prev.Hour()*60 + prev.Minute()
	nowMin := now.Hour()*60 + now.Minute()
	sameDay := prev.Year() == now.Year() && prev.YearDay() == now.YearDay()
	switch {
	case sameDay && prevMin <= nowMin:
		return trigger > prevMin && trigger <= nowMin
	case !sameDay && !now.Before(prev):
		// Crossed midnight: yesterday's tail plus today's head.
		return trigger > prevMin || trigger <= nowMin
	default:
		// Wall clock stepped backwards within the same day.
		return trigger == nowMin
	}
}

// fireWeekday returns the weekday a trigger inside the (prev, now] window
// belongs to: a trigger in yesterday's tail (window crossing midnight)
// keeps yesterday's weekday for day-of-week matching.
func fireWeekday(triggerHHMM string, prev, now time.Time) int {
	t, err := time.Parse("15:04", triggerHHMM)
	if err != nil {
		return int(now.Weekday())
	}
	trigger := t.Hour()*60 + t.Minute()
	sameDay := prev.Year() == now.Year() && prev.YearDay() == now.YearDay()
	if !sameDay && trigger > prev.Hour()*60+prev.Minute() {
		return int(prev.Weekday())
	}
	return int(now.Weekday())
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
