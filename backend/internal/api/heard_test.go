package api

import (
	"net/http"
	"testing"
	"time"

	"homehub/internal/sonos"
	"homehub/internal/store"
)

// The endpoints. What gets written into the log, and when, is
// internal/listening's own test — these hand the recorder a reading only where
// a handler's answer depends on there being one.

func playing(track *sonos.Track, position string) sonos.SpeakerState {
	return sonos.SpeakerState{
		Reachable: true,
		State:     &sonos.State{Playing: true, TransportState: "PLAYING", Track: track, Position: position},
		At:        time.Now(),
	}
}

func snapshotOf(states map[string]sonos.SpeakerState) sonos.Snapshot {
	return sonos.Snapshot{Speakers: states, Live: true}
}

type heardResponse struct {
	Tracks    []store.HeardTrack `json:"tracks"`
	Household bool               `json:"household"`
}

func TestMediaHeardAnswersARoomThenTheHousehold(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	seedSonos(t, srv, "sp1")
	srv.Store.RecordHeard("sonos:sp1", store.HeardTrack{Title: "Song", RoomName: "Room sp1", At: time.Now()})

	rec := doAs(t, srv, admin, pass, http.MethodGet, "/api/media/heard?room=sonos:sp1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET heard = %d, want 200", rec.Code)
	}
	var own heardResponse
	decodeJSON(t, rec, &own)
	if len(own.Tracks) != 1 || own.Household {
		t.Errorf("own log = %+v, want one track not flagged as the household's", own)
	}

	// A room with nothing of its own falls back to the house, and says so.
	rec = doAs(t, srv, admin, pass, http.MethodGet, "/api/media/heard?room=kef:elsewhere", "")
	var fallback heardResponse
	decodeJSON(t, rec, &fallback)
	if len(fallback.Tracks) != 1 || !fallback.Household {
		t.Errorf("fallback = %+v, want the household's list, flagged", fallback)
	}
	if fallback.Tracks[0].RoomName != "Room sp1" {
		t.Error("a household row must name the room it was heard in")
	}
}

func TestMediaForgetHeardClearsARoomAndItsWatch(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	seedSonos(t, srv, "sp1")
	track := &sonos.Track{Title: "Song", Artist: "Band", SpotifyURI: "spotify:track:1"}
	srv.Listening.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:45")}))

	if rec := doAs(t, srv, admin, pass, http.MethodDelete, "/api/media/heard", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE with no room = %d, want 400", rec.Code)
	}
	rec := doAs(t, srv, admin, pass, http.MethodDelete, "/api/media/heard?room=sonos:sp1", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE heard = %d, want 204", rec.Code)
	}
	if n := len(srv.Store.HeardIn("sonos:sp1")); n != 0 {
		t.Fatalf("log kept %d entries after being cleared", n)
	}
	// The watch went with it, so the song still playing is filed again
	// rather than being remembered as already logged.
	srv.Listening.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:01:10")}))
	if n := len(srv.Store.HeardIn("sonos:sp1")); n != 1 {
		t.Errorf("log = %d entries after clearing, want the playing track back", n)
	}

	// Clearing a room that has nothing is still a 204: the caller's goal is
	// a state, not a deletion.
	if rec := doAs(t, srv, admin, pass, http.MethodDelete, "/api/media/heard?room=kef:nobody", ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE for an empty room = %d, want 204", rec.Code)
	}
}
