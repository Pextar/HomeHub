package sonos

import (
	"bytes"
	"context"
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

// notifyBody wraps an inner document the way a speaker does: one property
// carrying an escaped XML document as its text.
func notifyBody(prop, inner string) string {
	var esc bytes.Buffer
	_ = xml.EscapeText(&esc, []byte(inner))
	return `<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">` +
		`<e:property><` + prop + `>` + esc.String() + `</` + prop + `></e:property>` +
		`</e:propertyset>`
}

func TestParsePropertySet(t *testing.T) {
	body := notifyBody("LastChange", `<Event><InstanceID val="0"/></Event>`)
	got := ParsePropertySet(body)
	want := `<Event><InstanceID val="0"/></Event>`
	if got["LastChange"] != want {
		t.Errorf("LastChange = %q, want %q", got["LastChange"], want)
	}
}

func TestParsePropertySetMultipleProperties(t *testing.T) {
	body := `<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">` +
		`<e:property><ZoneGroupState>topology</ZoneGroupState></e:property>` +
		`<e:property><ThirdPartyMediaServersX>junk</ThirdPartyMediaServersX></e:property>` +
		`</e:propertyset>`
	got := ParsePropertySet(body)
	if got["ZoneGroupState"] != "topology" {
		t.Errorf("ZoneGroupState = %q, want %q", got["ZoneGroupState"], "topology")
	}
	if len(got) != 2 {
		t.Errorf("got %d properties, want 2: %v", len(got), got)
	}
}

// A long document arrives as several CharData chunks; the parser must
// reassemble it rather than keeping only the last piece.
func TestParsePropertySetLongValueIsWhole(t *testing.T) {
	inner := "<Event>" + strings.Repeat("<Pad val=\"x\"/>", 4000) + "</Event>"
	got := ParsePropertySet(notifyBody("LastChange", inner))
	if got["LastChange"] != inner {
		t.Errorf("long value truncated: got %d bytes, want %d", len(got["LastChange"]), len(inner))
	}
}

func TestParsePropertySetGarbage(t *testing.T) {
	if got := ParsePropertySet("not xml at all"); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestParseLastChangeTransport(t *testing.T) {
	doc := `<Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/">` +
		`<InstanceID val="0">` +
		`<TransportState val="PLAYING"/>` +
		`<CurrentPlayMode val="SHUFFLE_NOREPEAT"/>` +
		`<NumberOfTracks val="12"/>` +
		`<CurrentTrack val="3"/>` +
		`<CurrentTrackDuration val="0:03:21"/>` +
		`<CurrentCrossfadeMode val="1"/>` +
		`</InstanceID></Event>`
	got := ParseLastChange(doc)
	for k, want := range map[string]string{
		"TransportState":       "PLAYING",
		"CurrentPlayMode":      "SHUFFLE_NOREPEAT",
		"NumberOfTracks":       "12",
		"CurrentTrack":         "3",
		"CurrentTrackDuration": "0:03:21",
		"CurrentCrossfadeMode": "1",
	} {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
}

// RenderingControl reports the same variable once per channel. Only Master
// is the one the bridge reads and sets, so the others must not overwrite it.
func TestParseLastChangeKeepsOnlyMasterChannel(t *testing.T) {
	doc := `<Event xmlns="urn:schemas-upnp-org:metadata-1-0/RCS/">` +
		`<InstanceID val="0">` +
		`<Volume channel="Master" val="35"/>` +
		`<Volume channel="LF" val="100"/>` +
		`<Volume channel="RF" val="100"/>` +
		`<Mute channel="Master" val="0"/>` +
		`<Mute channel="LF" val="1"/>` +
		`</InstanceID></Event>`
	got := ParseLastChange(doc)
	if got["Volume"] != "35" {
		t.Errorf("Volume = %q, want 35 (the Master channel)", got["Volume"])
	}
	if got["Mute"] != "0" {
		t.Errorf("Mute = %q, want 0 (the Master channel)", got["Mute"])
	}
}

// Sonos only ever uses instance 0, but the format allows more; anything else
// must not leak into the flattened values.
func TestParseLastChangeIgnoresOtherInstances(t *testing.T) {
	doc := `<Event><InstanceID val="1"><TransportState val="STOPPED"/></InstanceID>` +
		`<InstanceID val="0"><TransportState val="PLAYING"/></InstanceID></Event>`
	if got := ParseLastChange(doc)["TransportState"]; got != "PLAYING" {
		t.Errorf("TransportState = %q, want PLAYING", got)
	}
}

func TestParseLastChangeGarbage(t *testing.T) {
	if got := ParseLastChange("<<<"); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
	if got := ParseLastChange(""); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestParseTimeoutHeader(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"Second-1800", 1800 * time.Second},
		{"Second-86400", 86400 * time.Second},
		{" Second-300 ", 300 * time.Second},
		// Below the floor, unparseable, or the legal-but-useless "infinite":
		// all fall back so the renewal loop always has a finite interval.
		{"Second-5", minGrantedTimeout},
		{"infinite", minGrantedTimeout},
		{"", minGrantedTimeout},
		{"Second-abc", minGrantedTimeout},
		{"Second--10", minGrantedTimeout},
	}
	for _, tt := range tests {
		if got := parseTimeoutHeader(tt.in); got != tt.want {
			t.Errorf("parseTimeoutHeader(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestTimeoutHeader(t *testing.T) {
	if got := timeoutHeader(30 * time.Minute); got != "Second-1800" {
		t.Errorf("timeoutHeader(30m) = %q, want Second-1800", got)
	}
}

// The callback goes into a header verbatim, so anything that could break the
// header — or point the speaker somewhere else — has to be refused.
func TestValidCallback(t *testing.T) {
	bad := []string{
		"",
		"https://192.168.1.10:8080/sonos/event", // speakers won't do TLS
		"http://192.168.1.10:8080/x\r\nX-Evil: 1",
		"http://192.168.1.10:8080/x\nX-Evil: 1",
		"http://192.168.1.10:8080/a b",
		"http://192.168.1.10:8080/<x>",
		"ftp://192.168.1.10/x",
	}
	for _, c := range bad {
		if err := validCallback(c); err == nil {
			t.Errorf("validCallback(%q) = nil, want an error", c)
		}
	}
	if err := validCallback("http://192.168.1.10:8080/sonos/event/abc123"); err != nil {
		t.Errorf("validCallback(good) = %v, want nil", err)
	}
}

func TestValidSID(t *testing.T) {
	for _, s := range []string{"", "uuid:x\r\ny", "uuid: x"} {
		if err := validSID(s); err == nil {
			t.Errorf("validSID(%q) = nil, want an error", s)
		}
	}
	if err := validSID("uuid:RINCON_1234567890"); err != nil {
		t.Errorf("validSID(good) = %v, want nil", err)
	}
}

// Subscribe must refuse before touching the network when its inputs are
// unusable — the same posture ValidateHost gives the SOAP path.
func TestSubscribeRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	good := "http://192.168.1.10:8080/sonos/event/tok"

	if _, _, err := Subscribe(ctx, "192.168.1.42", EventTransport, "https://nope/x", SubscribeTimeout); err == nil {
		t.Error("Subscribe with an https callback = nil, want an error")
	}
	if _, _, err := Subscribe(ctx, "127.0.0.1", EventTransport, good, SubscribeTimeout); err == nil {
		t.Error("Subscribe to loopback = nil, want an error")
	}
	if _, _, err := Subscribe(ctx, "192.168.1.42/../x", EventTransport, good, SubscribeTimeout); err == nil {
		t.Error("Subscribe to a malformed host = nil, want an error")
	}
	// ContentDirectory has no event path; asking to subscribe to it is a
	// programming error, not a network one.
	notEvented := EventService{Key: "content", svc: contentDirectory}
	if _, _, err := Subscribe(ctx, "192.168.1.42", notEvented, good, SubscribeTimeout); err == nil {
		t.Error("Subscribe to a non-evented service = nil, want an error")
	}
}

func TestRenewAndUnsubscribeRejectBadSID(t *testing.T) {
	ctx := context.Background()
	if _, err := Renew(ctx, "192.168.1.42", EventTransport, "bad\r\nsid", SubscribeTimeout); err == nil {
		t.Error("Renew with a header-breaking SID = nil, want an error")
	}
	if err := Unsubscribe(ctx, "192.168.1.42", EventTransport, ""); err == nil {
		t.Error("Unsubscribe with an empty SID = nil, want an error")
	}
}
