package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/store"
)

// The HTTP surface for music timers. The engine is in musictimer.go.
//
// Two shapes of the same thing, because the two things people actually set
// are asked for differently. A wake-up is arranged in advance and described
// in full — this playlist, that room, weekdays, arriving at 20 over ten
// minutes — and belongs in ordinary CRUD. A sleep timer is set in the moment
// by someone already in bed: it is "forty minutes", nothing else, about the
// room in front of them. Making that second gesture assemble a resource with
// eight fields would be the wall panel asking the user to do the backend's
// arithmetic, so /sleep does it instead.

// musicTimerView is a timer plus what the frontend would otherwise have to
// resolve for itself.
type musicTimerView struct {
	*store.MusicTimer
	// RoomName is what the house calls the destination now, which is not
	// necessarily what it was called when the timer was set.
	RoomName string `json:"room_name"`
	// NextAt is when this will next fire, so a row can say "in 6 hours"
	// without reimplementing the weekday arithmetic in TypeScript. Zero
	// when the timer is disabled or its schedule can't be read.
	NextAt time.Time `json:"next_at,omitempty"`
	// Fading marks a room with a ramp in flight right now — the state
	// between "sleep timer set" and "room quiet", which is otherwise
	// invisible except as volume drifting on its own.
	Fading bool `json:"fading"`
}

// musicTimers handles GET /api/media/timers.
func (s *Server) musicTimers(w http.ResponseWriter, r *http.Request) {
	fading := s.fadingRooms()
	now := time.Now()

	var out []musicTimerView
	s.Store.View(func() {
		out = make([]musicTimerView, 0, len(s.Store.MusicTimers))
		for _, t := range s.Store.MusicTimers {
			cp := *t
			cp.Days = append([]int(nil), t.Days...)
			out = append(out, musicTimerView{
				MusicTimer: &cp,
				RoomName:   s.musicRoomNameLocked(cp.Room),
				NextAt:     nextMusicFire(&cp, now),
				Fading:     fading[cp.Room],
			})
		}
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].NextAt.Equal(out[j].NextAt) {
			return out[i].ID < out[j].ID
		}
		// Zero times (disabled) sort last rather than first, which is
		// where "nothing is going to happen" belongs in a list of what is
		// about to happen.
		if out[i].NextAt.IsZero() || out[j].NextAt.IsZero() {
			return out[j].NextAt.IsZero()
		}
		return out[i].NextAt.Before(out[j].NextAt)
	})
	writeJSON(w, http.StatusOK, out)
}

