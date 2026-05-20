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
	if !scheduleMatchesNow(s, now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected schedule to match at its trigger minute")
	}
}

func TestScheduleMatchesNow_WrongMinute(t *testing.T) {
	now := refNow()
	s := &store.Schedule{Time: "07:31"}
	if scheduleMatchesNow(s, now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected no match one minute off")
	}
}

func TestScheduleMatchesNow_WrongDay(t *testing.T) {
	now := refNow() // Wednesday == weekday 3
	s := &store.Schedule{Time: "07:30", Days: []int{1, 2}}
	if scheduleMatchesNow(s, now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected no match on a non-listed weekday")
	}
}

func TestScheduleMatchesNow_DoubleFireGuard(t *testing.T) {
	now := refNow()
	s := &store.Schedule{Time: "07:30"}
	// Already fired this minute -> must not match again.
	if scheduleMatchesNow(s, now, &store.Settings{}, stampOf(now), stampOf(now)) {
		t.Error("expected the double-fire guard to suppress a repeat in the same minute")
	}
	// A different prior stamp (e.g. yesterday) -> should match.
	if !scheduleMatchesNow(s, now, &store.Settings{}, "2025-06-03 07:30", stampOf(now)) {
		t.Error("expected match when last fired was a previous minute")
	}
}

func TestScheduleMatchesNow_EmptyFixedTimeNeverMatches(t *testing.T) {
	now := refNow()
	s := &store.Schedule{} // no time, fixed mode
	if scheduleMatchesNow(s, now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected a fixed schedule with no time to never match")
	}
}

func TestScheduleMatchesNow_SolarWithoutLocationNeverMatches(t *testing.T) {
	now := refNow()
	s := &store.Schedule{TimeMode: store.ModeSunset}
	if scheduleMatchesNow(s, now, &store.Settings{}, "", stampOf(now)) {
		t.Error("expected a solar schedule with no location configured to never match")
	}
}
