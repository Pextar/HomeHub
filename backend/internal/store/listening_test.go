package store

import (
	"testing"
	"time"
)

// at builds a local time on a fixed day at the given hour, so the histogram
// assertions below don't depend on when the suite runs.
func at(hour, min int) time.Time {
	return time.Date(2026, 3, 14, hour, min, 0, 0, time.Local)
}

// The de-dupe in RecordPlay is what makes the shelf readable; the tally is
// what stops it also throwing away the only thing that separates the record
// this room lives on from the one someone tried once.
func TestRecordPlayCountsRepeatsInsteadOfLosingThem(t *testing.T) {
	s := New(t.TempDir(), nil)
	s.RecordPlay("sonos:a", play("spotify:album:x", "X", at(8, 0)))
	s.RecordPlay("sonos:a", play("spotify:album:y", "Y", at(9, 0)))
	s.RecordPlay("sonos:a", play("spotify:album:x", "X", at(8, 30)))
	s.RecordPlay("sonos:a", play("spotify:album:x", "X", at(20, 0)))

	got := s.History("sonos:a")
	if len(got) != 2 {
		t.Fatalf("history = %d entries, want 2", len(got))
	}
	x := got[0]
	if x.URI != "spotify:album:x" {
		t.Fatalf("newest = %q, want album:x", x.URI)
	}
	if x.Plays() != 3 {
		t.Errorf("plays = %d, want 3", x.Plays())
	}
	if !x.FirstAt.Equal(at(8, 0)) {
		t.Errorf("first_at = %v, want the 08:00 play", x.FirstAt)
	}
	if !x.At.Equal(at(20, 0)) {
		t.Errorf("at = %v, want the 20:00 play", x.At)
	}
	if x.PlaysAt(8) != 2 || x.PlaysAt(20) != 1 || x.PlaysAt(9) != 0 {
		t.Errorf("hours = %v, want two at 08 and one at 20", x.Hours)
	}
}

// An entry written before the tally existed carries no count and no
// histogram. It must still read as the one play that created it, and folding
// a new play into it must not restart the count at one.
func TestPlaysReadsAPreTallyEntryAsOnePlay(t *testing.T) {
	s := New(t.TempDir(), nil)
	old := MediaPlay{Provider: "spotify", Kind: "track", URI: "spotify:track:1", Title: "One", At: at(7, 0)}
	s.MediaHistory = map[string][]MediaPlay{"sonos:a": {old}}

	if got := old.Plays(); got != 1 {
		t.Errorf("Plays() on an untallied entry = %d, want 1", got)
	}
	if got := old.PlaysAt(7); got != 0 {
		t.Errorf("PlaysAt on an entry with no histogram = %d, want 0 — it is not evidence about any hour", got)
	}

	s.RecordPlay("sonos:a", play("spotify:track:1", "One", at(21, 0)))
	got := s.History("sonos:a")[0]
	if got.Plays() != 2 {
		t.Errorf("plays after folding in a new play = %d, want 2", got.Plays())
	}
	if !got.FirstAt.Equal(at(7, 0)) {
		t.Errorf("first_at = %v, want the old entry's own time — the oldest moment we can claim", got.FirstAt)
	}
	if got.PlaysAt(21) != 1 {
		t.Errorf("hours = %v, want the new play counted at 21", got.Hours)
	}
}

func TestTopPlaysRanksByCountThenRecency(t *testing.T) {
	s := New(t.TempDir(), nil)
	s.RecordPlay("sonos:a", play("spotify:track:once", "Once", at(10, 0)))
	s.RecordPlay("sonos:a", play("spotify:track:twice", "Twice", at(11, 0)))
	s.RecordPlay("sonos:a", play("spotify:track:twice", "Twice", at(12, 0)))
	s.RecordPlay("sonos:a", play("spotify:track:also", "Also", at(13, 0)))
	s.RecordPlay("sonos:a", play("spotify:track:also", "Also", at(14, 0)))

	got := s.TopPlays("sonos:a", 10)
	if len(got) != 3 {
		t.Fatalf("top = %d entries, want 3", len(got))
	}
	if got[0].URI != "spotify:track:also" || got[1].URI != "spotify:track:twice" {
		t.Errorf("top = %q then %q, want the two-play tie broken by recency", got[0].URI, got[1].URI)
	}
	if got[2].URI != "spotify:track:once" {
		t.Errorf("last = %q, want the single play", got[2].URI)
	}
	if n := len(s.TopPlays("sonos:a", 1)); n != 1 {
		t.Errorf("TopPlays(1) returned %d, want the limit honoured", n)
	}
	if n := len(s.TopPlays("sonos:never", 5)); n != 0 {
		t.Errorf("a room that has played nothing returned %d entries, want none", n)
	}
}

