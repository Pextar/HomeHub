package api

import (
	"net/http"
	"testing"
	"time"

	"homehub/internal/sonos"
	"homehub/internal/store"
)

// The recorder never reaches a speaker: everything below hands it the same
// readings a monitor would have cached, which is exactly what the live hooks
// do (heard.go).

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

// The rule that keeps the log worth reading: a track someone skipped past
// was not something the room played.
func TestNoteHeardWaitsOutTheDwell(t *testing.T) {
	srv, _ := actionServer(t)
	seedSonos(t, srv, "sp1")
	track := &sonos.Track{Title: "Song", Artist: "Band", SpotifyURI: "spotify:track:1"}

	srv.noteHeardSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:03")}))
	if n := len(srv.Store.HeardIn("sonos:sp1")); n != 0 {
		t.Fatalf("a track three seconds in was logged (%d entries)", n)
	}

	// The same track seen again, now past the dwell: this is the reading
	// every path produces a few seconds later.
	srv.noteHeardSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:42")}))
	log := srv.Store.HeardIn("sonos:sp1")
	if len(log) != 1 {
		t.Fatalf("log = %d entries, want 1", len(log))
	}
	if log[0].Title != "Song" || log[0].URI != "spotify:track:1" || log[0].Provider != "spotify" {
		t.Errorf("entry = %+v, want the track with its service URI", log[0])
	}
	if log[0].RoomName != "Room sp1" {
		t.Errorf("room name = %q, want the speaker's name", log[0].RoomName)
	}
	// Dated from the *first* sighting, backed off by how far in the track
	// was then — three seconds. The later reading doesn't re-date it: the
	// first observation is the closest thing to a start time there is, and
	// on a live house the two agree anyway.
	if since := time.Since(log[0].At); since < 2*time.Second || since > 30*time.Second {
		t.Errorf("At is %v ago, want ~3s — the first sighting, dated back by its position", since)
	}

	// Every further reading of the same track settles in memory.
	for i := 0; i < 3; i++ {
		srv.noteHeardSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:01:20")}))
	}
	if n := len(srv.Store.HeardIn("sonos:sp1")); n != 1 {
		t.Errorf("log grew to %d entries while one track played", n)
	}
}

// A room joined mid-song — the app opening, or HomeHub restarting — is
// logged from the first reading rather than made to wait out a dwell the
// song is already well past, and dated by where it says it is.
func TestNoteHeardLogsATrackAlreadyPlaying(t *testing.T) {
	srv, _ := actionServer(t)
	seedSonos(t, srv, "sp1")
	track := &sonos.Track{Title: "Song", Artist: "Band", SpotifyURI: "spotify:track:1"}

	srv.noteHeardSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:02:00")}))
	log := srv.Store.HeardIn("sonos:sp1")
	if len(log) != 1 {
		t.Fatalf("log = %d entries, want the track that was already playing", len(log))
	}
	if since := time.Since(log[0].At); since < 110*time.Second || since > 150*time.Second {
		t.Errorf("At is %v ago, want ~2 minutes — where the track said it was", since)
	}
}

// A speaker that isn't playing, or has nothing to say about what it is
// playing, contributes nothing — including a paused one, which still reports
// its track.
func TestNoteHeardIgnoresSilenceAndEmptyTracks(t *testing.T) {
	srv, _ := actionServer(t)
	seedSonos(t, srv, "sp1", "sp2", "sp3")

	paused := playing(&sonos.Track{Title: "Song"}, "0:02:00")
	paused.State.Playing = false
	unreachable := playing(&sonos.Track{Title: "Song"}, "0:02:00")
	unreachable.Reachable = false

	srv.noteHeardSonos(snapshotOf(map[string]sonos.SpeakerState{
		"sp1": paused,
		"sp2": unreachable,
		"sp3": playing(&sonos.Track{Title: "   "}, "0:02:00"),
	}))
	for _, key := range []string{"sonos:sp1", "sonos:sp2", "sonos:sp3"} {
		if n := len(srv.Store.HeardIn(key)); n != 0 {
			t.Errorf("%s logged %d entries, want none", key, n)
		}
	}
}

// Radio is the case the log is most useful for and the one with the least to
// go on: the station leaves the song in streamContent and the title is the
// stream itself.
func TestNoteHeardReadsRadioTheWayThePlayerDoes(t *testing.T) {
	srv, _ := actionServer(t)
	seedSonos(t, srv, "sp1")
	radio := &sonos.Track{Title: "P3 Live", Stream: "Band - Song", Station: "P3"}

	// A stream's position is time since tune-in, so it is past the dwell
	// from the first reading of any song after the first minute.
	srv.noteHeardSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(radio, "0:31:00")}))
	log := srv.Store.HeardIn("sonos:sp1")
	if len(log) != 1 {
		t.Fatalf("log = %d entries, want 1", len(log))
	}
	if log[0].Title != "Band - Song" || log[0].Artist != "P3" {
		t.Errorf("entry = %+v, want the song as the headline and the station under it", log[0])
	}
	if log[0].URI != "" {
		t.Errorf("URI = %q, want none — radio has nothing to play again", log[0].URI)
	}
}

// What the whole feature is for: the queue is replaced, and the log isn't.
func TestNoteHeardSurvivesTheQueueBeingReplaced(t *testing.T) {
	srv, _ := actionServer(t)
	seedSonos(t, srv, "sp1")

	for _, title := range []string{"First", "Second", "Third"} {
		track := &sonos.Track{Title: title, Artist: "Band", SpotifyURI: "spotify:track:" + title}
		srv.noteHeardSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:45")}))
	}
	log := srv.Store.HeardIn("sonos:sp1")
	if len(log) != 3 {
		t.Fatalf("log = %d entries, want 3", len(log))
	}
	if log[0].Title != "Third" || log[2].Title != "First" {
		t.Errorf("log = %q…%q, want newest first", log[0].Title, log[2].Title)
	}
}

// ── The endpoints ────────────────────────────────────────────────────────

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
	srv.noteHeardSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:45")}))

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
	srv.noteHeardSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:01:10")}))
	if n := len(srv.Store.HeardIn("sonos:sp1")); n != 1 {
		t.Errorf("log = %d entries after clearing, want the playing track back", n)
	}

	// Clearing a room that has nothing is still a 204: the caller's goal is
	// a state, not a deletion.
	if rec := doAs(t, srv, admin, pass, http.MethodDelete, "/api/media/heard?room=kef:nobody", ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE for an empty room = %d, want 204", rec.Code)
	}
}

func TestSecsOfReadsSonosPositions(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want time.Duration
	}{
		{"0:00:42", 42 * time.Second},
		{"1:02:03", 3723 * time.Second},
		{"", 0},
		{"NOT_IMPLEMENTED", 0},
		{"0:0x:12", 0},
	} {
		if got := secsOf(tc.in); got != tc.want {
			t.Errorf("secsOf(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
