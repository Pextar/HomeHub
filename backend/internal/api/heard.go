package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"homehub/internal/kef"
	"homehub/internal/sonos"
	"homehub/internal/store"
)

// The listening log — what each room was *heard* playing (store/heard.go).
//
// history.go records intent: every surface that starts music does it through
// a handler, and that handler files what was started. That is the right
// record for a shelf and the wrong one for "what was that song?", because
// almost nothing anyone wants to ask that about was ever chosen. It was track
// nine, or what autoplay found, or what the radio played next — and the queue
// it lived in has since been replaced by another one, which is exactly the
// case the queue pane cannot answer no matter how far it scrolls.
//
// So this half writes from what the speakers *report*. It costs no polling of
// its own: every path that already has a fresh reading in hand hands it here
// — the monitors' change hook (which is GENA-driven for Sonos), the autoplay
// tick, and the status handlers the app's own polling calls. A house nobody
// is looking at and whose speakers can't be subscribed to records nothing,
// which is honest: nothing observed it.
//
// Zones are absent on purpose. A room HomeHub streams to is playing an HTTP
// stream whose "title" is the stream, not the song — the track identity lives
// with the Spotify session driving it, and inventing a name from the transport
// would put a row in the log that names the plumbing.

// heardDwell is how long a track must have been playing before it counts as
// heard. It keeps the log from filling with the eight tracks someone skipped
// through looking for the ninth — a skipped track is not something the room
// played, and it is precisely the noise that makes a log useless for finding
// the song you actually liked.
const heardDwell = 20 * time.Second

// heardWatch is what the recorder remembers between readings about one room:
// what it is playing, since when as far as we know, and whether the log
// already has it. Purely in-memory — on a restart the store's own head-of-log
// de-dupe covers the same ground.
type heardWatch struct {
	fp       string
	since    time.Time
	recorded bool
}

// noteHeardCached files what both monitors already know, reading neither
// speaker. Wired to the monitors' OnChange, which for Sonos is a GENA
// notification and for KEF the poller noticing a change — so a track change
// reaches the log about as fast as it reaches the screen.
func (s *Server) noteHeardCached() {
	s.noteHeardSonos(s.Speakers.Sonos.Cached())
	s.noteHeardKEF(s.Speakers.KEF.Cached())
}

// noteHeardSonos files what a Sonos snapshot shows each room playing. Safe to
// call with any snapshot, cached or freshly read: the recorder decides what
// is new.
func (s *Server) noteHeardSonos(snap sonos.Snapshot) {
	for id, cached := range snap.Speakers {
		if !cached.Reachable || cached.State == nil || !cached.State.Playing {
			continue
		}
		h, ok := heardFromSonos(cached.State.Track)
		if !ok {
			continue
		}
		// Art comes off the speaker on a relative path; the log outlives
		// the reading, so it stores the URL a browser can actually fetch.
		h.ArtURI = s.sonosArtURL(id, h.ArtURI)
		s.noteHeard(store.QualifySonos(id), h, secsOf(cached.State.Position))
	}
}

// noteHeardKEF does the same for the KEF speakers. A KEF names the track it
// is playing but never says which service item it is, so these rows are names
// rather than something to play again — see HeardTrack.URI.
func (s *Server) noteHeardKEF(snap kef.Snapshot) {
	for id, cached := range snap.Speakers {
		if !cached.Reachable || cached.State == nil || !cached.State.Playing {
			continue
		}
		t := cached.State.Track
		if t == nil || strings.TrimSpace(t.Title) == "" {
			continue
		}
		s.noteHeard(store.QualifyKEF(id), store.HeardTrack{
			Title:  t.Title,
			Artist: t.Artist,
			Album:  t.Album,
			ArtURI: s.kefArtURL(id, t.ArtURI),
		}, time.Duration(cached.State.PositionMS)*time.Millisecond)
	}
}

// heardFromSonos turns a reading into a log entry, using the same rule the
// player's own two lines do (frontend `trackLines`): a station puts the song
// in streamContent and leaves the title as the stream itself, so on radio the
// song is the headline and the station is the line under it. Radio never
// carries a service URI, which is why those rows are names only.
func heardFromSonos(t *sonos.Track) (store.HeardTrack, bool) {
	if t == nil {
		return store.HeardTrack{}, false
	}
	h := store.HeardTrack{ArtURI: t.ArtURI}
	if stream := strings.TrimSpace(t.Stream); stream != "" {
		h.Title = stream
		h.Artist = firstNonBlank(t.Station, t.Title)
	} else {
		h.Title = firstNonBlank(t.Title, t.Station)
		h.Artist = t.Artist
		h.Album = t.Album
		if uri := strings.TrimSpace(t.SpotifyURI); uri != "" {
			h.URI, h.Provider = uri, "spotify"
		}
	}
	if strings.TrimSpace(h.Title) == "" {
		return store.HeardTrack{}, false
	}
	return h, true
}