// The point of the hour histogram: what a room plays at breakfast is not what
// it plays at dinner, and by evening both are "recent".
func TestPlaysAtHourPrefersTheRoomsHabitAndStaysSilentWithoutOne(t *testing.T) {
	s := New(t.TempDir(), nil)
	for _, h := range []int{7, 8, 8} {
		s.RecordPlay("sonos:kitchen", play("spotify:playlist:radio", "Radio", at(h, 0)))
	}
	for _, h := range []int{18, 19, 19, 20} {
		s.RecordPlay("sonos:kitchen", play("spotify:album:dinner", "Dinner", at(h, 0)))
	}

	morning := s.PlaysAtHour("sonos:kitchen", 8, 5)
	if len(morning) != 1 || morning[0].URI != "spotify:playlist:radio" {
		t.Errorf("08:00 = %+v, want only the breakfast radio", morning)
	}
	evening := s.PlaysAtHour("sonos:kitchen", 19, 5)
	if len(evening) != 1 || evening[0].URI != "spotify:album:dinner" {
		t.Errorf("19:00 = %+v, want only the dinner record", evening)
	}
	if got := s.PlaysAtHour("sonos:kitchen", 3, 5); len(got) != 0 {
		t.Errorf("03:00 = %+v, want nothing — this room has no habit then", got)
	}
	// Dinner is the room's favourite overall, which is what an hour with no
	// habit should fall back to.
	if top := s.TopPlays("sonos:kitchen", 1); len(top) != 1 || top[0].URI != "spotify:album:dinner" {
		t.Errorf("overall top = %+v, want the dinner record", top)
	}
}

func TestSummariseMergesRoomsWithoutDoubleCountingItems(t *testing.T) {
	s := New(t.TempDir(), nil)
	rec := func(room, name, kind, uri, title, sub string, hours ...int) {
		for _, h := range hours {
			p := MediaPlay{
				Provider: "spotify", Kind: kind, URI: uri, Title: title,
				Sub: sub, RoomName: name, At: at(h, 0),
			}
			s.RecordPlay(room, p)
		}
	}
	rec("sonos:kitchen", "Kitchen", "track", "spotify:track:1", "One", "Nils Frahm", 8, 9)
	rec("sonos:kitchen", "Kitchen", "track", "spotify:track:2", "Two", "Nils Frahm", 8)
	rec("kef:study", "Study", "track", "spotify:track:1", "One", "Nils Frahm", 22)
	rec("kef:study", "Study", "playlist", "spotify:playlist:p", "Mix", "Spotify", 22)

	got := s.Summarise(8)
	if got.Plays != 5 {
		t.Errorf("plays = %d, want 5", got.Plays)
	}
	if got.Items != 3 {
		t.Errorf("items = %d, want 3 — track:1 is one item played in two rooms", got.Items)
	}
	if len(got.Top) == 0 || got.Top[0].URI != "spotify:track:1" || got.Top[0].Plays() != 3 {
		t.Errorf("top = %+v, want track:1 with its three plays merged across rooms", got.Top)
	}
	if got.Top[0].RoomName != "" {
		t.Errorf("merged item names a room (%q); once merged it belongs to no one room", got.Top[0].RoomName)
	}
	if len(got.Rooms) != 2 || got.Rooms[0].Name != "Kitchen" || got.Rooms[0].Plays != 3 {
		t.Errorf("rooms = %+v, want Kitchen busiest with 3", got.Rooms)
	}
	// A playlist's second line is its owner, not an artist.
	if len(got.Artists) != 1 || got.Artists[0].Name != "Nils Frahm" || got.Artists[0].Plays != 4 {
		t.Errorf("artists = %+v, want only Nils Frahm with 4 plays", got.Artists)
	}
	if len(got.Hours) != hoursInDay || got.Hours[8] != 2 || got.Hours[22] != 2 {
		t.Errorf("hours = %v, want two at 08 and two at 22", got.Hours)
	}
	if !got.Since.Equal(at(8, 0)) {
		t.Errorf("since = %v, want the earliest first play", got.Since)
	}
}

func TestSummariseOnAnEmptyHouseAnswersEmptyListsNotNulls(t *testing.T) {
	s := New(t.TempDir(), nil)
	got := s.Summarise(0)
	if got.Plays != 0 || got.Items != 0 {
		t.Errorf("empty house summarised as %+v", got)
	}
	if got.Rooms == nil || got.Artists == nil || got.Top == nil {
		t.Error("nil lists would encode as JSON null; the frontend maps over these")
	}
	if len(got.Hours) != hoursInDay {
		t.Errorf("hours = %d slots, want %d even when empty", len(got.Hours), hoursInDay)
	}
	if !got.Since.IsZero() {
		t.Errorf("since = %v, want zero when nothing has been played", got.Since)
	}
}
