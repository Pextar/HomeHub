package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Music that starts and stops without anyone tapping anything.
//
// Everything else in this file's neighbourhood — Schedule, Timer, Automation —
// drives sockets, and none of them can reach a speaker: ExecuteAction knows
// about sockets, groups, rooms and scenes, and the store deliberately has no
// way to talk to a bridge. So the house could turn a lamp on at seven and
// could not put the radio on with it, which is the wrong way round for the
// two things a home actually wants on a timer.
//
// A MusicTimer is the missing half, and it is one type rather than two
// because the two uses share everything except which end of the fade they are
// on:
//
//	waking    at 06:45 on weekdays, start this playlist in the bedroom,
//	          arriving at volume 20 over ten minutes from near silence
//	sleeping  in forty minutes, take the living room down to nothing and
//	          pause it — then put the volume back where it was
//
// That last clause is the part that is easy to leave out and impossible to
// live with: a room faded to two and paused is a room that is inaudible the
// next morning, and the person who set a sleep timer at midnight is not the
// person who finds out at breakfast. The engine restores what it lowered.
//
// The store holds these and validates them; it does not run them, because
// running one is device I/O. See internal/api/musictimer.go for the engine.

// MusicTimerAction is what a timer does when it comes due.
type MusicTimerAction string

const (
	// MusicStart puts something on, optionally fading up to a volume.
	MusicStart MusicTimerAction = "start"
	// MusicStop fades down and pauses, restoring the volume afterwards.
	MusicStop MusicTimerAction = "stop"
)

// MaxFadeMinutes caps a ramp. Longer than this and the timer stops being a
// fade and becomes a schedule with an opinion; it is also long enough that a
// restart in the middle would leave a room stuck at an interim volume with
// nothing left running to move it.
const MaxFadeMinutes = 60

// MusicTimer is one standing instruction to start or stop music in one room.
//
// Exactly one of the two schedules applies, which is the same split Timer and
// Schedule already make and for the same reason: "in forty minutes" and
// "every weekday at 06:45" are different enough that folding them into one
// field would mean a field that is a duration on Tuesdays.
type MusicTimer struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Room is the media layer's destination key — "sonos:<id>", "kef:<id>"
	// or "zone:<id>" — the same vocabulary the play history uses, so a
	// timer and a shelf name the same room the same way.
	Room    string           `json:"room"`
	Action  MusicTimerAction `json:"action"`
	Enabled bool             `json:"enabled"`

	// FiresAt set makes this a one-shot: it runs once and is deleted,
	// exactly like a Timer. This is what "sleep in forty minutes" is.
	FiresAt time.Time `json:"fires_at,omitempty"`
	// Time ("HH:MM") plus Days makes it recurring, exactly like a Schedule.
	// Empty Days means every day.
	Time string `json:"time,omitempty"`
	Days []int  `json:"days,omitempty"`

	// Item is what to put on. Required for MusicStart, ignored for
	// MusicStop — stopping needs no idea of what is playing.
	Item MusicTimerItem `json:"item,omitempty"`

	// Volume is where the room should end up: the level to arrive at for a
	// start, the level to fade down to for a stop (normally 0). Nil leaves
	// the volume alone entirely, which for a start means "come on at
	// whatever it was left at" — the honest default, since guessing a
	// wake-up volume for someone is how an alarm becomes a fright.
	Volume *int `json:"volume,omitempty"`
	// FadeMinutes is how long to take getting there. Zero is a jump.
	// Ignored when Volume is nil: there is nothing to ramp toward.
	FadeMinutes int `json:"fade_minutes,omitempty"`

	LastFiredAt time.Time `json:"last_fired_at,omitempty"`
}

// MusicTimerItem is what a starting timer puts on. The extra fields beyond
// the URI are what the play history and the room's own now-playing need, for
// the same reason the play handlers take them: asking the catalog to describe
// something we already had in hand costs a service round trip at 06:45, which
// is the worst possible moment for one.
type MusicTimerItem struct {
	Provider string `json:"provider,omitempty"`
	Kind     string `json:"kind,omitempty"` // track|album|playlist|artist|station
	URI      string `json:"uri,omitempty"`
	Title    string `json:"title,omitempty"`
	Sub      string `json:"sub,omitempty"`
	ArtURI   string `json:"art_uri,omitempty"`
}

