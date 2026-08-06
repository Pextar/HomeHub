package api

import (
	"net/http"
	"testing"
	"time"

	"homehub/internal/sonos"
	"homehub/internal/store"
)

// Autoplay is on unless a room opted out: starting a song asks for music,
// not for that song and then silence. These pin the default, the opt-out
// round trip, and — the part with teeth — which live states the tick acts
// on, since "keep the room going" must never turn into "restart what
// somebody just paused".

func TestAutoplayDefaultsOnAndRemembersTheOptOut(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	srv.Store.Sonos["sp1"] = &store.SonosSpeaker{
		ID: "sp1", Name: "Kitchen", IP: "10.0.0.5", UUID: "RINCON_AAA",
	}

	if !srv.autoplayEnabled("sp1") {
		t.Fatal("a speaker nobody has touched has autoplay off, want on")
	}
	// An id that isn't registered at all still reads as on — the setting is
	// an opt-out list, and the tick is what decides a speaker is relevant.
	if !srv.autoplayEnabled("nope") {
		t.Error("unknown id reads as off, want on")
	}

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodPut,
		"/api/sonos/sp1/autoplay", `{"enabled":false}`), http.StatusNoContent)
	if srv.autoplayEnabled("sp1") {
		t.Error("autoplay still on after opting out")
	}

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodPut,
		"/api/sonos/sp1/autoplay", `{"enabled":true}`), http.StatusNoContent)
	if !srv.autoplayEnabled("sp1") {
		t.Error("autoplay still off after turning it back on")
	}
}

func TestAutoplayDecide(t *testing.T) {
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
		want  autoplayAction
	}{
		{"follower has no group state", playing(1), nil, true, autoplayIdle},
		{"speaker never answered", nil, queue(3), true, autoplayIdle},
		{"mid-queue", playing(2), queue(5), true, autoplayIdle},
		{"last track tops up", playing(5), queue(5), false, autoplayAppend},
		{"single track queue tops up", playing(1), queue(1), false, autoplayAppend},
		{
			"radio is left alone",
			playing(0),
			&sonos.GroupState{QueueLength: 4, FromQueue: false},
			true, autoplayIdle,
		},
		{
			"a pause stays paused",
			&sonos.State{TransportState: "PAUSED_PLAYBACK", QueueTrack: 5},
			queue(5), true, autoplayIdle,
		},
		{
			"stopped mid-queue is left alone",
			&sonos.State{TransportState: "STOPPED", QueueTrack: 2},
			queue(5), true, autoplayIdle,
		},
		{
			"spent queue is picked back up",
			&sonos.State{TransportState: "STOPPED", QueueTrack: 5},
			queue(5), true, autoplayRestart,
		},
		{
			"but not a room that was already quiet",
			&sonos.State{TransportState: "STOPPED", QueueTrack: 5},
			queue(5), false, autoplayIdle,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := autoplayDecide(tc.st, tc.gs, tc.heard); got != tc.want {
				t.Errorf("autoplayDecide = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAutoplayHeardRecentlyExpires(t *testing.T) {
	srv, _ := actionServer(t)
	if srv.autoplayHeardRecently("sp1") {
		t.Error("a speaker never heard from reads as recently playing")
	}
	srv.autoplayHeardNow("sp1")
	if !srv.autoplayHeardRecently("sp1") {
		t.Error("a speaker heard just now reads as quiet")
	}
	srv.autoplayMu.Lock()
	srv.autoplayHeard["sp1"] = time.Now().Add(-autoplayResumeWindow - time.Minute)
	srv.autoplayMu.Unlock()
	if srv.autoplayHeardRecently("sp1") {
		t.Error("a speaker last heard before the window still counts as recent")
	}
}
