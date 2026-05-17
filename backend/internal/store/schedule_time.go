package store

import (
	"time"

	"rf-socket-controller/internal/solar"
)

// Schedule time modes. An empty string is treated as ModeFixed for
// backwards compatibility with schedules created before sunrise/sunset
// support landed.
const (
	ModeFixed   = "fixed"
	ModeSunrise = "sunrise"
	ModeSunset  = "sunset"
)

// EffectiveHHMM returns the "HH:MM" the schedule should fire at on the
// local date of now. ok is false when the trigger cannot be determined
// (e.g. sunrise/sunset requested but no location configured, or polar
// day/night). For fixed-time schedules the configured Time is returned
// unchanged.
func (s *Schedule) EffectiveHHMM(now time.Time, settings *Settings) (string, bool) {
	mode := s.TimeMode
	if mode == "" {
		mode = ModeFixed
	}
	if mode == ModeFixed {
		if s.Time == "" {
			return "", false
		}
		return s.Time, true
	}
	if !settings.HasLocation() {
		return "", false
	}
	sunrise, sunset, ok := solar.Times(now, settings.Latitude, settings.Longitude)
	if !ok {
		return "", false
	}
	var base time.Time
	switch mode {
	case ModeSunrise:
		base = sunrise
	case ModeSunset:
		base = sunset
	default:
		return "", false
	}
	t := base.Add(time.Duration(s.SolarOffsetMinutes) * time.Minute)
	return t.Format("15:04"), true
}
