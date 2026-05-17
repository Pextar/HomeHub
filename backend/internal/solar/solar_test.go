package solar

import (
	"math"
	"testing"
	"time"
)

// Equinox: day length should be close to 12 hours (slightly longer
// because of atmospheric refraction). Around 12h00 ± 15 min at any
// non-polar latitude is the textbook expectation.
func TestEquinoxDayLength(t *testing.T) {
	utc := time.Date(2025, 3, 20, 12, 0, 0, 0, time.UTC)
	for _, lat := range []float64{-45, -10, 0, 10, 45, 59.33} {
		rise, set, ok := Times(utc, lat, 0)
		if !ok {
			t.Fatalf("lat=%v: expected sunrise/sunset, got ok=false", lat)
		}
		day := set.Sub(rise).Minutes()
		if day < 12*60-30 || day > 12*60+30 {
			t.Errorf("lat=%v: equinox day length %.1f min, want ~720", lat, day)
		}
	}
}

// Sunrise + (sunset - sunrise)/2 should equal solar noon, which for an
// observer at longitude L is at (12:00 UTC - 4L min - eqtime). With
// eqtime ~ -7.5 min on March 20 and L=0, solar noon should be at
// roughly 12:07 UTC.
func TestEquinoxSolarNoonAtPrimeMeridian(t *testing.T) {
	utc := time.Date(2025, 3, 20, 12, 0, 0, 0, time.UTC)
	rise, set, ok := Times(utc, 0, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	midpoint := rise.Add(set.Sub(rise) / 2).UTC()
	expected := time.Date(2025, 3, 20, 12, 7, 30, 0, time.UTC)
	if d := math.Abs(midpoint.Sub(expected).Minutes()); d > 3 {
		t.Errorf("solar noon %s, want ~%s (off by %.1f min)",
			midpoint.Format("15:04:05"), expected.Format("15:04:05"), d)
	}
}

// Polar night: Svalbard in late December — sun never rises.
func TestPolarNight(t *testing.T) {
	utc := time.Date(2025, 12, 21, 12, 0, 0, 0, time.UTC)
	if _, _, ok := Times(utc, 78.0, 16.0); ok {
		t.Fatal("expected polar night (ok=false)")
	}
}

// Polar day: Svalbard at midsummer — sun never sets.
func TestPolarDay(t *testing.T) {
	utc := time.Date(2025, 6, 21, 12, 0, 0, 0, time.UTC)
	if _, _, ok := Times(utc, 78.0, 16.0); ok {
		t.Fatal("expected polar day (ok=false)")
	}
}

// Output should be expressed in the input time zone so the caller can
// extract HH:MM directly.
func TestTimesInInputTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata for America/New_York not available")
	}
	when := time.Date(2025, 6, 21, 12, 0, 0, 0, loc)
	rise, set, ok := Times(when, 40.7128, -74.0060)
	if !ok {
		t.Fatal("expected ok")
	}
	if rise.Location().String() != loc.String() {
		t.Errorf("sunrise zone = %s, want %s", rise.Location(), loc)
	}
	if set.Location().String() != loc.String() {
		t.Errorf("sunset zone = %s, want %s", set.Location(), loc)
	}
	// NYC midsummer sunrise is around 05:25 EDT; allow a few minutes of
	// drift in the approximate algorithm.
	if h := rise.Hour(); h < 5 || h > 6 {
		t.Errorf("NYC midsummer sunrise hour = %d, want 5 or 6", h)
	}
	if h := set.Hour(); h < 20 || h > 21 {
		t.Errorf("NYC midsummer sunset hour = %d, want 20 or 21", h)
	}
}
