package mediabridge

import (
	"context"
	"errors"
	"fmt"

	"homehub/internal/media"
	"homehub/internal/qobuz"
)

// QobuzProvider adapts internal/qobuz to media.Provider.
//
// It is the mirror image of SpotifyProvider, and the contrast is the point.
// Spotify is served four ways and is lossy on the only two HomeHub controls;
// Qobuz is served exactly one way and is lossless on it. That single route is
// not a limitation to apologise for — it is the route where HomeHub holds the
// audio, which is precisely why it can promise anything about it at all.
//
// No speaker in this house has a native Qobuz account link that HomeHub knows
// how to drive, and Qobuz has no Connect equivalent, so those routes are not
// advertised. Declaring routes a provider cannot actually serve would have the
// router pick one and fail at the tap, which is the failure mode the whole
// capability model exists to prevent.
type QobuzProvider struct {
	client  QobuzAccount
	decoder Decoder
}

// QobuzAccount is the slice of *qobuz.Client this provider needs. Narrow, and
// an interface rather than the concrete client, because quality reporting is
// built entirely on MaxFormat and a test that could not vary it would be
// asserting the default rather than the logic.
//
// Callers holding a possibly-nil *qobuz.Client must nil-check before passing
// it: a nil pointer in a non-nil interface is not nil, and would turn "Qobuz
// isn't set up" into a panic on the first search.
type QobuzAccount interface {
	Status() qobuz.Status
	MaxFormat() qobuz.FormatID
	Search(ctx context.Context, query string, limit int) (*qobuz.Results, error)
	Favorites(ctx context.Context, limit int) ([]qobuz.Item, error)
}

// NewQobuzProvider wraps a Qobuz account and its decoder. Both may be nil: a
// nil account reports the provider unconfigured rather than panicking, which is
// what the API layer relies on for an unwired integration.
func NewQobuzProvider(c QobuzAccount, d Decoder) *QobuzProvider {
	return &QobuzProvider{client: c, decoder: d}
}

func (p *QobuzProvider) ID() string   { return "qobuz" }
func (p *QobuzProvider) Name() string { return "Qobuz" }

func (p *QobuzProvider) Available() media.Availability {
	if p.client == nil {
		return media.Availability{Reason: "Qobuz isn't set up on this server"}
	}
	st := p.client.Status()
	switch {
	case !st.Configured:
		// Deliberately specific. Qobuz issues these to the application
		// rather than to the listener, HomeHub ships none, and a household
		// that doesn't know that will otherwise hunt for a password field.
		return media.Availability{
			Reason: "Add your Qobuz app ID and secret under Settings — Qobuz issues them on request to api@qobuz.com",
		}
	case !st.Connected:
		return media.Availability{
			Configured: true,
			Reason:     "Sign in to your Qobuz account to search and play",
		}
	}
	return media.Availability{OK: true, Configured: true}
}

// Routes is the single route Qobuz can be served over. See the type comment.
func (p *QobuzProvider) Routes() media.RouteSet {
	return media.RouteSet{media.RouteAirPlay, media.RouteStream}
}

// SourceQuality implements media.QualityReporter.
//
// This is the first source in this codebase that can answer "yes" — and the
// answer still has to be qualified honestly. What arrives depends on the
// subscription: a Studio plan streams hi-res, an older one caps at CD, and both
// are lossless. So the codec is FLAC and Lossless is true on both, while the
// rate and depth are the entitlement's ceiling and are marked approximate,
// because the track decides within it. A 16-bit/44.1 album on a hi-res plan
// arrives at 16/44.1, and claiming 24/192 for it would be inventing.
func (p *QobuzProvider) SourceQuality(media.Route) media.Quality {
	q := media.Quality{Codec: media.CodecFLAC, Channels: 2, Lossless: true, Approximate: true}
	if p.client == nil {
		q.SampleRate, q.BitDepth = 44100, 16
		return q
	}
	switch p.client.MaxFormat() {
	case qobuz.FormatHiRes192:
		q.SampleRate, q.BitDepth = 192000, 24
	case qobuz.FormatHiRes96:
		q.SampleRate, q.BitDepth = 96000, 24
	case qobuz.FormatMP3320:
		// A subscription with no lossless entitlement at all. The decoder
		// refuses to play it rather than serving MP3 down a path advertised
		// as lossless, and the report says so before the tap.
		return media.Quality{Codec: media.CodecMP3, SampleRate: 44100, Channels: 2, BitrateKbps: 320}
	default:
		q.SampleRate, q.BitDepth = 44100, 16
	}
	return q
}

