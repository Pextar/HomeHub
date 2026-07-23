package sonos

import (
	"testing"
)

func TestValidateHost(t *testing.T) {
	valid := []string{"192.168.1.50", "10.0.0.7", "sonos-living.local", "Speaker-1"}
	for _, h := range valid {
		if err := ValidateHost(h); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", h, err)
		}
	}
	invalid := []string{
		"", " ", "127.0.0.1", "169.254.169.254", "0.0.0.0", "224.0.0.1",
		"192.168.1.50:1400", // no ports — Sonos is always :1400
		"host/path", "a?b", "a#b", "u@h", "host name", "evil\\host",
	}
	for _, h := range invalid {
		if err := ValidateHost(h); err == nil {
			t.Errorf("ValidateHost(%q) = nil, want error", h)
		}
	}
}

func TestExtractTag(t *testing.T) {
	body := `<s:Envelope><s:Body><u:GetTransportInfoResponse>` +
		`<CurrentTransportState>PLAYING</CurrentTransportState>` +
		`<Escaped>a &amp; b</Escaped>` +
		`</u:GetTransportInfoResponse></s:Body></s:Envelope>`
	if got := extractTag(body, "CurrentTransportState"); got != "PLAYING" {
		t.Errorf("extractTag transport = %q, want PLAYING", got)
	}
	if got := extractTag(body, "Escaped"); got != "a & b" {
		t.Errorf("extractTag escaped = %q, want %q", got, "a & b")
	}
	if got := extractTag(body, "Missing"); got != "" {
		t.Errorf("extractTag missing = %q, want empty", got)
	}
}

func TestParseDescription(t *testing.T) {
	body := `<?xml version="1.0"?><root><device>` +
		`<deviceType>urn:schemas-upnp-org:device:ZonePlayer:1</deviceType>` +
		`<roomName>Living Room</roomName>` +
		`<modelName>Sonos One</modelName>` +
		`<UDN>uuid:RINCON_949F3EC2E15A01400</UDN>` +
		`</device></root>`
	d := ParseDescription(body)
	if d.UUID != "RINCON_949F3EC2E15A01400" {
		t.Errorf("UUID = %q", d.UUID)
	}
	if d.Room != "Living Room" || d.Model != "Sonos One" {
		t.Errorf("Room/Model = %q/%q", d.Room, d.Model)
	}
	// A non-Sonos UPnP device must not pass for a speaker.
	if d := ParseDescription(`<root><device><UDN>uuid:abc-123</UDN></device></root>`); d.UUID != "" {
		t.Errorf("non-Sonos UDN accepted: %q", d.UUID)
	}
}

func TestParseTrackMeta(t *testing.T) {
	meta := `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" ` +
		`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">` +
		`<item id="-1" parentID="-1">` +
		`<res protocolInfo="sonos.com-spotify:*:audio/x-spotify:*">x-sonos-spotify:track</res>` +
		`<upnp:albumArtURI>/getaa?s=1&amp;u=x-sonos-spotify</upnp:albumArtURI>` +
		`<dc:title>Karma Police</dc:title>` +
		`<dc:creator>Radiohead</dc:creator>` +
		`<upnp:album>OK Computer</upnp:album>` +
		`</item></DIDL-Lite>`
	tr := ParseTrackMeta(meta)
	if tr == nil {
		t.Fatal("ParseTrackMeta returned nil")
	}
	if tr.Title != "Karma Police" || tr.Artist != "Radiohead" || tr.Album != "OK Computer" {
		t.Errorf("track = %+v", tr)
	}
	if tr.ArtURI != "/getaa?s=1&u=x-sonos-spotify" {
		t.Errorf("art = %q", tr.ArtURI)
	}
	if ParseTrackMeta("not xml") != nil {
		t.Error("garbage metadata should return nil")
	}
	if ParseTrackMeta(`<DIDL-Lite><item id="-1"></item></DIDL-Lite>`) != nil {
		t.Error("empty item should return nil")
	}
}

