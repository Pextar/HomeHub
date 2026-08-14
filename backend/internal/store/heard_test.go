package store

import (
	"testing"
	"time"
)

func heard(title, artist, uri string, at time.Time) HeardTrack {
	return HeardTrack{Title: title, Artist: artist, URI: uri, Provider: "spotify", At: at}
}

// The reason the recorder can be careless: a playing speaker is read every
// few seconds, and every one of those readings says the same thing.
func TestRecordHeardIgnoresTheSameTrackAgain(t *testing.T) {
	s := New(t.TempDir(), nil)
	base := time.Now().Add(-time.Hour)

	if !s.RecordHeard("sonos:a", heard("Song", "Band", "spotify:track:1", base)) {
		t.Fatal("first reading was not recorded")
	}
	for i := 0; i < 5; i++ {
		if s.RecordHeard("sonos:a", heard("Song", "Band", "spotify:track:1", base.Add(time.Duration(i)*time.Minute))) {
			t.Fatalf("reading %d was recorded again", i)
		}
	}
	log := s.HeardIn("sonos:a")
	if len(log) != 1 {
		t.Fatalf("log = %d entries, want 1", len(log))
	}
	if !log[0].At.Equal(base) {
		t.Errorf("At = %v, want the time it started (%v)", log[0].At, base)
	}
}

// The difference from the play shelf: this is a log, and a record played
// twice in an evening was played twice.
func TestRecordHeardKeepsANonConsecutiveRepeat(t *testing.T) {
	s := New(t.TempDir(), nil)
	base := time.Now().Add(-time.Hour)
	s.RecordHeard("sonos:a", heard("One", "Band", "spotify:track:1", base))
	s.RecordHeard("sonos:a", heard("Two", "Band", "spotify:track:2", base.Add(time.Minute)))
	s.RecordHeard("sonos:a", heard("One", "Band", "spotify:track:1", base.Add(2*time.Minute)))

	log := s.HeardIn("sonos:a")
	if len(log) != 3 {
		t.Fatalf("log = %d entries, want 3", len(log))
	}
	if log[0].Title != "One" || log[1].Title != "Two" || log[2].Title != "One" {
		t.Errorf("log = %q/%q/%q, want newest first", log[0].Title, log[1].Title, log[2].Title)
	}
}

// Radio carries no URI at all, so identity falls to the name — and the name
// is all a station gives when it says what it is playing.
func TestRecordHeardMatchesRadioByName(t *testing.T) {
	s := New(t.TempDir(), nil)
	now := time.Now()
	s.RecordHeard("sonos:a", HeardTrack{Title: "Song", Artist: "P3", At: now})
	if s.RecordHeard("sonos:a", HeardTrack{Title: "  song ", Artist: "p3", At: now.Add(time.Minute)}) {
		t.Error("the same stream line was recorded twice")
	}
	if !s.RecordHeard("sonos:a", HeardTrack{Title: "Another", Artist: "P3", At: now.Add(2 * time.Minute)}) {
		t.Error("a new stream line was not recorded")
	}
}

func TestRecordHeardRefusesJunkAndStaysBounded(t *testing.T) {
	s := New(t.TempDir(), nil)
	if s.RecordHeard("", heard("Song", "Band", "spotify:track:1", time.Now())) {
		t.Error("recorded against an empty room key")
	}
	if s.RecordHeard("sonos:a", HeardTrack{Title: "   ", At: time.Now()}) {
		t.Error("recorded a track with no name")
	}
	for i := 0; i < HeardLogSize+20; i++ {
		s.RecordHeard("sonos:a", heard("Song", "Band", "spotify:track:"+itoa(i), time.Now()))
	}
	if n := len(s.HeardIn("sonos:a")); n != HeardLogSize {
		t.Errorf("log kept %d entries, want %d", n, HeardLogSize)
	}
	// Newest first means the cap drops the oldest, not the latest.
	if got := s.HeardIn("sonos:a")[0].URI; got != "spotify:track:"+itoa(HeardLogSize+19) {
		t.Errorf("newest = %q, want the last one recorded", got)
	}
}

// A missing entry is what an empty log looks like, and a caller must not be
// able to reach into the store through what it hands back.
func TestHeardInCopies(t *testing.T) {
	s := New(t.TempDir(), nil)
	s.RecordHeard("sonos:a", heard("Song", "Band", "spotify:track:1", time.Now()))
	got := s.HeardIn("sonos:a")
	got[0].Title = "Rewritten"
	if s.HeardIn("sonos:a")[0].Title != "Song" {
		t.Error("HeardIn handed out the store's own slice")
	}
	if len(s.HeardIn("kef:nobody")) != 0 {
		t.Error("a room with no log answered with something")
	}
}

// The fallback behind an empty room: the household's own listening, newest
// first, with every row still naming where it was heard.
func TestRecentHeardMergesRoomsNewestFirst(t *testing.T) {
	s := New(t.TempDir(), nil)
	base := time.Now().Add(-time.Hour)
	s.RecordHeard("sonos:a", HeardTrack{Title: "Old", RoomName: "Kitchen", At: base})
	s.RecordHeard("kef:b", HeardTrack{Title: "New", RoomName: "Study", At: base.Add(10 * time.Minute)})
	s.RecordHeard("sonos:a", HeardTrack{Title: "Newest", RoomName: "Kitchen", At: base.Add(20 * time.Minute)})

	got := s.RecentHeard(10)
	if len(got) != 3 {
		t.Fatalf("merged = %d entries, want 3", len(got))
	}
	if got[0].Title != "Newest" || got[1].Title != "New" || got[2].Title != "Old" {
		t.Errorf("merged order = %q/%q/%q", got[0].Title, got[1].Title, got[2].Title)
	}
	if got[1].RoomName != "Study" {
		t.Errorf("room name = %q, want the room it was heard in", got[1].RoomName)
	}
	if n := len(s.RecentHeard(2)); n != 2 {
		t.Errorf("limit ignored: got %d entries, want 2", n)
	}
}

// Same promise as PruneHistory: a deleted speaker leaves nothing behind.
func TestPruneAndForgetHeard(t *testing.T) {
	s := New(t.TempDir(), nil)
	now := time.Now()
	s.RecordHeard("sonos:a", HeardTrack{Title: "A", At: now})
	s.RecordHeard("kef:gone", HeardTrack{Title: "B", At: now})

	if !s.PruneHeard(func(key string) bool { return key == "sonos:a" }) {
		t.Error("pruning reported nothing dropped")
	}
	if _, ok := s.Heard["kef:gone"]; ok {
		t.Error("a dead room kept its log")
	}
	if len(s.HeardIn("sonos:a")) != 1 {
		t.Error("a live room lost its log")
	}
	if s.PruneHeard(func(string) bool { return true }) {
		t.Error("pruning with nothing to drop reported a change")
	}

	if !s.ForgetHeard("sonos:a") {
		t.Error("forgetting a room with a log reported nothing")
	}
	if _, ok := s.Heard["sonos:a"]; ok {
		t.Error("the room kept its key after being forgotten")
	}
	if s.ForgetHeard("sonos:a") || s.ForgetHeard("") {
		t.Error("forgetting nothing reported a change")
	}
}

// itoa without importing strconv into a test file that needs nothing else
// from it.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
