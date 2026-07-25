package kef

// The configuration half of the speaker: the DSP block KEF Connect calls
// "EQ settings" (placement, treble trim, phase, high-pass, subwoofer), the
// volume ceiling, the auto-standby timer, and the device's own identity.
//
// Two rules carried over from the Sonos bridge, for the same reasons:
//
//   - Render only what the speaker answered for. Models differ — an LSX II
//     has no subwoofer output — and there is no "what can you do" call, so
//     every field is a pointer: a path that faults simply isn't in the
//     payload and the UI doesn't draw a control the speaker would refuse.
//   - Configuration is read on demand, never polled. None of this changes on
//     its own, so it is fetched when the settings pane opens and nowhere
//     else. Only *state* belongs in the status poll.

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

const (
	pathBassExtension  = "settings:/kef/dsp/bassExtension"
	pathDeskMode       = "settings:/kef/dsp/deskMode"
	pathDeskModeSet    = "settings:/kef/dsp/deskModeSetting"
	pathWallMode       = "settings:/kef/dsp/wallMode"
	pathWallModeSet    = "settings:/kef/dsp/wallModeSetting"
	pathTrebleAmount   = "settings:/kef/dsp/trebleAmount"
	pathPhaseCorrect   = "settings:/kef/dsp/phaseCorrection"
	pathHighPassMode   = "settings:/kef/dsp/highPassMode"
	pathHighPassFreq   = "settings:/kef/dsp/highPassModeFreq"
	pathSubwooferOut   = "settings:/kef/dsp/subwooferOut"
	pathSubOutLPFreq   = "settings:/kef/dsp/subOutLPFreq"
	pathSubwooferGain  = "settings:/kef/dsp/subwooferGain"
	pathSubwooferPhase = "settings:/kef/dsp/subwooferPolarity"
)

// Bass extension presets.
const (
	BassLess     = "less"
	BassStandard = "standard"
	BassExtra    = "extra"
)

// Subwoofer polarity values.
const (
	Phase0   = "phase0"
	Phase180 = "phase180"
)

// Ranges the DSP controls accept. These are the bounds KEF Connect itself
// offers; the speaker is the final authority and will refuse anything it
// dislikes, but validating here means a bad request fails with a sentence
// instead of an opaque HTTP 400 from the speaker.
//
// Placement and treble trim are in tenths of a dB, which is how the speaker
// stores them: -60 is -6.0 dB.
const (
	DeskMinTenthsDB   = -60
	DeskMaxTenthsDB   = 0
	WallMinTenthsDB   = -60
	WallMaxTenthsDB   = 0
	TrebleMinTenthsDB = -20
	TrebleMaxTenthsDB = 20

	HighPassMinHz = 50
	HighPassMaxHz = 120
	SubLPMinHz    = 40
	SubLPMaxHz    = 250
	SubGainMinDB  = -10
	SubGainMaxDB  = 10
)

// Settings is everything the settings pane shows for one speaker. Every
// field is a pointer: nil means "this speaker didn't answer for it", which
// is how a model without a subwoofer output is told apart from one whose
// output is off.
type Settings struct {
	Info Info `json:"info"`

	// Sound — the DSP block.
	BassExtension *string `json:"bass_extension,omitempty"` // less | standard | extra
	DeskMode      *bool   `json:"desk_mode,omitempty"`
	DeskGain      *int    `json:"desk_gain,omitempty"` // tenths of a dB
	WallMode      *bool   `json:"wall_mode,omitempty"`
	WallGain      *int    `json:"wall_gain,omitempty"` // tenths of a dB
	Treble        *int    `json:"treble,omitempty"`    // tenths of a dB
	PhaseCorrect  *bool   `json:"phase_correction,omitempty"`

	// Subwoofer & high-pass. Present only on models with a sub output.
	HighPassMode *bool   `json:"high_pass_mode,omitempty"`
	HighPassFreq *int    `json:"high_pass_freq,omitempty"` // Hz
	SubwooferOut *bool   `json:"subwoofer_out,omitempty"`
	SubLPFreq    *int    `json:"sub_lp_freq,omitempty"` // Hz
	SubGain      *int    `json:"sub_gain,omitempty"`    // dB
	SubPhase     *string `json:"sub_phase,omitempty"`   // phase0 | phase180

	// Power & volume.
	StandbyMode *string `json:"standby_mode,omitempty"`
	MaxVolume   *int    `json:"max_volume,omitempty"`
	VolumeLimit *bool   `json:"volume_limit,omitempty"`
}

