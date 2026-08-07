package store

import (
	"testing"
	"time"
)

func play(uri, title string, at time.Time) MediaPlay {
	return MediaPlay{Provider: "spotify", Kind: "track", URI: uri, Title: title, At: at}
}

// A room left on the same record all evening is one row, not five: the
// shelves this feeds are read as "what could go on next", and the same album
// repeated is a shelf with one thing on it.
func TestRecordPlayDeDupesByURIKeepingTheLatest(t *testing.T) {
	s := New(t.TempDir(), nil)
	base := time.Now().Add(-time.Hour)
	s.RecordPlay("sonos:a", play("spotify:album:x", "X", base))
	s.RecordPlay("sonos:a", play("spotify:album:y", "Y", base.Add(time.Minute)))
	s.RecordPlay("sonos:a", play("spotify:album:x", "X", base.Add(2*time.Minute)))

	got := s.History("sonos:a")
	if len(got) != 2 {
		t.Fatalf("history = %d entries, want 2", len(got))
	}
	if got[0].URI != "spotify:album:x" || !got[0].At.Equal(base.Add(2*time.Minute)) {
		t.Errorf("newest = %+v, want the second X with its later time", got[0])
	}
	if got[1].URI != "spotify:album:y" {
		t.Errorf("second = %q, want Y", got[1].URI)
	}
}

func TestRecordPlayTrimsAndIgnoresJunk(t *testing.T) {
	s := New(t.TempDir(), nil)
	for i := 0; i < MediaHistorySize+10; i++ {
		s.RecordPlay("zone:z", play("spotify:track:"+string(rune('a'+i%26))+string(rune('a'+i/26)), "T", time.Now()))
	}
	if n := len(s.History("zone:z")); n > MediaHistorySize {
		t.Errorf("history kept %d entries, want at most %d", n, MediaHistorySize)
	}
	s.RecordPlay("", play("spotify:track:x", "X", time.Now()))
	s.RecordPlay("kef:k", play("", "no uri", time.Now()))
	if len(s.MediaHistory[""]) != 0 || len(s.History("kef:k")) != 0 {
		t.Error("a play with no room or no URI was recorded")
	}
}

// History is per room; the household merge is the fallback a room with none
// of its own gets, and it must not repeat a record two rooms both played.
func TestRecentPlaysMergesNewestFirstWithoutRepeats(t *testing.T) {
	s := New(t.TempDir(), nil)
	base := time.Now().Add(-time.Hour)
	s.RecordPlay("sonos:a", play("spotify:track:1", "One", base))
	s.RecordPlay("kef:b", play("spotify:track:2", "Two", base.Add(2*time.Minute)))
	s.RecordPlay("kef:b", play("spotify:track:1", "One", base.Add(3*time.Minute)))

	got := s.RecentPlays(10)
	if len(got) != 2 {
		t.Fatalf("recent = %d entries, want 2", len(got))
	}
	if got[0].URI != "spotify:track:1" || got[1].URI != "spotify:track:2" {
		t.Errorf("recent = %q then %q, want track:1 (newest) then track:2", got[0].URI, got[1].URI)
	}
	if n := len(s.RecentPlays(1)); n != 1 {
		t.Errorf("RecentPlays(1) returned %d, want the limit honoured", n)
	}
}

// A deleted speaker must not leave a shelf behind that plays to nothing —
// the same promise CascadeDeleteSocket makes for everything else.
func TestPruneHistoryDropsDeadRooms(t *testing.T) {
	s := New(t.TempDir(), nil)
	s.RecordPlay("sonos:live", play("spotify:track:1", "One", time.Now()))
	s.RecordPlay("kef:gone", play("spotify:track:2", "Two", time.Now()))

	if !s.PruneHistory(func(key string) bool { return key == "sonos:live" }) {
		t.Error("PruneHistory reported no change, want true")
	}
	if len(s.History("kef:gone")) != 0 {
		t.Error("a deleted room kept its history")
	}
	if len(s.History("sonos:live")) != 1 {
		t.Error("a live room lost its history")
	}
	if s.PruneHistory(func(string) bool { return true }) {
		t.Error("PruneHistory reported a change when nothing was dropped")
	}
}

// The history file is its own collection, so a round trip has to survive
// Load — that is the whole point of not folding it into Save.
func TestHistorySurvivesAReload(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, nil)
	s.RecordPlay("sonos:a", play("spotify:album:x", "X", time.Now()))
	if err := s.SaveHistory(); err != nil {
		t.Fatal(err)
	}

	again := New(dir, nil)
	if err := again.Load(); err != nil {
		t.Fatal(err)
	}
	if got := again.History("sonos:a"); len(got) != 1 || got[0].Title != "X" {
		t.Errorf("after reload history = %+v, want the one play", got)
	}
}
