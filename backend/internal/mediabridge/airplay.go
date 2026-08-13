package mediabridge

import (
	"context"
	"fmt"
	"time"

	"homehub/internal/media"
	"homehub/internal/store"
)

// AirPlayCaps is what a registered AirPlay receiver can do, and the absences
// are as deliberate as the presences.
//
// CapAirPlay is the one that matters: it is the only route that reaches these
// at all. CapVolume is real — volume is the receiver's own, set over the RTSP
// session. CapTransport is the awkward one and is claimed with a caveat spelt
// out on Next/Previous below.
//
// Everything else is absent because a receiver genuinely has none of it. There
// is no queue to inspect, no position to seek within, no account link to
// stream a service from, and no URL it could be told to fetch: an AirPlay
// receiver holds nothing. It plays what it is sent, for as long as it is sent.
const AirPlayCaps = media.CapTransport | media.CapVolume | media.CapAirPlay

// AirPlayLive finds the live cast driving a receiver, when one exists. Wired
// by the API layer to the running caster.
type AirPlayLive func(id string) (media.AirPlayControl, bool)

// AirPlayEndpoint adapts one registered AirPlay receiver.
type AirPlayEndpoint struct {
	sp   store.AirPlaySpeaker
	live AirPlayLive
}

// NewAirPlayEndpoint wraps a stored receiver. A nil live function is legal and
// means "nothing is casting", which is the correct answer on a server that has
// never sent to one.
func NewAirPlayEndpoint(sp store.AirPlaySpeaker, live AirPlayLive) *AirPlayEndpoint {
	if live == nil {
		live = func(string) (media.AirPlayControl, bool) { return nil, false }
	}
	return &AirPlayEndpoint{sp: sp, live: live}
}

func (e *AirPlayEndpoint) Descriptor() media.Descriptor {
	return media.Descriptor{
		ID:     e.sp.ID,
		Name:   e.sp.Name,
		Room:   e.sp.Room,
		Vendor: media.VendorAirPlay,
		Model:  e.sp.Model,
		Caps:   AirPlayCaps,
		// No GroupKey. AirPlay receivers do play together, but not by
		// grouping with each other — the sender drives them all, which is
		// the AirPlay route, not the group route. Leaving this empty is
		// what stops the route engine trying to make one of them lead.
		GroupKey: "",
	}
}

// Speaker exposes the underlying record.
func (e *AirPlayEndpoint) Speaker() store.AirPlaySpeaker { return e.sp }

// AirPlayDest implements media.AirPlayTarget.
func (e *AirPlayEndpoint) AirPlayDest() media.AirPlayDest {
	return media.AirPlayDest{
		ID:              e.sp.ID,
		Name:            e.sp.Name,
		Host:            e.sp.IP,
		Port:            e.sp.Port,
		PCM:             e.sp.PCM,
		ALAC:            e.sp.ALAC,
		NeedsEncryption: e.sp.NeedsEncryption,
		Metadata:        e.sp.Metadata,
		Volume:          e.sp.Volume,
	}
}

// State reports what HomeHub is sending this receiver.
//
// There is no device to poll. A receiver has no state of its own to report —
// it does not know what it is playing, only that samples keep arriving — so
// the honest answer is what the sender is doing, and "stopped" when the sender
// is doing nothing. Reading it from anywhere else would mean inventing a
// picture of a device that holds no picture.
func (e *AirPlayEndpoint) State(ctx context.Context) (*media.NowPlaying, error) {
	np := &media.NowPlaying{State: media.StateStopped, Volume: e.sp.Volume, At: time.Now()}
	if ctrl, ok := e.live(e.sp.ID); ok {
		np.State = media.StatePaused
		if ctrl.Playing() {
			np.State = media.StatePlaying
		}
	}
	np.SyncWire()
	return np, nil
}

func (e *AirPlayEndpoint) Play(ctx context.Context) error {
	ctrl, ok := e.live(e.sp.ID)
	if !ok {
		return e.idle("start")
	}
	return ctrl.Resume(ctx)
}

func (e *AirPlayEndpoint) Pause(ctx context.Context) error {
	ctrl, ok := e.live(e.sp.ID)
	if !ok {
		return nil // nothing is being sent; there is nothing to stop
	}
	return ctrl.Pause(ctx)
}

// Next and Previous are the caveat on CapTransport.
//
// A receiver has no queue and no idea what a track is: skipping means asking
// whatever is feeding it to move on, and that is the provider's business, not
// this speaker's. The alternative to this error was dropping CapTransport
// altogether, which would also have taken away play and pause — both of which
// genuinely work. So the capability is claimed, and the two verbs it overstates
// say exactly what they cannot do.
func (e *AirPlayEndpoint) Next(ctx context.Context) error     { return e.noSkip() }
func (e *AirPlayEndpoint) Previous(ctx context.Context) error { return e.noSkip() }

func (e *AirPlayEndpoint) noSkip() error {
	return fmt.Errorf(
		"%s is being sent audio by HomeHub, so skipping tracks is the music service's job, not the speaker's",
		e.sp.Name)
}

func (e *AirPlayEndpoint) idle(verb string) error {
	return fmt.Errorf("nothing is being sent to %s, so there's nothing to %s", e.sp.Name, verb)
}

// SetVolume sets the receiver's own volume.
//
// With nothing being sent there is no session to carry it, and this reports
// success rather than an error — the one place this adapter is deliberately
// quiet about something it did not do. The reason is the caller: this is
// reached by a *zone* volume change, and failing it would mean a slider that
// refuses to move the Sonos speakers in the room because a receiver in the
// same room happens to be idle. The level is not lost either: the API layer
// stores it against the receiver, which is what the next cast opens with.
func (e *AirPlayEndpoint) SetVolume(ctx context.Context, level int) error {
	ctrl, ok := e.live(e.sp.ID)
	if !ok {
		return nil
	}
	return ctrl.SetVolume(ctx, level)
}

// SetMute is volume 0 and back.
//
// AirPlay has a mute — it is what the bottom of the volume scale means — but
// no separate mute state to restore from, so unmuting has to pick a level.
// Rather than invent one, this restores the level the household last set,
// which is the only remembered number that is actually theirs.
func (e *AirPlayEndpoint) SetMute(ctx context.Context, muted bool) error {
	level := e.sp.Volume
	if muted {
		level = 0
	}
	if level == 0 && !muted {
		level = 50 // never stored a level, and unmuting to silence is useless
	}
	return e.SetVolume(ctx, level)
}
