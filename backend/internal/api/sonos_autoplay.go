package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"homehub/internal/sonos"
	"homehub/internal/store"
)

// Autoplay: "continue with similar music" once a group's queue runs out —
// the auto-continuing counterpart to a manual "play similar" button. Sonos
// has no such concept, so HomeHub keeps the setting itself, in memory, keyed
// by the coordinator's registered speaker id — it does not survive a
// restart, the same way the rest of a group's live playback state doesn't.
//
// It is on unless a room says otherwise: starting a song is a request for
// music, not for exactly that song and then silence, so the queue keeps
// topping itself up until someone turns it off. What is remembered is
// therefore the opt-out, and a restart lands back on "keep playing" rather
// than on silence.
//
// A dedicated background tick (RunAutoplay) tops up a coordinator's queue
// the moment it starts playing its last queued track, using AddToQueue —
// which never touches the transport, so what's playing keeps playing
// straight through into the new tracks once it ends. Proactively topping up
// on the last track, rather than waiting for the queue to actually empty,
// is what keeps this gapless without needing to catch a STOPPED transition.
//
// The tick catches that transition anyway, as the net under the trapeze: a
// room that fell silent at the end of its queue — because the top-up failed,
// or because the hub restarted mid-song — gets topped up and started again.
// Only from STOPPED, only at the end of the queue, and only while the room
// was playing recently, so a pause stays a pause.

// autoplayThrottle is the minimum gap between "find similar tracks" attempts
// for one coordinator, so a search that keeps failing (no network, no artist
// match) doesn't hammer Spotify every tick.
const autoplayThrottle = 30 * time.Second

// autoplayTopUpTracks is how many similar tracks land in the queue per
// top-up — enough to outlast a lap of the loop before the next one is due.
const autoplayTopUpTracks = 5

// autoplayResumeWindow is how long after a coordinator was last heard
// playing its queue that finding it stopped still reads as "the queue just
// ran dry". Past it, the room is simply quiet, and quiet rooms are left
// alone — nobody wants music starting itself hours later.
const autoplayResumeWindow = 15 * time.Minute

// sonosSetAutoplay handles PUT /api/sonos/{id}/autoplay with
// {"enabled": bool}. id must be a group's coordinator — the same requirement
// every other group-level control here has.
func (s *Server) sonosSetAutoplay(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	s.autoplayMu.Lock()
	if body.Enabled {
		delete(s.autoplayOff, sp.ID)
	} else {
		if s.autoplayOff == nil {
			s.autoplayOff = make(map[string]bool)
		}
		s.autoplayOff[sp.ID] = true
		delete(s.autoplayRecent, sp.ID)
		delete(s.autoplayHeard, sp.ID)
	}
	s.autoplayMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) autoplayEnabled(id string) bool {
	s.autoplayMu.Lock()
	defer s.autoplayMu.Unlock()
	return !s.autoplayOff[id]
}

// RunAutoplay ticks until ctx is cancelled, keeping every coordinator that
// hasn't opted out from running out of music. It rides on the scheduler's
// context like the KEF poller: nothing here needs a clean release on
// shutdown.
func (s *Server) RunAutoplay(ctx context.Context) {
	if s.Spotify == nil {
		return // nothing to seed similar tracks from
	}
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.autoplayTick(ctx)
		}
	}
}

// autoplayAction is what one tick decides to do about one coordinator.
type autoplayAction int

const (
	autoplayIdle    autoplayAction = iota // leave it alone
	autoplayAppend                        // playing its last track: add more behind it
	autoplayRestart                       // stopped with the queue spent: add more and play
)

// autoplayDecide reads a coordinator's live state. heardRecently is whether
// the room was seen playing its queue inside autoplayResumeWindow; it only
// matters for the restart case, which must not fire for a group that was
// already sitting stopped when HomeHub first laid eyes on it.
func autoplayDecide(st *sonos.State, gs *sonos.GroupState, heardRecently bool) autoplayAction {
	if st == nil || gs == nil {
		return autoplayIdle // not a coordinator, or it hasn't answered
	}
	if !gs.FromQueue || gs.QueueLength == 0 {
		return autoplayIdle // radio, line-in, TV: no queue to top up
	}
	if st.Playing {
		if st.QueueTrack != gs.QueueLength {
			return autoplayIdle // not on the last queued track yet
		}
		return autoplayAppend
	}
	// Silent. A pause is a decision and stays one — only a queue that ran
	// off its own end gets picked back up.
	if st.TransportState != "STOPPED" || st.QueueTrack < gs.QueueLength {
		return autoplayIdle
	}
	if !heardRecently {
		return autoplayIdle
	}
	return autoplayRestart
}

