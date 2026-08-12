package airplay

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"homehub/internal/media"
)

// pcmSource is an endless decoder: it hands out CD-rate samples as fast as
// they are read, which is the burst case the pump's pacing exists to absorb.
type pcmSource struct {
	closed chan struct{}
}

func newPCMSource() *pcmSource { return &pcmSource{closed: make(chan struct{})} }

func (s *pcmSource) Read(p []byte) (int, error) {
	select {
	case <-s.closed:
		return 0, io.EOF
	default:
	}
	for i := range p {
		p[i] = byte(i)
	}
	return len(p), nil
}

func (s *pcmSource) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return nil
}

func castStream(body io.ReadCloser) *media.Stream {
	pcm := media.CDQuality
	return &media.Stream{
		Body:        body,
		ContentType: "audio/wav",
		PCM:         &pcm,
		Meta:        media.Metadata{Title: "Song", Artist: "Band", Album: "Record"},
	}
}

func dest(f *fakeReceiver, id string) media.AirPlayDest {
	d := f.device()
	return media.AirPlayDest{
		ID: id, Name: d.Name, Host: d.IP, Port: d.Port,
		PCM: true, ALAC: true, Metadata: true, Volume: 30,
	}
}

// The whole point of the route: two receivers, one decode, one clock.
func TestCastDrivesEveryReceiverFromOneStream(t *testing.T) {
	a, b := newFakeReceiver(t), newFakeReceiver(t)
	src := newPCMSource()
	c := NewCaster(nil)
	defer c.Close()

	stop, err := c.Cast(context.Background(), castStream(src),
		[]media.AirPlayDest{dest(a, "one"), dest(b, "two")})
	if err != nil {
		t.Fatalf("cast: %v", err)
	}
	defer stop()

	for _, f := range []*fakeReceiver{a, b} {
		if !eventually(func() bool { return len(f.received()) > 0 }) {
			t.Fatal("a receiver in the cast got no audio")
		}
		if _, ok := f.request("RECORD"); !ok {
			t.Error("a receiver in the cast was never started")
		}
		// Metadata is what fills in a RoPieee's display.
		if req, ok := f.request("SET_PARAMETER"); ok && strings.Contains(req.Body, "volume") {
			// The volume is sent first; the metadata follows as a second
			// SET_PARAMETER, so look for the DAAP one specifically.
			found := false
			f.mu.Lock()
			for _, r := range f.requests {
				if r.Headers["content-type"] == daapContentType && strings.Contains(r.Body, "Song") {
					found = true
				}
			}
			f.mu.Unlock()
			if !found {
				t.Error("the receiver was told nothing about what is playing")
			}
		}
	}
}

// A cast is all receivers or none. Half a room playing with no error to
// explain the other half is worse than a clean failure.
func TestCastUnwindsWhenOneReceiverRefuses(t *testing.T) {
	good, bad := newFakeReceiver(t), newFakeReceiver(t)
	bad.mu.Lock()
	bad.refuse = 453
	bad.mu.Unlock()

	src := newPCMSource()
	c := NewCaster(nil)
	defer c.Close()

	if _, err := c.Cast(context.Background(), castStream(src),
		[]media.AirPlayDest{dest(good, "one"), dest(bad, "two")}); err == nil {
		t.Fatal("want an error")
	}
	// The one that accepted must have been released again, or it would sit
	// holding a session for a cast that never ran.
	if !eventually(func() bool { _, ok := good.request("TEARDOWN"); return ok }) {
		t.Error("the receiver that accepted was left holding the session")
	}
}

func TestCastRefusesAnythingButCDQualityPCM(t *testing.T) {
	f := newFakeReceiver(t)
	c := NewCaster(nil)
	defer c.Close()

	// A stream with no PCM description at all: a container this package
	// cannot parse, and sending it would put its header through a speaker.
	s := castStream(newPCMSource())
	s.PCM = nil
	if _, err := c.Cast(context.Background(), s, []media.AirPlayDest{dest(f, "one")}); err == nil {
		t.Fatal("want an error for a non-PCM source")
	}

	// The right shape, the wrong rate. Resampling is the one lossy step
	// this path must not silently take.
	s = castStream(newPCMSource())
	s.PCM = &media.PCMFormat{SampleRate: 48000, BitDepth: 16, Channels: 2, LittleEndian: true}
	if _, err := c.Cast(context.Background(), s, []media.AirPlayDest{dest(f, "one")}); err == nil {
		t.Fatal("want an error for 48 kHz audio")
	}
}

