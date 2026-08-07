package media

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// testStep paces the multi-step ramps below. Fade's real FadeStep is fifteen
// seconds, which is right against a speaker and absurd against a test.
const testStep = 5 * time.Millisecond

// volumeEndpoint is a speaker that remembers what it was set to, so a fade
// can be checked by where the room ended up rather than by counting calls.
type volumeEndpoint struct {
	*fakeEndpoint
	mu      sync.Mutex
	level   int
	written []int
	refuse  bool
	// unreadable makes State fail, which is how a speaker that is off the
	// network behaves — the case Volumes must not report as zero.
	unreadable bool
}

func volEP(name string, at int) *volumeEndpoint {
	return &volumeEndpoint{fakeEndpoint: sonosEP(name), level: at}
}

func (v *volumeEndpoint) State(context.Context) (*NowPlaying, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.unreadable {
		return nil, errors.New("off the network")
	}
	return &NowPlaying{State: StatePlaying, Volume: v.level}, nil
}

func (v *volumeEndpoint) SetVolume(_ context.Context, level int) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.refuse {
		return errors.New("busy")
	}
	v.level = level
	v.written = append(v.written, level)
	return nil
}

func (v *volumeEndpoint) at() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.level
}

func (v *volumeEndpoint) writes() []int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]int(nil), v.written...)
}

// A zone's speakers rarely sit at the same volume. Fading them to a common
// floor and back to a common level would flatten an arrangement someone made
// on purpose, so each speaker walks from where it actually is.
func TestFadeWalksEachSpeakerFromItsOwnLevel(t *testing.T) {
	loud, quiet := volEP("kitchen", 40), volEP("hall", 10)
	eps := []Endpoint{loud, quiet}

	from, err := fadeEvery(context.Background(), eps, 0, 4*testStep, testStep)
	if err != nil {
		t.Fatal(err)
	}
	if from["kitchen"] != 40 || from["hall"] != 10 {
		t.Errorf("starting levels = %v, want the two the speakers were at", from)
	}
	if loud.at() != 0 || quiet.at() != 0 {
		t.Errorf("ended at %d and %d, want both at the target", loud.at(), quiet.at())
	}

	// The loud one has further to travel, so at the halfway step it must
	// still be above the quiet one rather than level with it.
	lw, qw := loud.writes(), quiet.writes()
	if len(lw) < 2 || len(qw) < 2 {
		t.Fatalf("fade wrote %d and %d levels, want one per step", len(lw), len(qw))
	}
	if lw[1] <= qw[1] {
		t.Errorf("mid-fade the loud speaker was at %d and the quiet one at %d; they were flattened", lw[1], qw[1])
	}
}

// A zero duration is a jump, which is what makes "no fade" the same code path
// as a fade rather than a branch at every call site.
func TestFadeWithNoDurationIsAJumpThatStillReportsWhereItWas(t *testing.T) {
	e := volEP("bedroom", 35)
	from, err := Fade(context.Background(), []Endpoint{e}, 12, 0)
	if err != nil {
		t.Fatal(err)
	}
	if from["bedroom"] != 35 {
		t.Errorf("starting level = %v, want 35", from)
	}
	if e.at() != 12 {
		t.Errorf("ended at %d, want 12", e.at())
	}
}

// An interrupted fade is not a failure to be unwound — it is a fade someone
// interrupted, and the caller needs the starting levels to restore the room.
func TestFadeCancelledMidRampReturnsWhereItStarted(t *testing.T) {
	e := volEP("lounge", 30)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	var from map[string]int
	var err error
	go func() {
		from, err = Fade(ctx, []Endpoint{e}, 0, time.Hour)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled fade did not return")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if from["lounge"] != 30 {
		t.Errorf("starting levels = %v, want the 30 the room was at", from)
	}
}

// A speaker that refuses one write in the middle of a ten-minute fade is far
// more likely to be briefly busy than gone. Abandoning the room for it would
// leave the fade half-applied, which is worse than either outcome.
func TestFadeCarriesOnPastASpeakerThatRefusesAStep(t *testing.T) {
	good, bad := volEP("kitchen", 20), volEP("hall", 20)
	bad.refuse = true

	if _, err := fadeEvery(context.Background(), []Endpoint{good, bad}, 0, 3*testStep, testStep); err != nil {
		t.Errorf("err = %v, want the ramp to finish anyway", err)
	}
	if good.at() != 0 {
		t.Errorf("the working speaker ended at %d, want 0", good.at())
	}
}

// A speaker that could not be read must not be restored to zero: absent and
// "at zero" are different, and only one of them is safe to write back.
func TestVolumesOmitsSpeakersThatDidNotAnswerAndSetVolumesLeavesThemAlone(t *testing.T) {
	ok, off := volEP("kitchen", 25), volEP("hall", 25)
	off.unreadable = true
	eps := []Endpoint{ok, off}

	levels := Volumes(context.Background(), eps)
	if _, present := levels["hall"]; present {
		t.Error("an unreadable speaker appeared in the volume map")
	}
	if levels["kitchen"] != 25 {
		t.Errorf("levels = %v, want the readable speaker at 25", levels)
	}

	off.unreadable = false
	if err := SetVolumes(context.Background(), eps, levels); err != nil {
		t.Fatal(err)
	}
	if off.at() != 25 {
		t.Errorf("the omitted speaker was written to %d; it should have been left alone", off.at())
	}
	if n := len(off.writes()); n != 0 {
		t.Errorf("the omitted speaker got %d writes, want none", n)
	}
}

// Rounding rather than truncating, so a fade up reaches its first increment
// early instead of sitting at the floor for a third of the ramp.
func TestLerpRoundsInBothDirections(t *testing.T) {
	cases := []struct{ from, to, i, n, want int }{
		{0, 10, 0, 10, 0},
		{0, 10, 10, 10, 10},
		{0, 10, 5, 10, 5},
		{0, 3, 1, 6, 1},   // 0.5 up rounds to 1, not down to 0
		{30, 0, 1, 6, 25}, // -5 exactly
		{30, 0, 5, 6, 5},
		{10, 10, 3, 6, 10},
	}
	for _, c := range cases {
		if got := lerp(c.from, c.to, c.i, c.n); got != c.want {
			t.Errorf("lerp(%d→%d, %d/%d) = %d, want %d", c.from, c.to, c.i, c.n, got, c.want)
		}
	}
	if got := lerp(0, 200, 1, 1); got != 100 {
		t.Errorf("lerp past the top = %d, want it clamped to 100", got)
	}
}

func TestFadeOnAnEmptyZoneSaysSo(t *testing.T) {
	if _, err := Fade(context.Background(), nil, 10, 0); !errors.Is(err, ErrEmptyZone) {
		t.Errorf("err = %v, want ErrEmptyZone", err)
	}
}
