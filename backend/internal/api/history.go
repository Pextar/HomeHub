package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"homehub/internal/store"
)

// What each room has been asked to play (store/history.go), recorded here
// because this is the layer that knows a play succeeded: every surface that
// starts music does it through one of the four handlers below, and a play
// that the speaker refused is not something to offer again from a shelf.
//
// The record is written after the device has accepted it and never while a
// lock is held for I/O, following the same rule the rest of the store does.

// playSuffix is the extra body every play handler accepts purely so the
// history has something worth showing. None of it reaches the speaker: a
// Sonos play needs a URI and a title, but a shelf tile needs a picture and a
// second line, and asking the catalog for them again later would mean a
// service round-trip to redraw a row we already had in hand.
type playSuffix struct {
	Kind   string `json:"kind"`
	Sub    string `json:"sub"`
	ArtURI string `json:"art_uri"`
}

// recordPlay files one play under a destination key. Takes the write lock
// briefly, then persists off-lock — history is never worth failing a play
// that already happened, so a write error is logged and swallowed.
func (s *Server) recordPlay(roomKey, roomName string, p store.MediaPlay) {
	if strings.TrimSpace(roomKey) == "" || strings.TrimSpace(p.URI) == "" {
		return
	}
	p.RoomName = roomName
	s.Store.Mutate(func() { s.Store.RecordPlay(roomKey, p) })
	// Mutate rather than Update: Update pairs a mutation with a full Save,
	// and history has its own file precisely so that starting a song does
	// not rewrite every socket in the house.
	if err := s.Store.SaveHistory(); err != nil {
		s.mediaLogf("history: %v", err)
	}
}

// mediaHistory handles GET /api/media/history?room=sonos:abc&limit=8 — one
// room's plays, newest first. A room with none of its own falls back to the
// household's, which is what makes this useful on the day a speaker is
// added: an empty shelf is worse than the house's own recent listening, and
// both are honest about what they are because the room is named on each row.
func (s *Server) mediaHistory(w http.ResponseWriter, r *http.Request) {
	limit := 12
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= store.MediaHistorySize {
		limit = n
	}
	room := strings.TrimSpace(r.URL.Query().Get("room"))

	var plays []store.MediaPlay
	var fallback bool
	s.Store.View(func() {
		plays = s.Store.History(room)
		if len(plays) == 0 {
			plays = s.Store.RecentPlays(limit)
			fallback = len(plays) > 0
		}
	})

	if len(plays) > limit {
		plays = plays[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plays": plays,
		// Whether these are this room's own plays or the household's. The
		// shelf says "Played here" for the first and "Played recently" for
		// the second, because a wall must never imply a room played
		// something it didn't.
		"household": fallback,
	})
}

// mediaTopPlays handles GET /api/media/history/top?room=&limit=&hour= — what
// a room keeps coming back to, rather than what it happened to play last.
//
// hour is optional and takes a local hour (0–23) or "now". With it the answer
// is ranked by what this room plays *at that hour*, which is the difference
// between offering the kitchen its breakfast radio at eight in the morning
// and offering it last night's dinner record. A room with no habit at that
// hour falls back to its overall favourites rather than answering nothing,
// and the response says which of the two it gave.
func (s *Server) mediaTopPlays(w http.ResponseWriter, r *http.Request) {
	limit := 8
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= store.MediaHistorySize {
		limit = n
	}
	room := strings.TrimSpace(r.URL.Query().Get("room"))

	hour, byHour := parseHour(r.URL.Query().Get("hour"))
	var plays []store.MediaPlay
	matchedHour := false
	s.Store.View(func() {
		if byHour {
			plays = s.Store.PlaysAtHour(room, hour, limit)
			matchedHour = len(plays) > 0
		}
		if len(plays) == 0 {
			plays = s.Store.TopPlays(room, limit)
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"plays": plays,
		// Whether these are this room's habit at the asked-for hour or its
		// favourites overall. The shelf's own label depends on it, and a
		// wall that says "you play this now" about a record this room has
		// never played in the morning is exactly the kind of confident
		// wrongness DESIGN.md §15 rules out.
		"by_hour": matchedHour,
		"hour":    hour,
	})
}

// parseHour reads the hour query parameter: a local hour 0–23, or "now" for
// the hour it currently is. Anything else asks for no hour at all.
func parseHour(raw string) (hour int, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if strings.EqualFold(raw, "now") {
		return time.Now().Hour(), true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 23 {
		return 0, false
	}
	return n, true
}

// mediaInsights handles GET /api/media/insights?limit= — what the household
// listens to, summed over every room.
//
// Deliberately not per-room: the per-room questions are already answered by
// the two handlers above, and what this adds is the one picture none of them
// can give — which rooms do the listening, which artists the house keeps
// coming back to, and when in the day it is loud.
func (s *Server) mediaInsights(w http.ResponseWriter, r *http.Request) {
	limit := 8
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= store.MediaHistorySize {
		limit = n
	}
	var summary store.Listening
	s.Store.View(func() { summary = s.Store.Summarise(limit) })
	writeJSON(w, http.StatusOK, summary)
}

// pruneHistory drops history for destinations that no longer exist. Called
// after a speaker or zone is deleted, mirroring CascadeDeleteSocket's
// promise that nothing outlives the thing it referenced.
//
// Caller must not hold Mu.
func (s *Server) pruneHistory() {
	var dropped bool
	s.Store.Mutate(func() {
		live := make(map[string]bool, len(s.Store.Sonos)+len(s.Store.KEF)+len(s.Store.Zones))
		for id := range s.Store.Sonos {
			live["sonos:"+id] = true
		}
		for id := range s.Store.KEF {
			live["kef:"+id] = true
		}
		for id := range s.Store.Zones {
			live["zone:"+id] = true
		}
		dropped = s.Store.PruneHistory(func(key string) bool { return live[key] })
	})
	if dropped {
		if err := s.Store.SaveHistory(); err != nil {
			s.mediaLogf("history: %v", err)
		}
	}
}
