package api

import (
	"net/http"
	"strconv"
	"strings"

	"homehub/internal/store"
)

// The HTTP face of the listening log. What gets written into it, and when,
// is internal/listening; store/heard.go is where it is kept.

// ── The API ──────────────────────────────────────────────────────────────

// mediaHeard handles GET /api/media/heard?room=sonos:abc&limit=40 — what one
// room has been heard playing, newest first.
//
// A room with nothing of its own answers with the household's, flagged, the
// same way the play shelves do: on the day a speaker is added, "what this
// house has been listening to" beats an empty screen, and every row names the
// room it was heard in so nothing can imply this room played it.
func (s *Server) mediaHeard(w http.ResponseWriter, r *http.Request) {
	limit := 40
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= store.HeardLogSize {
		limit = n
	}
	room := strings.TrimSpace(r.URL.Query().Get("room"))

	var tracks []store.HeardTrack
	var household bool
	s.Store.View(func() {
		tracks = s.Store.HeardIn(room)
		if len(tracks) == 0 {
			tracks = s.Store.RecentHeard(limit)
			household = len(tracks) > 0
		}
	})
	if len(tracks) > limit {
		tracks = tracks[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tracks":    tracks,
		"household": household,
	})
}

// mediaForgetHeard handles DELETE /api/media/heard?room=sonos:abc — one room
// stops remembering what it played.
//
// Whole-room only, unlike the play history's per-URI forget: nothing ranks
// this list or offers it back, so a single wrong row costs one line on a
// screen someone asked to open, and "stop keeping this" about a log of what
// was audible in a room is a room-sized wish.
//
// Answers 204 whether or not there was anything, for the same reason the play
// history does: the caller's goal is a state, not a deletion.
func (s *Server) mediaForgetHeard(w http.ResponseWriter, r *http.Request) {
	room := strings.TrimSpace(r.URL.Query().Get("room"))
	if room == "" {
		writeError(w, http.StatusBadRequest, "room is required")
		return
	}
	var changed bool
	s.Store.Mutate(func() { changed = s.Store.ForgetHeard(room) })
	// The watch goes with it, or the track playing right now would never be
	// re-recorded: the recorder would still believe it had filed it.
	s.Listening.Forget(room)
	if changed {
		if err := s.Store.SaveHeard(); err != nil {
			s.mediaLogf("heard: %v", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
