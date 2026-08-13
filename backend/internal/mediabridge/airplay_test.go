package mediabridge

import (
	"context"
	"strings"
	"testing"

	"homehub/internal/media"
	"homehub/internal/store"
)

// fakeControl stands in for a running cast.
type fakeControl struct {
	playing bool
	volume  int
	paused  int
	resumed int
}

func (c *fakeControl) SetVolume(_ context.Context, level int) error {
	c.volume = level
	return nil
}
func (c *fakeControl) Pause(context.Context) error {
	c.paused++
	c.playing = false
	return nil
}
func (c *fakeControl) Resume(context.Context) error {
	c.resumed++
	c.playing = true
	return nil
}
func (c *fakeControl) Playing() bool { return c.playing }

func receiver() store.AirPlaySpeaker {
	return store.AirPlaySpeaker{
		ID: "ap1", Name: "Study Pi", IP: "192.0.2.30", Port: 7000,
		PCM: true, ALAC: true, Metadata: true, Volume: 40,
	}
}

func withCast(sp store.AirPlaySpeaker, c *fakeControl) *AirPlayEndpoint {
	return NewAirPlayEndpoint(sp, func(id string) (media.AirPlayControl, bool) {
		if id != sp.ID {
			return nil, false
		}
		return c, true
	})
}

// The destination carries what the receiver advertised, because that is only
// visible during a scan — nothing recovers it from an address afterwards.
func TestAirPlayDestCarriesWhatTheReceiverAdvertised(t *testing.T) {
	d := NewAirPlayEndpoint(receiver(), nil).AirPlayDest()
	if d.Host != "192.0.2.30" || d.Port != 7000 {
		t.Errorf("dest = %+v", d)
	}
	if !d.PCM || !d.ALAC || !d.Metadata {
		t.Errorf("advertised formats lost: %+v", d)
	}
	// Per receiver, not per cast: levelling a house together would undo what
	// the household set room by room.
	if d.Volume != 40 {
		t.Errorf("volume = %d, want the stored level", d.Volume)
	}
}

// A receiver has no state to poll, so State reports what the *sender* is
// doing. Reading it from anywhere else would mean inventing a picture of a
// device that holds none.
func TestStateReflectsTheSenderNotTheDevice(t *testing.T) {
	idle := NewAirPlayEndpoint(receiver(), nil)
	st, err := idle.State(context.Background())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if st.State != media.StateStopped {
		t.Errorf("an idle receiver is stopped, got %q", st.State)
	}
	if st.Volume != 40 {
		t.Errorf("volume = %d, want the stored level", st.Volume)
	}

	c := &fakeControl{playing: true}
	live := withCast(receiver(), c)
	if st, _ = live.State(context.Background()); st.State != media.StatePlaying {
		t.Errorf("a cast in flight is playing, got %q", st.State)
	}
	c.playing = false
	if st, _ = live.State(context.Background()); st.State != media.StatePaused {
		t.Errorf("a paused cast is paused, got %q", st.State)
	}
}

// The documented half-truth in CapTransport: play and pause work, skipping
// does not, and the error says whose job it is instead of failing blankly.
func TestSkippingSaysWhoseJobItIs(t *testing.T) {
	e := withCast(receiver(), &fakeControl{playing: true})
	for _, err := range []error{e.Next(context.Background()), e.Previous(context.Background())} {
		if err == nil {
			t.Fatal("skipping a pushed stream cannot work")
		}
		if !strings.Contains(err.Error(), "Study Pi") ||
			!strings.Contains(err.Error(), "music service") {
			t.Errorf("error should name the receiver and the real owner: %v", err)
		}
	}
}

func TestPauseAndResumeDriveTheCast(t *testing.T) {
	c := &fakeControl{playing: true}
	e := withCast(receiver(), c)

	if err := e.Pause(context.Background()); err != nil || c.paused != 1 {
		t.Errorf("pause: %v, count %d", err, c.paused)
	}
	if err := e.Play(context.Background()); err != nil || c.resumed != 1 {
		t.Errorf("play: %v, count %d", err, c.resumed)
	}
}

// Pausing something that isn't playing is not an error — there is nothing to
// stop — but starting one is, because there is nothing to start either and
// silently doing nothing would look like a dead button.
func TestIdleReceiverTransport(t *testing.T) {
	e := NewAirPlayEndpoint(receiver(), nil)
	if err := e.Pause(context.Background()); err != nil {
		t.Errorf("pausing nothing should be a no-op, got %v", err)
	}
	err := e.Play(context.Background())
	if err == nil || !strings.Contains(err.Error(), "nothing is being sent") {
		t.Errorf("play on an idle receiver: %v", err)
	}
}

// The one place this adapter is deliberately quiet: a zone volume change must
// not fail because a receiver in the room happens to be idle, or the slider
// would refuse to move the speakers that *can* take it.
func TestVolumeOnAnIdleReceiverIsNotAnError(t *testing.T) {
	if err := NewAirPlayEndpoint(receiver(), nil).SetVolume(context.Background(), 60); err != nil {
		t.Errorf("want a quiet no-op, got %v", err)
	}

	c := &fakeControl{playing: true}
	if err := withCast(receiver(), c).SetVolume(context.Background(), 60); err != nil {
		t.Fatalf("set volume: %v", err)
	}
	if c.volume != 60 {
		t.Errorf("the live cast should have taken it, got %d", c.volume)
	}
}

// Unmuting has to pick a level, and the only honest one is what the household
// last set — inventing a number would be louder or quieter than they chose.
func TestMuteRestoresTheStoredLevel(t *testing.T) {
	c := &fakeControl{playing: true}
	e := withCast(receiver(), c)

	if err := e.SetMute(context.Background(), true); err != nil {
		t.Fatalf("mute: %v", err)
	}
	if c.volume != 0 {
		t.Errorf("muted to %d, want 0", c.volume)
	}
	if err := e.SetMute(context.Background(), false); err != nil {
		t.Fatalf("unmute: %v", err)
	}
	if c.volume != 40 {
		t.Errorf("unmuted to %d, want the stored 40", c.volume)
	}
}

func TestReceiverClaimsNothingItCannotDo(t *testing.T) {
	d := NewAirPlayEndpoint(receiver(), nil).Descriptor()
	if d.Vendor != media.VendorAirPlay {
		t.Errorf("vendor = %q", d.Vendor)
	}
	for _, forbidden := range []media.Capability{
		media.CapPlayURI, media.CapNativeService, media.CapConnect,
		media.CapGroup, media.CapQueue, media.CapSeek, media.CapWake,
	} {
		if d.Caps.Has(forbidden) {
			t.Errorf("a receiver must not claim %v", forbidden)
		}
	}
	if !d.Caps.Has(media.CapAirPlay) {
		t.Error("the one capability that reaches it is missing")
	}
	// No GroupKey: receivers play together by being driven by one sender,
	// not by grouping with each other, and a key here would have the route
	// engine try to make one of them lead.
	if d.GroupKey != "" {
		t.Errorf("group key = %q, want none", d.GroupKey)
	}
}
