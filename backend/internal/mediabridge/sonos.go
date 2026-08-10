// Package mediabridge adapts the existing speaker and service bridges to the
// vendor-neutral interfaces in internal/media.
//
// It is the only package that imports both sides, which is what keeps
// internal/media free of hardware knowledge and the bridges free of any
// knowledge that the media layer exists. Adapters translate and nothing more:
// they must never emulate a capability the hardware lacks, because the route
// engine trusts the capability set to decide where music comes out.
package mediabridge

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"homehub/internal/media"
	"homehub/internal/sonos"
	"homehub/internal/store"
)

// SonosCaps is what every Sonos zone player can do. Declared once, as a
// constant, because a capability set that varies per instance would let two
// speakers of the same kind disagree and make route selection unpredictable.
//
// Notably absent: CapConnect. Sonos speakers do appear in Spotify's device
// list, but they are served by the household's own account link rather than
// by the Web API's player endpoints, and claiming CapConnect would let the
// engine pick a route that fights with the native one for the account's
// single playback session.
const SonosCaps = media.CapTransport | media.CapVolume | media.CapSeek |
	media.CapQueue | media.CapGroup | media.CapPlayURI | media.CapNativeService

// SonosStateFunc supplies a speaker's live state. The API layer wires this to
// the GENA monitor's cache so the media layer inherits event-driven state for
// free; a direct sonos.GetState is the fallback when no monitor is running.
type SonosStateFunc func(ctx context.Context, sp store.SonosSpeaker) (*sonos.State, error)

// SonosEndpoint adapts one registered Sonos speaker.
type SonosEndpoint struct {
	sp    store.SonosSpeaker
	state SonosStateFunc
	// household identifies which Sonos system this speaker belongs to.
	// Grouping only works within one, so it becomes the GroupKey.
	household string
}

// NewSonosEndpoint wraps a stored speaker. household may be empty, in which
// case every Sonos speaker is assumed to share one system — true for all but
// the unusual two-household LAN, and the route engine degrades safely either
// way because it compares keys rather than trusting a single value.
func NewSonosEndpoint(sp store.SonosSpeaker, household string, state SonosStateFunc) *SonosEndpoint {
	if state == nil {
		state = func(ctx context.Context, sp store.SonosSpeaker) (*sonos.State, error) {
			return sonos.GetState(ctx, sp.IP)
		}
	}
	if household == "" {
		household = "sonos"
	}
	return &SonosEndpoint{sp: sp, state: state, household: household}
}

func (e *SonosEndpoint) Descriptor() media.Descriptor {
	return media.Descriptor{
		ID:       e.sp.ID,
		Name:     e.sp.Name,
		Room:     e.sp.Room,
		Vendor:   media.VendorSonos,
		Model:    e.sp.Model,
		Caps:     SonosCaps,
		GroupKey: e.household,
	}
}

// Speaker exposes the underlying record, for callers that still need vendor
// specifics (the per-speaker detail views).
func (e *SonosEndpoint) Speaker() store.SonosSpeaker { return e.sp }

func (e *SonosEndpoint) State(ctx context.Context) (*media.NowPlaying, error) {
	st, err := e.state(ctx, e.sp)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("sonos: no state for %s", e.sp.Name)
	}
	np := &media.NowPlaying{
		State:    sonosPlayState(st.TransportState),
		Volume:   st.Volume,
		Muted:    st.Muted,
		Position: parseClock(st.Position),
		Duration: parseClock(st.Duration),
		At:       time.Now(),
	}
	if st.Track != nil {
		np.Track = &media.Track{
			Title:   st.Track.Title,
			Artist:  st.Track.Artist,
			Album:   st.Track.Album,
			ArtURI:  st.Track.ArtURI,
			Stream:  st.Track.Stream,
			Station: st.Track.Station,
		}
	}
	np.SyncWire()
	return np, nil
}