// Info is the device block: what this speaker is, not how it is configured.
type Info struct {
	Name     string `json:"name,omitempty"`
	Model    string `json:"model,omitempty"`
	MAC      string `json:"mac,omitempty"`
	Firmware string `json:"firmware,omitempty"`
	Release  string `json:"release,omitempty"`
}

// GetSettings reads the whole configuration snapshot. Twenty-odd paths, run
// in parallel branches so opening the pane costs one slow round-trip rather
// than twenty. Every read is best-effort: a path the model doesn't have
// leaves its field nil, which is the signal the UI wants.
func GetSettings(ctx context.Context, ip string) (*Settings, error) {
	// Identity doubles as the reachability check: if the speaker won't say
	// what it is, nothing else in here is worth reporting.
	dev, err := Describe(ctx, ip)
	if err != nil {
		return nil, err
	}
	s := &Settings{Info: Info{Name: dev.Name, Model: dev.Model, MAC: dev.MAC}}
	s.Info.Firmware, s.Info.Release = Firmware(ctx, ip)

	var mu sync.Mutex
	var wg sync.WaitGroup

	str := func(path string, dst **string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := getString(ctx, ip, path)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			v = strings.TrimSpace(v)
			*dst = &v
		}()
	}
	enum := func(path, typ string, dst **string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := getEnum(ctx, ip, path, typ)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			*dst = &v
		}()
	}
	num := func(path string, dst **int) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := getInt(ctx, ip, path)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			*dst = &v
		}()
	}
	flag := func(path string, dst **bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := getBool(ctx, ip, path)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			*dst = &v
		}()
	}

	str(pathBassExtension, &s.BassExtension)
	flag(pathDeskMode, &s.DeskMode)
	num(pathDeskModeSet, &s.DeskGain)
	flag(pathWallMode, &s.WallMode)
	num(pathWallModeSet, &s.WallGain)
	num(pathTrebleAmount, &s.Treble)
	flag(pathPhaseCorrect, &s.PhaseCorrect)
	flag(pathHighPassMode, &s.HighPassMode)
	num(pathHighPassFreq, &s.HighPassFreq)
	flag(pathSubwooferOut, &s.SubwooferOut)
	num(pathSubOutLPFreq, &s.SubLPFreq)
	num(pathSubwooferGain, &s.SubGain)
	str(pathSubwooferPhase, &s.SubPhase)
	enum(pathStandbyMode, typeStandbyMode, &s.StandbyMode)
	num(pathMaxVolume, &s.MaxVolume)
	flag(pathVolumeLimit, &s.VolumeLimit)

	wg.Wait()
	return s, nil
}

// SettingsPatch is a partial update. Only the fields present are written, so
// a switch flipped in the UI costs exactly one call to the speaker.
type SettingsPatch struct {
	BassExtension *string `json:"bass_extension,omitempty"`
	DeskMode      *bool   `json:"desk_mode,omitempty"`
	DeskGain      *int    `json:"desk_gain,omitempty"`
	WallMode      *bool   `json:"wall_mode,omitempty"`
	WallGain      *int    `json:"wall_gain,omitempty"`
	Treble        *int    `json:"treble,omitempty"`
	PhaseCorrect  *bool   `json:"phase_correction,omitempty"`
	HighPassMode  *bool   `json:"high_pass_mode,omitempty"`
	HighPassFreq  *int    `json:"high_pass_freq,omitempty"`
	SubwooferOut  *bool   `json:"subwoofer_out,omitempty"`
	SubLPFreq     *int    `json:"sub_lp_freq,omitempty"`
	SubGain       *int    `json:"sub_gain,omitempty"`
	SubPhase      *string `json:"sub_phase,omitempty"`
	StandbyMode   *string `json:"standby_mode,omitempty"`
	MaxVolume     *int    `json:"max_volume,omitempty"`
	VolumeLimit   *bool   `json:"volume_limit,omitempty"`
}

// Empty reports whether the patch asks for nothing.
func (p SettingsPatch) Empty() bool { return p == SettingsPatch{} }

