// Package listening records what each room was heard playing.
//
// It is the other half of the play history. That one records intent: every
// surface that starts music does it through a handler, and that handler files
// what was started. It is the right record for a shelf and the wrong one for
// "what was that song?", because almost nothing anyone wants to ask that about
// was ever chosen. It was track nine, or what autoplay found, or what the
// radio played next — and the queue it lived in has since been replaced by
// another one, which is exactly the case the queue pane cannot answer no
// matter how far it scrolls.
//
// So this half writes from what the speakers *report*. It costs no polling of
// its own: every path that already has a fresh reading in hand hands it here —
// the monitors' change hook (GENA-driven for Sonos), the autoplay tick, and
// the status handlers the app's own polling calls. A house nobody is looking
// at, whose speakers can't be subscribed to, records nothing. That is honest:
// nothing observed it.
//
// Zones are absent on purpose. A room HomeHub streams to is playing an HTTP
// stream whose "title" is the stream, not the song — the track identity lives
// with the Spotify session driving it, and inventing a name from the transport
// would put a row in the log that names the plumbing.
package listening

import (
	"strings"
	"sync"
	"time"

	"homehub/internal/kef"
	"homehub/internal/sonos"
	"homehub/internal/speakermon"
	"homehub/internal/store"
)

// dwell is how long a track must have been playing before it counts as heard.
//
// It keeps the log from filling with the eight tracks someone skipped through
// looking for the ninth. A skipped track is not something the room played, and
// it is precisely the noise that makes a log useless for finding the song you
// actually liked.
const dwell = 20 * time.Second

// Config is what the recorder needs from the rest of the application.
type Config struct {
	// Store is where the log lives.
	Store *store.Store
	// Speakers is the cached view both monitors keep, for NoteCached.
	Speakers *speakermon.Monitors

	// SonosArt and KEFArt rewrite a speaker-relative artwork path into
	// something a browser can fetch later.
	//
	// They are functions because the answer is an HTTP route, which this
	// package has no business knowing — and because the log outlives the
	// reading: a relative path that was fine while the speaker was being
	// polled is useless by the time someone scrolls back to the row.
	SonosArt func(speakerID, artURI string) string
	KEFArt   func(speakerID, artURI string) string

	Logf func(format string, args ...any)
}

// watch is what the recorder remembers between readings about one room: what
// it is playing, since when as far as we know, and whether the log already has
// it.
//
// Purely in-memory. On a restart the store's own head-of-log de-dupe covers
// the same ground.
type watch struct {
	fp       string
	since    time.Time
	recorded bool
}

// Recorder files what rooms are heard playing.
type Recorder struct {
	cfg Config

	mu      sync.Mutex
	watches map[string]watch
}

// New returns a recorder. It observes nothing on its own; something with a
// reading in hand has to hand it over.
func New(cfg Config) *Recorder {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.SonosArt == nil {
		cfg.SonosArt = func(_, art string) string { return art }
	}
	if cfg.KEFArt == nil {
		cfg.KEFArt = func(_, art string) string { return art }
	}
	return &Recorder{cfg: cfg, watches: map[string]watch{}}
}

// NoteCached files what both monitors already know, reading neither speaker.
//
// Wired to the monitors' change hook, which for Sonos is a GENA notification
// and for KEF the poller noticing a change — so a track change reaches the log
// about as fast as it reaches the screen.
func (r *Recorder) NoteCached() {
	r.NoteSonos(r.cfg.Speakers.Sonos.Cached())
	r.NoteKEF(r.cfg.Speakers.KEF.Cached())
}

// NoteSonos files what a snapshot shows each room playing. Safe to call with
// any snapshot, cached or freshly read: the recorder decides what is new.
func (r *Recorder) NoteSonos(snap sonos.Snapshot) {
	for id, cached := range snap.Speakers {
		if !cached.Reachable || cached.State == nil || !cached.State.Playing {
			continue
		}
		h, ok := trackFromSonos(cached.State.Track)
		if !ok {
			continue
		}
		h.ArtURI = r.cfg.SonosArt(id, h.ArtURI)
		r.note(store.QualifySonos(id), h, secsOf(cached.State.Position))
	}
}

