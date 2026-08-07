package store

import (
	"sort"
	"strings"
	"time"
)

// What each room has been asked to play, newest first.
//
// This is deliberately HomeHub's own memory and not Spotify's. The account's
// "recently played" is one list for the whole household — it cannot say that
// the kitchen gets radio at breakfast and the kids' room gets the same three
// records every evening, and on a shared account it is mostly somebody
// else's afternoon. A wall panel's most-wanted gesture is *put something on*
// (DESIGN.md §16), and the best answer to that in a given room is usually
// what that room played last.
//
// It records intent, not audio: what someone started here, at what time.
// Nothing tracks whether it finished, because nothing on the surfaces this
// feeds asks.
//
// The key is the destination as the media layer names it — "sonos:<id>",
// "kef:<id>", "zone:<id>" — so a room keeps its history when it is renamed
// and loses it when the device itself is deleted (see PruneHistory).

// MediaHistorySize caps how many plays are kept per room. The shelves that
// read this show at most a handful; the rest is there so that de-duplicating
// a room left on repeat still leaves something to show.
const MediaHistorySize = 30

// MediaPlay is one thing someone started in one room.
type MediaPlay struct {
	// Provider is the service the URI belongs to ("spotify"), or "sonos"
	// for a household favorite, which has no service URI of its own.
	Provider string `json:"provider"`
	// Kind is track/album/playlist/artist/station, as the media layer
	// spells them. Empty when whatever started it didn't say.
	Kind  string `json:"kind,omitempty"`
	URI   string `json:"uri"`
	Title string `json:"title"`
	// Sub is the artist, owner or service line — whatever the surface that
	// started it had to show. Optional: a favorite often has none.
	Sub    string `json:"sub,omitempty"`
	ArtURI string `json:"art_uri,omitempty"`
	// RoomName is what the room was called at the time. Stored so a shelf
	// can name where something came from without resolving a device that
	// may since have gone; the key remains the identity.
	RoomName string    `json:"room_name,omitempty"`
	At       time.Time `json:"at"`
}

// RecordPlay files one play under a room key, newest first, de-duplicated by
// URI: starting the same record twice in an evening is one entry with the
// later time, not two rows saying the same thing.
//
// Caller must hold Mu.
func (s *Store) RecordPlay(roomKey string, p MediaPlay) {
	roomKey = strings.TrimSpace(roomKey)
	if roomKey == "" || strings.TrimSpace(p.URI) == "" {
		return
	}
	if p.At.IsZero() {
		p.At = time.Now()
	}
	if s.MediaHistory == nil {
		s.MediaHistory = make(map[string][]MediaPlay)
	}
	kept := make([]MediaPlay, 0, MediaHistorySize)
	kept = append(kept, p)
	for _, old := range s.MediaHistory[roomKey] {
		if old.URI == p.URI {
			continue
		}
		kept = append(kept, old)
		if len(kept) == MediaHistorySize {
			break
		}
	}
	s.MediaHistory[roomKey] = kept
}

// History returns one room's plays, newest first. Caller must hold Mu.
func (s *Store) History(roomKey string) []MediaPlay {
	out := make([]MediaPlay, len(s.MediaHistory[roomKey]))
	copy(out, s.MediaHistory[roomKey])
	return out
}

// RecentPlays merges every room's history into one list, newest first and
// de-duplicated by URI. This is what a room with no history of its own is
// shown: a KEF that has never been played from HomeHub still sits in a house
// where something was, and "what this home plays" beats an empty shelf.
//
// Caller must hold Mu.
func (s *Store) RecentPlays(limit int) []MediaPlay {
	all := make([]MediaPlay, 0, len(s.MediaHistory)*4)
	for _, plays := range s.MediaHistory {
		all = append(all, plays...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].At.After(all[j].At) })
	out := make([]MediaPlay, 0, limit)
	seen := make(map[string]bool, len(all))
	for _, p := range all {
		if seen[p.URI] {
			continue
		}
		seen[p.URI] = true
		out = append(out, p)
		if len(out) == limit {
			break
		}
	}
	return out
}

// PruneHistory drops the history of rooms whose key is no longer live. It is
// the same reasoning as CascadeDeleteSocket: a deleted speaker must not leave
// a shelf behind that plays to nothing. Reports whether anything was dropped.
//
// Caller must hold Mu.
func (s *Store) PruneHistory(live func(roomKey string) bool) bool {
	dropped := false
	for key := range s.MediaHistory {
		if !live(key) {
			delete(s.MediaHistory, key)
			dropped = true
		}
	}
	return dropped
}

// SaveHistory writes only the history file. Recording a play is not a reason
// to rewrite the whole store, and it happens on every tap of a shelf.
func (s *Store) SaveHistory() error {
	return s.saveMatching(func(c collection) bool { return c.label == "media history" })
}
