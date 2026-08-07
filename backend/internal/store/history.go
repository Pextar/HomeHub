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
	// Count is how many times this URI has been started in this room. The
	// de-dupe above is what makes the shelf readable; without a tally it
	// also threw away the only thing that separates the record this room
	// lives on from the one someone tried once. Entries written before this
	// field existed carry 0, which Plays() reads as one play.
	Count int `json:"count,omitempty"`
	// FirstAt is when this room first played it — "since March" rather than
	// just "an hour ago". Zero on entries that predate the field.
	FirstAt time.Time `json:"first_at,omitempty"`
	// Hours tallies plays by local hour of day, 24 slots. This is what lets
	// a shelf answer *put something on* with what this room actually plays
	// at this hour, which on a wall panel is a different and better answer
	// than what it played last: the kitchen's breakfast radio and its
	// dinner records are both "recent" by eight in the evening, and only
	// one of them is right. Nil until the entry has been recorded once
	// under the new shape, so an old file costs nothing.
	Hours []int `json:"hours,omitempty"`
}

// Plays is the entry's tally, reading a missing count as the one play that
// must have created the entry.
func (p MediaPlay) Plays() int {
	if p.Count < 1 {
		return 1
	}
	return p.Count
}

// PlaysAt is how many of this entry's plays started in the given local hour.
// Zero for entries written before the histogram existed, which is honest:
// they are not evidence about any hour in particular.
func (p MediaPlay) PlaysAt(hour int) int {
	if hour < 0 || hour >= hoursInDay || len(p.Hours) != hoursInDay {
		return 0
	}
	return p.Hours[hour]
}

// hoursInDay sizes the per-entry histogram.
const hoursInDay = 24

// RecordPlay files one play under a room key, newest first, de-duplicated by
// URI: starting the same record twice in an evening is one entry with the
// later time, not two rows saying the same thing.
//
// The entry that survives the de-dupe inherits the old one's tally, its
// first-played time and its hour histogram, so folding two rows into one
// loses the duplicate row and nothing else. That is the difference between a
// list of what happened and a memory of what this room listens to.
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
	kept = append(kept, firstPlay(p))
	for _, old := range s.MediaHistory[roomKey] {
		if old.URI == p.URI {
			kept[0] = carryForward(old, p)
			continue
		}
		kept = append(kept, old)
		if len(kept) == MediaHistorySize {
			break
		}
	}
	s.MediaHistory[roomKey] = kept
}

// firstPlay is what a URI this room has never played looks like.
func firstPlay(p MediaPlay) MediaPlay {
	p.Count = 1
	p.FirstAt = p.At
	p.Hours = make([]int, hoursInDay)
	p.Hours[p.At.Hour()]++
	return p
}

// carryForward folds a new play into what the room already remembered about
// the same URI.
//
// Everything describing the *item* comes from the new play — a title or a
// picture the catalog has since improved should win — while everything
// describing the room's relationship to it accumulates.
func carryForward(old, fresh MediaPlay) MediaPlay {
	out := firstPlay(fresh)
	out.Count = old.Plays() + 1

	// An entry written before FirstAt existed can still date itself: the
	// time it carries is the oldest moment we can honestly claim.
	out.FirstAt = old.FirstAt
	if out.FirstAt.IsZero() {
		out.FirstAt = old.At
	}
	if out.FirstAt.IsZero() || out.FirstAt.After(fresh.At) {
		out.FirstAt = fresh.At
	}

	if len(old.Hours) == hoursInDay {
		for h, n := range old.Hours {
			out.Hours[h] += n
		}
	}
	return out
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
