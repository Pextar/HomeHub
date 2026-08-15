package mediabridge

import (
	"context"
	"testing"

	"homehub/internal/media"
	"homehub/internal/qobuz"
	"homehub/internal/store"
)

func renderer(name string, withVolume bool) store.UPnPRenderer {
	rn := store.UPnPRenderer{
		ID: "rp", Name: name, IP: "10.0.0.9", Port: 49152,
		AVTransportURL: "http://10.0.0.9:49152/AVTransport/ctrl",
		PlaysPCM:       true,
	}
	if withVolume {
		rn.RenderingControlURL = "http://10.0.0.9:49152/RenderingControl/ctrl"
	}
	return rn
}

// The whole reason this bridge exists. The same physical box is capped at CD
// quality over AirPlay because RAOP carries nothing else, and unlimited here
// because it fetches the stream and reads the header. Nothing about the
// hardware changed between these two lines.
func TestARendererCarriesWhatAirPlayCannot(t *testing.T) {
	e := NewUPnPEndpoint(renderer("RoPieee", true))
	caps := e.Descriptor().Caps

	if !caps.Has(media.CapPlayURI) {
		t.Fatal("a renderer fetches, and that capability is the entire point")
	}
	if caps.Has(media.CapAirPlay) {
		t.Error("a renderer is not pushed audio and must not claim to be")
	}

	// Routing a hi-res track: AirPlay would have to reduce it, the stream
	// route would not.
	hi := media.PCMFormat{SampleRate: 192000, BitDepth: 24, Channels: 2, LittleEndian: true}
	if _, yes := media.RouteReduces(media.RouteAirPlay, &hi); !yes {
		t.Error("AirPlay cannot carry 24/192")
	}
	if _, yes := media.RouteReduces(media.RouteStream, &hi); yes {
		t.Error("the stream route has no ceiling and must carry 24/192 intact")
	}
}

// End to end: a hi-res Qobuz album to a zone of one renderer resolves to the
// stream route and plays. This is the case that produced "no route can play
// Qobuz to these speakers" when the only endpoint was AirPlay-only.
func TestHiResReachesARendererOverTheStreamRoute(t *testing.T) {
	eps := []media.Endpoint{NewUPnPEndpoint(renderer("RoPieee", true))}
	p := NewQobuzProvider(fakeAccount{
		status: qobuz.Status{Configured: true, Connected: true, MaxFormat: qobuz.FormatHiRes192},
		max:    qobuz.FormatHiRes192,
	}, okDecoder{})

	hi := media.PCMFormat{SampleRate: 192000, BitDepth: 24, Channels: 2, LittleEndian: true}
	plan, err := media.ResolveFor(p, eps, &hi)
	if err != nil {
		t.Fatalf("a hi-res track must reach a renderer: %v", err)
	}
	if plan.Route != media.RouteStream {
		t.Errorf("route = %s, want stream", plan.Route)
	}
	if len(plan.Targets) != 1 {
		t.Errorf("targets = %v, want the renderer addressed directly", plan.Targets)
	}
}

// A renderer with no RenderingControl service has no volume HomeHub can set.
// That is a real configuration rather than an error, and the capability has to
// go with it — advertising a slider that fails on every drag is worse than not
// showing one.
func TestARendererWithoutRenderingControlHasNoVolume(t *testing.T) {
	if caps := NewUPnPEndpoint(renderer("Bare", false)).Descriptor().Caps; caps.Has(media.CapVolume) {
		t.Error("no RenderingControl service means no volume to offer")
	}
	if caps := NewUPnPEndpoint(renderer("Full", true)).Descriptor().Caps; !caps.Has(media.CapVolume) {
		t.Error("a renderer with RenderingControl should offer volume")
	}
}

// Renderers have no multi-room bus of their own, so none of them may ever be
// made to lead another. Two playing together is HomeHub feeding both, which is
// the stream route, not a group.
func TestRenderersNeverGroup(t *testing.T) {
	d := NewUPnPEndpoint(renderer("RoPieee", true)).Descriptor()
	if d.GroupKey != "" {
		t.Errorf("group key = %q, want none", d.GroupKey)
	}
	if d.Caps.Has(media.CapGroup) {
		t.Error("UPnP has no grouping and must not claim it")
	}
	if d.Caps.Has(media.CapNativeService) {
		t.Error("a renderer holds no service account of its own")
	}
}

// Two renderers still play together — over the stream route, buffered rather
// than clocked, which is what the sync report has to say.
func TestTwoRenderersPlayTogetherBuffered(t *testing.T) {
	eps := []media.Endpoint{
		NewUPnPEndpoint(renderer("Study", true)),
		NewUPnPEndpoint(renderer("Hall", true)),
	}
	// Distinct ids, or the zone is one speaker listed twice.
	eps[1].(*UPnPEndpoint).rn.ID = "rp2"

	p := NewQobuzProvider(fakeAccount{
		status: qobuz.Status{Configured: true, Connected: true, MaxFormat: qobuz.FormatCD},
		max:    qobuz.FormatCD,
	}, okDecoder{})

	plan, err := media.Resolve(p, eps)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Route != media.RouteStream {
		t.Errorf("route = %s, want stream", plan.Route)
	}
	if plan.Sync != media.SyncBuffered {
		t.Errorf("sync = %s, want buffered — there is no clock on this route", plan.Sync)
	}
}

// okDecoder is a decoder that is simply available, for the routing tests that
// never open a stream.
type okDecoder struct{}

func (okDecoder) Available() media.Availability {
	return media.Availability{OK: true, Configured: true}
}
func (okDecoder) Open(context.Context, string) (*media.Stream, error) { return nil, nil }
