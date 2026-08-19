package api

// Music as part of a scene or an automation.
//
// Until this existed the house was two houses: "Film" could dim the lamps but
// not quiet the kitchen radio, "leaving" could switch off every socket and
// leave a speaker playing to an empty room, and the master All off meant all
// *sockets* off. Both halves of the machinery were already here — the staged
// socket flow on one side, the vendor-neutral media layer the music timers
// drive on the other — and nothing joined them.
//
// This is the join, and it is deliberately thin:
//
//   - The store holds the actions (store.MusicAction) and knows nothing about
//     how a room is reached. It calls Store.OnMusic, which is installed as
//     runSceneMusic below at wiring time — the same shape as OnChange.
//   - The verbs are the three that need no catalog item: pause, resume and
//     volume. A scene expresses a *moment* in the house; "put this record on"
//     needs something to play and a picker to choose it with, and that is
//     what the music timers already are (musictimer.go). Adding it here would
//     be a second, worse copy of a feature rather than a new one.
//   - Every action runs even if an earlier one failed, because the rooms are
//     independent and a scene that quiets three rooms should not stop at the
//     first speaker that has gone offline. Failures are logged, never
//     surfaced as a scene failure: the sockets are the scene's contract.

import (
	"context"
	"strings"
	"sync"
	"time"

	"homehub/internal/media"
	"homehub/internal/store"
)

// sceneMusicTimeout caps one room's worth of work. Generous next to a single
// SOAP call because a zone fans out across several speakers, and short next
// to the delay between scene steps.
const sceneMusicTimeout = 10 * time.Second

// runSceneMusic carries out one scene step's or automation rule's music
// actions. Installed as store.OnMusic.
//
// It returns as soon as the work is handed to a goroutine. The store calls
// this from a scene activation, a scheduler tick and a timer callback, and
// none of those may wait on a speaker: the same rule that keeps device I/O
// out from under Mu keeps it out of the tick.
func (s *Server) runSceneMusic(actions []store.MusicAction) {
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
				if err := s.applyMusicAction(a); err != nil {
					s.mediaLogf("scene music: %s %s: %v", a.Action, a.Room, err)
				}
			}(a)
		}
		wg.Wait()
	}()
}

// applyMusicAction is one room, one verb. Caller must NOT hold Mu.
func (s *Server) applyMusicAction(a store.MusicAction) error {
	eps, _, err := s.Music.Room(a.Room)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), sceneMusicTimeout)
	defer cancel()

	switch a.Action {
	case store.MusicPause:
		// Through the service's pause rather than a bare transport call: it
		// ends a streamed zone's Spotify session too, and a scene that left
		// one held would be the same bug the sleep timer already fixed.
		return s.Music.Pause(ctx, a.Room, eps)

	case store.MusicResume:
		// Whatever the room had loaded. A room with an empty queue refuses
		// this and says so in the log — there is nothing a scene could
		// usefully do about it, and the alternative (silently starting
		// something else) is the one thing a scene must not do.
		zoneID, isZone := strings.CutPrefix(a.Room, "zone:")
		plan := s.Music.Plan(zoneID, eps)
		if err := media.Control(ctx, plan, media.TransportPlay); err != nil {
			return err
		}
		if isZone {
			s.Music.Touch(eps)
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
		s.MusicTimers.CancelFade(a.Room)
		return media.SetVolume(ctx, eps, *a.Volume)
	}
	return nil
}