func firstNonBlank(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// secsOf reads Sonos's H:MM:SS position. Anything unparseable is no position
// at all, which only means the dwell is measured by the clock instead.
func secsOf(pos string) time.Duration {
	parts := strings.Split(strings.TrimSpace(pos), ":")
	total := 0
	for _, p := range parts {
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return 0
			}
			n = n*10 + int(c-'0')
		}
		total = total*60 + n
	}
	if len(parts) == 0 || total < 0 {
		return 0
	}
	return time.Duration(total) * time.Second
}

// noteHeard files one room's current track, once it has played long enough to
// count. `pos` is how far into the track the reading was, which is what lets
// a room joined mid-song be logged from the first reading rather than waiting
// out a dwell the song is already past.
//
// Takes no store lock in the common case — the same track observed again is
// settled from memory, and a playing house produces one of those every few
// seconds per speaker.
func (s *Server) noteHeard(roomKey string, h store.HeardTrack, pos time.Duration) {
	fp := heardFingerprint(h)
	if fp == "" {
		return
	}
	now := time.Now()

	s.heardMu.Lock()
	if s.heardWatches == nil {
		s.heardWatches = make(map[string]heardWatch)
	}
	w, ok := s.heardWatches[roomKey]
	if !ok || w.fp != fp {
		// A track we haven't seen in this room before: start its clock at
		// where it says it is, so a song already two minutes in is dated
		// from when it must have started.
		w = heardWatch{fp: fp, since: now.Add(-pos)}
		s.heardWatches[roomKey] = w
	}
	if w.recorded || (pos < heardDwell && now.Sub(w.since) < heardDwell) {
		s.heardMu.Unlock()
		return
	}
	w.recorded = true
	s.heardWatches[roomKey] = w
	s.heardMu.Unlock()

	h.At = w.since
	var changed bool
	s.Store.Mutate(func() {
		h.RoomName = s.roomNameFor(roomKey)
		changed = s.Store.RecordHeard(roomKey, h)
	})
	if !changed {
		return
	}
	// Off-lock, and its own file: a song changing is no reason to rewrite
	// every socket in the house, and this fires on every song in every room.
	if err := s.Store.SaveHeard(); err != nil {
		s.mediaLogf("heard: %v", err)
	}
}

// heardFingerprint is what "the same track, still playing" means to the
// recorder. The URI where there is one; the name where there isn't, which is
// all radio ever gives.
func heardFingerprint(h store.HeardTrack) string {
	if uri := strings.TrimSpace(h.URI); uri != "" {
		return uri
	}
	title := strings.ToLower(strings.TrimSpace(h.Title))
	if title == "" {
		return ""
	}
	return title + "\x00" + strings.ToLower(strings.TrimSpace(h.Artist))
}

// roomNameFor is what to call the room a key points at, so a merged list can
// name where a track was heard. Caller must hold Mu.
func (s *Server) roomNameFor(roomKey string) string {
	bridge, id, ok := store.SplitMember(roomKey)
	if !ok {
		return ""
	}
	switch bridge {
	case "sonos":
		if sp := s.Store.Sonos[id]; sp != nil {
			return sp.Name
		}
	case "kef":
		if sp := s.Store.KEF[id]; sp != nil {
			return sp.Name
		}
	}
	return ""
}

// forgetHeardWatches drops the recorder's memory of rooms that no longer
// exist, so a deleted and re-added speaker doesn't inherit a watch.
func (s *Server) forgetHeardWatches(live func(roomKey string) bool) {
	s.heardMu.Lock()
	defer s.heardMu.Unlock()
	for key := range s.heardWatches {
		if !live(key) {
			delete(s.heardWatches, key)
		}
	}
}

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
	s.heardMu.Lock()
	delete(s.heardWatches, room)
	s.heardMu.Unlock()
	if changed {
		if err := s.Store.SaveHeard(); err != nil {
			s.mediaLogf("heard: %v", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
