package mediabridge

import (
	"context"
	"errors"
	"fmt"

	"homehub/internal/media"
	"homehub/internal/sonos"
	"homehub/internal/spotify"
)

// SpotifyProvider adapts internal/spotify to media.Provider.
//
// Spotify is the awkward case the protocol was designed around, and it is
// worth being precise about why. It can be served three different ways and
// no two of them are interchangeable:
//
//   - Sonos streams it from the household's own account link, over SMAPI.
//     HomeHub's command never leaves the LAN.
//   - A KEF is served by Spotify's cloud through Connect, which reaches
//     exactly one device at a time.
//   - Anything else — a KEF and a Sonos together, two KEFs — needs HomeHub
//     to hold the single Connect session itself and fan the audio out.
//
// The provider therefore advertises all four routes and lets the route engine
// choose, rather than encoding any of this at the call site.
type SpotifyProvider struct {
	client *spotify.Client
	// decoder is what turns a Spotify URI into audio HomeHub can re-serve.
	// Optional: without it the provider is fully functional for search and
	// for both native routes, and only the stream route reports unavailable.
	decoder Decoder
	// bitrate is what the decoder is currently configured to ask Spotify
	// for, in kbps. Carried on the provider rather than read from the
	// decoder so that quality can be reported for a household that has no
	// librespot installed — the answer to "what would this sound like" does
	// not depend on whether the binary is there.
	bitrate int
}

// Decoder produces playable audio for a provider URI. Implemented by
// internal/stream over librespot; kept as an interface here so the provider
// does not depend on a running subprocess, and so tests can substitute a
// generator.
type Decoder interface {
	// Open begins decoding and returns the audio plus its content type.
	Open(ctx context.Context, uri string) (*media.Stream, error)
	// Available reports whether decoding could work right now — binary
	// present, account eligible — separately from whether search works.
	Available() media.Availability
}

// NewSpotifyProvider wraps a Spotify client at the household's chosen stream
// quality. Both the client and the decoder may be nil: a nil client makes the
// provider report itself unconfigured rather than panicking, which is what the
// API layer already relies on for an unwired integration.
func NewSpotifyProvider(c *spotify.Client, d Decoder, q media.StreamQuality) *SpotifyProvider {
	return &SpotifyProvider{client: c, decoder: d, bitrate: q.Normalize().Bitrate()}
}

func (p *SpotifyProvider) ID() string   { return "spotify" }
func (p *SpotifyProvider) Name() string { return "Spotify" }

// SonosServiceName is what Spotify is called in a Sonos household's service
// list, which is the name GetServiceAccount matches on.
const SonosServiceName = "Spotify"

func (p *SpotifyProvider) Available() media.Availability {
	if p.client == nil {
		return media.Availability{Reason: "Spotify isn't set up on this server"}
	}
	st := p.client.Status()
	switch {
	case !st.Configured:
		return media.Availability{
			Reason: "Add your Spotify client ID under Settings to search Spotify",
		}
	case !st.Connected:
		return media.Availability{
			Configured: true,
			Reason:     "Connect your Spotify account to search and play",
		}
	}
	return media.Availability{OK: true, Configured: true}
}

// Routes is every route Spotify can be served over. Which one is actually
// used depends on the speakers, and is the route engine's decision.
func (p *SpotifyProvider) Routes() media.RouteSet {
	return media.RouteSet{
		media.RouteNative, media.RouteGroup, media.RouteConnect,
		media.RouteAirPlay, media.RouteStream,
	}
}

// SourceQuality implements media.QualityReporter: what Spotify hands over, per
// route.
//
// Two different answers, because they are two different clients — and since
// September 2025 they are two different *formats*, which is the part that
// matters and the part this file used to get wrong.
//
// Spotify Premium now streams up to 24-bit/44.1 kHz FLAC. A speaker that holds
// the household's own account link — a Sonos over SMAPI, anything targeted by
// Connect — can fetch that stream directly, so on those routes the ceiling is
// lossless. Whether a given speaker reached it depends on the plan, the market
// and that speaker's own settings, none of which HomeHub can read, so the
// answer is marked approximate and shown as "up to" rather than printed as a
// measurement it isn't.
//
// The routes HomeHub decodes for itself do not get that. The only licensed
// Spotify decoder HomeHub can run is librespot, which fetches the Ogg Vorbis
// stream; Spotify's FLAC tier is not available to it (librespot-org/librespot
// issue 1583, open and unimplemented). So on those routes the codec is Vorbis
// at exactly the bitrate this household configured — and the ceiling is
// HomeHub's, not Spotify's. media.DescribeQuality reads that difference off
// these two answers and attributes the limit accordingly; getting it wrong here
// is what would have the UI blame Spotify's catalogue for HomeHub's decoder.
func (p *SpotifyProvider) SourceQuality(r media.Route) media.Quality {
	if media.RouteStream == r || media.RouteAirPlay == r {
		return media.Quality{
			Codec: media.CodecVorbis, SampleRate: 44100, Channels: 2,
			BitrateKbps: p.bitrate,
		}
	}
	return media.Quality{
		Codec: media.CodecFLAC, SampleRate: 44100, BitDepth: 24, Channels: 2,
		Lossless: true, Approximate: true,
	}
}

