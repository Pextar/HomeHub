// Package musictimer runs store.MusicTimer: music that starts and stops on
// its own.
//
// It lives outside the scheduler for the same reason every other speaker call
// does — running a timer is device I/O across a route, and the store has no way
// to reach a bridge. The scheduler's own tick stays about sockets.
//
// Two things make this more than a scheduler that calls Play.
//
// The first is the fade. A jump from silence to twenty-five is an alarm clock;
// the same twenty-five arrived at over ten minutes is being woken by music.
// Fades run detached from the tick, because a ten-minute ramp cannot hold up
// the loop that would notice the next timer.
//
// The second is that a sleep fade puts the volume back. A room faded to nothing
// and paused is a room that is silent the next morning at a volume nobody
// chose, and the person who set the timer at midnight is not the person who
// finds out at breakfast. Restoring is not a nicety here — without it the
// feature is a trap, which is why it happens even when the fade is interrupted.
package musictimer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"homehub/internal/media"
	"homehub/internal/music"
	"homehub/internal/store"
)

// Config is what the engine needs from the rest of the application.
type Config struct {
	// Store holds the timers and the activity log.
	Store *store.Store
	// Music resolves the room, picks the provider and owns the session a
	// started timer may leave running.
	Music *music.Service

	// Changed, if set, is called after a timer runs, so the app sees the
	// room move without waiting for the next poll.
	Changed func()

	Logf func(format string, args ...any)
}

// Engine ticks the timers and owns the fades they start.
type Engine struct {
	cfg Config

	// fades holds the cancel func of each room's in-flight volume ramp,
	// keyed by destination key. One ramp per room: anything starting a new
	// one cancels the old, which is what stops a wake-up fade and a sleep
	// fade from walking the same speakers in opposite directions.
	fadeMu sync.Mutex
	fades  map[string]context.CancelFunc
}

// New returns an engine. It ticks only once Run is called.
func New(cfg Config) *Engine {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	return &Engine{cfg: cfg, fades: map[string]context.CancelFunc{}}
}

func (e *Engine) changed() {
	if e.cfg.Changed != nil {
		e.cfg.Changed()
	}
}

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

// tickInterval matches the socket scheduler's. The (prev, now] window
// below is what actually decides whether a timer is due, so this only sets
// how promptly a due one is noticed.
const tickInterval = 5 * time.Second

// FadeFloor is where a fade-up starts, as a fraction of its target: a
// fifth, never less than 1. Not zero — a wake-up that spends its first
// minutes at literal silence reads as a timer that failed, and someone lying
// awake wondering is worse off than someone hearing the first bar quietly.
func FadeFloor(target int) int {
	if floor := target / 5; floor > 1 {
		return floor
	}
	return 1
}

// Run ticks until ctx is cancelled, firing timers as they come due and
// stopping every ramp on the way out. Spawn it in a goroutine.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
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
			e.cancelAllFades()
			return
		case <-ticker.C:
		}

		now := time.Now()
		stamp := now.Format("2006-01-02 15:04")
		due := e.collectDue(prevTick, now, stamp, fired)
		prevTick = now

		for _, t := range due {
			// Detached: a start can take most of a minute (waking a
			// speaker, a cloud round trip, several UPnP calls) and the
			// tick has to stay free to notice the next timer.
			go e.fire(ctx, t)
		}
	}
}

// collectDue collects the timers that have come due, marks recurring ones
// as fired for this minute and removes the one-shots. Takes the write lock
// once; everything that touches a speaker happens after it is released.
func (e *Engine) collectDue(prev, now time.Time, stamp string, fired map[string]string) []store.MusicTimer {
	var due []store.MusicTimer
	var consumed bool

	e.cfg.Store.Mutate(func() {
		for id, t := range e.cfg.Store.MusicTimers {
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
				delete(e.cfg.Store.MusicTimers, id)
				consumed = true
			}
		}
		// Bookkeeping for timers that no longer exist, so the map doesn't
		// grow forever on a long-running install.
		for id := range fired {
			if _, ok := e.cfg.Store.MusicTimers[id]; !ok {
				delete(fired, id)
			}
		}
	})

	if consumed || len(due) > 0 {
		if err := e.cfg.Store.Update(func() error { return nil }); err != nil {
			e.cfg.Logf("music timer: saving: %v", err)
		}
	}
	return due
}