// Validate checks every field the patch carries against the ranges and
// vocabularies the speaker accepts, so a bad value is refused with a
// sentence naming the limit rather than forwarded to the device.
func (p SettingsPatch) Validate() error {
	if p.BassExtension != nil {
		switch *p.BassExtension {
		case BassLess, BassStandard, BassExtra:
		default:
			return fmt.Errorf("bass_extension must be %q, %q or %q", BassLess, BassStandard, BassExtra)
		}
	}
	if p.SubPhase != nil && *p.SubPhase != Phase0 && *p.SubPhase != Phase180 {
		return fmt.Errorf("sub_phase must be %q or %q", Phase0, Phase180)
	}
	if p.StandbyMode != nil && !ValidStandbyMode(*p.StandbyMode) {
		return fmt.Errorf("standby_mode must be %q, %q or %q", StandbyNever, Standby20Min, Standby60Min)
	}
	checks := []struct {
		v      *int
		name   string
		lo, hi int
		unit   string
	}{
		{p.DeskGain, "desk_gain", DeskMinTenthsDB, DeskMaxTenthsDB, "tenths of a dB"},
		{p.WallGain, "wall_gain", WallMinTenthsDB, WallMaxTenthsDB, "tenths of a dB"},
		{p.Treble, "treble", TrebleMinTenthsDB, TrebleMaxTenthsDB, "tenths of a dB"},
		{p.HighPassFreq, "high_pass_freq", HighPassMinHz, HighPassMaxHz, "Hz"},
		{p.SubLPFreq, "sub_lp_freq", SubLPMinHz, SubLPMaxHz, "Hz"},
		{p.SubGain, "sub_gain", SubGainMinDB, SubGainMaxDB, "dB"},
		{p.MaxVolume, "max_volume", 0, 100, ""},
	}
	for _, c := range checks {
		if c.v == nil {
			continue
		}
		if *c.v < c.lo || *c.v > c.hi {
			if c.unit != "" {
				return fmt.Errorf("%s must be between %d and %d %s", c.name, c.lo, c.hi, c.unit)
			}
			return fmt.Errorf("%s must be between %d and %d", c.name, c.lo, c.hi)
		}
	}
	return nil
}

// ApplySettings writes every field the patch carries. Writes run in
// parallel — they touch independent paths — and the first refusal is
// returned, so the caller can roll that one field back on screen while the
// rest of the patch stands.
func ApplySettings(ctx context.Context, ip string, p SettingsPatch) error {
	if err := p.Validate(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	run := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}

	if v := p.BassExtension; v != nil {
		run(func() error { return setString(ctx, ip, pathBassExtension, *v) })
	}
	if v := p.DeskMode; v != nil {
		run(func() error { return setBool(ctx, ip, pathDeskMode, *v) })
	}
	if v := p.DeskGain; v != nil {
		run(func() error { return setInt(ctx, ip, pathDeskModeSet, *v) })
	}
	if v := p.WallMode; v != nil {
		run(func() error { return setBool(ctx, ip, pathWallMode, *v) })
	}
	if v := p.WallGain; v != nil {
		run(func() error { return setInt(ctx, ip, pathWallModeSet, *v) })
	}
	if v := p.Treble; v != nil {
		run(func() error { return setInt(ctx, ip, pathTrebleAmount, *v) })
	}
	if v := p.PhaseCorrect; v != nil {
		run(func() error { return setBool(ctx, ip, pathPhaseCorrect, *v) })
	}
	if v := p.HighPassMode; v != nil {
		run(func() error { return setBool(ctx, ip, pathHighPassMode, *v) })
	}
	if v := p.HighPassFreq; v != nil {
		run(func() error { return setInt(ctx, ip, pathHighPassFreq, *v) })
	}
	if v := p.SubwooferOut; v != nil {
		run(func() error { return setBool(ctx, ip, pathSubwooferOut, *v) })
	}
	if v := p.SubLPFreq; v != nil {
		run(func() error { return setInt(ctx, ip, pathSubOutLPFreq, *v) })
	}
	if v := p.SubGain; v != nil {
		run(func() error { return setInt(ctx, ip, pathSubwooferGain, *v) })
	}
	if v := p.SubPhase; v != nil {
		run(func() error { return setString(ctx, ip, pathSubwooferPhase, *v) })
	}
	if v := p.StandbyMode; v != nil {
		run(func() error { return SetStandbyMode(ctx, ip, *v) })
	}
	if v := p.MaxVolume; v != nil {
		run(func() error { return SetMaxVolume(ctx, ip, *v) })
	}
	if v := p.VolumeLimit; v != nil {
		run(func() error { return SetVolumeLimit(ctx, ip, *v) })
	}

	wg.Wait()
	return firstErr
}
