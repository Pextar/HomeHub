package sonos

import "testing"

const roamBattery = `<?xml version="1.0" ?>
<ZPSupportInfo>
<LocalBatteryStatus>
<Data name="Health">GREEN</Data>
<Data name="Level">84</Data>
<Data name="Temperature">NORMAL</Data>
<Data name="PowerSource">BATTERY</Data>
</LocalBatteryStatus>
</ZPSupportInfo>`

func TestParseBattery(t *testing.T) {
	b := ParseBattery(roamBattery)
	if b == nil {
		t.Fatal("ParseBattery returned nil for a speaker that reported one")
	}
	if b.Level != 84 {
		t.Errorf("level = %d, want 84", b.Level)
	}
	if b.Charging {
		t.Error("charging = true although the power source is the battery")
	}
	if b.Health != "GREEN" || b.Temperature != "NORMAL" {
		t.Errorf("health/temperature = %q/%q", b.Health, b.Temperature)
	}
	if !b.OK() {
		t.Error("OK = false for a green battery at a normal temperature")
	}
}

// The speaker names more supplies than the UI needs to distinguish. What
// anyone acts on is "is it draining", so everything that isn't the battery
// itself reads as charging.
func TestParseBatteryChargingSources(t *testing.T) {
	for _, src := range []string{"SONOS_CHARGING_RING", "USB_POWER", "DOCK"} {
		body := `<ZPSupportInfo><LocalBatteryStatus>` +
			`<Data name="Level">100</Data>` +
			`<Data name="PowerSource">` + src + `</Data>` +
			`</LocalBatteryStatus></ZPSupportInfo>`
		if b := ParseBattery(body); b == nil || !b.Charging {
			t.Errorf("%s: charging = false, want true", src)
		}
	}
}

// A mains speaker has nothing to say here, and neither does a malformed
// answer. Both must come back nil rather than as a battery at 0% — "no
// battery" and "flat battery" are different things and the UI shows them
// differently.
func TestParseBatteryAbsent(t *testing.T) {
	for name, body := range map[string]string{
		"empty document":  ``,
		"not xml":         `404 not found`,
		"no local block":  `<ZPSupportInfo></ZPSupportInfo>`,
		"block, no level": `<ZPSupportInfo><LocalBatteryStatus><Data name="Health">GREEN</Data></LocalBatteryStatus></ZPSupportInfo>`,
		"unparseable level": `<ZPSupportInfo><LocalBatteryStatus>` +
			`<Data name="Level">NOT_IMPLEMENTED</Data></LocalBatteryStatus></ZPSupportInfo>`,
	} {
		if b := ParseBattery(body); b != nil {
			t.Errorf("%s: got %+v, want nil", name, b)
		}
	}
}

// An unhappy battery is the reason someone opens this screen, so the two
// fields that can complain are the ones OK() reads.
func TestBatteryOK(t *testing.T) {
	cases := []struct {
		name   string
		bat    *Battery
		wantOK bool
	}{
		{"no battery at all", nil, true},
		{"said nothing", &Battery{Level: 50}, true},
		{"green and normal", &Battery{Health: "GREEN", Temperature: "NORMAL"}, true},
		{"poor health", &Battery{Health: "RED", Temperature: "NORMAL"}, false},
		{"too hot", &Battery{Health: "GREEN", Temperature: "HIGH"}, false},
	}
	for _, tc := range cases {
		if got := tc.bat.OK(); got != tc.wantOK {
			t.Errorf("%s: OK = %v, want %v", tc.name, got, tc.wantOK)
		}
	}
}

// Sonos has no reason to send a level outside 0–100, but a battery drawn as
// a bar 112% wide is a worse failure than a clamp.
func TestParseBatteryClampsLevel(t *testing.T) {
	body := `<ZPSupportInfo><LocalBatteryStatus><Data name="Level">112</Data></LocalBatteryStatus></ZPSupportInfo>`
	if b := ParseBattery(body); b == nil || b.Level != 100 {
		t.Errorf("level = %+v, want 100", b)
	}
}
