package sonos

import "testing"

func TestParseZoneInfo(t *testing.T) {
	body := `<?xml version="1.0"?>
<s:Envelope><s:Body><u:GetZoneInfoResponse>
<SerialNumber>00-0E-58-AA-BB-CC:9</SerialNumber>
<SoftwareVersion>81.1-56110</SoftwareVersion>
<DisplaySoftwareVersion>16.1</DisplaySoftwareVersion>
<HardwareVersion>1.20.1.6-1.1</HardwareVersion>
<MACAddress>00:0E:58:AA:BB:CC</MACAddress>
</u:GetZoneInfoResponse></s:Body></s:Envelope>`

	info := ParseZoneInfo(body)
	if info.SerialNumber != "00-0E-58-AA-BB-CC:9" {
		t.Errorf("serial = %q", info.SerialNumber)
	}
	// The user-facing version wins over the internal build string.
	if info.SoftwareVersion != "16.1" {
		t.Errorf("software version = %q, want the display version", info.SoftwareVersion)
	}
	if info.HardwareVersion != "1.20.1.6-1.1" {
		t.Errorf("hardware version = %q", info.HardwareVersion)
	}
	if info.MACAddress != "00:0E:58:AA:BB:CC" {
		t.Errorf("mac = %q", info.MACAddress)
	}
}

func TestParseZoneInfoFallsBackToBuildVersion(t *testing.T) {
	// Older firmware doesn't send DisplaySoftwareVersion at all.
	info := ParseZoneInfo(`<SoftwareVersion>34.7-34220</SoftwareVersion>`)
	if info.SoftwareVersion != "34.7-34220" {
		t.Errorf("software version = %q, want the build string", info.SoftwareVersion)
	}
}

const descriptionDoc = `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
<specVersion><major>1</major><minor>0</minor></specVersion>
<device>
<deviceType>urn:schemas-upnp-org:device:ZonePlayer:1</deviceType>
<friendlyName>192.168.1.50 - Sonos One</friendlyName>
<manufacturer>Sonos, Inc.</manufacturer>
<modelNumber>S13</modelNumber>
<modelName>Sonos One</modelName>
<displayName>One</displayName>
<UDN>uuid:RINCON_347E5CAABBCC01400</UDN>
<roomName>Kitchen</roomName>
<iconList>
<icon><id>0</id><mimetype>image/png</mimetype><width>48</width><height>48</height><depth>24</depth><url>/img/icon-S13.png</url></icon>
<icon><id>1</id><mimetype>image/png</mimetype><width>120</width><height>120</height><depth>24</depth><url>/img/icon-S13-large.png</url></icon>
</iconList>
<deviceList>
<device>
<deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
<iconList><icon><width>16</width><height>16</height><url>/img/renderer.png</url></icon></iconList>
</device>
</deviceList>
</device>
</root>`

func TestParseDeviceInfo(t *testing.T) {
	info := ParseDeviceInfo(descriptionDoc)

	if info.UUID != "RINCON_347E5CAABBCC01400" {
		t.Errorf("uuid = %q", info.UUID)
	}
	if info.Room != "Kitchen" {
		t.Errorf("room = %q", info.Room)
	}
	if info.Model != "Sonos One" {
		t.Errorf("model = %q", info.Model)
	}
	if info.ModelNumber != "S13" {
		t.Errorf("model number = %q", info.ModelNumber)
	}
	if info.DisplayName != "One" {
		t.Errorf("display name = %q", info.DisplayName)
	}
	// Largest icon from the *root* device — never the sub-device's UPnP glyph.
	if info.IconPath != "/img/icon-S13-large.png" {
		t.Errorf("icon path = %q, want the largest root icon", info.IconPath)
	}
}

func TestParseDeviceInfoNoIcons(t *testing.T) {
	info := ParseDeviceInfo(`<root><device><UDN>uuid:RINCON_1</UDN><roomName>Den</roomName></device></root>`)
	if info.IconPath != "" {
		t.Errorf("icon path = %q, want empty when the description lists none", info.IconPath)
	}
}

func TestParseDeviceInfoRejectsUnsafeIconPaths(t *testing.T) {
	// The icon path comes off the network and ends up in a URL we fetch, so
	// anything absolute or traversing must not survive parsing.
	for _, url := range []string{
		"http://evil.example/x.png",
		"//evil.example/x.png",
		"/img/../../etc/passwd",
		"img/icon.png",
	} {
		doc := `<root><device><UDN>uuid:RINCON_1</UDN><iconList><icon><width>48</width><url>` +
			url + `</url></icon></iconList></device></root>`
		if got := ParseDeviceInfo(doc).IconPath; got != "" {
			t.Errorf("icon path for %q = %q, want it dropped", url, got)
		}
	}
}

func TestMinutesToClock(t *testing.T) {
	cases := map[int]string{
		0:   "",
		1:   "00:01:00",
		30:  "00:30:00",
		60:  "01:00:00",
		95:  "01:35:00",
		600: "10:00:00",
	}
	for mins, want := range cases {
		if got := minutesToClock(mins); got != want {
			t.Errorf("minutesToClock(%d) = %q, want %q", mins, got, want)
		}
	}
}

func TestClockToMinutes(t *testing.T) {
	cases := map[string]int{
		"":                0,
		"NOT_IMPLEMENTED": 0,
		"00:00:00":        0,
		"0:00:00":         0,
		"00:00:30":        1, // rounds up — 30s left is not "no timer"
		"00:29:53":        30,
		"0:30:00":         30,
		"01:00:00":        60,
		"garbage":         0,
		"1:2":             0,
	}
	for clock, want := range cases {
		if got := clockToMinutes(clock); got != want {
			t.Errorf("clockToMinutes(%q) = %d, want %d", clock, got, want)
		}
	}
}

func TestSetSleepTimerRejectsOutOfRange(t *testing.T) {
	// Validated before any network call, so a bad value never reaches a speaker.
	if err := SetSleepTimer(t.Context(), "192.168.1.50", -1); err == nil {
		t.Error("negative minutes accepted")
	}
	if err := SetSleepTimer(t.Context(), "192.168.1.50", MaxSleepMinutes+1); err == nil {
		t.Error("over-long timer accepted")
	}
}
