package music

// Music as part of a scene or an automation — both directions.
//
// Until this existed the house was two houses: "Film" could dim the lamps but
// not quiet the kitchen radio, "leaving" could switch off every socket and
// leave a speaker playing to an empty room, and the master All off meant all
// *sockets* off. Both halves of the machinery were already there — the staged
// socket flow on one side, the vendor-neutral media layer on the other — and
// nothing joined them.
//
// This is the join, and it is deliberately thin. The store holds the rules and
// must not know how a room is reached; the scheduler that evaluates them lives
// in a third package that knows even less. So the store calls two hooks, both
// installed here at wiring time:
//
//   - RunActions (store.OnMusic) is the write half: a scene step or a rule can
//     pause a room, resume it, set its volume. The verbs are the three that
//     need no catalogue item. A scene expresses a *moment* in the house; "put
//     this record on" needs something to play and a picker to choose it with,
//     which is what the music timers already are — adding it here would be a
//     second, worse copy of a feature rather than a new one.
//
//   - Playing (store.MusicPlaying) is the read half: a rule can watch a room
//     and act when it goes quiet, which is the difference between "when I
//     press the scene, dim the bedroom" and "when the film ends, dim the
//     bedroom".
//
// Every action runs even if an earlier one failed, because the rooms are
// independent: a scene that quiets three rooms should not stop at the first
// speaker that has gone offline. Failures are logged, never surfaced as a
// scene failure — the sockets are the scene's contract.

import (
	"context"
	"strings"
	"sync"
	"time"

	"homehub/internal/kef"
	"homehub/internal/media"
	"homehub/internal/sonos"
	"homehub/internal/store"
)

// actionTimeout caps one room's worth of work. Generous next to a single
// SOAP call because a zone fans out across several speakers, and short next
// to the delay between scene steps.
const actionTimeout = 10 * time.Second

// RunActions carries out one scene step's or automation rule's music actions.
// Installed as store.OnMusic.
//
// It returns as soon as the work is handed to a goroutine. The store calls
// this from a scene activation, a scheduler tick and a timer callback, and
// none of those may wait on a speaker: the same rule that keeps device I/O
// out from under Mu keeps it out of the tick.
func (s *Service) RunActions(actions []store.MusicAction) {
	if len(actions) == 0 {
		return
	}
	go func() {
		var wg sync.WaitGroup
		for _, a := range actions {
			// Rooms are independent, so they go at once: four rooms quieted
			// one after another is four round trips of latency in a gesture
			// that should read as instant.
			wg.Add(1)
			go func(a store.MusicAction) {
				defer wg.Done()
				if err := s.applyAction(a); err != nil {
					s.cfg.Logf("scene music: %s %s: %v", a.Action, a.Room, err)
				}
			}(a)
		}
		wg.Wait()
	}()
}

// applyMusicAction is one room, one verb. Caller must NOT hold Mu.
func (s *Service) applyAction(a store.MusicAction) error {
	eps, _, err := s.Room(a.Room)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()

	switch a.Action {
	case store.MusicPause:
		// Through the service's pause rather than a bare transport call: it
		// ends a streamed zone's Spotify session too, and a scene that left
		// one held would be the same bug the sleep timer already fixed.
		return s.Pause(ctx, a.Room, eps)

	case store.MusicResume:
		// Whatever the room had loaded. A room with an empty queue refuses
		// this and says so in the log — there is nothing a scene could
		// usefully do about it, and the alternative (silently starting
		// something else) is the one thing a scene must not do.
		zoneID, isZone := strings.CutPrefix(a.Room, "zone:")
		plan := s.Plan(zoneID, eps)
		if err := media.Control(ctx, plan, media.TransportPlay); err != nil {
			return err
		}
		if isZone {
			s.Touch(eps)
		}
		return nil

	case store.MusicVolume:
		if a.Volume == nil {
			return nil // validation refuses this; belt and braces
		}
		// A ramp already walking this room would fight a flat set — and win,
		// since it writes every few seconds. The scene is the more recent
		// instruction, so it takes the room. (CancelFade on a sleep fade
		// also puts the volume back, which this immediately overwrites with
		// the level the scene asked for.)
		s.cfg.CancelFade(a.Room)
		return media.SetVolume(ctx, eps, *a.Volume)
	}
	return nil
}

// Playing reports whether a media room is making a sound right now.
// Installed as store.MusicPlaying.
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
func (s *Service) Playing(room string) (playing, known bool) {
	members := s.roomMembers(room)
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
				sonosSnap = s.cfg.Speakers.Sonos.Cached()
			}
			cached, have := sonosSnap.Speakers[id]
			if !have || !cached.Reachable || cached.State == nil {
				continue
			}
			known = true
			playing = playing || cached.State.Playing

		case "kef":
			if kefSnap.Speakers == nil {
				kefSnap = s.cfg.Speakers.KEF.Cached()
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
				live = s.cfg.Audio.Caster().Live
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
func (s *Service) roomMembers(key string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	var members []string
	s.cfg.Store.View(func() {
		if id, ok := strings.CutPrefix(key, "zone:"); ok {
			if z, exists := s.cfg.Store.Zones[id]; exists {
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
			_, exists = s.cfg.Store.KEF[id]
		case "airplay":
			_, exists = s.cfg.Store.AirPlay[id]
		default:
			_, exists = s.cfg.Store.Sonos[id]
		}
		if exists {
			members = []string{key}
		}
	})
	return members
}
