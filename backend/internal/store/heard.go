package store

import (
	"sort"
	"strings"
	"time"
)

// What each room was actually heard playing, newest first.
//
// history.go is the other half of this and answers a different question. It
// records *intent* — the album someone put on, de-duplicated by URI and
// tallied, so a shelf can offer a room what it keeps coming back to. It
// cannot answer "what was that song?", because nobody chose that song: it was
// track nine of something, or the fourth thing autoplay found, and by the
// time anyone asks, the queue it lived in has been replaced by another one.
//
// So this is a plain log of tracks, in the order they were heard, per room.
// Repetition is data here rather than noise (a house that plays one record
// twice in an evening heard it twice), and nothing is ranked — the newest
// entry is the most useful one, which is the opposite of what a shelf wants.
//
// It is written from what the speakers report, not from what HomeHub was
// asked to do, which is why it survives a queue being replaced, music started
// from the Sonos app, or a track autoplay chose. Every entry keeps the
// service URI when the source had one, because a row you can play again is
// worth more than a row you can only read.
//
// The key is the destination as the media layer names it — "sonos:<id>",
// "kef:<id>" — matching MediaHistory, so both are pruned by the same rule
// when a speaker is deleted.

// HeardLogSize caps how many tracks each room keeps. An evening of listening
// is thirty or forty tracks; twice that is enough to answer "what was that,
// two records ago?" without the file growing without end.
const HeardLogSize = 80

// HeardTrack is one track a room was heard playing.
type HeardTrack struct {
	Title  string `json:"title"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	ArtURI string `json:"art_uri,omitempty"`
	// URI is the canonical service URI ("spotify:track:…") where the source
	// had one, and empty for radio, line-in, a local library file or a
	// speaker whose API never says. Its presence is exactly what separates
	// a row that can be played again from a row that is only a name, and
	// the surfaces render the difference rather than offering a replay that
	// would fail.
	URI string `json:"uri,omitempty"`
	// Provider names the service the URI belongs to ("spotify"). Empty
	// alongside an empty URI.
	Provider string `json:"provider,omitempty"`
	// RoomName is what the room was called when this played, so a merged
	// household list can say where a track came from without resolving a
	// device that may since have gone.
	RoomName string `json:"room_name,omitempty"`
	// At is when the track was first seen playing. It is not moved by
	// seeing the same track again a minute later: the entry means "this
	// started here", and a paused-and-resumed song did not start twice.
	At time.Time `json:"at"`
}

// sameHeard reports whether two readings are the same track.
//
// A service URI settles it where both have one. Otherwise the name does, and
// it has to: radio and line-in carry no URI at all, and a station that names
// its current song is the one case where the log has nothing else to go on.
func sameHeard(a, b HeardTrack) bool {
	if a.URI != "" && b.URI != "" {
		return a.URI == b.URI
	}
	eq := func(x, y string) bool { return strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(y)) }
	return eq(a.Title, b.Title) && eq(a.Artist, b.Artist)
}

// RecordHeard files a track under a room key, newest first, reporting whether
// it was new. Seeing the same track again — which every reading of a playing
// speaker does, several times a minute — changes nothing and reports false,
// so the caller knows not to write the file.
//
// Only a *consecutive* repeat is suppressed. A record played twice in an
// evening is two entries, because that is what happened, and a log that
// silently folded them would be answering the shelf's question instead of
// this one.
//
// Caller must hold Mu.
func (s *Store) RecordHeard(roomKey string, t HeardTrack) bool {
	roomKey = strings.TrimSpace(roomKey)
	t.Title = strings.TrimSpace(t.Title)
	if roomKey == "" || t.Title == "" {
		return false
	}
	if t.At.IsZero() {
		t.At = time.Now()
	}
	if s.Heard == nil {
		s.Heard = make(map[string][]HeardTrack)
	}
	log := s.Heard[roomKey]
	if len(log) > 0 && sameHeard(log[0], t) {
		return false
	}
	log = append([]HeardTrack{t}, log...)
	if len(log) > HeardLogSize {
		log = log[:HeardLogSize]
	}
	s.Heard[roomKey] = log
	return true
}

// HeardIn returns one room's log, newest first. Caller must hold Mu.
func (s *Store) HeardIn(roomKey string) []HeardTrack {
	out := make([]HeardTrack, len(s.Heard[roomKey]))
	copy(out, s.Heard[roomKey])
	return out
}

// RecentHeard merges every room's log into one, newest first. This is what a
// room that has never played anything is shown — the same fallback the play
// shelves make, and honest for the same reason: every entry names the room it
// was heard in.
//
// Consecutive repeats were suppressed per room, so the merge can still show
// the same track twice when two rooms played it. That is not a duplicate: it
// is two rooms.
//
// Caller must hold Mu.
func (s *Store) RecentHeard(limit int) []HeardTrack {
	all := make([]HeardTrack, 0, len(s.Heard)*8)
	for _, log := range s.Heard {
		all = append(all, log...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].At.After(all[j].At) })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// PruneHeard drops the logs of rooms whose key is no longer live, reporting
// whether anything went. Same promise as PruneHistory: a deleted speaker
// leaves nothing behind that refers to it.
//
// Caller must hold Mu.
func (s *Store) PruneHeard(live func(roomKey string) bool) bool {
	dropped := false
	for key := range s.Heard {
		if !live(key) {
			delete(s.Heard, key)
			dropped = true
		}
	}
	return dropped
}

// ForgetHeard drops one room's log, reporting whether it had one.
//
// There is no per-track forget here, unlike ForgetPlay. Nothing ranks this
// list or offers it back as a suggestion, so a single wrong row costs a line
// on a screen someone asked to see rather than a shelf the house is offered
// every evening — and the answer to "I don't want this remembered" about a
// log of what was audible in a room is to clear the room's log.
//
// Caller must hold Mu.
func (s *Store) ForgetHeard(roomKey string) bool {
	roomKey = strings.TrimSpace(roomKey)
	if roomKey == "" {
		return false
	}
	if _, ok := s.Heard[roomKey]; !ok {
		return false
	}
	delete(s.Heard, roomKey)
	return true
}

// SaveHeard writes only the heard-track file. A track change is not a reason
// to rewrite every socket in the house — the same exception SaveHistory is,
// and this one fires more often.
func (s *Store) SaveHeard() error {
	return s.saveMatching(func(c collection) bool { return c.label == "heard tracks" })
}