func (e *SonosEndpoint) Play(ctx context.Context) error     { return sonos.Play(ctx, e.sp.IP) }
func (e *SonosEndpoint) Pause(ctx context.Context) error    { return sonos.Pause(ctx, e.sp.IP) }
func (e *SonosEndpoint) Next(ctx context.Context) error     { return sonos.Next(ctx, e.sp.IP) }
func (e *SonosEndpoint) Previous(ctx context.Context) error { return sonos.Previous(ctx, e.sp.IP) }

func (e *SonosEndpoint) SetVolume(ctx context.Context, level int) error {
	return sonos.SetVolume(ctx, e.sp.IP, level)
}

func (e *SonosEndpoint) SetMute(ctx context.Context, muted bool) error {
	return sonos.SetMute(ctx, e.sp.IP, muted)
}

// Seek implements media.Seeker.
func (e *SonosEndpoint) Seek(ctx context.Context, pos time.Duration) error {
	return sonos.Seek(ctx, e.sp.IP, formatClock(pos))
}

// Queue implements media.Queuer.
func (e *SonosEndpoint) Queue(ctx context.Context) ([]media.QueueItem, error) {
	items, err := sonos.ListQueue(ctx, e.sp.IP)
	if err != nil {
		return nil, err
	}
	out := make([]media.QueueItem, len(items))
	for i, it := range items {
		out[i] = media.QueueItem{
			Position: it.Track,
			Title:    it.Title,
			Artist:   it.Artist,
			Album:    it.Album,
			ArtURI:   it.ArtURI,
		}
	}
	return out, nil
}

// ClearQueue implements media.Queuer.
func (e *SonosEndpoint) ClearQueue(ctx context.Context) error {
	return sonos.ClearQueue(ctx, e.sp.IP)
}

// Join implements media.Grouper. The coordinator must be another Sonos
// endpoint; the route engine guarantees this by only grouping endpoints that
// share a GroupKey, and the check here is the belt to that braces — joining a
// non-Sonos coordinator would otherwise fail deep inside a SOAP call with an
// error naming neither speaker.
func (e *SonosEndpoint) Join(ctx context.Context, coordinator media.Endpoint) error {
	other, ok := coordinator.(*SonosEndpoint)
	if !ok {
		return fmt.Errorf("sonos: %s can't group with %s — not a Sonos speaker",
			e.sp.Name, coordinator.Descriptor().Name)
	}
	if other.sp.UUID == "" {
		return fmt.Errorf("sonos: %s has no device id to group onto", other.sp.Name)
	}
	return sonos.Join(ctx, e.sp.IP, other.sp.UUID)
}

// Leave implements media.Grouper.
func (e *SonosEndpoint) Leave(ctx context.Context) error { return sonos.Leave(ctx, e.sp.IP) }

// Coordinator implements media.Grouper, reporting the UUID of whichever
// speaker currently leads this one's group.
func (e *SonosEndpoint) Coordinator(ctx context.Context) (string, error) {
	groups, err := sonos.GetTopology(ctx, e.sp.IP)
	if err != nil {
		return "", err
	}
	for _, g := range groups {
		for _, m := range g.Members {
			if m.UUID == e.sp.UUID {
				return g.CoordinatorUUID, nil
			}
		}
	}
	return "", nil
}

// PlayNative implements media.NativeServicePlayer: hand the speaker a service
// URI and let it stream from the household's own account link.
func (e *SonosEndpoint) PlayNative(ctx context.Context, uri, metadata string) error {
	return sonos.PlayServiceItem(ctx, e.sp.IP, e.sp.UUID, uri, metadata)
}

// ServiceAccount implements media.NativeServicePlayer.
func (e *SonosEndpoint) ServiceAccount(ctx context.Context, service string) (media.Account, error) {
	acct, err := sonos.GetServiceAccount(ctx, e.sp.IP, service)
	if err != nil {
		return media.Account{}, err
	}
	return media.Account{SID: acct.SID, Serial: acct.SerialNum, Type: acct.ServiceType}, nil
}

