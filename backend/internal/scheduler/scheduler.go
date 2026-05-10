// Package scheduler ticks every 5 seconds, fires due one-shot timers,
// and (per-minute) any enabled schedules whose HH:MM + weekday match
// the current local time. It owns no state of its own — everything
// runs against an injected *store.Store.
package scheduler

import (
	"context"
	"log"
	"time"

	"rf-socket-controller/internal/store"
)

// Run blocks until ctx is cancelled. Spawn it in a goroutine.
func Run(ctx context.Context, st *store.Store) {
	lastFired := make(map[string]string)
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
		hhmm := now.Format("15:04")
		weekday := int(now.Weekday())

		// Collect due schedules and timers under a read lock.
		var dueSchedules []store.Schedule
		var dueTimers []store.Timer
		st.Mu.RLock()
		for _, s := range st.Schedules {
			if !s.Enabled || s.Time != hhmm {
				continue
			}
			if !dayMatches(s.Days, weekday) {
				continue
			}
			if lastFired[s.ID] == stamp {
				continue
			}
			dueSchedules = append(dueSchedules, *s)
		}
		for _, t := range st.Timers {
			if !now.Before(t.FiresAt) {
				dueTimers = append(dueTimers, *t)
			}
		}
		st.Mu.RUnlock()

		for _, s := range dueSchedules {
			lastFired[s.ID] = stamp
			if err := executeSchedule(st, s); err != nil {
				log.Printf("scheduler: schedule %s failed: %v", s.ID, err)
			}
		}
		for _, t := range dueTimers {
			if err := executeTimer(st, t); err != nil {
				log.Printf("scheduler: timer %s failed: %v", t.ID, err)
			}
		}
	}
}

// executeTimer fires a one-shot timer and removes it from the persistent
// store regardless of success — the user already saw it scheduled and
// will see the resulting state on the next refresh.
func executeTimer(st *store.Store, t store.Timer) error {
	st.Mu.Lock()
	defer st.Mu.Unlock()

	delete(st.Timers, t.ID)
	err := st.ExecuteAction(t.TargetType, t.TargetID, t.Action)
	if saveErr := st.Save(); err == nil && saveErr != nil {
		err = saveErr
	}
	if err == nil {
		log.Printf("timer fired: %s on %s/%s", t.Action, t.TargetType, t.TargetID)
	}
	return err
}

func executeSchedule(st *store.Store, s store.Schedule) error {
	st.Mu.Lock()
	defer st.Mu.Unlock()

	tt, tid, action := s.TargetType, s.TargetID, s.Action
	if tt == "" && s.SocketID != "" {
		tt, tid = "socket", s.SocketID
	}
	if err := st.ExecuteAction(tt, tid, action); err != nil {
		return err
	}

	if existing, ok := st.Schedules[s.ID]; ok {
		existing.LastFiredAt = time.Now().UTC()
	}
	if err := st.Save(); err != nil {
		return err
	}
	log.Printf("scheduler: %s %s (%s/%s)", action, s.ID, tt, tid)
	return nil
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
