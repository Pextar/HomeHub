package musictimer

import (
	"context"
	"testing"
	"time"

	"homehub/internal/audio"
	"homehub/internal/music"
	"homehub/internal/speakermon"
	"homehub/internal/store"
)

// The engine's real work — playing, fading, restoring — is speaker I/O and is
// covered where the pieces live (media.Fade, store.ValidateMusicTimer). What is
// worth pinning here is the bookkeeping around it, because getting that wrong
// leaves a room permanently marked as fading with a cancel func nobody calls.

func testEngine(t *testing.T) (*Engine, *store.Store) {
	t.Helper()
	st := store.New(t.TempDir(), nil)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	st.Sonos["sp1"] = &store.SonosSpeaker{
		ID: "sp1", Name: "Bedroom", IP: "10.0.0.5", UUID: "RINCON_AAA",
	}
	speakers := speakermon.New(speakermon.Config{Store: st})
	return New(Config{
		Store: st,
		Music: music.New(music.Config{
			Store:    st,
			Speakers: speakers,
			Audio:    audio.New(audio.Config{}),
		}),
	}), st
}

// Every path out of a timer that doesn't start a ramp — no fade asked for, or
// a failure before one could start — has to release the room, or it reads as
// fading for the life of the process and its cancel func is never called.
func TestATimerWithNoRampLeavesNoRoomMarkedFading(t *testing.T) {
	e, _ := testEngine(t)
	vol := 0
	// No speaker answers in a test, so this fails at the point it reaches the
	// network — which is exactly the "failed before a ramp started" path that
	// must still tidy up. The deadline is the test's own: without one this
	// waits out music.Timeout against an address nobody is at.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	e.fire(ctx, store.MusicTimer{
		ID: "mt_1", Room: "sonos:sp1", Action: store.MusicStop,
		Enabled: true, Volume: &vol,
	})

	if fading := e.FadingRooms(); len(fading) != 0 {
		t.Errorf("rooms fading after a timer with no ramp: %v", fading)
	}
	if e.CancelFade("sonos:sp1") {
		t.Error("a fade was still registered for the room")
	}
}

// A timer aimed at a room that no longer exists is reported, not silently
// dropped — and it must not leave a fade behind either.
func TestATimerForAMissingRoomIsRecordedAsAnError(t *testing.T) {
	e, st := testEngine(t)
	e.fire(t.Context(), store.MusicTimer{
		ID: "mt_1", Room: "sonos:gone", Action: store.MusicStop, Enabled: true,
	})

	entries := st.Activity.Recent(1)
	if len(entries) != 1 {
		t.Fatal("nothing was written to the activity log")
	}
	if entries[0].Status != "error" || entries[0].Source != "music-timer" {
		t.Errorf("entry = %+v, want a music-timer error row", entries[0])
	}
	if fading := e.FadingRooms(); len(fading) != 0 {
		t.Errorf("rooms fading after a timer that could not resolve: %v", fading)
	}
}

// A second ramp on the same room replaces the first, which is what stops a
// wake-up fade and a sleep fade from walking the same speakers in opposite
// directions.
func TestBeginFadeCancelsTheRampAlreadyRunning(t *testing.T) {
	e, _ := testEngine(t)
	first := e.beginFade(t.Context(), "sonos:sp1")
	second := e.beginFade(t.Context(), "sonos:sp1")

	select {
	case <-first.Done():
	default:
		t.Error("the ramp already running was not cancelled")
	}
	select {
	case <-second.Done():
		t.Error("the new ramp was cancelled too")
	default:
	}
	e.endFade("sonos:sp1")
}

// Shutdown stops every ramp: one left running past it would leave a room at an
// interim volume with nothing left to move it.
func TestCancelAllFadesStopsEveryRamp(t *testing.T) {
	e, _ := testEngine(t)
	rooms := []context.Context{
		e.beginFade(t.Context(), "sonos:sp1"),
		e.beginFade(t.Context(), "zone:z1"),
	}
	e.cancelAllFades()

	for i, ctx := range rooms {
		select {
		case <-ctx.Done():
		default:
			t.Errorf("ramp %d survived shutdown", i)
		}
	}
	if fading := e.FadingRooms(); len(fading) != 0 {
		t.Errorf("rooms still fading after shutdown: %v", fading)
	}
}

// The floor a wake-up starts from is never literal silence: a timer that
// spends its first minutes at zero reads as one that failed, and someone lying
// awake wondering is worse off than someone hearing the first bar quietly.
func TestFadeFloorIsNeverSilence(t *testing.T) {
	for _, target := range []int{1, 2, 5, 25, 100} {
		if got := FadeFloor(target); got < 1 {
			t.Errorf("FadeFloor(%d) = %d, want at least 1", target, got)
		} else if got > target {
			t.Errorf("FadeFloor(%d) = %d, want no louder than the target", target, got)
		}
	}
	if got, want := FadeFloor(25), 5; got != want {
		t.Errorf("FadeFloor(25) = %d, want a fifth (%d)", got, want)
	}
}
