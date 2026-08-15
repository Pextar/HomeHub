package api

import (
	"testing"

	"homehub/internal/store"
)

// roomPlaying is what an automation asks about a room, and the answer it must
// never give is a confident "quiet" about a room nothing can answer for — see
// the store.MusicPlaying contract. These cover the resolution half; the edge
// detection built on top of it lives in internal/scheduler.

func TestMediaRoomMembersResolvesTheThreeKindsOfKey(t *testing.T) {
	srv := withSpeakers(t)
	srv.Store.Zones["z1"] = &store.Zone{
		ID: "z1", Name: "Downstairs", Members: []string{"sonos:son_1", "kef:kef_1"},
	}

	cases := map[string][]string{
		"sonos:son_1": {"sonos:son_1"},
		"kef:kef_1":   {"kef:kef_1"},
		"zone:z1":     {"sonos:son_1", "kef:kef_1"},
	}
	for key, want := range cases {
		got := srv.mediaRoomMembers(key)
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

func TestMediaRoomMembersEmptyForAnythingTheHouseLacks(t *testing.T) {
	srv := withSpeakers(t)
	srv.Store.Zones["empty"] = &store.Zone{ID: "empty", Name: "Nobody"}

	for _, key := range []string{
		"", "  ", "sonos:gone", "kef:gone", "zone:gone", "zone:empty", "living room",
	} {
		if got := srv.mediaRoomMembers(key); len(got) != 0 {
			t.Errorf("%q resolved to %v, want nothing", key, got)
		}
	}
}

// A room the house doesn't have, or one whose speakers no monitor has heard
// from, is unknown rather than quiet. A rule that dimmed the bedroom every
// time a speaker fell off the network is exactly what this prevents.
func TestRoomPlayingIsUnknownWithoutAReading(t *testing.T) {
	srv := withSpeakers(t)

	if _, known := srv.roomPlaying("sonos:gone"); known {
		t.Error("a room this house does not have answered")
	}
	// Registered, but the monitors have never read it: Cached() never
	// touches a speaker, so this is the state a cold start is in.
	if playing, known := srv.roomPlaying("sonos:son_1"); known || playing {
		t.Errorf("a speaker with no cached reading answered playing=%v known=%v", playing, known)
	}
	if playing, known := srv.roomPlaying("kef:kef_1"); known || playing {
		t.Errorf("a KEF with no cached reading answered playing=%v known=%v", playing, known)
	}
}

// An AirPlay receiver holds no state of its own — the cast HomeHub is running
// is the reading — so "nothing is casting" is a real answer rather than a
// missing one.
func TestRoomPlayingAnswersForAnIdleAirPlayReceiver(t *testing.T) {
	srv := testServer(t)
	srv.Store.AirPlay["ap_1"] = &store.AirPlaySpeaker{ID: "ap_1", Name: "Kitchen", IP: "192.0.2.30"}

	playing, known := srv.roomPlaying("airplay:ap_1")
	if !known {
		t.Fatal("an idle receiver should be a known answer, not an absent one")
	}
	if playing {
		t.Error("nothing is being cast, so nothing is playing")
	}
}

// The store calls this through its hook, so the hook has to be installed.
// Without it every music rule silently never fires.
func TestMusicPlayingHookIsInstalled(t *testing.T) {
	srv := testServer(t)
	srv.Handler() // wiring happens where OnMusic's does

	if srv.Store.MusicPlaying == nil {
		t.Fatal("Store.MusicPlaying was never wired to the api layer")
	}
	if _, known := srv.Store.RoomPlaying("sonos:gone"); known {
		t.Error("the hook claimed to know about a room the house lacks")
	}
}