// PlayURI implements media.URIPlayer — the stream route's entry point.
//
// The URI is set directly rather than enqueued: a HomeHub stream is a live
// source with no end and nothing after it, so putting it in the queue would
// leave the speaker trying to advance to a next track that will never come.
func (e *SonosEndpoint) PlayURI(ctx context.Context, uri string, meta media.Metadata) error {
	didl := streamDIDL(uri, meta)
	if err := sonos.SetAVTransportURI(ctx, e.sp.IP, uri, didl); err != nil {
		return err
	}
	return sonos.Play(ctx, e.sp.IP)
}

// streamDIDL builds the metadata document that decides how the speaker
// presents a stream. The upnp:class is the audioBroadcast one, which is what
// tells Sonos to show a station rather than a track with a scrubber it can't
// honour.
func streamDIDL(uri string, meta media.Metadata) string {
	title := meta.Title
	if title == "" {
		title = "HomeHub"
	}
	var b strings.Builder
	b.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/"`)
	b.WriteString(` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"`)
	b.WriteString(` xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/"`)
	b.WriteString(` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`)
	b.WriteString(`<item id="homehub-stream" parentID="-1" restricted="true">`)
	b.WriteString(`<dc:title>` + xmlEscape(title) + `</dc:title>`)
	if meta.Artist != "" {
		b.WriteString(`<dc:creator>` + xmlEscape(meta.Artist) + `</dc:creator>`)
		b.WriteString(`<upnp:artist>` + xmlEscape(meta.Artist) + `</upnp:artist>`)
	}
	if meta.Album != "" {
		b.WriteString(`<upnp:album>` + xmlEscape(meta.Album) + `</upnp:album>`)
	}
	if meta.ArtURI != "" {
		b.WriteString(`<upnp:albumArtURI>` + xmlEscape(meta.ArtURI) + `</upnp:albumArtURI>`)
	}
	if meta.Live {
		b.WriteString(`<upnp:class>object.item.audioItem.audioBroadcast</upnp:class>`)
	} else {
		b.WriteString(`<upnp:class>object.item.audioItem.musicTrack</upnp:class>`)
	}
	b.WriteString(`<res protocolInfo="http-get:*:` + xmlEscape(contentTypeOf(meta)) +
		`:*">` + xmlEscape(uri) + `</res>`)
	b.WriteString(`</item></DIDL-Lite>`)
	return b.String()
}

// contentTypeOf is the MIME type advertised in the DIDL res element. Sonos
// uses it to pick a decoder before it has any bytes, so it has to match what
// the stream transport actually serves.
func contentTypeOf(meta media.Metadata) string {
	if meta.ContentType != "" {
		return meta.ContentType
	}
	return "audio/mpeg"
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

// sonosPlayState normalises Sonos' transport vocabulary. Unknown values map
// to stopped rather than to a guess: a state the UI shows as playing when it
// isn't is worse than one that understates.
func sonosPlayState(s string) media.PlayState {
	switch strings.ToUpper(s) {
	case "PLAYING":
		return media.StatePlaying
	case "PAUSED_PLAYBACK":
		return media.StatePaused
	case "TRANSITIONING":
		return media.StateTransitioning
	default:
		return media.StateStopped
	}
}

// parseClock reads Sonos' H:MM:SS. A malformed or "NOT_IMPLEMENTED" value
// (which speakers do return for live streams) yields zero, matching the
// "duration is zero when there is nothing to seek" contract.
func parseClock(s string) time.Duration {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 3 {
		return 0
	}
	var total time.Duration
	units := []time.Duration{time.Hour, time.Minute, time.Second}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0
		}
		total += time.Duration(n) * units[i]
	}
	return total
}

// formatClock renders H:MM:SS for Seek. Negative positions clamp to zero
// rather than producing a string the speaker would reject.
func formatClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d / time.Hour)
	m := int(d/time.Minute) % 60
	s := int(d/time.Second) % 60
	return fmt.Sprintf("%d:%02d:%02d", h, m, s)
}