func TestLiveControlPausesAndResumes(t *testing.T) {
	f := newFakeReceiver(t)
	src := newPCMSource()
	c := NewCaster(nil)
	defer c.Close()

	stop, err := c.Cast(context.Background(), castStream(src), []media.AirPlayDest{dest(f, "one")})
	if err != nil {
		t.Fatalf("cast: %v", err)
	}
	defer stop()

	ctrl, ok := c.Live("one")
	if !ok {
		t.Fatal("a running cast should be reachable by endpoint id")
	}
	if !ctrl.Playing() {
		t.Error("a fresh cast is playing")
	}
	if _, ok := c.Live("nobody"); ok {
		t.Error("an endpoint outside the cast must not be reachable")
	}

	if err := ctrl.Pause(context.Background()); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if ctrl.Playing() {
		t.Error("a paused cast is not playing")
	}
	// Pause drops what the receiver has buffered, or the two seconds
	// already sent would play on after the button.
	if _, ok := f.request("FLUSH"); !ok {
		t.Error("pause should flush the receiver")
	}

	// Nothing more should arrive while paused.
	before := len(f.received())
	time.Sleep(150 * time.Millisecond)
	if after := len(f.received()); after > before+1 {
		t.Errorf("a paused cast kept sending: %d → %d packets", before, after)
	}

	if err := ctrl.Resume(context.Background()); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !eventually(func() bool { return len(f.received()) > before+1 }) {
		t.Error("resuming did not restart the audio")
	}
}

func TestStoppingACastReleasesTheDecoder(t *testing.T) {
	f := newFakeReceiver(t)
	src := newPCMSource()
	c := NewCaster(nil)

	stop, err := c.Cast(context.Background(), castStream(src), []media.AirPlayDest{dest(f, "one")})
	if err != nil {
		t.Fatalf("cast: %v", err)
	}
	stop()
	stop() // must be safe twice

	// The decoder holds the account's service session; leaving it running
	// would keep the user's Spotify pointed at a HomeHub sending nothing.
	if !eventually(func() bool {
		select {
		case <-src.closed:
			return true
		default:
			return false
		}
	}) {
		t.Error("stopping the cast should close the source")
	}
	if _, ok := c.Live("one"); ok {
		t.Error("a stopped cast should not still be reachable")
	}
}

// eventually polls a condition for up to two seconds — long enough for a UDP
// round trip on loopback, short enough that a broken test fails quickly.
func eventually(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// Replacing a cast has to release the receiver before asking for it again: a
// receiver holds one sender at a time and answers a second with "busy", so
// opening first would fail against a session this process is still holding.
func TestReplacingACastReleasesTheReceiverFirst(t *testing.T) {
	f := newFakeReceiver(t)
	c := NewCaster(nil)
	defer c.Close()

	first := newPCMSource()
	stop, err := c.Cast(context.Background(), castStream(first), []media.AirPlayDest{dest(f, "one")})
	if err != nil {
		t.Fatalf("first cast: %v", err)
	}
	defer stop()
	if !eventually(func() bool { return len(f.received()) > 0 }) {
		t.Fatal("the first cast never started")
	}

	second := newPCMSource()
	if _, err := c.Cast(context.Background(), castStream(second),
		[]media.AirPlayDest{dest(f, "one")}); err != nil {
		t.Fatalf("second cast: %v", err)
	}
	// The first source is released, and the receiver is being driven by the
	// new one rather than by a session nobody holds.
	if !eventually(func() bool {
		select {
		case <-first.closed:
			return true
		default:
			return false
		}
	}) {
		t.Error("the replaced cast should have released its decoder")
	}
	if _, ok := c.Live("one"); !ok {
		t.Error("the new cast should be the live one")
	}
}
