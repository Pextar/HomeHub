package sonos

import (
	"testing"
)

func TestParsePlayMode(t *testing.T) {
	cases := map[string]PlayMode{
		"NORMAL":             {false, RepeatOff},
		"REPEAT_ALL":         {false, RepeatAll},
		"REPEAT_ONE":         {false, RepeatOne},
		"SHUFFLE_NOREPEAT":   {true, RepeatOff},
		"SHUFFLE":            {true, RepeatAll}, // shuffle + repeat all
		"SHUFFLE_REPEAT_ONE": {true, RepeatOne},
		" shuffle ":          {true, RepeatAll}, // trimmed and case-folded
		"WHAT_IS_THIS":       {false, RepeatOff},
		"":                   {false, RepeatOff},
	}
	for in, want := range cases {
		if got := ParsePlayMode(in); got != want {
			t.Errorf("ParsePlayMode(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestPlayModeString(t *testing.T) {
	// Every mode must survive a round trip, or a toggle in the UI would
	// silently reset the other axis.
	for name := range playModes {
		if got := ParsePlayMode(name).String(); got != name {
			t.Errorf("round trip of %q = %q", name, got)
		}
	}
	if got := (PlayMode{Repeat: "nonsense"}).String(); got != "NORMAL" {
		t.Errorf("String() of invalid mode = %q, want NORMAL", got)
	}
}

func TestPlayModeValid(t *testing.T) {
	if (PlayMode{Repeat: "sometimes"}).Valid() {
		t.Error("Valid() accepted an unknown repeat mode")
	}
	for _, r := range []string{RepeatOff, RepeatAll, RepeatOne} {
		if !(PlayMode{Repeat: r}).Valid() {
			t.Errorf("Valid() rejected %q", r)
		}
	}
}

func TestParseQueue(t *testing.T) {
	result := `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" ` +
		`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">` +
		`<item id="Q:0/1" parentID="Q:0"><dc:title>First</dc:title>` +
		`<dc:creator>Artist A</dc:creator><upnp:album>Album A</upnp:album>` +
		`<upnp:albumArtURI>/getaa?u=1</upnp:albumArtURI>` +
		`<res duration="0:03:12">x-sonos-spotify:track1</res></item>` +
		`<item id="Q:0/2" parentID="Q:0"><dc:title>Second</dc:title>` +
		`<res duration="0:00:00">x-sonos-spotify:track2</res></item>` +
		`</DIDL-Lite>`

	items, err := ParseQueue(result)
	if err != nil {
		t.Fatalf("ParseQueue: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	first := items[0]
	if first.Track != 1 || first.Title != "First" || first.Artist != "Artist A" ||
		first.Album != "Album A" || first.ArtURI != "/getaa?u=1" || first.Duration != "0:03:12" {
		t.Errorf("first item = %+v", first)
	}
	// A zero duration is a placeholder, not a real length.
	if items[1].Track != 2 || items[1].Duration != "" {
		t.Errorf("second item = %+v", items[1])
	}
}

func TestParseQueueEmpty(t *testing.T) {
	items, err := ParseQueue(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"></DIDL-Lite>`)
	if err != nil {
		t.Fatalf("ParseQueue: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("got %d items, want 0", len(items))
	}
}

func TestTrackFromQueueID(t *testing.T) {
	cases := map[string]int{
		"Q:0/1":  1,
		"Q:0/47": 47,
		"Q:0":    0,
		"Q:0/x":  0,
		"":       0,
	}
	for in, want := range cases {
		if got := trackFromQueueID(in); got != want {
			t.Errorf("trackFromQueueID(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSeekRejectsBadPosition(t *testing.T) {
	// Guards against a malformed clock reaching the speaker as a SOAP arg.
	bad := []string{"", "3:12", "1:2:3", "0:60:00", "0:00:99", "abc", "0:03:12 "}
	for _, p := range bad {
		if err := Seek(t.Context(), "192.168.1.50", p); err == nil {
			t.Errorf("Seek(%q) = nil, want error", p)
		}
	}
}
