// Package autoplay keeps a room from running out of music.
//
// "Continue with similar music" once a group's queue runs out — the
// auto-continuing counterpart to a manual "play similar" button. Sonos has no
// such concept, so HomeHub keeps the setting itself, in memory, keyed by the
// coordinator's registered speaker id. It does not survive a restart, the same
// way the rest of a group's live playback state doesn't.
//
// It is on unless a room says otherwise: starting a song is a request for
// music, not for exactly that song and then silence, so the queue keeps topping
// itself up until someone turns it off. What is remembered is therefore the
// opt-out, and a restart lands back on "keep playing" rather than on silence.
//
// The tick tops a coordinator's queue up the moment it starts playing its last
// queued track, using AddToQueue — which never touches the transport, so what
// is playing keeps playing straight through into the new tracks once it ends.
// Topping up proactively on the last track, rather than waiting for the queue
// to empty, is what keeps this gapless without needing to catch a STOPPED
// transition.
//
// The tick catches that transition anyway, as the net under the trapeze: a room
// that fell silent at the end of its queue — because the top-up failed, or
// because the hub restarted mid-song — gets topped up and started again. Only
// from STOPPED, only at the end of the queue, and only while the room was
// playing recently, so a pause stays a pause.
package autoplay

import (
	"context"
	"strings"
	"sync"
	"time"

	"homehub/internal/sonos"
	"homehub/internal/speakermon"
	"homehub/internal/spotify"
	"homehub/internal/store"
)

const (
	// tickInterval is how often the household is looked at. Short enough to
	// catch the last track before it ends, long enough to be free.
	tickInterval = 15 * time.Second

	// throttle is the minimum gap between "find similar tracks" attempts for
	// one coordinator, so a search that keeps failing (no network, no artist
	// match) doesn't hammer the catalogue every tick.
	throttle = 30 * time.Second

	// topUpTracks is how many similar tracks land in the queue per top-up —
	// enough to outlast a lap of the loop before the next one is due.
	topUpTracks = 5

	// resumeWindow is how long after a coordinator was last heard playing its
	// queue that finding it stopped still reads as "the queue just ran dry".
	// Past it the room is simply quiet, and quiet rooms are left alone —
	// nobody wants music starting itself hours later.
	resumeWindow = 15 * time.Minute

	// recentMemory is how many recently-queued URIs are remembered per
	// coordinator, so a short discography doesn't loop the same handful of
	// songs back around.
	recentMemory = 40

	// snapshotBudget and topUpBudget bound the two round trips a tick makes.
	snapshotBudget = 10 * time.Second
	topUpBudget    = 15 * time.Second
)

// SimilarFinder finds tracks like an artist's, excluding what a room has just
// heard. Implemented by *spotify.Client; an interface here so that a house
// with no Spotify simply has no autoplay rather than a nil check at every use.
type SimilarFinder interface {
	SimilarTracks(ctx context.Context, artist string, exclude map[string]bool, limit int) ([]spotify.Item, error)
}

// Config is what the engine needs from the rest of the application.
type Config struct {
	// Store holds the registered speakers.
	Store *store.Store
	// Speakers is the cached view of what they are doing, and where the
	// service account for queueing comes from.
	Speakers *speakermon.Monitors
	// Similar seeds the top-up. Nil disables autoplay entirely — there is
	// nothing to continue *with*.
	Similar SimilarFinder

	// Observed, if set, receives each tick's household snapshot. The tick
	// reads the whole house anyway, which makes it the listening log's
	// fallback for a household whose speakers refuse GENA subscriptions.
	Observed func(sonos.Snapshot)

	// Changed, if set, is called after the queue is altered. Without it the
	// new tracks would not appear until the next status poll.
	Changed func()
}

// Engine is the per-coordinator memory and the loop that uses it.
type Engine struct {
	cfg Config

	mu sync.Mutex
	// off is the opt-out — see the package comment on why the absence of an
	// entry means "keep playing".
	off map[string]bool
	// attempt throttles retries when finding similar tracks keeps failing.
	attempt map[string]time.Time
	// recent is what a coordinator was just topped up with.
	recent map[string][]string
	// heard is when each coordinator was last seen actually playing its
	// queue — what separates "the queue just ran dry" from "this room has
	// been quiet all evening".
	heard map[string]time.Time
}

// New returns an engine. It ticks only once Run is called.
func New(cfg Config) *Engine {
	return &Engine{
		cfg:     cfg,
		off:     map[string]bool{},
		attempt: map[string]time.Time{},
		recent:  map[string][]string{},
		heard:   map[string]time.Time{},
	}
}

// Enabled reports whether a coordinator tops its queue up.
func (e *Engine) Enabled(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.off[id]
}

// SetEnabled turns autoplay on or off for one coordinator.
//
// Turning it off also forgets what that room was topped up with and when it
// was last heard: both exist only to serve the next top-up, and keeping them
// would mean a room switched back on hours later inherits a stale idea of
// whether its queue "just" ran dry.
func (e *Engine) SetEnabled(id string, on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if on {
		delete(e.off, id)
		return
	}
	e.off[id] = true
	delete(e.recent, id)
	delete(e.heard, id)
}