func TestParseZoneGroupState(t *testing.T) {
	state := `<ZoneGroupState><ZoneGroups>` +
		`<ZoneGroup Coordinator="RINCON_AAA" ID="RINCON_AAA:42">` +
		`<ZoneGroupMember UUID="RINCON_AAA" Location="http://192.168.1.50:1400/xml/device_description.xml" ZoneName="Living Room"/>` +
		`<ZoneGroupMember UUID="RINCON_BBB" Location="http://192.168.1.51:1400/xml/device_description.xml" ZoneName="Kitchen"/>` +
		`<ZoneGroupMember UUID="RINCON_SUB" Location="http://192.168.1.52:1400/xml/device_description.xml" ZoneName="Living Room" Invisible="1"/>` +
		`</ZoneGroup>` +
		`<ZoneGroup Coordinator="RINCON_CCC" ID="RINCON_CCC:12">` +
		`<ZoneGroupMember UUID="RINCON_CCC" Location="http://192.168.1.53:1400/xml/device_description.xml" ZoneName="Bedroom"/>` +
		`</ZoneGroup>` +
		`</ZoneGroups></ZoneGroupState>`
	groups, err := ParseZoneGroupState(state)
	if err != nil {
		t.Fatalf("ParseZoneGroupState: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	g := groups[0]
	if g.CoordinatorUUID != "RINCON_AAA" {
		t.Errorf("coordinator = %q", g.CoordinatorUUID)
	}
	// The invisible Sub must be dropped.
	if len(g.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(g.Members))
	}
	if g.Members[1].IP != "192.168.1.51" || g.Members[1].Name != "Kitchen" {
		t.Errorf("member[1] = %+v", g.Members[1])
	}
	if _, err := ParseZoneGroupState("<nope/>"); err == nil {
		t.Error("missing ZoneGroups should error")
	}
}

func TestParseFavorites(t *testing.T) {
	result := `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" ` +
		`xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" ` +
		`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">` +
		`<item id="FV:2/13" parentID="FV:2" restricted="false">` +
		`<dc:title>P3 Radio</dc:title>` +
		`<upnp:albumArtURI>https://cdn.example/p3.png</upnp:albumArtURI>` +
		`<r:description>TuneIn</r:description>` +
		`<res protocolInfo="x-sonosapi-stream:*:*:*">x-sonosapi-stream:s24860?sid=254</res>` +
		`<r:resMD>&lt;DIDL-Lite&gt;…&lt;/DIDL-Lite&gt;</r:resMD>` +
		`</item>` +
		`<item id="FV:2/14" parentID="FV:2" restricted="false">` +
		`<dc:title>Broken (no res)</dc:title>` +
		`</item>` +
		`</DIDL-Lite>`
	favs, err := ParseFavorites(result)
	if err != nil {
		t.Fatalf("ParseFavorites: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("favorites = %d, want 1 (res-less item dropped)", len(favs))
	}
	f := favs[0]
	if f.Title != "P3 Radio" || f.Service != "TuneIn" {
		t.Errorf("favorite = %+v", f)
	}
	if f.URI != "x-sonosapi-stream:s24860?sid=254" {
		t.Errorf("uri = %q", f.URI)
	}
	if f.Metadata != "<DIDL-Lite>…</DIDL-Lite>" {
		t.Errorf("metadata = %q", f.Metadata)
	}
}

func TestIsContainerURI(t *testing.T) {
	if !isContainerURI("x-rincon-cpcontainer:1006206ccatalog") {
		t.Error("cpcontainer should be a container")
	}
	if isContainerURI("x-sonosapi-stream:s24860?sid=254") {
		t.Error("radio stream is not a container")
	}
}

func TestParseSSDPLocation(t *testing.T) {
	resp := "HTTP/1.1 200 OK\r\n" +
		"CACHE-CONTROL: max-age = 1800\r\n" +
		"LOCATION: http://192.168.1.50:1400/xml/device_description.xml\r\n" +
		"ST: urn:schemas-upnp-org:device:ZonePlayer:1\r\n\r\n"
	if got := parseSSDPLocation(resp); got != "192.168.1.50" {
		t.Errorf("location ip = %q", got)
	}
	// A malicious responder pointing at loopback must be rejected.
	evil := "HTTP/1.1 200 OK\r\nLOCATION: http://127.0.0.1:1400/x\r\n\r\n"
	if got := parseSSDPLocation(evil); got != "" {
		t.Errorf("loopback location accepted: %q", got)
	}
	if got := parseSSDPLocation("HTTP/1.1 200 OK\r\n\r\n"); got != "" {
		t.Errorf("missing location accepted: %q", got)
	}
}

func TestNormalizeClock(t *testing.T) {
	if normalizeClock("NOT_IMPLEMENTED") != "" || normalizeClock("0:00:00") != "" {
		t.Error("placeholders should normalize to empty")
	}
	if normalizeClock("0:03:12") != "0:03:12" {
		t.Error("real times should pass through")
	}
}