// DecodedFormat implements media.PCMReporter: what HomeHub's decode of Spotify
// actually produces. librespot's pipe backend writes raw S16LE at CD rate, and
// Vorbis is a 44.1 kHz format, so this is CD quality exactly — not a ceiling
// and not a preference. It is stated rather than assumed so the router can see
// that every route carries it intact, and so the day a decoder here produces
// something larger, the router notices instead of the receiver doing so.
func (p *SpotifyProvider) DecodedFormat() media.PCMFormat { return media.CDQuality }

// SourceDetail implements media.QualityExplainer: the caveat that belongs with
// each of those two answers, and that only this file knows.
func (p *SpotifyProvider) SourceDetail(r media.Route) string {
	if media.RouteStream == r || media.RouteAirPlay == r {
		return "librespot, the decoder HomeHub runs for Spotify, can only fetch the " +
			"Ogg Vorbis stream — Spotify's lossless tier isn't available to it. " +
			"See docs/MEDIA-PROTOCOL.md."
	}
	return "Lossless needs Premium with lossless switched on for that speaker. " +
		"HomeHub can't read either, so this is the ceiling rather than a measurement."
}

func (p *SpotifyProvider) Search(ctx context.Context, query string, limit int) (*media.Results, error) {
	if av := p.Available(); !av.OK {
		return nil, errors.New(av.Reason)
	}
	res, err := p.client.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := &media.Results{
		Tracks:    p.items(res.Tracks, media.KindTrack),
		Albums:    p.items(res.Albums, media.KindAlbum),
		Playlists: p.items(res.Playlists, media.KindPlaylist),
		Artists:   p.items(res.Artists, media.KindArtist),
	}
	return out, nil
}

func (p *SpotifyProvider) Browse(ctx context.Context, limit int) ([]media.Item, error) {
	if av := p.Available(); !av.OK {
		return nil, errors.New(av.Reason)
	}
	items, err := p.client.MyPlaylists(ctx, limit)
	if err != nil {
		return nil, err
	}
	return p.items(items, media.KindPlaylist), nil
}

// items maps the Spotify client's shape onto the neutral one. The kind is
// passed in rather than read from the item because the client's own Kind
// field is only set on search results, and a playlist browsed from the
// account is still a playlist.
func (p *SpotifyProvider) items(in []spotify.Item, kind media.ItemKind) []media.Item {
	out := make([]media.Item, 0, len(in))
	for _, it := range in {
		k := kind
		if it.Kind != "" {
			k = media.ItemKind(it.Kind)
		}
		out = append(out, media.Item{
			Provider: p.ID(),
			Kind:     k,
			URI:      it.URI,
			Title:    it.Name,
			Subtitle: it.Sub,
			ArtURI:   it.ArtURL,
		})
	}
	return out
}

// NativeItem implements media.NativeProvider: build the vendor URI + DIDL a
// speaker needs to stream Spotify from its own account link.
func (p *SpotifyProvider) NativeItem(vendor media.Vendor, item media.Item, acct media.Account) (string, string, error) {
	if vendor != media.VendorSonos {
		// Only Sonos has a native Spotify integration. Saying so plainly
		// beats returning a URI that the speaker would silently ignore.
		return "", "", fmt.Errorf("spotify: %s speakers can't stream Spotify themselves", vendor)
	}
	return sonos.SpotifyItem(item.URI, item.Title, &sonos.ServiceAccount{
		Name:        SonosServiceName,
		SID:         acct.SID,
		SerialNum:   acct.Serial,
		ServiceType: acct.Type,
	})
}

// ServiceName implements media.NativeProvider.
func (p *SpotifyProvider) ServiceName(vendor media.Vendor) string { return SonosServiceName }

// ConnectDevices implements media.ConnectProvider.
func (p *SpotifyProvider) ConnectDevices(ctx context.Context) ([]media.ConnectDevice, error) {
	if p.client == nil {
		return nil, spotify.ErrNotConnected
	}
	devs, err := p.client.Devices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]media.ConnectDevice, len(devs))
	for i, d := range devs {
		out[i] = media.ConnectDevice{
			ID: d.ID, Name: d.Name, Type: d.Type,
			Active: d.Active, Restricted: d.Restricted,
		}
	}
	return out, nil
}

// PlayOn implements media.ConnectProvider.
func (p *SpotifyProvider) PlayOn(ctx context.Context, deviceID string, item media.Item) error {
	if p.client == nil {
		return spotify.ErrNotConnected
	}
	return p.client.PlayOn(ctx, deviceID, item.URI)
}

// StreamAvailable implements media.StreamProvider. It reports the union of
// two independent requirements — an account that can play, and a decoder that
// can run — because a user missing either needs a different sentence.
func (p *SpotifyProvider) StreamAvailable() media.Availability {
	if av := p.Available(); !av.OK {
		return av
	}
	if !p.client.Status().Playback {
		return media.Availability{
			Configured: true,
			Reason:     spotify.ErrPlaybackScope.Error(),
		}
	}
	if p.decoder == nil {
		return media.Availability{
			Configured: true,
			Reason: "playing to speakers of different makes at once needs librespot " +
				"on the HomeHub host — see docs/MEDIA-PROTOCOL.md",
		}
	}
	return p.decoder.Available()
}

// OpenStream implements media.StreamProvider.
func (p *SpotifyProvider) OpenStream(ctx context.Context, item media.Item) (*media.Stream, error) {
	if av := p.StreamAvailable(); !av.OK {
		return nil, errors.New(av.Reason)
	}
	return p.decoder.Open(ctx, item.URI)
}
