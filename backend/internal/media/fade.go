package media

import (
	"context"
	"time"
)

// Ramping volume over time.
//
// SetVolume is a tap: one level, every speaker, now. That is the right shape
// for a slider and the wrong shape for the two things a room does on its own
// — coming up in the morning and going quiet at night. A jump from silence to
// twenty-five is an alarm clock; the same twenty-five arrived at over ten
// minutes is being woken by music, and they are not the same experience.
//
// Fades are deliberately per endpoint rather than one level applied to all.
// A zone's speakers rarely sit at the same volume — the kitchen pair is loud,
// the one in the hall is not — and fading them to a common floor and back to
// a common level would quietly flatten an arrangement someone made on
// purpose. Each speaker walks from where it actually is.

// FadeStep is how often a fade writes a new level. Fifteen seconds is chosen
// against the traffic, not the ear: every step is a round trip to every
// speaker in the zone, and a forty-minute sleep fade at this rate is 160
// writes rather than the 2,400 a per-second ramp would send. Volume steps are
// integers anyway, so a slow fade over a small range is a staircase however
// often it is written.
const FadeStep = 15 * time.Second

// Volumes reads every endpoint's current level. Endpoints that don't answer
// are absent from the map rather than present as zero — the difference
// matters, because a caller restoring volumes must not set a speaker it never
// managed to read to silence.
func Volumes(ctx context.Context, eps []Endpoint) map[string]int {
	out := make(map[string]int, len(eps))
	for id, st := range States(ctx, eps) {
		if st != nil {
			out[id] = st.Volume
		}
	}
	return out
}

// SetVolumes puts each endpoint back to a specific level, by descriptor id.
// Endpoints missing from the map are left alone — see Volumes on why that is
// the only safe reading of an absent entry.
func SetVolumes(ctx context.Context, eps []Endpoint, levels map[string]int) error {
	targets := make([]Endpoint, 0, len(eps))
	for _, e := range eps {
		if _, ok := levels[e.Descriptor().ID]; ok {
			targets = append(targets, e)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	return fanOut(ctx, targets, func(ctx context.Context, e Endpoint) error {
		return e.SetVolume(ctx, levels[e.Descriptor().ID])
	})
}

// Fade walks every endpoint from where it is now to target over d, and
// returns the levels they started at so a caller can put them back.
//
// A zero or negative d is a jump, which is what makes "no fade" the same code
// path as a fade rather than a branch at every call site.
//
// Cancelling ctx stops the ramp where it stands and returns ctx's error along
// with the starting levels: an interrupted fade is not a failure to be
// unwound, it is a fade someone interrupted, and the caller holds what it
// needs to restore the room either way.
//
// Errors from individual steps do not abort the ramp. A speaker that refuses
// one write in the middle of a ten-minute fade is far more likely to be
// briefly busy than gone, and giving up on the whole room for it would leave
// the fade half-applied — which is the one outcome worse than either finishing
// or not starting.
func Fade(ctx context.Context, eps []Endpoint, target int, d time.Duration) (map[string]int, error) {
	return fadeEvery(ctx, eps, target, d, FadeStep)
}

// fadeEvery is Fade with the step interval as an argument. Exists so the
// tests can exercise a multi-step ramp without waiting the minutes a
// FadeStep-paced one takes; nothing else should need it.
func fadeEvery(ctx context.Context, eps []Endpoint, target int, d, step time.Duration) (map[string]int, error) {
	if len(eps) == 0 {
		return nil, ErrEmptyZone
	}
	target = clampVolume(target)
	from := Volumes(ctx, eps)

	if d <= 0 {
		return from, SetVolume(ctx, eps, target)
	}

	steps := int(d / step)
	if steps < 1 {
		steps = 1
	}
	ticker := time.NewTicker(d / time.Duration(steps))
	defer ticker.Stop()

	for i := 1; i <= steps; i++ {
		select {
		case <-ctx.Done():
			return from, ctx.Err()
		case <-ticker.C:
		}
		levels := make(map[string]int, len(from))
		for id, start := range from {
			levels[id] = lerp(start, target, i, steps)
		}
		// The last step lands exactly on target for every endpoint,
		// including any that could not be read at the start.
		if i == steps {
			_ = SetVolume(ctx, eps, target)
			continue
		}
		_ = SetVolumes(ctx, eps, levels)
	}
	return from, nil
}

// lerp is where a walk from `from` to `to` stands after step i of n. Rounded
// rather than truncated so a fade up actually reaches its first increment
// early instead of sitting at the floor for a third of the ramp.
func lerp(from, to, i, n int) int {
	delta := (to - from) * i
	if delta < 0 {
		return clampVolume(from + (delta-n/2)/n)
	}
	return clampVolume(from + (delta+n/2)/n)
}

func clampVolume(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