// NoteKEF does the same for the KEF speakers. A KEF names the track it is
// playing but never says which service item it is, so these rows are names
// rather than something to play again — see store.HeardTrack.URI.
func (r *Recorder) NoteKEF(snap kef.Snapshot) {
	for id, cached := range snap.Speakers {
		if !cached.Reachable || cached.State == nil || !cached.State.Playing {
			continue
		}
		t := cached.State.Track
		if t == nil || strings.TrimSpace(t.Title) == "" {
			continue
		}
		r.note(store.QualifyKEF(id), store.HeardTrack{
			Title:  t.Title,
			Artist: t.Artist,
			Album:  t.Album,
			ArtURI: r.cfg.KEFArt(id, t.ArtURI),
		}, time.Duration(cached.State.PositionMS)*time.Millisecond)
	}
}

// Forget drops one room's watch, so the track playing right now is recorded
// again. Called when a room's log is cleared: without it the recorder would
// still believe it had filed what is audible.
func (r *Recorder) Forget(roomKey string) {
	r.mu.Lock()
	delete(r.watches, roomKey)
	r.mu.Unlock()
}

// ForgetMissing drops the memory of rooms that no longer exist, so a deleted
// and re-added speaker doesn't inherit a watch.
func (r *Recorder) ForgetMissing(live func(roomKey string) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.watches {
		if !live(key) {
			delete(r.watches, key)
		}
	}
}

// note files one room's current track, once it has played long enough to
// count.
//
// pos is how far into the track the reading was, which is what lets a room
// joined mid-song be logged from the first reading rather than waiting out a
// dwell the song is already past.
//
// It takes no store lock in the common case — the same track observed again is
// settled from memory — and a playing house produces one of those every few
// seconds per speaker.
func (r *Recorder) note(roomKey string, h store.HeardTrack, pos time.Duration) {
	fp := fingerprint(h)
	if fp == "" {
		return
	}
	now := time.Now()

	r.mu.Lock()
	w, ok := r.watches[roomKey]
	if !ok || w.fp != fp {
		// A track we haven't seen in this room before: start its clock at
		// where it says it is, so a song already two minutes in is dated
		// from when it must have started.
		w = watch{fp: fp, since: now.Add(-pos)}
		r.watches[roomKey] = w
	}
	if w.recorded || (pos < dwell && now.Sub(w.since) < dwell) {
		r.mu.Unlock()
		return
	}
	w.recorded = true
	r.watches[roomKey] = w
	r.mu.Unlock()

	h.At = w.since
	var changed bool
	r.cfg.Store.Mutate(func() {
		h.RoomName = roomName(r.cfg.Store, roomKey)
		changed = r.cfg.Store.RecordHeard(roomKey, h)
	})
	if !changed {
		return
	}
	// Off-lock, and its own file: a song changing is no reason to rewrite
	// every socket in the house, and this fires on every song in every room.
	if err := r.cfg.Store.SaveHeard(); err != nil {
		r.cfg.Logf("heard: %v", err)
	}
}

// trackFromSonos turns a reading into a log entry, using the same rule the
// player's own two lines do (the frontend's trackLines): a station puts the
// song in streamContent and leaves the title as the stream itself, so on radio
// the song is the headline and the station is the line under it. Radio never
// carries a service URI, which is why those rows are names only.
func trackFromSonos(t *sonos.Track) (store.HeardTrack, bool) {
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

// fingerprint is what "the same track, still playing" means to the recorder.
// The URI where there is one; the name where there isn't, which is all radio
// ever gives.
func fingerprint(h store.HeardTrack) string {
	if uri := strings.TrimSpace(h.URI); uri != "" {
		return uri
	}
	title := strings.ToLower(strings.TrimSpace(h.Title))
	if title == "" {
		return ""
	}
	return title + "\x00" + strings.ToLower(strings.TrimSpace(h.Artist))
}

// roomName is what to call the room a key points at, so a merged list can name
// where a track was heard. Caller must hold the store lock.
func roomName(st *store.Store, roomKey string) string {
	bridge, id, ok := store.SplitMember(roomKey)
	if !ok {
		return ""
	}
	switch bridge {
	case "sonos":
		if sp := st.Sonos[id]; sp != nil {
			return sp.Name
		}
	case "kef":
		if sp := st.KEF[id]; sp != nil {
			return sp.Name
		}
	}
	return ""
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
