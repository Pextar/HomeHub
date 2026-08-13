package media

import (
	"strings"
	"testing"
)

// airplayCaps is what the AirPlay adapter declares. A receiver is a sink: it
// can be sent audio, and it has a volume, and that is the whole list.
const airplayCaps = CapTransport | CapVolume | CapAirPlay

func airplayEP(name string) *fakeEndpoint { return ep(name, VendorAirPlay, airplayCaps, "") }

// spotifyLikeWithAirPlay is the real provider shape now: every route,
// including the one that pushes.
func spotifyLikeWithAirPlay() Provider {
	return (&fakeProvider{
		routes: RouteSet{RouteNative, RouteConnect, RouteGroup,
			RouteAirPlay, RouteStream},
		native:      true,
		connect:     true,
		stream:      true,
		streamAvail: Availability{OK: true, Configured: true},
	}).build()
}

func TestResolveSendsReceiversOverAirPlay(t *testing.T) {
	p := spotifyLikeWithAirPlay()

	t.Run("one receiver", func(t *testing.T) {
		plan, err := Resolve(p, []Endpoint{airplayEP("Study Pi")})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if plan.Route != RouteAirPlay {
			t.Fatalf("route = %q, want airplay", plan.Route)
		}
		if plan.Sync != SyncSingle {
			t.Errorf("sync = %q, want single", plan.Sync)
		}
		if len(plan.Targets) != 1 || plan.Coordinator != nil {
			t.Error("the airplay route addresses targets directly and has no coordinator")
		}
	})

	t.Run("several receivers", func(t *testing.T) {
		plan, err := Resolve(p, []Endpoint{airplayEP("Study"), airplayEP("Kitchen")})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if plan.Route != RouteAirPlay {
			t.Fatalf("route = %q, want airplay", plan.Route)
		}
		// The claim the route is worth making: better than buffered,
		// short of a vendor's own bus.
		if plan.Sync != SyncClocked {
			t.Errorf("sync = %q, want clocked", plan.Sync)
		}
		if len(plan.Endpoints()) != 2 {
			t.Errorf("both receivers should be addressed, got %d", len(plan.Endpoints()))
		}
	})
}

// The ranking guarantee, from the other side: a house of Sonos speakers must
// not notice that AirPlay exists. Adding a route above `stream` would be a
// regression if it ever outranked the native paths.
func TestAirPlayNeverOutranksTheNativeRoutes(t *testing.T) {
	p := spotifyLikeWithAirPlay()

	plan, err := Resolve(p, []Endpoint{sonosEP("Living Room")})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Route != RouteNative {
		t.Errorf("one Sonos should still be native, got %q", plan.Route)
	}

	plan, err = Resolve(p, []Endpoint{sonosEP("Living"), sonosEP("Kitchen")})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Route != RouteGroup {
		t.Errorf("two Sonos should still group, got %q", plan.Route)
	}

	plan, err = Resolve(p, []Endpoint{kefEP("Study")})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Route != RouteConnect {
		t.Errorf("one KEF should still be Connect, got %q", plan.Route)
	}
}

// AirPlay is preferred over the HTTP stream for a zone that could take either
// — which today means never, since no endpoint declares both, but the
// ordering is the guarantee and it is cheap to pin.
func TestAirPlayOutranksTheStreamRoute(t *testing.T) {
	both := ep("Hybrid", VendorAirPlay, airplayCaps|CapPlayURI, "")
	plan, err := Resolve(spotifyLikeWithAirPlay(), []Endpoint{both})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Route != RouteAirPlay {
		t.Errorf("route = %q, want airplay to win", plan.Route)
	}
	if RouteAirPlay.Rank() >= RouteStream.Rank() {
		t.Error("airplay must rank ahead of the stream route")
	}
}

// The one arrangement nothing serves: a speaker that fetches and a receiver
// that is pushed to. It has to fail with both halves named, because the fix
// is the user's choice between them.
func TestMixedFetchersAndReceiversExplainBothSides(t *testing.T) {
	_, err := Resolve(spotifyLikeWithAirPlay(),
		[]Endpoint{sonosEP("Living Room"), airplayEP("Study Pi")})
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Living Room") || !strings.Contains(msg, "Study Pi") {
		t.Errorf("both speakers should be named: %v", msg)
	}
	if !strings.Contains(msg, "AirPlay stream") {
		t.Errorf("the airplay rejection should say what the speaker can't take: %v", msg)
	}
	if !strings.Contains(msg, "stream URL") {
		t.Errorf("the stream rejection should say what the receiver can't fetch: %v", msg)
	}
}

// A provider with no decoder cannot serve receivers at all — there is nothing
// to push. The reason must be the provider's, not the speaker's, or the user
// goes looking at the wrong end.
func TestAirPlayNeedsADecoder(t *testing.T) {
	p := (&fakeProvider{
		routes: RouteSet{RouteAirPlay, RouteStream},
		stream: true,
		streamAvail: Availability{
			Configured: true,
			Reason:     "librespot isn't installed",
		},
	}).build()

	_, err := Resolve(p, []Endpoint{airplayEP("Study")})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "librespot isn't installed") {
		t.Errorf("error should carry the decoder's reason: %v", err)
	}
}