func (s *Server) autoplayTick(ctx context.Context) {
	var speakers []store.SonosSpeaker
	s.Store.View(func() {
		speakers = make([]store.SonosSpeaker, 0, len(s.Store.Sonos))
		for _, sp := range s.Store.Sonos {
			speakers = append(speakers, *sp)
		}
	})
	if len(speakers) == 0 {
		return
	}

	snapCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	snap := s.sonosEvents().Snapshot(snapCtx)
	cancel()

	for _, sp := range speakers {
		if !s.autoplayEnabled(sp.ID) {
			continue
		}
		cached := snap.Speakers[sp.ID]
		st, gs := cached.State, cached.GroupState
		if st != nil && gs != nil && st.Playing && gs.FromQueue {
			s.autoplayHeardNow(sp.ID)
		}
		action := autoplayDecide(st, gs, s.autoplayHeardRecently(sp.ID))
		if action == autoplayIdle {
			continue
		}
		if st.Track == nil || strings.TrimSpace(st.Track.Artist) == "" {
			continue // nothing to seed similar tracks from
		}
		s.autoplayTopUp(ctx, sp, st.Track.Artist, action == autoplayRestart)
	}
}

// autoplayHeardNow records that a coordinator is playing its queue right now.
func (s *Server) autoplayHeardNow(id string) {
	s.autoplayMu.Lock()
	defer s.autoplayMu.Unlock()
	if s.autoplayHeard == nil {
		s.autoplayHeard = make(map[string]time.Time)
	}
	s.autoplayHeard[id] = time.Now()
}

func (s *Server) autoplayHeardRecently(id string) bool {
	s.autoplayMu.Lock()
	defer s.autoplayMu.Unlock()
	at, ok := s.autoplayHeard[id]
	return ok && time.Since(at) < autoplayResumeWindow
}

// autoplayTopUp finds tracks similar to artist and appends them to sp's
// queue; with restart it also starts the group on the first of them, which
// is what a room that already fell silent needs. Failures are swallowed —
// this is a background convenience, not a user-initiated action with a place
// to report an error — and the next tick tries again once autoplayThrottle
// has passed.
func (s *Server) autoplayTopUp(ctx context.Context, sp store.SonosSpeaker, artist string, restart bool) {
	s.autoplayMu.Lock()
	if time.Since(s.autoplayAttempt[sp.ID]) < autoplayThrottle {
		s.autoplayMu.Unlock()
		return
	}
	if s.autoplayAttempt == nil {
		s.autoplayAttempt = make(map[string]time.Time)
	}
	s.autoplayAttempt[sp.ID] = time.Now()
	recent := append([]string(nil), s.autoplayRecent[sp.ID]...)
	s.autoplayMu.Unlock()

	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	exclude := make(map[string]bool, len(recent))
	for _, u := range recent {
		exclude[u] = true
	}
	tracks, err := s.Spotify.SimilarTracks(tctx, artist, exclude, autoplayTopUpTracks)
	if err != nil || len(tracks) == 0 {
		return
	}
	acct, err := s.sonosServiceAccount(tctx, sp.IP, "Spotify")
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

	if restart && first > 0 {
		// The transport is parked at the end of a spent queue, so resuming
		// would replay the last track: name the slot instead. Failure leaves
		// the tracks queued, which is what the next tick would have added
		// anyway.
		if err := sonos.PlayFromQueue(tctx, sp.IP, sp.UUID, first); err == nil {
			s.autoplayHeardNow(sp.ID)
		}
	}

	s.autoplayMu.Lock()
	if s.autoplayRecent == nil {
		s.autoplayRecent = make(map[string][]string)
	}
	next := append(s.autoplayRecent[sp.ID], added...)
	if len(next) > 40 {
		next = next[len(next)-40:]
	}
	s.autoplayRecent[sp.ID] = next
	s.autoplayMu.Unlock()

	// The new tracks won't show up until the next status poll otherwise —
	// same reasoning as every other change that alters a group's queue.
	s.broadcastMusic()
}
