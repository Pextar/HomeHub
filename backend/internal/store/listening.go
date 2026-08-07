package store

import (
	"sort"
	"strings"
	"time"
)

// What the play history adds up to.
//
// history.go remembers each room's plays as a list, which answers "what was
// on last" and nothing else. Everything here reads the same data as evidence
// instead: what this room keeps coming back to, what it plays at this hour,
// and what the household as a whole listens to.
//
// None of it is a second store — these are derived on read from the same
// bounded per-room lists. That caps how much can be claimed (MediaHistorySize
// entries per room, so a house that plays two hundred different records only
// remembers the last thirty of them) and the doc comments below say so where
// it matters, because a shelf labelled "you play this most" that is really
// "of the last thirty" would be a lie the panel repeats every evening.

// Tally is one name and how many plays sit behind it.
type Tally struct {
	// Key identifies the thing counted: a room key for rooms, an artist
	// line for artists. Present so a caller can act on the row rather than
	// only print it.
	Key   string `json:"key"`
	Name  string `json:"name"`
	Plays int    `json:"plays"`
	// At is the most recent play in this tally.
	At time.Time `json:"at"`
}

// TopPlays ranks one room's remembered plays by how often it has started
// them, most-played first and ties broken by recency. Returns the room's own
// history only — a room that has never played anything gets nothing, because
// "you play this most" about a room that has played nothing is not a claim
// worth softening into the household's list the way a plain shelf can be.
//
// Caller must hold Mu.
func (s *Store) TopPlays(roomKey string, limit int) []MediaPlay {
	return rank(s.History(roomKey), limit, func(p MediaPlay) int { return p.Plays() })
}

// PlaysAtHour ranks a room's plays by how many of them started in the given
// local hour, so a panel can offer the kitchen's breakfast radio at breakfast
// and its dinner records at dinner. Entries with no plays in that hour are
// left out entirely rather than ranked last: an empty answer means "this room
// has no habit at this hour", and a caller that falls back to TopPlays or to
// plain recency is giving a better answer than a padded list would.
//
// Caller must hold Mu.
func (s *Store) PlaysAtHour(roomKey string, hour, limit int) []MediaPlay {
	return rank(s.History(roomKey), limit, func(p MediaPlay) int { return p.PlaysAt(hour) })
}

// rank orders plays by a score, dropping anything scoring zero. Ties go to
// the more recent play, which is what makes a room that has heard two records
// twice each offer the one it heard this week.
func rank(plays []MediaPlay, limit int, score func(MediaPlay) int) []MediaPlay {
	out := make([]MediaPlay, 0, len(plays))
	for _, p := range plays {
		if score(p) > 0 {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := score(out[i]), score(out[j])
		if si != sj {
			return si > sj
		}
		return out[i].At.After(out[j].At)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Listening is the household's listening summarised — the shape behind an
// insights panel, and the answer to "what does this house play" that no
// single room can give.
type Listening struct {
	// Plays is every remembered start, across every room. Bounded by what
	// the per-room lists still hold, so it is "plays we remember", never a
	// lifetime total.
	Plays int `json:"plays"`
	// Items is how many distinct things those plays were.
	Items int `json:"items"`
	// Rooms is which rooms did the playing, busiest first.
	Rooms []Tally `json:"rooms"`
	// Artists is the artist line of the tracks and albums played, busiest
	// first. Playlists and stations are left out: their second line is an
	// owner or a service, and counting those as artists would put "Spotify"
	// at the top of the list of what this house listens to.
	Artists []Tally `json:"artists"`
	// Top is the most-played items themselves, merged across rooms.
	Top []MediaPlay `json:"top"`
	// Hours is when the house listens: 24 slots, local time, summed over
	// every room. Entries recorded before the histogram existed contribute
	// to Plays but not here, so a long-running install's early history is
	// missing from the shape rather than misplaced in it.
	Hours []int `json:"hours"`
	// Since is the oldest first-played moment still remembered, so a
	// surface can say what window these numbers cover instead of implying
	// they cover everything. Zero when nothing has been played.
	Since time.Time `json:"since,omitempty"`
}

// Summarise builds the household's listening picture. limit caps each of the
// three lists (rooms are never capped — a house has as many as it has).
//
// Caller must hold Mu.
func (s *Store) Summarise(limit int) Listening {
	if limit <= 0 {
		limit = 8
	}
	out := Listening{
		Rooms:   []Tally{},
		Artists: []Tally{},
		Top:     []MediaPlay{},
		Hours:   make([]int, hoursInDay),
	}

	rooms := make(map[string]*Tally)
	artists := make(map[string]*Tally)
	merged := make(map[string]*MediaPlay)

	for key, plays := range s.MediaHistory {
		for _, p := range plays {
			n := p.Plays()
			out.Plays += n
			if len(p.Hours) == hoursInDay {
				for h, c := range p.Hours {
					out.Hours[h] += c
				}
			}
			if !p.FirstAt.IsZero() && (out.Since.IsZero() || p.FirstAt.Before(out.Since)) {
				out.Since = p.FirstAt
			}

			bump(rooms, key, displayRoom(p, key), n, p.At)
			if name := artistOf(p); name != "" {
				bump(artists, strings.ToLower(name), name, n, p.At)
			}

			// The same record played in two rooms is one item played
			// twice, which is what makes this the household's list and
			// not a concatenation of the rooms'.
			seen, ok := merged[p.URI]
			if !ok {
				cp := p
				cp.Count = n
				// Whose room it was stops meaning anything once the item
				// is merged across rooms; dropping it is better than
				// naming one of them.
				cp.RoomName = ""
				merged[p.URI] = &cp
				continue
			}
			total := seen.Plays() + n
			if p.At.After(seen.At) {
				// The fresher row wins on everything describing the item,
				// for the same reason carryForward prefers it.
				*seen = p
				seen.RoomName = ""
			}
			seen.Count = total
		}
	}

	out.Items = len(merged)
	items := make([]MediaPlay, 0, len(merged))
	for _, p := range merged {
		items = append(items, *p)
	}
	out.Top = rank(items, limit, func(p MediaPlay) int { return p.Plays() })
	out.Rooms = sortTallies(rooms, 0)
	out.Artists = sortTallies(artists, limit)
	return out
}

// displayRoom is what to call a room in a tally: the name it carried when
// something was played there, falling back to its key so a row is never blank.
func displayRoom(p MediaPlay, key string) string {
	if strings.TrimSpace(p.RoomName) != "" {
		return p.RoomName
	}
	return key
}

// artistOf is the artist line of a play, or "" for kinds whose second line
// is not an artist. Stations and playlists are named after their maker.
func artistOf(p MediaPlay) string {
	switch p.Kind {
	case "track", "album", "artist":
		return strings.TrimSpace(p.Sub)
	}
	return ""
}

func bump(into map[string]*Tally, key, name string, plays int, at time.Time) {
	t, ok := into[key]
	if !ok {
		into[key] = &Tally{Key: key, Name: name, Plays: plays, At: at}
		return
	}
	t.Plays += plays
	if at.After(t.At) {
		t.At = at
		t.Name = name // the most recent spelling of a renamed room wins
	}
}

// sortTallies orders busiest first, ties by recency. limit 0 keeps them all.
func sortTallies(in map[string]*Tally, limit int) []Tally {
	out := make([]Tally, 0, len(in))
	for _, t := range in {
		out = append(out, *t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Plays != out[j].Plays {
			return out[i].Plays > out[j].Plays
		}
		return out[i].At.After(out[j].At)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
