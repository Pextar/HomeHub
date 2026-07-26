package mediabridge

import (
	"context"
	"fmt"
	"time"

	"homehub/internal/kef"
	"homehub/internal/media"
	"homehub/internal/store"
)

// KEFCaps is what every KEF wireless speaker can do.
//
// The two absences are the whole reason the route engine exists.
// CapNativeService: the speaker's local API has transport control but nothing
// that takes content — no queue, no URI, no favorites — so it cannot be told
// to stream a service on its own. CapGroup: KEF has no multi-room grouping at
// all; a pair of LS50s is one stereo speaker, not a group.
//
// CapPlayURI is present but reached over UPnP rather than the local HTTP API,
// which is a separate protocol on the same box (see PlayURI).
const KEFCaps = media.CapTransport | media.CapVolume | media.CapPlayURI |
	media.CapConnect | media.CapWake

// KEFStateFunc supplies a speaker's live state, wired by the API layer to the
// polling monitor so the media layer reuses its cache rather than adding a
// second poll to every speaker.
type KEFStateFunc func(ctx context.Context, sp store.KEFSpeaker) (*kef.State, error)

// KEFEndpoint adapts one registered KEF speaker.
type KEFEndpoint struct {
	sp    store.KEFSpeaker
	state KEFStateFunc
}

// NewKEFEndpoint wraps a stored speaker.
func NewKEFEndpoint(sp store.KEFSpeaker, state KEFStateFunc) *KEFEndpoint {
	if state == nil {
		state = func(ctx context.Context, sp store.KEFSpeaker) (*kef.State, error) {
			return kef.GetState(ctx, sp.IP)
		}
	}
	return &KEFEndpoint{sp: sp, state: state}
}

func (e *KEFEndpoint) Descriptor() media.Descriptor {
	return media.Descriptor{
		ID:     e.sp.ID,
		Name:   e.sp.Name,
		Room:   e.sp.Room,
		Vendor: media.VendorKEF,
		Model:  e.sp.Model,
		Caps:   KEFCaps,
		// No GroupKey: KEF has no native grouping, so there is no domain to
		// belong to. Leaving it empty also makes the route engine's
		// same-key check fail closed for KEF pairs, which is correct.
		GroupKey: "",
	}
}

// Speaker exposes the underlying record.
func (e *KEFEndpoint) Speaker() store.KEFSpeaker { return e.sp }

func (e *KEFEndpoint) State(ctx context.Context) (*media.NowPlaying, error) {
	st, err := e.state(ctx, e.sp)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("kef: no state for %s", e.sp.Name)
	}
	np := &media.NowPlaying{
		State:    kefPlayState(st),
		Volume:   st.Volume,
		Muted:    st.Muted,
		Position: time.Duration(st.PositionMS) * time.Millisecond,
		Duration: time.Duration(st.DurationMS) * time.Millisecond,
		At:       time.Now(),
	}
	if st.Track != nil {
		np.Track = &media.Track{
			Title:  st.Track.Title,
			Artist: st.Track.Artist,
			Album:  st.Track.Album,
			ArtURI: st.Track.ArtURI,
		}
	}
	np.SyncWire()
	return np, nil
}

func (e *KEFEndpoint) Play(ctx context.Context) error     { return kef.Play(ctx, e.sp.IP) }
func (e *KEFEndpoint) Pause(ctx context.Context) error    { return kef.Pause(ctx, e.sp.IP) }
func (e *KEFEndpoint) Next(ctx context.Context) error     { return kef.Next(ctx, e.sp.IP) }
func (e *KEFEndpoint) Previous(ctx context.Context) error { return kef.Previous(ctx, e.sp.IP) }

func (e *KEFEndpoint) SetVolume(ctx context.Context, level int) error {
	return kef.SetVolume(ctx, e.sp.IP, level)
}

func (e *KEFEndpoint) SetMute(ctx context.Context, muted bool) error {
	return kef.SetMute(ctx, e.sp.IP, muted)
}

// Wake implements media.Waker. Selecting the Wi-Fi source both brings the
// speaker out of standby and takes it off whatever physical input it was on —
// a speaker sitting on optical is awake but is not a network device, and
// would fail every route with a "not found" that names nothing useful.
//
// Idempotent: selecting the source a speaker is already on is a no-op, which
// is why routes call this unconditionally instead of reading state first.
func (e *KEFEndpoint) Wake(ctx context.Context) error {
	return kef.SetSource(ctx, e.sp.IP, kef.SourceWiFi)
}

// ConnectHint implements media.ConnectTarget. The pinned id wins when set;
// otherwise the names are tried most-specific first — the pinned name before
// the speaker's own, so a pin whose device id rotated (which Spotify does on
// re-registration) still resolves.
func (e *KEFEndpoint) ConnectHint() (string, []string) {
	return e.sp.SpotifyDeviceID, []string{e.sp.SpotifyDeviceName, e.sp.Name}
}

// PlayURI implements media.URIPlayer, the stream route's entry point.
//
// This goes over UPnP AVTransport rather than the local HTTP API the rest of
// this adapter uses. The two coexist on the speaker: KEF's own API has no way
// to accept content, but the DLNA renderer it also runs does. The speaker has
// to be woken first, because the renderer is only reachable once it is on the
// network.
func (e *KEFEndpoint) PlayURI(ctx context.Context, uri string, meta media.Metadata) error {
	if err := e.Wake(ctx); err != nil {
		return fmt.Errorf("kef: waking %s: %w", e.sp.Name, err)
	}
	return kef.PlayStreamURI(ctx, e.sp.IP, uri, streamDIDL(uri, meta))
}

// kefPlayState normalises KEF's vocabulary. A speaker in standby reports
// stopped regardless of what its player field says: the player state is stale
// the moment the speaker sleeps, and reporting "paused" for a speaker that is
// off would show the UI a transport the user cannot resume.
func kefPlayState(st *kef.State) media.PlayState {
	if !st.PoweredOn {
		return media.StateStopped
	}
	switch st.Status {
	case kef.StatusPlaying:
		return media.StatePlaying
	case kef.StatusPaused:
		return media.StatePaused
	default:
		return media.StateStopped
	}
}
