package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"homehub/internal/media"
	"homehub/internal/store"
)

// The engine behind store.MusicTimer: music that starts and stops on its own.
//
// It lives here rather than in the scheduler package for the same reason
// every other speaker call does — running a timer is device I/O across a
// route, and the store has no way to reach a bridge. The scheduler's own tick
// stays about sockets.
//
// Two things make this more than a scheduler that calls Play.
//
// The first is the fade. A jump from silence to twenty-five is an alarm
// clock; the same twenty-five arrived at over ten minutes is being woken by
// music. Fades run detached from the tick, because a ten-minute ramp cannot
// hold up the loop that would notice the next timer.
//
// The second is that a sleep fade puts the volume back. A room faded to
// nothing and paused is a room that is silent the next morning at a volume
// nobody chose, and the person who set the timer at midnight is not the
// person who finds out at breakfast. Restoring is not a nicety here — without
// it the feature is a trap, which is why it happens even when the fade is
// interrupted.

// musicTickInterval matches the socket scheduler's. The (prev, now] window
// below is what actually decides whether a timer is due, so this only sets
// how promptly a due one is noticed.
const musicTickInterval = 5 * time.Second

// musicFadeFloor is where a fade-up starts, as a fraction of its target: a
// fifth, never less than 1. Not zero — a wake-up that spends its first
// minutes at literal silence reads as a timer that failed, and someone lying
// awake wondering is worse off than someone hearing the first bar quietly.
func musicFadeFloor(target int) int {
	if floor := target / 5; floor > 1 {
		return floor
	}
	return 1
}

// RunMusicTimers blocks until ctx is cancelled. Spawn it in a goroutine.
func (s *Server) RunMusicTimers(ctx context.Context) {
	ticker := time.NewTicker(musicTickInterval)
	defer ticker.Stop()

	// prevTick anchors the (prev, now] window, seeded just inside the
	// current minute so a timer due in the startup minute still fires.
	prevTick := time.Now().Truncate(time.Minute).Add(-time.Second)
	// fired de-dupes recurring timers within a minute, the same guard the
	// socket scheduler keeps: a wall clock stepping backwards must not run
	// the same 06:45 twice.
	fired := make(map[string]string)

	for {
		select {
		case <-ctx.Done():
			s.cancelAllFades()
			return
		case <-ticker.C:
		}

		now := time.Now()
		stamp := now.Format("2006-01-02 15:04")
		due := s.dueMusicTimers(prevTick, now, stamp, fired)
		prevTick = now

		for _, t := range due {
			// Detached: a start can take most of a minute (waking a
			// speaker, a cloud round trip, several UPnP calls) and the
			// tick has to stay free to notice the next timer.
			go s.fireMusicTimer(ctx, t)
		}
	}
}

// dueMusicTimers collects the timers that have come due, marks recurring ones
// as fired for this minute and removes the one-shots. Takes the write lock
// once; everything that touches a speaker happens after it is released.
func (s *Server) dueMusicTimers(prev, now time.Time, stamp string, fired map[string]string) []store.MusicTimer {
	var due []store.MusicTimer
	var consumed bool

	s.Store.Mutate(func() {
		for id, t := range s.Store.MusicTimers {
			if !t.Enabled {
				continue
			}
			switch {
			case t.Recurring():
				if fired[id] == stamp || !t.DueAt(prev, now) {
					continue
				}
				fired[id] = stamp
				t.LastFiredAt = now.UTC()
				due = append(due, *t)
			case !t.FiresAt.IsZero() && !now.Before(t.FiresAt):
				// One-shots are removed before they run, exactly like
				// store.Timer: the user already saw it scheduled, and a
				// timer that survives its own failure would fire again
				// every five seconds forever.
				due = append(due, *t)
				delete(s.Store.MusicTimers, id)
				consumed = true
			}
		}
		// Bookkeeping for timers that no longer exist, so the map doesn't
		// grow forever on a long-running install.
		for id := range fired {
			if _, ok := s.Store.MusicTimers[id]; !ok {
				delete(fired, id)
			}
		}
	})

	if consumed || len(due) > 0 {
		if err := s.Store.Update(func() error { return nil }); err != nil {
			s.mediaLogf("music timer: saving: %v", err)
		}
	}
	return due
}

// fireMusicTimer runs one timer. Never called with a lock held.
func (s *Server) fireMusicTimer(ctx context.Context, t store.MusicTimer) {
	label := s.musicTimerLabel(t)
	err := s.runMusicTimer(ctx, t)

	entry := store.ActivityEntry{
		Kind: "music", Source: "music-timer", Action: string(t.Action), Label: label,
	}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
		s.mediaLogf("music timer %s (%s %s): %v", t.ID, t.Action, t.Room, err)
	}
	s.Store.Mutate(func() { s.Store.Activity.Add(entry) })
	s.SpeakersChanged()
}