// Run ticks until ctx is cancelled, keeping every coordinator that hasn't
// opted out from running out of music.
//
// Nothing here needs a clean release on shutdown: the queue changes are the
// speaker's to keep, and an abandoned tick leaves no subscription behind.
func (e *Engine) Run(ctx context.Context) {
	if e.cfg.Similar == nil {
		return // nothing to seed similar tracks from
	}
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.tick(ctx)
		}
	}
}

// action is what one tick decides to do about one coordinator.
type action int

const (
	idle     action = iota // leave it alone
	appendTo               // playing its last track: add more behind it
	restart                // stopped with the queue spent: add more and play
)

// decide reads a coordinator's live state. heardRecently is whether the room
// was seen playing its queue inside resumeWindow; it only matters for the
// restart case, which must not fire for a group that was already sitting
// stopped when HomeHub first laid eyes on it.
func decide(st *sonos.State, gs *sonos.GroupState, heardRecently bool) action {
	if st == nil || gs == nil {
		return idle // not a coordinator, or it hasn't answered
	}
	if !gs.FromQueue || gs.QueueLength == 0 {
		return idle // radio, line-in, TV: no queue to top up
	}
	if st.Playing {
		if st.QueueTrack != gs.QueueLength {
			return idle // not on the last queued track yet
		}
		return appendTo
	}
	// Silent. A pause is a decision and stays one — only a queue that ran
	// off its own end gets picked back up.
	if st.TransportState != "STOPPED" || st.QueueTrack < gs.QueueLength {
		return idle
	}
	if !heardRecently {
		return idle
	}
	return restart
}

func (e *Engine) tick(ctx context.Context) {
	speakers := store.ViewValue(e.cfg.Store, func() []store.SonosSpeaker {
		out := make([]store.SonosSpeaker, 0, len(e.cfg.Store.Sonos))
		for _, sp := range e.cfg.Store.Sonos {
			out = append(out, *sp)
		}
		return out
	})
	if len(speakers) == 0 {
		return
	}

	snapCtx, cancel := context.WithTimeout(ctx, snapshotBudget)
	snap := e.cfg.Speakers.Sonos.Snapshot(snapCtx)
	cancel()
	if e.cfg.Observed != nil {
		e.cfg.Observed(snap)
	}

	for _, sp := range speakers {
		if !e.Enabled(sp.ID) {
			continue
		}
		cached := snap.Speakers[sp.ID]
		st, gs := cached.State, cached.GroupState
		if st != nil && gs != nil && st.Playing && gs.FromQueue {
			e.noteHeard(sp.ID)
		}
		act := decide(st, gs, e.heardRecently(sp.ID))
		if act == idle {
			continue
		}
		if st.Track == nil || strings.TrimSpace(st.Track.Artist) == "" {
			continue // nothing to seed similar tracks from
		}
		e.topUp(ctx, sp, st.Track.Artist, act == restart)
	}
}

// noteHeard records that a coordinator is playing its queue right now.
func (e *Engine) noteHeard(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.heard[id] = time.Now()
}

func (e *Engine) heardRecently(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	at, ok := e.heard[id]
	return ok && time.Since(at) < resumeWindow
}

// topUp finds tracks similar to artist and appends them to sp's queue; with
// restart it also starts the group on the first of them, which is what a room
// that already fell silent needs.
//
// Failures are swallowed — this is a background convenience, not a
// user-initiated action with a place to report an error — and the next tick
// tries again once the throttle has passed.
func (e *Engine) topUp(ctx context.Context, sp store.SonosSpeaker, artist string, restartQueue bool) {
	e.mu.Lock()
	if time.Since(e.attempt[sp.ID]) < throttle {
		e.mu.Unlock()
		return
	}
	e.attempt[sp.ID] = time.Now()
	recent := append([]string(nil), e.recent[sp.ID]...)
	e.mu.Unlock()

	tctx, cancel := context.WithTimeout(ctx, topUpBudget)
	defer cancel()

	exclude := make(map[string]bool, len(recent))
	for _, u := range recent {
		exclude[u] = true
	}
	tracks, err := e.cfg.Similar.SimilarTracks(tctx, artist, exclude, topUpTracks)
	if err != nil || len(tracks) == 0 {
		return
	}
	acct, err := e.cfg.Speakers.ServiceAccount(tctx, sp.IP, "Spotify")
	if err != nil {
		return
	}

	added := make([]string, 0, len(tracks))
	first := 0 // queue position of the first track we managed to add
	for _, it := range tracks {
		uri, meta, err := sonos.SpotifyItem(it.URI, it.Name, acct)
		if err != nil {
			continue
		}
		add, err := sonos.AddToQueue(tctx, sp.IP, uri, meta, false)
		if err != nil {
			continue
		}
		if first == 0 && add != nil {
			first = add.Track
		}
		added = append(added, it.URI)
	}
	if len(added) == 0 {
		return
	}

	if restartQueue && first > 0 {
		// The transport is parked at the end of a spent queue, so resuming
		// would replay the last track: name the slot instead. Failure leaves
		// the tracks queued, which is what the next tick would have added
		// anyway.
		if err := sonos.PlayFromQueue(tctx, sp.IP, sp.UUID, first); err == nil {
			e.noteHeard(sp.ID)
		}
	}

	e.mu.Lock()
	next := append(e.recent[sp.ID], added...)
	if len(next) > recentMemory {
		next = next[len(next)-recentMemory:]
	}
	e.recent[sp.ID] = next
	e.mu.Unlock()

	if e.cfg.Changed != nil {
		e.cfg.Changed()
	}
}