// SourceDetail implements media.QualityExplainer.
func (p *QobuzProvider) SourceDetail(media.Route) string {
	if p.client == nil {
		return ""
	}
	st := p.client.Status()
	if st.MaxFormat == qobuz.FormatMP3320 {
		return "This Qobuz subscription doesn't include lossless streaming, so HomeHub won't play it — the point of this route is the FLAC."
	}
	base := "HomeHub fetches the FLAC and decodes it here; the samples are the master, not a re-encode."
	if st.Plan != "" {
		return fmt.Sprintf("%s Your %s plan tops out at %s, and each track arrives at its own rate within that.",
			base, st.Plan, st.MaxFormat.Label())
	}
	return base
}

// DecodedFormat implements media.PCMReporter: the ceiling of what this decoder
// can put on a wire, so the router can tell before opening anything whether a
// route could carry it.
//
// It reports the entitlement's maximum rather than any particular track's
// format, and that is the conservative direction: a hi-res account will
// sometimes play a CD-quality album that AirPlay could have carried perfectly
// well, and this sends it over the stream route instead. Losing AirPlay's
// clock on a track that did not need to lose it is a smaller harm than
// planning an AirPlay cast that has to be refused once the file turns out to
// be 24-bit.
func (p *QobuzProvider) DecodedFormat() media.PCMFormat {
	q := p.SourceQuality(media.RouteStream)
	return media.PCMFormat{
		SampleRate: q.SampleRate, BitDepth: q.BitDepth, Channels: 2, LittleEndian: true,
	}
}

func (p *QobuzProvider) Search(ctx context.Context, query string, limit int) (*media.Results, error) {
	if av := p.Available(); !av.OK {
		return nil, errors.New(av.Reason)
	}
	res, err := p.client.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	return &media.Results{
		Tracks:    p.items(res.Tracks),
		Albums:    p.items(res.Albums),
		Playlists: p.items(res.Playlists),
		Artists:   p.items(res.Artists),
	}, nil
}

func (p *QobuzProvider) Browse(ctx context.Context, limit int) ([]media.Item, error) {
	if av := p.Available(); !av.OK {
		return nil, errors.New(av.Reason)
	}
	items, err := p.client.Favorites(ctx, limit)
	if err != nil {
		return nil, err
	}
	return p.items(items), nil
}

// items maps the Qobuz client's shape onto the neutral one.
func (p *QobuzProvider) items(in []qobuz.Item) []media.Item {
	out := make([]media.Item, 0, len(in))
	for _, it := range in {
		out = append(out, media.Item{
			Provider: p.ID(),
			Kind:     media.ItemKind(it.Kind),
			URI:      it.URI,
			Title:    it.Name,
			Subtitle: it.Sub,
			ArtURI:   it.ArtURL,
		})
	}
	return out
}

// StreamAvailable implements media.StreamProvider.
func (p *QobuzProvider) StreamAvailable() media.Availability {
	if av := p.Available(); !av.OK {
		return av
	}
	if p.client.MaxFormat() == qobuz.FormatMP3320 {
		return media.Availability{
			Configured: true,
			Reason:     "This Qobuz subscription doesn't include lossless streaming",
		}
	}
	if p.decoder == nil {
		return media.Availability{Configured: true, Reason: "Qobuz decoding isn't wired up on this server"}
	}
	return p.decoder.Available()
}

// OpenStream implements media.StreamProvider.
func (p *QobuzProvider) OpenStream(ctx context.Context, item media.Item) (*media.Stream, error) {
	if av := p.StreamAvailable(); !av.OK {
		return nil, errors.New(av.Reason)
	}
	return p.decoder.Open(ctx, item.URI)
}