// Recurring reports whether this timer repeats. The two schedules are
// mutually exclusive; ValidateMusicTimer is what guarantees that.
func (t *MusicTimer) Recurring() bool { return t.Time != "" }

// DueAt reports whether a recurring timer's wall-clock trigger falls in the
// half-open window (prev, now] on a day it runs. One-shots are not asked:
// their FiresAt is compared directly by the engine.
//
// The window rather than an exact minute for the same reason the socket
// scheduler uses one: a suspend/resume or a DST jump that skips 06:45 should
// still wake the house, and the caller's per-minute de-dupe stops a backwards
// clock step from firing twice.
func (t *MusicTimer) DueAt(prev, now time.Time) bool {
	if !t.Recurring() {
		return false
	}
	return musicTimeInWindow(t.Time, prev, now) && musicDayMatches(t.Days, prev, now, t.Time)
}

// ValidateMusicTimer normalises and checks a timer. Caller must hold Mu.
func (s *Store) ValidateMusicTimer(t *MusicTimer) error {
	t.Name = strings.TrimSpace(t.Name)
	t.Room = strings.TrimSpace(t.Room)
	t.Time = strings.TrimSpace(t.Time)
	t.Item.URI = strings.TrimSpace(t.Item.URI)
	t.Item.Title = strings.TrimSpace(t.Item.Title)

	if t.Room == "" {
		return errors.New("a room is required")
	}
	if !s.mediaRoomExists(t.Room) {
		return fmt.Errorf("%q is not a speaker or zone in this house", t.Room)
	}
	switch t.Action {
	case MusicStart:
		if t.Item.URI == "" {
			return errors.New("a starting timer needs something to play")
		}
	case MusicStop:
		// Nothing to check: stopping a room needs no idea of what is on.
		t.Item = MusicTimerItem{}
	default:
		return fmt.Errorf("action must be %q or %q", MusicStart, MusicStop)
	}

	switch {
	case t.Time != "" && !t.FiresAt.IsZero():
		return errors.New("a timer either repeats at a time of day or fires once, not both")
	case t.Time != "":
		if _, err := time.Parse("15:04", t.Time); err != nil {
			return fmt.Errorf("time must be HH:MM, got %q", t.Time)
		}
		days := make([]int, 0, len(t.Days))
		seen := make(map[int]bool, len(t.Days))
		for _, d := range t.Days {
			if d < 0 || d > 6 {
				return fmt.Errorf("days must be 0 (Sunday) to 6, got %d", d)
			}
			if !seen[d] {
				seen[d] = true
				days = append(days, d)
			}
		}
		sort.Ints(days)
		// An explicit every-day list and an empty one mean the same thing;
		// storing the empty form keeps one representation of one idea.
		if len(days) == 7 {
			days = nil
		}
		t.Days = days
	case !t.FiresAt.IsZero():
		t.Days = nil
	default:
		return errors.New("a timer needs either a time of day or a moment to fire at")
	}

	if t.Volume != nil {
		if *t.Volume < 0 || *t.Volume > 100 {
			return fmt.Errorf("volume must be 0 to 100, got %d", *t.Volume)
		}
	} else {
		// A fade with no destination has nothing to ramp toward, and
		// keeping the number would show a fade length the engine ignores.
		t.FadeMinutes = 0
	}
	if t.FadeMinutes < 0 || t.FadeMinutes > MaxFadeMinutes {
		return fmt.Errorf("fade must be 0 to %d minutes, got %d", MaxFadeMinutes, t.FadeMinutes)
	}
	return nil
}

