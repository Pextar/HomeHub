package music

import (
	"testing"

	"homehub/internal/audio"
	"homehub/internal/speakermon"
	"homehub/internal/store"
)

// Playing is what an automation asks about a room, and the answer it must
// never give is a confident "quiet" about a room nothing can answer for — see
// the store.MusicPlaying contract. These cover the resolution half; the edge
// detection built on top of it lives in internal/scheduler.

// speakerService is a service over a house with one speaker of each kind.
func speakerService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st := store.New(t.TempDir(), nil)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	st.Sonos["son_1"] = &store.SonosSpeaker{
		ID: "son_1", Name: "Living Room", IP: "192.0.2.10", UUID: "RINCON_TEST01",
	}
	st.KEF["kef_1"] = &store.KEFSpeaker{
		ID: "kef_1", Name: "Study", IP: "192.0.2.20", MAC: "a1b2c3d4e5f6",
	}
	return New(Config{
		Store:    st,
		Speakers: speakermon.New(speakermon.Config{Store: st}),
		Audio:    audio.New(audio.Config{}),
	}), st
}

func TestRoomMembersResolvesTheThreeKindsOfKey(t *testing.T) {
	svc, st := speakerService(t)
	st.Zones["z1"] = &store.Zone{
		ID: "z1", Name: "Downstairs", Members: []string{"sonos:son_1", "kef:kef_1"},
	}

	cases := map[string][]string{
		"sonos:son_1": {"sonos:son_1"},
		"kef:kef_1":   {"kef:kef_1"},
		"zone:z1":     {"sonos:son_1", "kef:kef_1"},
	}
	for key, want := range cases {
		got := svc.roomMembers(key)
		if len(got) != len(want) {
			t.Errorf("%s -> %v, want %v", key, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s -> %v, want %v", key, got, want)
				break
			}
		}
	}
}

func TestRoomMembersEmptyForAnythingTheHouseLacks(t *testing.T) {
	svc, st := speakerService(t)
	st.Zones["empty"] = &store.Zone{ID: "empty", Name: "Nobody"}

	for _, key := range []string{
		"", "  ", "sonos:gone", "kef:gone", "zone:gone", "zone:empty", "living room",
	} {
		if got := svc.roomMembers(key); len(got) != 0 {
			t.Errorf("%q resolved to %v, want nothing", key, got)
		}
	}
}

// A room the house doesn't have, or one whose speakers no monitor has heard
// from, is unknown rather than quiet. A rule that dimmed the bedroom every
// time a speaker fell off the network is exactly what this prevents.
func TestRoomPlayingIsUnknownWithoutAReading(t *testing.T) {
	svc, _ := speakerService(t)

	if _, known := svc.Playing("sonos:gone"); known {
		t.Error("a room this house does not have answered")
	}
	// Registered, but the monitors have never read it: Cached() never
	// touches a speaker, so this is the state a cold start is in.
	if playing, known := svc.Playing("sonos:son_1"); known || playing {
		t.Errorf("a speaker with no cached reading answered playing=%v known=%v", playing, known)
	}
	if playing, known := svc.Playing("kef:kef_1"); known || playing {
		t.Errorf("a KEF with no cached reading answered playing=%v known=%v", playing, known)
	}
}

// An AirPlay receiver holds no state of its own — the cast HomeHub is running
// is the reading — so "nothing is casting" is a real answer rather than a
// missing one.
func TestRoomPlayingAnswersForAnIdleAirPlayReceiver(t *testing.T) {
	svc, st := speakerService(t)
	st.AirPlay["ap_1"] = &store.AirPlaySpeaker{ID: "ap_1", Name: "Kitchen", IP: "192.0.2.30"}

	playing, known := svc.Playing("airplay:ap_1")
	if !known {
		t.Fatal("an idle receiver should be a known answer, not an absent one")
	}
	if playing {
		t.Error("nothing is being cast, so nothing is playing")
	}
}
