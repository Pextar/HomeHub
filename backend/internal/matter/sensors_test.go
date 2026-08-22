package matter

import "testing"

func TestSensorField(t *testing.T) {
	cases := []struct {
		name  string
		field string
		kind  string
		want  string
	}{
		{"explicit humidity wins", "humidity", "temperature", "humidity"},
		{"explicit temperature wins", "temperature", "humidity", "temperature"},
		{"empty field falls back to humidity kind", "", "humidity", "humidity"},
		{"empty field falls back to temperature kind", "", "temperature", "temperature"},
		{"unknown kind defaults to temperature", "", "custom", "temperature"},
		{"unrecognised field falls back to kind", "temperature_C", "humidity", "humidity"},
		{"case-insensitive", "Humidity", "custom", "humidity"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sensorField(c.field, c.kind); got != c.want {
				t.Errorf("sensorField(%q, %q) = %q, want %q", c.field, c.kind, got, c.want)
			}
		})
	}
}
