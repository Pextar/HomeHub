package push

import (
	"testing"
	"time"
)

func at(h, m int) time.Time {
	return time.Date(2026, 1, 2, h, m, 0, 0, time.UTC)
}

func TestInQuietHours(t *testing.T) {
	cases := []struct {
		name       string
		now        time.Time
		start, end string
		want       bool
	}{
		{"same-day inside", at(13, 0), "09:00", "17:00", true},
		{"same-day before", at(8, 59), "09:00", "17:00", false},
		{"same-day at end is exclusive", at(17, 0), "09:00", "17:00", false},
		{"wrap inside late", at(23, 30), "22:00", "07:00", true},
		{"wrap inside early", at(6, 59), "22:00", "07:00", true},
		{"wrap outside", at(12, 0), "22:00", "07:00", false},
		{"wrap at start", at(22, 0), "22:00", "07:00", true},
		{"wrap at end exclusive", at(7, 0), "22:00", "07:00", false},
		{"empty bounds", at(12, 0), "", "", false},
		{"equal bounds", at(12, 0), "09:00", "09:00", false},
		{"malformed", at(12, 0), "9oclock", "17:00", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := inQuietHours(c.now, c.start, c.end); got != c.want {
				t.Errorf("inQuietHours(%v, %q, %q) = %v, want %v", c.now, c.start, c.end, got, c.want)
			}
		})
	}
}

func TestCategoryEnabled(t *testing.T) {
	u := UserPrefs{SensorAlerts: true, DeviceOffline: true}
	if !u.categoryEnabled(CategorySensorAlerts) {
		t.Error("sensor alerts should be enabled")
	}
	if u.categoryEnabled(CategoryStateChanges) {
		t.Error("state changes should be disabled")
	}
	if !u.categoryEnabled(CategoryDeviceOffline) {
		t.Error("device offline should be enabled")
	}
	if u.categoryEnabled("Bogus") {
		t.Error("unknown category should be disabled")
	}
}
