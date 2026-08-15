package store

import (
	"errors"
	"strings"
)

// Zone is a named set of speakers that play together, of any mix of makes.
//
// It is the persisted half of the media protocol (see docs/MEDIA-PROTOCOL.md):
// which speakers belong together is a user's arrangement of their house, so it
// outlives a playback and belongs on disk. How those speakers are actually
// driven — natively, grouped, or through HomeHub's own stream — is decided per
// playback by the route engine and is never stored, because it depends on what
// is being played and on what the speakers can do at that moment.
//
// Members are stored as bridge-qualified ids ("sonos:abc", "kef:def") rather
// than as bare ids. The two bridges mint ids independently and nothing stops
// them colliding, so a bare id would be ambiguous the day it happened.
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Members are qualified speaker ids, in the order the user arranged
	// them. Order is meaningful: the first member that can lead becomes the
	// coordinator for routes that need one.
	Members []string `json:"members"`
	Room    string   `json:"room,omitempty"`
}

// Speaker id prefixes. Exported because the api layer builds and splits
// qualified ids when it resolves a zone to live endpoints.
const (
	SonosPrefix   = "sonos:"
	KEFPrefix     = "kef:"
	AirPlayPrefix = "airplay:"
	UPnPPrefix    = "upnp:"
)

// QualifySonos and the two beside it build a member id for a stored speaker,
// one per bridge.
func QualifySonos(id string) string   { return SonosPrefix + id }
func QualifyKEF(id string) string     { return KEFPrefix + id }
func QualifyAirPlay(id string) string { return AirPlayPrefix + id }
func QualifyUPnP(id string) string    { return UPnPPrefix + id }

// SplitMember separates a qualified member id into its bridge and bare id.
// Returns ok=false for anything unqualified, which is how a member written by
// an older or hand-edited file is rejected rather than silently misread.
func SplitMember(member string) (bridge, id string, ok bool) {
	switch {
	case strings.HasPrefix(member, SonosPrefix):
		return "sonos", strings.TrimPrefix(member, SonosPrefix), true
	case strings.HasPrefix(member, KEFPrefix):
		return "kef", strings.TrimPrefix(member, KEFPrefix), true
	case strings.HasPrefix(member, AirPlayPrefix):
		return "airplay", strings.TrimPrefix(member, AirPlayPrefix), true
	case strings.HasPrefix(member, UPnPPrefix):
		return "upnp", strings.TrimPrefix(member, UPnPPrefix), true
	}
	return "", "", false
}

// ValidateZone normalises and checks a zone. Caller must hold Mu.
//
// An empty zone is allowed: a user clearing the members in the UI has made a
// zone they are still editing, and refusing to save that would lose the name
// they just typed. Playing to an empty zone is what fails, in the media layer.
func (s *Store) ValidateZone(z *Zone) error {
	z.Name = strings.TrimSpace(z.Name)
	z.Room = strings.TrimSpace(z.Room)
	if z.Name == "" {
		return errors.New("name is required")
	}
	for _, existing := range s.Zones {
		if existing.ID != z.ID && strings.EqualFold(existing.Name, z.Name) {
			return errors.New("a zone with that name already exists")
		}
	}

	// Members must name speakers that exist, must be qualified, and must not
	// repeat. A duplicate would be sent every command twice and, on the
	// group route, told to join a group it already leads.
	seen := make(map[string]bool, len(z.Members))
	cleaned := make([]string, 0, len(z.Members))
	for _, m := range z.Members {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		bridge, id, ok := SplitMember(m)
		if !ok {
			return errors.New("speaker " + m + " is not a recognised speaker id")
		}
		switch bridge {
		case "sonos":
			if _, exists := s.Sonos[id]; !exists {
				return errors.New("no such Sonos speaker: " + id)
			}
		case "kef":
			if _, exists := s.KEF[id]; !exists {
				return errors.New("no such KEF speaker: " + id)
			}
		case "airplay":
			if _, exists := s.AirPlay[id]; !exists {
				return errors.New("no such AirPlay receiver: " + id)
			}
		case "upnp":
			if _, exists := s.UPnP[id]; !exists {
				return errors.New("no such UPnP renderer: " + id)
			}
		}
		if seen[m] {
			continue // a speaker listed twice is listed once
		}
		seen[m] = true
		cleaned = append(cleaned, m)
	}
	z.Members = cleaned
	return nil
}

// CascadeDeleteSpeaker drops a deleted speaker from every zone that held it.
// Caller must hold Mu.
//
// The sibling of CascadeDeleteSocket, and it exists for the same reason: a
// dangling member would fail validation on the next unrelated edit of that
// zone, presenting as "no such speaker" for a change the user did not make.
// A zone left empty is kept — the user named it, and they can refill it.
func (s *Store) CascadeDeleteSpeaker(member string) {
	for _, z := range s.Zones {
		kept := z.Members[:0]
		for _, m := range z.Members {
			if m != member {
				kept = append(kept, m)
			}
		}
		z.Members = kept
	}
}

// ZonesForSpeaker lists the zones a speaker belongs to. Caller must hold Mu.
func (s *Store) ZonesForSpeaker(member string) []*Zone {
	var out []*Zone
	for _, z := range s.Zones {
		for _, m := range z.Members {
			if m == member {
				out = append(out, z)
				break
			}
		}
	}
	return out
}
