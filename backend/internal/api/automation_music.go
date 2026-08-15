package api

// The read half of a scene's and an automation's relationship with the music.
//
// scene_music.go is the write half: a rule can pause a room, resume it, set
// its volume. This is the other direction — a rule can *watch* a room and act
// when it goes quiet, which is the difference between "when I press the scene,
// dim the bedroom" and "when the film ends, dim the bedroom".
//
// It is installed as store.MusicPlaying for exactly the reason runSceneMusic
// is installed as store.OnMusic: the store holds the rules and must not know
// how a room is reached, and the scheduler that evaluates them lives in a
// third package that knows even less.
//
// Two constraints shape everything below, and both come from the caller:
//
//   - It runs on the five-second scheduler tick, whether or not anyone is
//     looking at the app. So it reads the monitors' caches and never a
//     speaker: the Sonos cache is GENA-fed and the KEF one is filled by the
//     poller that runs anyway, so watching a room for a rule costs the house
//     no traffic at all. Calling Snapshot here instead of Cached would turn a
//     cold cache into a household-wide read every five seconds — see
//     sonos.Monitor.Cached.
//   - It is called off Mu and takes its own read lock to resolve a zone, so
//     it must never be called from inside a View.

import (
	"strings"

	"homehub/internal/kef"
	"homehub/internal/media"
	"homehub/internal/sonos"
	"homehub/internal/store"
)

// roomPlaying reports whether a media room is making a sound right now.
//
// known is false when nothing in this house can answer for that key: an id
// naming no registered speaker or zone, an empty zone, or speakers no monitor
// has a reading for. That third answer is the point — a speaker that has
// dropped off the network is not a quiet one, and a rule about a room going
// quiet must not fire because the Wi-Fi hiccuped.
//
// A room of several speakers is playing when *any* of them is. A zone is one
// sound in one place, and the answer to "is the living room playing" is yes
// while any part of it still is.
func (s *Server) roomPlaying(room string) (playing, known bool) {
	members := s.mediaRoomMembers(room)
	if len(members) == 0 {
		return false, false
	}

	// Both snapshots are read once even for a single-vendor room: they are
	// map copies off a cache, and branching to avoid one would cost more in
	// reasoning than it saves in work.
	var sonosSnap sonos.Snapshot
	var kefSnap kef.Snapshot
	var live func(string) (media.AirPlayControl, bool)

	for _, m := range members {
		bridge, id, ok := store.SplitMember(m)
		if !ok {
			continue // a hand-edited member id; not something to guess at
		}
		switch bridge {
		case "sonos":
			if sonosSnap.Speakers == nil {
				sonosSnap = s.sonosEvents().Cached()
			}
			cached, have := sonosSnap.Speakers[id]
			if !have || !cached.Reachable || cached.State == nil {
				continue
			}
			known = true
			playing = playing || cached.State.Playing

		case "kef":
			if kefSnap.Speakers == nil {
				kefSnap = s.kefEvents().Cached()
			}
			cached, have := kefSnap.Speakers[id]
			if !have || !cached.Reachable || cached.State == nil {
				continue
			}
			known = true
			// State.Playing is the speaker's own word, which covers a KEF on
			// its optical or TV input as readily as one streaming: what the
			// rule is asking about is whether the room is making a sound,
			// and the speaker answers that the same way either way.
			playing = playing || cached.State.Playing

		case "airplay":
			// A receiver holds no state — it is a sink, and what it is
			// playing is whatever HomeHub is sending it (mediabridge/
			// airplay.go). So the cast we are running *is* the reading, and
			// it is always available: nothing casting is a genuine "stopped"
			// rather than a missing answer.
			if live == nil {
				live = s.airplayCaster().Live
			}
			known = true
			if ctrl, casting := live(id); casting && ctrl.Playing() {
				playing = true
			}
		}
	}
	return playing, known
}

// mediaRoomMembers resolves a media room key to the qualified speaker ids
// behind it: a zone's members, or the one speaker a bridge-qualified key
// names. Empty for anything this house doesn't have.
//
// Deliberately not mediaRoom(): that builds live endpoints for every speaker
// in the house to answer, which is the right shape for a request handler and
// the wrong one for something asked every five seconds. Caller must NOT hold
// Mu.
func (s *Server) mediaRoomMembers(key string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	var members []string
	s.Store.View(func() {
		if id, ok := strings.CutPrefix(key, "zone:"); ok {
			if z, exists := s.Store.Zones[id]; exists {
				members = append([]string(nil), z.Members...)
			}
			return
		}
		bridge, id, ok := store.SplitMember(key)
		if !ok {
			return
		}
		var exists bool
		switch bridge {
		case "kef":
			_, exists = s.Store.KEF[id]
		case "airplay":
			_, exists = s.Store.AirPlay[id]
		default:
			_, exists = s.Store.Sonos[id]
		}
		if exists {
			members = []string{key}
		}
	})
	return members
}