// runMusicTimer is the timer's actual work, split out so fireMusicTimer owns
// the logging and the activity row and this owns only the music.
func (s *Server) runMusicTimer(ctx context.Context, t store.MusicTimer) error {
	eps, _, err := s.mediaRoom(t.Room)
	if err != nil {
		return err
	}

	// A timer starting or stopping a room supersedes whatever ramp was
	// already running there. Two fades walking the same speakers in
	// opposite directions is the one way this feature can produce a volume
	// nobody asked for.
	fadeCtx := s.beginFade(ctx, t.Room)

	// Whoever ends up owning the ramp releases it: the detached goroutines
	// below defer endFade, and every other path — no fade asked for, or a
	// failure before one could start — releases it here. Missing this leaves
	// the room reported as fading for the life of the process, and its
	// cancel func uncalled with it.
	ramping := false
	defer func() {
		if !ramping {
			s.endFade(t.Room)
		}
	}()

	switch t.Action {
	case store.MusicStart:
		return s.startForTimer(ctx, fadeCtx, t, eps, &ramping)
	case store.MusicStop:
		return s.stopForTimer(ctx, fadeCtx, t, eps, &ramping)
	}
	return fmt.Errorf("music timer: unknown action %q", t.Action)
}

// startForTimer puts the room's music on, coming up to volume if asked. It
// sets *ramping when it hands the room's fade to a detached ramp.
func (s *Server) startForTimer(ctx, fadeCtx context.Context, t store.MusicTimer, eps []media.Endpoint, ramping *bool) error {
	p, err := s.provider(t.Item.Provider)
	if err != nil {
		return err
	}
	if av := p.Available(); !av.OK {
		return errors.New(av.Reason)
	}
	plan, err := media.Resolve(p, eps)
	if err != nil {
		return err
	}

	playCtx, cancel := context.WithTimeout(ctx, mediaTimeout)
	defer cancel()

	fade := time.Duration(t.FadeMinutes) * time.Minute
	if t.Volume != nil {
		// Down to the floor *before* anything plays. Setting the volume
		// after starting is a burst at whatever the room was left at last
		// night, which is precisely the fright a fade exists to avoid.
		start := *t.Volume
		if fade > 0 {
			start = musicFadeFloor(*t.Volume)
		}
		if err := media.SetVolume(playCtx, eps, start); err != nil {
			// Not fatal: a speaker that refuses a volume write can still
			// be handed music, and a wake-up that happens too loudly beats
			// one that doesn't happen.
			s.mediaLogf("music timer: presetting volume in %s: %v", t.Room, err)
		}
	}

	sess, err := media.Play(playCtx, plan, p, media.Item{
		Provider: p.ID(),
		Kind:     media.ItemKind(t.Item.Kind),
		URI:      t.Item.URI,
		Title:    t.Item.Title,
	}, s.Audio.Deps())
	if err != nil {
		return err
	}
	if zoneID, ok := strings.CutPrefix(t.Room, "zone:"); ok {
		s.setZoneSession(zoneID, sess)
	}
	s.touchZone(eps)
	s.recordPlay(t.Room, s.musicRoomName(t.Room), store.MediaPlay{
		Provider: p.ID(),
		Kind:     t.Item.Kind,
		URI:      t.Item.URI,
		Title:    t.Item.Title,
		Sub:      t.Item.Sub,
		ArtURI:   t.Item.ArtURI,
	})

	if t.Volume != nil && fade > 0 {
		*ramping = true
		go s.rampUp(fadeCtx, t, eps)
	}
	return nil
}

// rampUp walks the room from the floor to the timer's volume. Detached, so
// a ten-minute wake-up doesn't hold the tick; failures are logged and not
// otherwise reported, because by this point the music is already playing and
// the only thing left to get wrong is how loud.
func (s *Server) rampUp(ctx context.Context, t store.MusicTimer, eps []media.Endpoint) {
	defer s.endFade(t.Room)
	if _, err := media.Fade(ctx, eps, *t.Volume, time.Duration(t.FadeMinutes)*time.Minute); err != nil {
		if !errors.Is(err, context.Canceled) {
			s.mediaLogf("music timer: fading up %s: %v", t.Room, err)
		}
	}
}

// stopForTimer takes the room down and pauses it, setting *ramping when it
// hands the room's fade to a detached ramp.
//
// Without a fade this is a plain pause and the volume is left exactly as the
// room had it. With one, the room walks down, pauses, and is put back where
// it was — see the package comment on why the restore is the feature and not
// a nicety.
func (s *Server) stopForTimer(ctx, fadeCtx context.Context, t store.MusicTimer, eps []media.Endpoint, ramping *bool) error {
	fade := time.Duration(t.FadeMinutes) * time.Minute
	if fade <= 0 {
		return s.pauseRoom(ctx, t.Room, eps)
	}
	*ramping = true
	go s.rampDownAndPause(fadeCtx, t, eps)
	return nil
}

