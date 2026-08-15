package mediabridge

import (
	"context"
	"time"

	"homehub/internal/media"
	"homehub/internal/store"
	"homehub/internal/upnp"
)

// UPnPCaps is what a generic DLNA renderer can do, and the absences matter as
// much as the presences.
//
// CapPlayURI is the whole point of this bridge. A renderer *fetches*, which is
// the opposite of what an AirPlay receiver does, and it is why this is the only
// endpoint in the house that can be handed hi-res audio: the stream route
// serves whatever format it holds under a header describing it, and a fetching
// speaker reads that header rather than being bound to a push protocol's fixed
// rate. The same RoPieee is 44.1 kHz/16-bit over AirPlay and 24-bit/192 kHz
// here, and nothing about the box changed.
//
// No CapNativeService: a renderer has no account link of its own, so there is
// no service it could stream by itself. No CapGroup: UPnP has no multi-room
// bus, and two renderers play together only because HomeHub is feeding both.
// No CapConnect, no CapWake.
const UPnPCaps = media.CapTransport | media.CapVolume | media.CapPlayURI

// UPnPEndpoint adapts one registered renderer.
type UPnPEndpoint struct {
	rn store.UPnPRenderer
}

// NewUPnPEndpoint wraps a stored renderer.
func NewUPnPEndpoint(rn store.UPnPRenderer) *UPnPEndpoint {
	return &UPnPEndpoint{rn: rn}
}

// device is the control-URL bundle the upnp package works from.
func (e *UPnPEndpoint) device() *upnp.Device {
	return &upnp.Device{
		UDN:                 e.rn.UDN,
		Name:                e.rn.Name,
		Model:               e.rn.Model,
		AVTransportURL:      e.rn.AVTransportURL,
		RenderingControlURL: e.rn.RenderingControlURL,
		ConnectionMgrURL:    e.rn.ConnectionMgrURL,
	}
}

// Descriptor implements media.Endpoint.
func (e *UPnPEndpoint) Descriptor() media.Descriptor {
	return media.Descriptor{
		ID:     e.rn.ID,
		Name:   e.rn.Name,
		Room:   e.rn.Room,
		Vendor: media.VendorUPnP,
		Model:  e.rn.Model,
		Caps:   e.caps(),
		// No GroupKey: UPnP has no grouping of its own, so nothing here may
		// ever be made to lead another renderer.
		GroupKey: "",
	}
}

// caps drops volume for a renderer that has no RenderingControl service.
// Advertising a control that isn't there would have the UI offer a slider that
// fails on every drag.
func (e *UPnPEndpoint) caps() media.Capability {
	if e.rn.RenderingControlURL == "" {
		return UPnPCaps &^ media.CapVolume
	}
	return UPnPCaps
}

// Renderer exposes the underlying record.
func (e *UPnPEndpoint) Renderer() store.UPnPRenderer { return e.rn }

// PlayURI implements media.URIPlayer: hand the renderer a URL and let it
// fetch. The metadata rides along because renderers with a display show it,
// and a few refuse a URI that arrives with none.
func (e *UPnPEndpoint) PlayURI(ctx context.Context, uri string, meta media.Metadata) error {
	didl := upnp.DIDL(meta.Title, meta.Artist, meta.ContentType)
	if err := upnp.SetURI(ctx, e.device(), uri, didl); err != nil {
		return err
	}
	return upnp.Play(ctx, e.device())
}

// State implements media.Endpoint by asking the renderer what it is doing.
//
// Unlike an AirPlay receiver this device genuinely knows: it fetched the
// stream itself and holds its own transport state, so the honest answer comes
// from the box rather than from what HomeHub last sent it.
func (e *UPnPEndpoint) State(ctx context.Context) (*media.NowPlaying, error) {
	np := &media.NowPlaying{State: media.StateStopped, At: time.Now()}
	st, err := upnp.State(ctx, e.device())
	if err != nil {
		return nil, err
	}
	switch st {
	case upnp.StatePlaying:
		np.State = media.StatePlaying
	case upnp.StatePaused:
		np.State = media.StatePaused
	case upnp.StateTransitioning:
		np.State = media.StateTransitioning
	}
	// Volume is a second round trip and only some renderers have it, so a
	// failure here leaves the level at zero rather than failing the read —
	// knowing what is playing is worth more than knowing how loudly.
	if e.rn.RenderingControlURL != "" {
		if v, err := upnp.Volume(ctx, e.device()); err == nil {
			np.Volume = v
		}
		if m, err := upnp.Muted(ctx, e.device()); err == nil {
			np.Muted = m
		}
	}
	np.SyncWire()
	return np, nil
}

// Play implements media.Transport.
func (e *UPnPEndpoint) Play(ctx context.Context) error { return upnp.Play(ctx, e.device()) }

// Pause implements media.Transport.
func (e *UPnPEndpoint) Pause(ctx context.Context) error { return upnp.Pause(ctx, e.device()) }

// Stop implements media.Transport.
func (e *UPnPEndpoint) Stop(ctx context.Context) error { return upnp.Stop(ctx, e.device()) }

// Next and Previous have no meaning here. A renderer is playing one URL that
// HomeHub chose; the queue lives on this side, so skipping is not something to
// ask the device for. Reported as unsupported rather than silently ignored.
func (e *UPnPEndpoint) Next(ctx context.Context) error {
	return media.ErrUnsupported
}
func (e *UPnPEndpoint) Previous(ctx context.Context) error {
	return media.ErrUnsupported
}

// SetVolume implements media.VolumeControl.
func (e *UPnPEndpoint) SetVolume(ctx context.Context, level int) error {
	return upnp.SetVolume(ctx, e.device(), level)
}

// SetMute implements media.VolumeControl.
func (e *UPnPEndpoint) SetMute(ctx context.Context, muted bool) error {
	return upnp.SetMute(ctx, e.device(), muted)
}
