package store

import (
	"strings"
	"testing"
	"time"
)

// housed returns a store with one Sonos speaker, one KEF and one zone, so a
// timer has something real to point at.
func housed(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir(), nil)
	s.Sonos["snd"] = &SonosSpeaker{ID: "snd", Name: "Bedroom", IP: "192.168.1.10"}
	s.KEF["kf"] = &KEFSpeaker{ID: "kf", Name: "Study", IP: "192.168.1.11"}
	s.Zones["z"] = &Zone{ID: "z", Name: "Downstairs", Members: []string{"sonos:snd"}}
	return s
}

func wake() *MusicTimer {
	vol := 20
	return &MusicTimer{
		Room: "sonos:snd", Action: MusicStart, Enabled: true,
		Time: "06:45", Days: []int{1, 2, 3, 4, 5},
		Item:        MusicTimerItem{URI: "spotify:playlist:p", Title: "Morning"},
		Volume:      &vol,
		FadeMinutes: 10,
	}
}

func TestValidateMusicTimerAcceptsTheThreeKindsOfRoom(t *testing.T) {
	s := housed(t)
	for _, room := range []string{"sonos:snd", "kef:kf", "zone:z"} {
		mt := wake()
		mt.Room = room
		if err := s.ValidateMusicTimer(mt); err != nil {
			t.Errorf("room %q rejected: %v", room, err)
		}
	}
}

// Checked when the timer is set, not discovered unfired at 06:45.
func TestValidateMusicTimerRefusesARoomThatIsNotInTheHouse(t *testing.T) {
	s := housed(t)
	for _, room := range []string{"sonos:gone", "kef:gone", "zone:gone", "bedroom", ""} {
		mt := wake()
		mt.Room = room
		if err := s.ValidateMusicTimer(mt); err == nil {
			t.Errorf("room %q was accepted", room)
		}
	}
}

func TestValidateMusicTimerChecksTheScheduleIsOneThingOrTheOther(t *testing.T) {
	s := housed(t)

	t.Run("a repeating time and a one-shot moment are not both", func(t *testing.T) {
		mt := wake()
		mt.FiresAt = time.Now().Add(time.Hour)
		if err := s.ValidateMusicTimer(mt); err == nil {
			t.Error("a timer with both schedules was accepted")
		}
	})

	t.Run("neither is refused", func(t *testing.T) {
		mt := wake()
		mt.Time = ""
		if err := s.ValidateMusicTimer(mt); err == nil {
			t.Error("a timer with no schedule at all was accepted")
		}
	})

	t.Run("a one-shot needs no time of day", func(t *testing.T) {
		mt := wake()
		mt.Time, mt.Days = "", nil
		mt.FiresAt = time.Now().Add(40 * time.Minute)
		if err := s.ValidateMusicTimer(mt); err != nil {
			t.Errorf("a one-shot was rejected: %v", err)
		}
	})

	t.Run("the time of day must be readable", func(t *testing.T) {
		mt := wake()
		mt.Time = "quarter to seven"
		if err := s.ValidateMusicTimer(mt); err == nil {
			t.Error("an unreadable time was accepted")
		}
	})
}

// An explicit list of all seven days and an empty one mean the same thing.
// Storing one representation of one idea keeps "every day" from rendering two
// different ways depending on how it was set.
func TestValidateMusicTimerNormalisesDays(t *testing.T) {
	s := housed(t)

	mt := wake()
	mt.Days = []int{5, 1, 3, 1}
	if err := s.ValidateMusicTimer(mt); err != nil {
		t.Fatal(err)
	}
	if len(mt.Days) != 3 || mt.Days[0] != 1 || mt.Days[1] != 3 || mt.Days[2] != 5 {
		t.Errorf("days = %v, want sorted and de-duplicated", mt.Days)
	}

	all := wake()
	all.Days = []int{0, 1, 2, 3, 4, 5, 6}
	if err := s.ValidateMusicTimer(all); err != nil {
		t.Fatal(err)
	}
	if all.Days != nil {
		t.Errorf("days = %v, want nil — all seven days is every day", all.Days)
	}

	bad := wake()
	bad.Days = []int{7}
	if err := s.ValidateMusicTimer(bad); err == nil {
		t.Error("weekday 7 was accepted")
	}
}

func TestValidateMusicTimerChecksWhatEachActionNeeds(t *testing.T) {
	s := housed(t)

	t.Run("starting needs something to play", func(t *testing.T) {
		mt := wake()
		mt.Item.URI = "   "
		err := s.ValidateMusicTimer(mt)
		if err == nil || !strings.Contains(err.Error(), "something to play") {
			t.Errorf("err = %v, want a complaint about the missing item", err)
		}
	})

	t.Run("stopping drops an item it would never use", func(t *testing.T) {
		mt := wake()
		mt.Action = MusicStop
		if err := s.ValidateMusicTimer(mt); err != nil {
			t.Fatal(err)
		}
		if mt.Item.URI != "" {
			t.Errorf("item = %+v, want it cleared — stopping needs no idea of what is on", mt.Item)
		}
	})

	t.Run("an unknown action is refused", func(t *testing.T) {
		mt := wake()
		mt.Action = "fade"
		if err := s.ValidateMusicTimer(mt); err == nil {
			t.Error("an unknown action was accepted")
		}
	})
}