// fire runs one timer. Never called with a lock held.
func (e *Engine) fire(ctx context.Context, t store.MusicTimer) {
	err := e.run(ctx, t)

	entry := store.ActivityEntry{
		Kind: "music", Source: "music-timer", Action: string(t.Action), Label: e.label(t),
	}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
		e.cfg.Logf("music timer %s (%s %s): %v", t.ID, t.Action, t.Room, err)
	}
	e.cfg.Store.Mutate(func() { e.cfg.Store.Activity.Add(entry) })
	e.changed()
}

// run is the timer's actual work, split out so fire owns the logging and the
// activity row and this owns only the music.
func (e *Engine) run(ctx context.Context, t store.MusicTimer) error {
	eps, _, err := e.cfg.Music.Room(t.Room)
	if err != nil {
		return err
	}

	// A timer starting or stopping a room supersedes whatever ramp was
	// already running there. Two fades walking the same speakers in
	// opposite directions is the one way this feature can produce a volume
	// nobody asked for.
	fadeCtx := e.beginFade(ctx, t.Room)

	// Whoever ends up owning the ramp releases it: the detached goroutines
	// below defer endFade, and every other path — no fade asked for, or a
	// failure before one could start — releases it here. Missing this leaves
	// the room reported as fading for the life of the process, and its
	// cancel func uncalled with it.
	ramping := false
	defer func() {
		if !ramping {
			e.endFade(t.Room)
		}
	}()

	switch t.Action {
	case store.MusicStart:
		return e.start(ctx, fadeCtx, t, eps, &ramping)
	case store.MusicStop:
		return e.stop(ctx, fadeCtx, t, eps, &ramping)
	}
	return fmt.Errorf("music timer: unknown action %q", t.Action)
}

// start puts the room's music on, coming up to volume if asked. It
// sets *ramping when it hands the room's fade to a detached ramp.
func (e *Engine) start(ctx, fadeCtx context.Context, t store.MusicTimer, eps []media.Endpoint, ramping *bool) error {
	p, err := e.cfg.Music.Provider(t.Item.Provider)
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

	playCtx, cancel := context.WithTimeout(ctx, music.Timeout)
	defer cancel()

	fade := time.Duration(t.FadeMinutes) * time.Minute
	if t.Volume != nil {
		// Down to the floor *before* anything plays. Setting the volume
		// after starting is a burst at whatever the room was left at last
		// night, which is precisely the fright a fade exists to avoid.
		start := *t.Volume
		if fade > 0 {
			start = FadeFloor(*t.Volume)
		}
		if err := media.SetVolume(playCtx, eps, start); err != nil {
			// Not fatal: a speaker that refuses a volume write can still
			// be handed music, and a wake-up that happens too loudly beats
			// one that doesn't happen.
			e.cfg.Logf("music timer: presetting volume in %s: %v", t.Room, err)
		}
	}

	sess, err := media.Play(playCtx, plan, p, media.Item{
		Provider: p.ID(),
		Kind:     media.ItemKind(t.Item.Kind),
		URI:      t.Item.URI,
		Title:    t.Item.Title,
	}, e.cfg.Music.Deps())
	if err != nil {
		return err
	}
	if zoneID, ok := strings.CutPrefix(t.Room, "zone:"); ok {
		e.cfg.Music.SetSession(zoneID, sess)
	}
	e.cfg.Music.Touch(eps)
	e.cfg.Music.RecordPlay(t.Room, e.cfg.Music.RoomName(t.Room), store.MediaPlay{
		Provider: p.ID(),
		Kind:     t.Item.Kind,
		URI:      t.Item.URI,
		Title:    t.Item.Title,
		Sub:      t.Item.Sub,
		ArtURI:   t.Item.ArtURI,
	})

	if t.Volume != nil && fade > 0 {
		*ramping = true
		go e.rampUp(fadeCtx, t, eps)
	}
	return nil
}

// rampUp walks the room from the floor to the timer's volume. Detached, so
// a ten-minute wake-up doesn't hold the tick; failures are logged and not
// otherwise reported, because by this point the music is already playing and
// the only thing left to get wrong is how loud.
func (e *Engine) rampUp(ctx context.Context, t store.MusicTimer, eps []media.Endpoint) {
	defer e.endFade(t.Room)
	if _, err := media.Fade(ctx, eps, *t.Volume, time.Duration(t.FadeMinutes)*time.Minute); err != nil {
		if !errors.Is(err, context.Canceled) {
			e.cfg.Logf("music timer: fading up %s: %v", t.Room, err)
		}
	}
}

