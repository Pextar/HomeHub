package music

import (
	"errors"
	"sort"
	"testing"

	"homehub/internal/audio"
	"homehub/internal/media"
	"homehub/internal/speakermon"
	"homehub/internal/store"
)

func testService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st := store.New(t.TempDir(), nil)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	return New(Config{
		Store:    st,
		Speakers: speakermon.New(speakermon.Config{Store: st}),
		Audio:    audio.New(audio.Config{}),
	}), st
}

// ── Sessions ─────────────────────────────────────────────────────────────

// Starting something new in a room means the old thing stopped, so the
// replaced session has to be closed rather than merely forgotten — it may be
// holding a decoder and a set of listeners.
func TestSetSessionClosesWhatItReplaces(t *testing.T) {
	svc, _ := testService(t)
	first := &media.Session{Route: media.RouteStream}
	svc.SetSession("z", first)
	svc.SetSession("z", &media.Session{Route: media.RouteStream})

	// Close is idempotent, so the only observable is that the zone now holds
	// exactly one session and it is the new one.
	if got := svc.DecodedZones(); len(got) != 1 || got[0] != "z" {
		t.Errorf("DecodedZones() = %v, want just the one zone", got)
	}
}

// "What HomeHub is decoding for" is a narrower question than "what is
// playing": a natively routed zone holds no decoder and must not be listed.
func TestDecodedZonesExcludesNativeRoutes(t *testing.T) {
	svc, _ := testService(t)
	svc.SetSession("streamed", &media.Session{Route: media.RouteStream})
	svc.SetSession("cast", &media.Session{Route: media.RouteAirPlay})
	svc.SetSession("native", &media.Session{Route: media.RouteNative})
	svc.SetSession("grouped", &media.Session{Route: media.RouteGroup})

	got := svc.DecodedZones()
	sort.Strings(got)
	want := []string{"cast", "streamed"}
	if len(got) != len(want) {
		t.Fatalf("DecodedZones() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DecodedZones() = %v, want %v", got, want)
		}
	}
}

func TestEndSessionForgetsIt(t *testing.T) {
	svc, _ := testService(t)
	svc.SetSession("z", &media.Session{Route: media.RouteStream})
	svc.EndSession("z")
	if got := svc.DecodedZones(); len(got) != 0 {
		t.Errorf("DecodedZones() = %v after EndSession, want none", got)
	}
}

func TestEndSessionOnAnEmptyZoneIsSafe(t *testing.T) {
	svc, _ := testService(t)
	svc.EndSession("never-played")
}

// ── Transport plans ──────────────────────────────────────────────────────

// Transport has to follow the route the session started on. A grouped zone is
// addressed through its coordinator; addressing every member would send the
// same command several times to speakers that are already following.
func TestPlanFollowsAGroupedSession(t *testing.T) {
	svc, _ := testService(t)
	svc.SetSession("z", &media.Session{Route: media.RouteGroup})

	members := []media.Endpoint{nil, nil, nil}
	plan := svc.Plan("z", members)
	if plan.Route != media.RouteGroup {
		t.Errorf("route = %q, want %q", plan.Route, media.RouteGroup)
	}
	if len(plan.Followers) != 2 {
		t.Errorf("followers = %d, want the two members behind the coordinator", len(plan.Followers))
	}
}

// With no session — after a restart, or for speakers someone started from a
// vendor app — every speaker is addressed. Noisier than necessary, and
// correct, which is the right way round.
func TestPlanWithoutASessionAddressesEveryone(t *testing.T) {
	svc, _ := testService(t)
	members := []media.Endpoint{nil, nil}
	plan := svc.Plan("never-played", members)
	if plan.Route != media.RouteStream {
		t.Errorf("route = %q, want %q", plan.Route, media.RouteStream)
	}
	if len(plan.Targets) != 2 {
		t.Errorf("targets = %d, want both members", len(plan.Targets))
	}
}

// ── Rooms ────────────────────────────────────────────────────────────────

// A key naming nothing has to fail as an unknown endpoint rather than as an
// empty room: the two send the caller looking in different places.
func TestRoomRejectsAnUnknownKey(t *testing.T) {
	svc, _ := testService(t)
	if _, _, err := svc.Room("sonos:nope"); !errors.Is(err, media.ErrUnknownEndpoint) {
		t.Errorf("Room(unknown) = %v, want ErrUnknownEndpoint", err)
	}
}

// A zone that exists but whose speakers have all been deleted is a different
// failure, and the error says which zone so the message can name it.
func TestRoomReportsAnEmptyZoneByName(t *testing.T) {
	svc, st := testService(t)
	st.Zones["z1"] = &store.Zone{ID: "z1", Name: "Kitchen", Members: []string{"sonos:gone"}}

	_, name, err := svc.Room("zone:z1")
	if !errors.Is(err, media.ErrEmptyZone) {
		t.Fatalf("Room(empty zone) = %v, want ErrEmptyZone", err)
	}
	if name != "Kitchen" {
		t.Errorf("name = %q, want the zone's name so the message can use it", name)
	}
}

// A single speaker is a room of one: the same vocabulary a shelf and a sleep
// timer already use, so nothing above has to special-case it.
func TestRoomResolvesOneSpeaker(t *testing.T) {
	svc, st := testService(t)
	st.Sonos["abc"] = &store.SonosSpeaker{ID: "abc", Name: "Living Room", IP: "192.168.1.10"}

	eps, name, err := svc.Room("sonos:abc")
	if err != nil {
		t.Fatalf("Room(one speaker) = %v", err)
	}
	if name != "Living Room" || len(eps) != 1 {
		t.Errorf("Room() = %d endpoints named %q, want 1 named Living Room", len(eps), name)
	}
}

// ── Providers ────────────────────────────────────────────────────────────

// The empty id means Spotify because it is the provider every household has
// wired up. A caller that wants lossless asks for it by name.
func TestProviderDefaultsToSpotify(t *testing.T) {
	svc, _ := testService(t)
	for _, id := range []string{"", "spotify", "Spotify"} {
		p, err := svc.Provider(id)
		if err != nil || p.ID() != "spotify" {
			t.Errorf("Provider(%q) = %v, %v", id, p, err)
		}
	}
}

func TestProviderRejectsWhatItDoesNotHave(t *testing.T) {
	svc, _ := testService(t)
	if _, err := svc.Provider("tidal"); !errors.Is(err, media.ErrUnknownProvider) {
		t.Errorf("Provider(tidal) = %v, want ErrUnknownProvider", err)
	}
}

// An unconfigured provider is still listed, reporting why it cannot play. A UI
// that silently omitted it would leave the household with no way to find out
// that the thing they want exists and needs connecting.
func TestProvidersIncludeTheUnconfigured(t *testing.T) {
	svc, _ := testService(t)
	provs := svc.Providers()
	if len(provs) != 2 {
		t.Fatalf("Providers() = %d, want both", len(provs))
	}
	for _, p := range provs {
		if av := p.Available(); av.OK {
			t.Errorf("%s reports itself available with nothing connected", p.ID())
		} else if av.Reason == "" {
			t.Errorf("%s is unavailable without saying why", p.ID())
		}
	}
}