func TestValidateMusicTimerChecksTheRamp(t *testing.T) {
	s := housed(t)

	for _, v := range []int{-1, 101} {
		mt := wake()
		mt.Volume = &v
		if err := s.ValidateMusicTimer(mt); err == nil {
			t.Errorf("volume %d was accepted", v)
		}
	}
	long := wake()
	long.FadeMinutes = MaxFadeMinutes + 1
	if err := s.ValidateMusicTimer(long); err == nil {
		t.Error("a fade past the cap was accepted")
	}

	// A fade with no volume to arrive at has nothing to ramp toward, and
	// keeping the number would show a fade length the engine ignores.
	noVol := wake()
	noVol.Volume = nil
	if err := s.ValidateMusicTimer(noVol); err != nil {
		t.Fatal(err)
	}
	if noVol.FadeMinutes != 0 {
		t.Errorf("fade = %d with no volume, want it cleared", noVol.FadeMinutes)
	}
}

// The (prev, now] window, not an exact minute: a suspend/resume or a DST jump
// that skips 06:45 should still wake the house.
func TestDueAtMatchesTheWindowAndTheWeekday(t *testing.T) {
	mt := wake() // 06:45, Monday–Friday
	day := func(weekday time.Weekday, h, m int) time.Time {
		// 2026-03-16 is a Monday.
		d := 16 + int(weekday) - int(time.Monday)
		return time.Date(2026, 3, d, h, m, 0, 0, time.Local)
	}

	if !mt.DueAt(day(time.Monday, 6, 44), day(time.Monday, 6, 45)) {
		t.Error("06:45 on a Monday did not fire")
	}
	if mt.DueAt(day(time.Monday, 6, 45), day(time.Monday, 6, 46)) {
		t.Error("06:45 fired again a minute later")
	}
	if mt.DueAt(day(time.Saturday, 6, 44), day(time.Saturday, 6, 45)) {
		t.Error("a weekday timer fired on Saturday")
	}
	// A clock that jumped over the trigger still fires it.
	if !mt.DueAt(day(time.Monday, 6, 40), day(time.Monday, 7, 10)) {
		t.Error("a trigger the clock skipped was lost")
	}

	once := wake()
	once.Time, once.Days = "", nil
	once.FiresAt = time.Now()
	if once.DueAt(time.Now().Add(-time.Minute), time.Now()) {
		t.Error("a one-shot answered DueAt; its FiresAt is compared directly")
	}
}

// A deleted speaker must not leave an alarm behind that fires into nothing —
// the same promise CascadeDeleteSocket makes for everything else.
func TestPruneMusicTimersDropsTimersForRoomsThatAreGone(t *testing.T) {
	s := housed(t)
	for _, room := range []string{"sonos:snd", "kef:kf", "zone:z"} {
		mt := wake()
		mt.Room = room
		if err := s.ValidateMusicTimer(mt); err != nil {
			t.Fatal(err)
		}
		s.MusicTimers[room] = mt
	}

	delete(s.KEF, "kf")
	delete(s.Zones, "z")
	if !s.PruneMusicTimers() {
		t.Error("PruneMusicTimers reported no change, want true")
	}
	if len(s.MusicTimers) != 1 || s.MusicTimers["sonos:snd"] == nil {
		t.Errorf("timers = %v, want only the live speaker's", s.MusicTimers)
	}
	if s.PruneMusicTimers() {
		t.Error("PruneMusicTimers reported a change when nothing was dropped")
	}
}

func TestMusicTimersSurviveAReload(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, nil)
	s.Sonos["snd"] = &SonosSpeaker{ID: "snd", Name: "Bedroom", IP: "192.168.1.10"}
	mt := wake()
	mt.ID = "mt_1"
	if err := s.ValidateMusicTimer(mt); err != nil {
		t.Fatal(err)
	}
	s.MusicTimers["mt_1"] = mt
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	again := New(dir, nil)
	if err := again.Load(); err != nil {
		t.Fatal(err)
	}
	got, ok := again.MusicTimers["mt_1"]
	if !ok {
		t.Fatal("the timer did not survive a reload")
	}
	if got.Time != "06:45" || got.Volume == nil || *got.Volume != 20 || got.FadeMinutes != 10 {
		t.Errorf("reloaded timer = %+v, want its schedule and ramp intact", got)
	}
}
