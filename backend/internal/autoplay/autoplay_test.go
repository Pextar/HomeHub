package autoplay

import (
	"testing"
	"time"

	"homehub/internal/sonos"
	"homehub/internal/store"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	return New(Config{Store: store.New(t.TempDir(), nil)})
}

// Autoplay is on unless a room opted out: starting a song asks for music, not
// for that song and then silence. What is remembered is therefore the opt-out,
// which also means a restart lands back on "keep playing".
func TestDefaultsOnAndRemembersTheOptOut(t *testing.T) {
	e := testEngine(t)

	if !e.Enabled("sp1") {
		t.Fatal("a speaker nobody has touched has autoplay off, want on")
	}
	// An id that isn't registered at all still reads as on — the setting is
	// an opt-out list, and the tick is what decides a speaker is relevant.
	if !e.Enabled("never-seen") {
		t.Error("unknown id reads as off, want on")
	}

	e.SetEnabled("sp1", false)
	if e.Enabled("sp1") {
		t.Error("autoplay still on after opting out")
	}

	e.SetEnabled("sp1", true)
	if !e.Enabled("sp1") {
		t.Error("autoplay still off after turning it back on")
	}
}

// Opting out forgets the room's history. Keeping it would mean a room switched
// back on hours later inherits a stale idea of whether its queue "just" ran
// dry — and gets restarted for it.
func TestOptingOutForgetsWhatTheRoomWasDoing(t *testing.T) {
	e := testEngine(t)
	e.noteHeard("sp1")
	e.SetEnabled("sp1", false)
	e.SetEnabled("sp1", true)
	if e.heardRecently("sp1") {
		t.Error("a room switched off and on again still counts as recently playing")
	}
}

// The part with teeth: which live states the tick acts on. "Keep the room
// going" must never turn into "restart what somebody just paused".
func TestDecide(t *testing.T) {
	playing := func(track int) *sonos.State {
		return &sonos.State{TransportState: "PLAYING", Playing: true, QueueTrack: track}
	}
	queue := func(n int) *sonos.GroupState {
		return &sonos.GroupState{QueueLength: n, FromQueue: true}
	}

	for _, tc := range []struct {
		name  string
		st    *sonos.State
		gs    *sonos.GroupState
		heard bool
		want  action
	}{
		{"follower has no group state", playing(1), nil, true, idle},
		{"speaker never answered", nil, queue(3), true, idle},
		{"mid-queue", playing(2), queue(5), true, idle},
		{"last track tops up", playing(5), queue(5), false, appendTo},
		{"single track queue tops up", playing(1), queue(1), false, appendTo},
		{
			"radio is left alone",
			playing(0),
			&sonos.GroupState{QueueLength: 4, FromQueue: false},
			true, idle,
		},
		{
			"a pause stays paused",
			&sonos.State{TransportState: "PAUSED_PLAYBACK", QueueTrack: 5},
			queue(5), true, idle,
		},
		{
			"stopped mid-queue is left alone",
			&sonos.State{TransportState: "STOPPED", QueueTrack: 2},
			queue(5), true, idle,
		},
		{
			"spent queue is picked back up",
			&sonos.State{TransportState: "STOPPED", QueueTrack: 5},
			queue(5), true, restart,
		},
		{
			"but not a room that was already quiet",
			&sonos.State{TransportState: "STOPPED", QueueTrack: 5},
			queue(5), false, idle,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := decide(tc.st, tc.gs, tc.heard); got != tc.want {
				t.Errorf("decide = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestHeardRecentlyExpires(t *testing.T) {
	e := testEngine(t)
	if e.heardRecently("sp1") {
		t.Error("a speaker never heard from reads as recently playing")
	}
	e.noteHeard("sp1")
	if !e.heardRecently("sp1") {
		t.Error("a speaker heard just now reads as quiet")
	}

	e.mu.Lock()
	e.heard["sp1"] = time.Now().Add(-resumeWindow - time.Minute)
	e.mu.Unlock()
	if e.heardRecently("sp1") {
		t.Error("a speaker last heard before the window still counts as recent")
	}
}

// With nothing to seed similar tracks from there is nothing to continue with,
// so Run must return rather than tick uselessly for the life of the process.
func TestRunStopsWithNoCatalogue(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		testEngine(t).Run(t.Context())
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Run kept ticking with no way to find similar tracks")
	}
}
