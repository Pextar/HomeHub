package store

import (
	"testing"
	"time"
)

func TestEffectiveHHMM_Fixed(t *testing.T) {
	s := &Schedule{TimeMode: ModeFixed, Time: "07:30"}
	got, ok := s.EffectiveHHMM(time.Now(), &Settings{})
	if !ok || got != "07:30" {
		t.Errorf("got (%q, %v), want (\"07:30\", true)", got, ok)
	}
}

func TestEffectiveHHMM_EmptyModeTreatedAsFixed(t *testing.T) {
	s := &Schedule{Time: "12:34"}
	got, ok := s.EffectiveHHMM(time.Now(), &Settings{})
	if !ok || got != "12:34" {
		t.Errorf("got (%q, %v), want (\"12:34\", true)", got, ok)
	}
}

func TestEffectiveHHMM_SunriseRequiresLocation(t *testing.T) {
	s := &Schedule{TimeMode: ModeSunrise}
	if _, ok := s.EffectiveHHMM(time.Now(), &Settings{}); ok {
		t.Errorf("expected ok=false when location not configured")
	}
}

func TestEffectiveHHMM_SunriseWithOffset(t *testing.T) {
	utc := time.Date(2025, 6, 21, 12, 0, 0, 0, time.UTC)
	settings := &Settings{Latitude: 0, Longitude: 0.0001} // tiny lon so HasLocation==true
	plain, ok := (&Schedule{TimeMode: ModeSunrise}).EffectiveHHMM(utc, settings)
	if !ok {
		t.Fatal("expected sunrise to resolve")
	}
	withOffset, ok := (&Schedule{TimeMode: ModeSunrise, SolarOffsetMinutes: 30}).EffectiveHHMM(utc, settings)
	if !ok {
		t.Fatal("expected offset sunrise to resolve")
	}
	plainT, _ := time.Parse("15:04", plain)
	offT, _ := time.Parse("15:04", withOffset)
	if d := offT.Sub(plainT); d != 30*time.Minute {
		t.Errorf("offset HH:MM diff = %v, want 30m", d)
	}
}

func TestEffectiveHHMM_SunsetWithNegativeOffset(t *testing.T) {
	utc := time.Date(2025, 6, 21, 12, 0, 0, 0, time.UTC)
	settings := &Settings{Latitude: 0, Longitude: 0.0001}
	plain, ok := (&Schedule{TimeMode: ModeSunset}).EffectiveHHMM(utc, settings)
	if !ok {
		t.Fatal("expected sunset to resolve")
	}
	earlier, ok := (&Schedule{TimeMode: ModeSunset, SolarOffsetMinutes: -45}).EffectiveHHMM(utc, settings)
	if !ok {
		t.Fatal("expected offset sunset to resolve")
	}
	plainT, _ := time.Parse("15:04", plain)
	earlT, _ := time.Parse("15:04", earlier)
	if d := plainT.Sub(earlT); d != 45*time.Minute {
		t.Errorf("expected earlier by 45m, got %v", d)
	}
}

func TestEffectiveHHMM_UnknownMode(t *testing.T) {
	s := &Schedule{TimeMode: "weird"}
	if _, ok := s.EffectiveHHMM(time.Now(), &Settings{Latitude: 10}); ok {
		t.Errorf("expected ok=false for unknown mode")
	}
}