// musicCreateTimer handles POST /api/media/timers.
func (s *Server) musicCreateTimer(w http.ResponseWriter, r *http.Request) {
	var t store.MusicTimer
	if !decodeBody(w, r, &t) {
		return
	}
	t.ID = fmt.Sprintf("mt_%d", time.Now().UnixNano())
	t.LastFiredAt = time.Time{}

	if !s.update(w, func() error {
		if err := s.Store.ValidateMusicTimer(&t); err != nil {
			return errInvalid(err)
		}
		s.Store.MusicTimers[t.ID] = &t
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// musicUpdateTimer handles PUT /api/media/timers/{id}.
//
// The body replaces the timer wholesale rather than patching it: the two
// schedules are mutually exclusive and the item is meaningful only for one
// action, so a partial update would have to define what clearing each of them
// looks like. Sending the whole timer says exactly what it should now be.
func (s *Server) musicUpdateTimer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updates store.MusicTimer
	if !decodeBody(w, r, &updates) {
		return
	}

	var saved *store.MusicTimer
	if !s.update(w, func() error {
		existing, ok := s.Store.MusicTimers[id]
		if !ok {
			return errStatus(http.StatusNotFound, "music timer not found")
		}
		merged := updates
		merged.ID = id
		merged.LastFiredAt = existing.LastFiredAt
		if err := s.Store.ValidateMusicTimer(&merged); err != nil {
			return errInvalid(err)
		}
		*existing = merged
		saved = existing
		return nil
	}) {
		return
	}
	// A timer that has just been rewritten should not have last night's
	// ramp still walking its room.
	s.CancelFade(saved.Room)
	writeJSON(w, http.StatusOK, saved)
}

// musicDeleteTimer handles DELETE /api/media/timers/{id}.
//
// Deleting a sleep timer that is already fading is how someone says "I'm
// still up". Cancelling the ramp puts the volume back and leaves the music
// playing — see rampDownAndPause.
func (s *Server) musicDeleteTimer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var room string
	if !s.update(w, func() error {
		t, ok := s.Store.MusicTimers[id]
		if !ok {
			return errStatus(http.StatusNotFound, "music timer not found")
		}
		room = t.Room
		delete(s.Store.MusicTimers, id)
		return nil
	}) {
		return
	}
	s.CancelFade(room)
	w.WriteHeader(http.StatusNoContent)
}

// musicSleep handles POST /api/media/timers/sleep with
// {"room":"sonos:abc","minutes":40,"fade_minutes":5}.
//
// The one gesture this whole file exists for. minutes is when the room goes
// quiet; fade_minutes is how much of the end of that is spent getting there,
// defaulting to a fade that is a fifth of the wait — long enough to be a
// fade, short enough that most of the timer is still music at full volume.
//
// Setting a sleep timer on a room that already has one replaces it. Two
// sleep timers on one room is never what someone means; it is what happens
// when they tap the button again because they changed their mind.
func (s *Server) musicSleep(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Room        string `json:"room"`
		Minutes     int    `json:"minutes"`
		FadeMinutes *int   `json:"fade_minutes"`
		// Volume is where to fade down to. Almost always 0 — the field
		// exists for "turn it down to background at bedtime" without
		// stopping, which is the same gesture with a floor.
		Volume *int `json:"volume"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	body.Room = strings.TrimSpace(body.Room)
	if body.Minutes < 1 || body.Minutes > 12*60 {
		writeError(w, http.StatusBadRequest, "minutes must be between 1 and 720")
		return
	}

	fade := body.Minutes / 5
	if body.FadeMinutes != nil {
		fade = *body.FadeMinutes
	}
	if fade > store.MaxFadeMinutes {
		fade = store.MaxFadeMinutes
	}
	if fade > body.Minutes {
		// A fade longer than the wait would start before the timer is set,
		// which is a request to turn the music down now.
		fade = body.Minutes
	}

	volume := 0
	if body.Volume != nil {
		volume = *body.Volume
	}

	t := store.MusicTimer{
		ID:          fmt.Sprintf("mt_%d", time.Now().UnixNano()),
		Room:        body.Room,
		Action:      store.MusicStop,
		Enabled:     true,
		FiresAt:     time.Now().Add(time.Duration(body.Minutes-fade) * time.Minute),
		Volume:      &volume,
		FadeMinutes: fade,
	}

	if !s.update(w, func() error {
		if err := s.Store.ValidateMusicTimer(&t); err != nil {
			return errInvalid(err)
		}
		for id, existing := range s.Store.MusicTimers {
			if existing.Room == t.Room && existing.Action == store.MusicStop && !existing.Recurring() {
				delete(s.Store.MusicTimers, id)
			}
		}
		s.Store.MusicTimers[t.ID] = &t
		return nil
	}) {
		return
	}
	// Whatever was already fading this room is not this timer's ramp.
	s.CancelFade(t.Room)

	writeJSON(w, http.StatusCreated, map[string]any{
		"timer": t,
		// When the room actually goes quiet, which is the number someone
		// wants read back to them — not when the fade starts.
		"quiet_at": t.FiresAt.Add(time.Duration(fade) * time.Minute),
	})
}

// musicCancelFade handles POST /api/media/timers/fade/cancel with
// {"room":"sonos:abc"} — stop a ramp in flight without deleting anything.
// The room keeps whatever volume it started the fade at.
func (s *Server) musicCancelFade(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Room string `json:"room"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": s.CancelFade(strings.TrimSpace(body.Room))})
}

// musicRoomNameLocked is musicRoomName for a caller already inside a View.
func (s *Server) musicRoomNameLocked(key string) string {
	if id, ok := strings.CutPrefix(key, "zone:"); ok {
		if z, exists := s.Store.Zones[id]; exists {
			return z.Name
		}
		return key
	}
	bridge, id, ok := store.SplitMember(key)
	if !ok {
		return key
	}
	if bridge == "kef" {
		if sp, exists := s.Store.KEF[id]; exists {
			return sp.Name
		}
		return key
	}
	if sp, exists := s.Store.Sonos[id]; exists {
		return sp.Name
	}
	return key
}

// nextMusicFire is when a timer will next run, or the zero time when it
// won't. Computed here rather than in the store because it is presentation:
// nothing about whether a timer fires depends on it.
func nextMusicFire(t *store.MusicTimer, now time.Time) time.Time {
	if !t.Enabled {
		return time.Time{}
	}
	if !t.Recurring() {
		return t.FiresAt
	}
	hhmm, err := time.Parse("15:04", t.Time)
	if err != nil {
		return time.Time{}
	}
	// Today's occurrence, then each following day until one matches. Eight
	// days rather than seven so a timer due later today is preferred over
	// the same weekday next week.
	for day := 0; day < 8; day++ {
		cand := time.Date(now.Year(), now.Month(), now.Day()+day,
			hhmm.Hour(), hhmm.Minute(), 0, 0, now.Location())
		if !cand.After(now) || !musicRunsOn(t.Days, int(cand.Weekday())) {
			continue
		}
		return cand
	}
	return time.Time{}
}

func musicRunsOn(days []int, weekday int) bool {
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
