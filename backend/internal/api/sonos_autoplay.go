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
// A dedicated background tick (RunAutoplay) tops up a coordinator's queue
// the moment it starts playing its last queued track, using AddToQueue —
// which never touches the transport, so what's playing keeps playing
// straight through into the new tracks once it ends. Proactively topping up
// on the last track, rather than waiting for the queue to actually empty,
// is what keeps this gapless without needing to catch a STOPPED transition.

// autoplayThrottle is the minimum gap between "find similar tracks" attempts
// for one coordinator, so a search that keeps failing (no network, no artist
// match) doesn't hammer Spotify every tick.
const autoplayThrottle = 30 * time.Second

// autoplayTopUpTracks is how many similar tracks land in the queue per
// top-up — enough to outlast a lap of the loop before the next one is due.
const autoplayTopUpTracks = 5

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
		if s.autoplay == nil {
			s.autoplay = make(map[string]bool)
		}
		s.autoplay[sp.ID] = true
	} else if s.autoplay != nil {
		delete(s.autoplay, sp.ID)
		delete(s.autoplayRecent, sp.ID)
	}
	s.autoplayMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) autoplayEnabled(id string) bool {
	s.autoplayMu.Lock()
	defer s.autoplayMu.Unlock()
	return s.autoplay[id]
}

// RunAutoplay ticks until ctx is cancelled, topping up any coordinator that
// has autoplay on and is playing the last track in its queue. It rides on
// the scheduler's context like the KEF poller: nothing here needs a clean
// release on shutdown.
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

func (s *Server) autoplayTick(ctx context.Context) {
	s.autoplayMu.Lock()
	enabled := make(map[string]bool, len(s.autoplay))
	for id, on := range s.autoplay {
		if on {
			enabled[id] = true
		}
	}
	s.autoplayMu.Unlock()
	if len(enabled) == 0 {
		return
	}

	var speakers map[string]store.SonosSpeaker
	s.Store.View(func() {
		speakers = make(map[string]store.SonosSpeaker, len(s.Store.Sonos))
		for id, sp := range s.Store.Sonos {
			speakers[id] = *sp
		}
	})

	snapCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	snap := s.sonosEvents().Snapshot(snapCtx)
	cancel()

	for id := range enabled {
		sp, ok := speakers[id]
		if !ok {
			continue
		}
		cached := snap.Speakers[id]
		if cached.State == nil || cached.GroupState == nil {
			continue // not a coordinator, or hasn't answered
		}
		gs, st := cached.GroupState, cached.State
		if !st.Playing || !gs.FromQueue || gs.QueueLength == 0 {
			continue
		}
		if st.QueueTrack != gs.QueueLength {
			continue // not on the last queued track yet
		}
		if st.Track == nil || strings.TrimSpace(st.Track.Artist) == "" {
			continue // nothing to seed similar tracks from
		}
		s.autoplayTopUp(ctx, sp, st.Track.Artist)
	}
}

// autoplayTopUp finds tracks similar to artist and appends them to sp's
// queue. Failures are swallowed — this is a background convenience, not a
// user-initiated action with a place to report an error — and the next tick
// tries again once autoplayThrottle has passed.
func (s *Server) autoplayTopUp(ctx context.Context, sp store.SonosSpeaker, artist string) {
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
	for _, it := range tracks {
		uri, meta, err := sonos.SpotifyItem(it.URI, it.Name, acct)
		if err != nil {
			continue
		}
		if _, err := sonos.AddToQueue(tctx, sp.IP, uri, meta, false); err != nil {
			continue
		}
		added = append(added, it.URI)
	}
	if len(added) == 0 {
		return
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
