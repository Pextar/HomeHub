package scheduler

import (
	"testing"
	"time"

	"rf-socket-controller/internal/store"
)

func TestDayMatches(t *testing.T) {
	cases := []struct {
		name    string
		days    []int
		weekday int
		want    bool
	}{
		{"empty means every day", nil, 3, true},
		{"empty slice means every day", []int{}, 0, true},
		{"weekday in list", []int{1, 3, 5}, 3, true},
		{"weekday not in list", []int{1, 3, 5}, 2, false},
		{"sunday is zero", []int{0}, 0, true},
		{"saturday", []int{6}, 6, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dayMatches(c.days, c.weekday); got != c.want {
				t.Errorf("dayMatches(%v, %d) = %v, want %v", c.days, c.weekday, got, c.want)
			}
		})
	}
}

// A Wednesday at 07:30 local time, used as the reference "now".
func refNow() time.Time {
	return time.Date(2025, 6, 4, 7, 30, 0, 0, time.Local)
}

func stampOf(t time.Time) string { return t.Format("2006-01-02 15:04") }

func TestScheduleMatchesNow_FiresAtMatchingMinute(t *testing.T) {
	now := refNow()
	s := &store.Schedule{Time: "07:30", Enabled: true}
	if !scheduleMatchesNow(s, now.Add(-5*time.Second), now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected schedule to match at its trigger minute")
	}
}

func TestScheduleMatchesNow_WrongMinute(t *testing.T) {
	now := refNow()
	s := &store.Schedule{Time: "07:31"}
	if scheduleMatchesNow(s, now.Add(-5*time.Second), now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected no match one minute off")
	}
}

func TestScheduleMatchesNow_WrongDay(t *testing.T) {
	now := refNow() // Wednesday == weekday 3
	s := &store.Schedule{Time: "07:30", Days: []int{1, 2}}
	if scheduleMatchesNow(s, now.Add(-5*time.Second), now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected no match on a non-listed weekday")
	}
}

func TestScheduleMatchesNow_DoubleFireGuard(t *testing.T) {
	now := refNow()
	s := &store.Schedule{Time: "07:30"}
	// Already fired this minute -> must not match again.
	if scheduleMatchesNow(s, now.Add(-5*time.Second), now, &store.Settings{}, stampOf(now), stampOf(now)) {
		t.Error("expected the double-fire guard to suppress a repeat in the same minute")
	}
	// A different prior stamp (e.g. yesterday) -> should match.
	if !scheduleMatchesNow(s, now.Add(-5*time.Second), now, &store.Settings{}, "2025-06-03 07:30", stampOf(now)) {
		t.Error("expected match when last fired was a previous minute")
	}
}

func TestScheduleMatchesNow_EmptyFixedTimeNeverMatches(t *testing.T) {
	now := refNow()
	s := &store.Schedule{} // no time, fixed mode
	if scheduleMatchesNow(s, now.Add(-5*time.Second), now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected a fixed schedule with no time to never match")
	}
}

func TestScheduleMatchesNow_SolarWithoutLocationNeverMatches(t *testing.T) {
	now := refNow()
	s := &store.Schedule{TimeMode: store.ModeSunset}
	if scheduleMatchesNow(s, now.Add(-5*time.Second), now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected a solar schedule with no location configured to never match")
	}
}

func TestScheduleMatchesNow_DSTSpringForwardGap(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	// Clocks jump 02:00 → 03:00 CEST on 2026-03-29: a tick straddling the
	// jump sees prev=01:59:58, now=03:00:03. A 02:30 schedule sits inside
	// the skipped hour and used to never fire that day.
	prev := time.Date(2026, 3, 29, 1, 59, 58, 0, loc)
	now := prev.Add(5 * time.Second)
	if now.Hour() != 3 {
		t.Fatalf("expected the tick after 01:59:58 to land at 03:00, got %v", now)
	}
	s := &store.Schedule{Time: "02:30"}
	if !scheduleMatchesNow(s, prev, now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected a schedule inside the spring-forward gap to fire")
	}
}

func TestScheduleMatchesNow_DSTFallBackNoSpuriousFire(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	// Clocks step 03:00 CEST → 02:00 CET on 2026-10-25 (01:00 UTC). The
	// wall clock goes backwards: the window logic must degrade to exact-
	// minute matching, not treat it as a midnight wrap and fire everything.
	prev := time.Date(2026, 10, 25, 0, 59, 58, 0, time.UTC).In(loc) // 02:59:58 CEST
	now := prev.Add(5 * time.Second)                                // 02:00:03 CET
	if now.Hour() != 2 || now.Minute() != 0 {
		t.Fatalf("expected the tick after 02:59:58 CEST to land at 02:00 CET, got %v", now)
	}
	if scheduleMatchesNow(&store.Schedule{Time: "22:00"}, prev, now, &store.Settings{}, "", stampOf(now)) {
		t.Error("a 22:00 schedule must not fire when the clock falls back at 03:00")
	}
	// The repeated 02:00 still matches by equality (the lastFired stamp
	// dedupes the second occurrence in the real loop).
	if !scheduleMatchesNow(&store.Schedule{Time: "02:00"}, prev, now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected an exact-minute match during the repeated hour")
	}
}

func TestTimeWindowMatches_MidnightWrap(t *testing.T) {
	prev := time.Date(2025, 6, 4, 23, 59, 58, 0, time.Local)
	now := prev.Add(5 * time.Second) // 00:00:03 next day
	if !timeWindowMatches("00:00", prev, now) {
		t.Error("expected a 00:00 trigger to fire on the tick crossing midnight")
	}
	if timeWindowMatches("12:00", prev, now) {
		t.Error("a midday trigger must not fire at midnight")
	}
	// A stalled tick that skipped 23:59 entirely: the trigger in
	// yesterday's tail keeps yesterday's weekday (Wed=3, not Thu=4).
	stalledPrev := time.Date(2025, 6, 4, 23, 58, 58, 0, time.Local)
	if !timeWindowMatches("23:59", stalledPrev, now) {
		t.Error("expected a 23:59 trigger to fire after a tick skipped it")
	}
	if got := fireWeekday("23:59", stalledPrev, now); got != 3 {
		t.Errorf("fireWeekday for yesterday's tail = %d, want 3 (Wednesday)", got)
	}
	if got := fireWeekday("00:00", stalledPrev, now); got != 4 {
		t.Errorf("fireWeekday for today's head = %d, want 4 (Thursday)", got)
	}
}
