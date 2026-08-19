package listening

import (
	"testing"
	"time"

	"homehub/internal/sonos"
	"homehub/internal/speakermon"
	"homehub/internal/store"
)

// The recorder never reaches a speaker: everything below hands it the same
// readings a monitor would have cached, which is exactly what the live hooks
// do.

func testRecorder(t *testing.T, speakerIDs ...string) (*Recorder, *store.Store) {
	t.Helper()
	st := store.New(t.TempDir(), nil)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	for i, id := range speakerIDs {
		st.Sonos[id] = &store.SonosSpeaker{
			ID:   id,
			Name: "Room " + id,
			IP:   "192.0.2." + string(rune('1'+i)),
			UUID: "RINCON_" + id,
		}
	}
	return New(Config{
		Store:    st,
		Speakers: speakermon.New(speakermon.Config{Store: st}),
	}), st
}

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

// The rule that keeps the log worth reading: a track someone skipped past was
// not something the room played.
func TestNoteWaitsOutTheDwell(t *testing.T) {
	rec, st := testRecorder(t, "sp1")
	track := &sonos.Track{Title: "Song", Artist: "Band", SpotifyURI: "spotify:track:1"}

	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:03")}))
	if n := len(st.HeardIn("sonos:sp1")); n != 0 {
		t.Fatalf("a track three seconds in was logged (%d entries)", n)
	}

	// The same track seen again, now past the dwell: this is the reading
	// every path produces a few seconds later.
	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:42")}))
	log := st.HeardIn("sonos:sp1")
	if len(log) != 1 {
		t.Fatalf("log = %d entries, want 1", len(log))
	}
	if log[0].Title != "Song" || log[0].URI != "spotify:track:1" || log[0].Provider != "spotify" {
		t.Errorf("entry = %+v, want the track with its service URI", log[0])
	}
	if log[0].RoomName != "Room sp1" {
		t.Errorf("room name = %q, want the speaker's name", log[0].RoomName)
	}
	// Dated from the *first* sighting, backed off by how far in the track was
	// then — three seconds. The later reading doesn't re-date it: the first
	// observation is the closest thing to a start time there is, and on a live
	// house the two agree anyway.
	if since := time.Since(log[0].At); since < 2*time.Second || since > 30*time.Second {
		t.Errorf("At is %v ago, want ~3s — the first sighting, dated back by its position", since)
	}

	// Every further reading of the same track settles in memory.
	for i := 0; i < 3; i++ {
		rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:01:20")}))
	}
	if n := len(st.HeardIn("sonos:sp1")); n != 1 {
		t.Errorf("log grew to %d entries while one track played", n)
	}
}

// A room joined mid-song — the app opening, or HomeHub restarting — is logged
// from the first reading rather than made to wait out a dwell the song is
// already well past, and dated by where it says it is.
func TestNoteLogsATrackAlreadyPlaying(t *testing.T) {
	rec, st := testRecorder(t, "sp1")
	track := &sonos.Track{Title: "Song", Artist: "Band", SpotifyURI: "spotify:track:1"}

	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:02:00")}))
	log := st.HeardIn("sonos:sp1")
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
func TestNoteIgnoresSilenceAndEmptyTracks(t *testing.T) {
	rec, st := testRecorder(t, "sp1", "sp2", "sp3")

	paused := playing(&sonos.Track{Title: "Song"}, "0:02:00")
	paused.State.Playing = false
	unreachable := playing(&sonos.Track{Title: "Song"}, "0:02:00")
	unreachable.Reachable = false

	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{
		"sp1": paused,
		"sp2": unreachable,
		"sp3": playing(&sonos.Track{Title: "   "}, "0:02:00"),
	}))
	for _, key := range []string{"sonos:sp1", "sonos:sp2", "sonos:sp3"} {
		if n := len(st.HeardIn(key)); n != 0 {
			t.Errorf("%s logged %d entries, want none", key, n)
		}
	}
}

// Radio is the case the log is most useful for and the one with the least to
// go on: the station leaves the song in streamContent and the title is the
// stream itself.
func TestNoteReadsRadioTheWayThePlayerDoes(t *testing.T) {
	rec, st := testRecorder(t, "sp1")
	radio := &sonos.Track{Title: "P3 Live", Stream: "Band - Song", Station: "P3"}

	// A stream's position is time since tune-in, so it is past the dwell from
	// the first reading of any song after the first minute.
	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(radio, "0:31:00")}))
	log := st.HeardIn("sonos:sp1")
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
func TestNoteSurvivesTheQueueBeingReplaced(t *testing.T) {
	rec, st := testRecorder(t, "sp1")

	for _, title := range []string{"First", "Second", "Third"} {
		track := &sonos.Track{Title: title, Artist: "Band", SpotifyURI: "spotify:track:" + title}
		rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:45")}))
	}
	log := st.HeardIn("sonos:sp1")
	if len(log) != 3 {
		t.Fatalf("log = %d entries, want 3", len(log))
	}
	if log[0].Title != "Third" || log[2].Title != "First" {
		t.Errorf("log = %q…%q, want newest first", log[0].Title, log[2].Title)
	}
}

// Forgetting a room drops its watch, or the song playing right now would never
// be re-recorded: the recorder would still believe it had filed it.
func TestForgetLetsThePlayingTrackBeFiledAgain(t *testing.T) {
	rec, st := testRecorder(t, "sp1")
	track := &sonos.Track{Title: "Song", Artist: "Band", SpotifyURI: "spotify:track:1"}
	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:45")}))

	st.Mutate(func() { st.ForgetHeard("sonos:sp1") })
	rec.Forget("sonos:sp1")

	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:01:10")}))
	if n := len(st.HeardIn("sonos:sp1")); n != 1 {
		t.Errorf("log = %d entries after forgetting, want the playing track back", n)
	}
}

// A deleted and re-added speaker must not inherit the old one's watch.
func TestForgetMissingDropsRoomsThatAreGone(t *testing.T) {
	rec, st := testRecorder(t, "sp1")
	track := &sonos.Track{Title: "Song", Artist: "Band", SpotifyURI: "spotify:track:1"}
	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:00:45")}))

	st.Mutate(func() { st.ForgetHeard("sonos:sp1") })
	rec.ForgetMissing(func(string) bool { return false })

	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{"sp1": playing(track, "0:01:10")}))
	if n := len(st.HeardIn("sonos:sp1")); n != 1 {
		t.Errorf("log = %d entries, want the track filed against the re-added room", n)
	}
}

// Artwork is rewritten on the way in, because the log outlives the reading: a
// speaker-relative path is useless by the time someone scrolls back to the row.
func TestArtIsRewrittenForLater(t *testing.T) {
	st := store.New(t.TempDir(), nil)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	st.Sonos["sp1"] = &store.SonosSpeaker{ID: "sp1", Name: "Kitchen", IP: "192.0.2.1"}
	rec := New(Config{
		Store:    st,
		Speakers: speakermon.New(speakermon.Config{Store: st}),
		SonosArt: func(id, art string) string { return "/proxy/" + id + art },
	})

	rec.NoteSonos(snapshotOf(map[string]sonos.SpeakerState{
		"sp1": playing(&sonos.Track{Title: "Song", ArtURI: "/getaa?x=1"}, "0:00:45"),
	}))
	log := st.HeardIn("sonos:sp1")
	if len(log) != 1 || log[0].ArtURI != "/proxy/sp1/getaa?x=1" {
		t.Errorf("art = %q, want it routed through the proxy", log[0].ArtURI)
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