// rampDownAndPause is the sleep timer proper. Detached for the same reason
// rampUp is: forty minutes is a long time to hold a five-second tick.
func (s *Server) rampDownAndPause(ctx context.Context, t store.MusicTimer, eps []media.Endpoint) {
	defer s.endFade(t.Room)

	floor := 0
	if t.Volume != nil {
		floor = *t.Volume
	}
	before, fadeErr := media.Fade(ctx, eps, floor, time.Duration(t.FadeMinutes)*time.Minute)

	// Restoring uses a fresh context: ctx is very likely the reason we are
	// here — cancelled, or timed out — and putting the volume back is the
	// one step that must happen either way.
	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaTimeout)
	defer cancel()

	if fadeErr != nil {
		// Interrupted: someone called the sleep off, or the process is
		// going down. Put the volume back and leave the music playing —
		// a cancelled sleep timer that silences the room anyway is worse
		// than one that does nothing.
		if err := media.SetVolumes(restoreCtx, eps, before); err != nil {
			s.mediaLogf("music timer: restoring volume in %s: %v", t.Room, err)
		}
		if !errors.Is(fadeErr, context.Canceled) {
			s.mediaLogf("music timer: fading down %s: %v", t.Room, fadeErr)
		}
		return
	}

	if err := s.pauseRoom(restoreCtx, t.Room, eps); err != nil {
		s.mediaLogf("music timer: pausing %s: %v", t.Room, err)
	}
	if err := media.SetVolumes(restoreCtx, eps, before); err != nil {
		s.mediaLogf("music timer: restoring volume in %s: %v", t.Room, err)
	}
	s.touchZone(eps)
}

// pauseRoom stops a room the way its own stop handler would: through the
// route it is actually on, and releasing any stream session it was holding.
func (s *Server) pauseRoom(ctx context.Context, room string, eps []media.Endpoint) error {
	ctx, cancel := context.WithTimeout(ctx, mediaTimeout)
	defer cancel()

	zoneID, isZone := strings.CutPrefix(room, "zone:")
	plan := s.zonePlan(zoneID, eps)
	err := media.Control(ctx, plan, media.TransportPause)
	if isZone {
		// A streamed zone leaves a decoder holding the account's Spotify
		// session; pausing the speakers alone would keep it held all night.
		s.endZoneSession(zoneID)
	}
	s.touchZone(eps)
	return err
}

// ── Fade bookkeeping ─────────────────────────────────────────────────────
//
// One ramp per room at a time. Anything that starts a new one on a room
// cancels the one in flight, which is what stops two fades walking the same
// speakers in opposite directions.

// beginFade cancels any ramp already running on a room and returns the
// context for the new one.
func (s *Server) beginFade(parent context.Context, room string) context.Context {
	ctx, cancel := context.WithCancel(parent)
	s.fadeMu.Lock()
	defer s.fadeMu.Unlock()
	if s.fades == nil {
		s.fades = make(map[string]context.CancelFunc)
	}
	if prev, ok := s.fades[room]; ok {
		prev()
	}
	s.fades[room] = cancel
	return ctx
}

// endFade releases a room's ramp, whether it finished or was cancelled.
func (s *Server) endFade(room string) {
	s.fadeMu.Lock()
	cancel, ok := s.fades[room]
	delete(s.fades, room)
	s.fadeMu.Unlock()
	if ok {
		cancel()
	}
}

// CancelFade stops a ramp in flight. A sleep fade cancelled this way puts the
// volume back and leaves the music playing (see rampDownAndPause), which is
// what "I'm still up" should mean.
func (s *Server) CancelFade(room string) bool {
	s.fadeMu.Lock()
	cancel, ok := s.fades[room]
	s.fadeMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// fadingRooms lists the rooms with a ramp in flight, so the API can say which
// rooms are mid-fade rather than leaving a panel to guess from volume drift.
func (s *Server) fadingRooms() map[string]bool {
	s.fadeMu.Lock()
	defer s.fadeMu.Unlock()
	out := make(map[string]bool, len(s.fades))
	for room := range s.fades {
		out[room] = true
	}
	return out
}

func (s *Server) cancelAllFades() {
	s.fadeMu.Lock()
	fades := s.fades
	s.fades = nil
	s.fadeMu.Unlock()
	for _, cancel := range fades {
		cancel()
	}
}

// ── Naming ───────────────────────────────────────────────────────────────

// musicTimerLabel is what an activity row calls this timer: its own name if
// it was given one, otherwise the room and what it does there.
func (s *Server) musicTimerLabel(t store.MusicTimer) string {
	if name := strings.TrimSpace(t.Name); name != "" {
		return name
	}
	room := s.musicRoomName(t.Room)
	if t.Action == store.MusicStop {
		return room
	}
	if title := strings.TrimSpace(t.Item.Title); title != "" {
		return title + " · " + room
	}
	return room
}

// musicRoomName resolves a destination key to what the house calls it,
// falling back to the key so a row is never blank.
func (s *Server) musicRoomName(key string) string {
	var name string
	s.Store.View(func() {
		if id, ok := strings.CutPrefix(key, "zone:"); ok {
			if z, exists := s.Store.Zones[id]; exists {
				name = z.Name
			}
			return
		}
		bridge, id, ok := store.SplitMember(key)
		if !ok {
			return
		}
		if bridge == "kef" {
			if sp, exists := s.Store.KEF[id]; exists {
				name = sp.Name
			}
			return
		}
		if sp, exists := s.Store.Sonos[id]; exists {
			name = sp.Name
		}
	})
	if name == "" {
		return key
	}
	return name
}