// mediaRoomExists reports whether a destination key names something in this
// house. Caller must hold Mu.
//
// Checked at write time rather than only at fire time so that a timer for a
// speaker someone has since removed is refused when it is set, not discovered
// silently unfired at 06:45.
func (s *Store) mediaRoomExists(key string) bool {
	if id, ok := strings.CutPrefix(key, "zone:"); ok {
		_, exists := s.Zones[id]
		return exists
	}
	bridge, id, ok := SplitMember(key)
	if !ok {
		return false
	}
	switch bridge {
	case "kef":
		_, exists := s.KEF[id]
		return exists
	case "airplay":
		_, exists := s.AirPlay[id]
		return exists
	case "upnp":
		_, exists := s.UPnP[id]
		return exists
	}
	_, exists := s.Sonos[id]
	return exists
}

// PruneMusicTimers drops timers whose room is gone, mirroring what
// PruneHistory does for shelves and CascadeDeleteSocket for everything else: a
// deleted speaker must not leave an instruction behind that fires into
// nothing. Reports whether anything was dropped. Caller must hold Mu.
func (s *Store) PruneMusicTimers() bool {
	dropped := false
	for id, t := range s.MusicTimers {
		if !s.mediaRoomExists(t.Room) {
			delete(s.MusicTimers, id)
			dropped = true
		}
	}
	return dropped
}

// PruneMusicRooms drops music actions aimed at a room that no longer exists,
// out of every scene step and every automation rule, and then drops anything
// those emptied. Same promise as PruneMusicTimers and CascadeDeleteSocket: a
// deleted speaker leaves nothing behind that fires into nothing.
//
// A step or rule that had *only* music goes with it, because what is left is
// an instruction to do nothing. One that still has sockets to switch keeps
// them — the scene has lost a part, not its purpose. Reports whether
// anything changed. Caller must hold Mu.
func (s *Store) PruneMusicRooms() bool {
	changed := false

	keep := func(in []MusicAction) []MusicAction {
		if len(in) == 0 {
			return in
		}
		out := in[:0]
		for _, m := range in {
			if s.mediaRoomExists(m.Room) {
				out = append(out, m)
				continue
			}
			changed = true
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}

	for _, sc := range s.Scenes {
		steps := sc.Steps[:0]
		for _, step := range sc.Steps {
			step.Music = keep(step.Music)
			if len(step.Actions) > 0 || len(step.Music) > 0 {
				steps = append(steps, step)
			}
		}
		sc.Steps = steps
	}

	for _, a := range s.Automations {
		rules := a.Rules[:0]
		for _, r := range a.Rules {
			r.Music = keep(r.Music)
			if len(r.Actions) > 0 || len(r.Music) > 0 {
				rules = append(rules, r)
			}
		}
		a.Rules = rules
	}
	// An automation with no rules left can never fire again. Dropping it is
	// what CascadeDeleteTarget already does for the same situation reached
	// from the other direction (a deleted socket).
	for id, a := range s.Automations {
		if len(a.Rules) == 0 {
			delete(s.Automations, id)
			changed = true
		}
	}
	return changed
}

// musicTimeInWindow is timeWindowMatches from the scheduler, kept here rather
// than shared because the scheduler package imports this one and not the
// reverse. The two are small, tested separately, and describe the same
// wall-clock reasoning about the same kind of trigger.
func musicTimeInWindow(hhmm string, prev, now time.Time) bool {
	t, err := time.Parse("15:04", hhmm)
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

// musicDayMatches decides which day a trigger inside the window belongs to. A
// trigger in yesterday's tail keeps yesterday's weekday, so an 06:45 timer
// that fires just after midnight-crossing tick is still Monday's.
func musicDayMatches(days []int, prev, now time.Time, hhmm string) bool {
	if len(days) == 0 {
		return true
	}
	weekday := int(now.Weekday())
	if t, err := time.Parse("15:04", hhmm); err == nil {
		trigger := t.Hour()*60 + t.Minute()
		sameDay := prev.Year() == now.Year() && prev.YearDay() == now.YearDay()
		if !sameDay && trigger > prev.Hour()*60+prev.Minute() {
			weekday = int(prev.Weekday())
		}
	}
	for _, d := range days {
		if d == weekday {
			return true
		}
	}
	return false
}