// stop takes the room down and pauses it, setting *ramping when it
// hands the room's fade to a detached ramp.
//
// Without a fade this is a plain pause and the volume is left exactly as the
// room had it. With one, the room walks down, pauses, and is put back where
// it was — see the package comment on why the restore is the feature and not
// a nicety.
func (e *Engine) stop(ctx, fadeCtx context.Context, t store.MusicTimer, eps []media.Endpoint, ramping *bool) error {
	fade := time.Duration(t.FadeMinutes) * time.Minute
	if fade <= 0 {
		return e.cfg.Music.Pause(ctx, t.Room, eps)
	}
	*ramping = true
	go e.rampDownAndPause(fadeCtx, t, eps)
	return nil
}

// rampDownAndPause is the sleep timer proper. Detached for the same reason
// rampUp is: forty minutes is a long time to hold a five-second tick.
func (e *Engine) rampDownAndPause(ctx context.Context, t store.MusicTimer, eps []media.Endpoint) {
	defer e.endFade(t.Room)

	floor := 0
	if t.Volume != nil {
		floor = *t.Volume
	}
	before, fadeErr := media.Fade(ctx, eps, floor, time.Duration(t.FadeMinutes)*time.Minute)

	// Restoring uses a fresh context: ctx is very likely the reason we are
	// here — cancelled, or timed out — and putting the volume back is the
	// one step that must happen either way.
	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), music.Timeout)
	defer cancel()

	if fadeErr != nil {
		// Interrupted: someone called the sleep off, or the process is
		// going down. Put the volume back and leave the music playing —
		// a cancelled sleep timer that silences the room anyway is worse
		// than one that does nothing.
		if err := media.SetVolumes(restoreCtx, eps, before); err != nil {
			e.cfg.Logf("music timer: restoring volume in %s: %v", t.Room, err)
		}
		if !errors.Is(fadeErr, context.Canceled) {
			e.cfg.Logf("music timer: fading down %s: %v", t.Room, fadeErr)
		}
		return
	}

	if err := e.cfg.Music.Pause(restoreCtx, t.Room, eps); err != nil {
		e.cfg.Logf("music timer: pausing %s: %v", t.Room, err)
	}
	if err := media.SetVolumes(restoreCtx, eps, before); err != nil {
		e.cfg.Logf("music timer: restoring volume in %s: %v", t.Room, err)
	}
	e.cfg.Music.Touch(eps)
}

// ── Fade bookkeeping ─────────────────────────────────────────────────────
//
// One ramp per room at a time. Anything that starts a new one on a room
// cancels the one in flight, which is what stops two fades walking the same
// speakers in opposite directions.

// beginFade cancels any ramp already running on a room and returns the
// context for the new one.
func (e *Engine) beginFade(parent context.Context, room string) context.Context {
	ctx, cancel := context.WithCancel(parent)
	e.fadeMu.Lock()
	defer e.fadeMu.Unlock()
	if prev, ok := e.fades[room]; ok {
		prev()
	}
	e.fades[room] = cancel
	return ctx
}

// endFade releases a room's ramp, whether it finished or was cancelled.
func (e *Engine) endFade(room string) {
	e.fadeMu.Lock()
	cancel, ok := e.fades[room]
	delete(e.fades, room)
	e.fadeMu.Unlock()
	if ok {
		cancel()
	}
}

// CancelFade stops a ramp in flight. A sleep fade cancelled this way puts the
// volume back and leaves the music playing (see rampDownAndPause), which is
// what "I'm still up" should mean.
func (e *Engine) CancelFade(room string) bool {
	e.fadeMu.Lock()
	cancel, ok := e.fades[room]
	e.fadeMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// FadingRooms lists the rooms with a ramp in flight, so the API can say which
// rooms are mid-fade rather than leaving a panel to guess from volume drift.
func (e *Engine) FadingRooms() map[string]bool {
	e.fadeMu.Lock()
	defer e.fadeMu.Unlock()
	out := make(map[string]bool, len(e.fades))
	for room := range e.fades {
		out[room] = true
	}
	return out
}

func (e *Engine) cancelAllFades() {
	e.fadeMu.Lock()
	fades := e.fades
	e.fades = map[string]context.CancelFunc{}
	e.fadeMu.Unlock()
	for _, cancel := range fades {
		cancel()
	}
}

// ── Naming ───────────────────────────────────────────────────────────────

// label is what an activity row calls this timer: its own name if
// it was given one, otherwise the room and what it does there.
func (e *Engine) label(t store.MusicTimer) string {
	if name := strings.TrimSpace(t.Name); name != "" {
		return name
	}
	room := e.cfg.Music.RoomName(t.Room)
	if t.Action == store.MusicStop {
		return room
	}
	if title := strings.TrimSpace(t.Item.Title); title != "" {
		return title + " · " + room
	}
	return room
}
