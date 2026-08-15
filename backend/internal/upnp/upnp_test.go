package upnp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// lanHost is a LAN-looking address the policy accepts. Traffic is redirected
// to the test server by the dialer below, so the address in every URL stays
// realistic while nothing leaves the process.
const lanHost = "10.0.0.9:49152"

// serveAs starts a test server and points the package client at it, returning
// the LAN-looking base URL to build control URLs from.
func serveAs(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().String()
	prev := client
	client = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}}
	t.Cleanup(func() { client = prev })
	return "http://" + lanHost
}

const description = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
    <friendlyName>RoPieee</friendlyName>
    <manufacturer>RoPieee</manufacturer>
    <modelName>UPnP Renderer</modelName>
    <UDN>uuid:4d696e69-444c-164e-9d41-b827ebaa1234</UDN>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
        <controlURL>/ctl/AVTransport</controlURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
        <controlURL>/ctl/RenderingControl</controlURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
        <controlURL>/ctl/ConnectionManager</controlURL>
      </service>
    </serviceList>
  </device>
</root>`

// soapEnvelope wraps a response body the way a renderer does.
func soapEnvelope(inner string) string {
	return `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>` +
		inner + `</s:Body></s:Envelope>`
}

// Control URLs are relative in the description and must be resolved against
// where the description itself was served. Getting this wrong produces control
// URLs that look right and address nothing.
func TestDescribeResolvesRelativeControlURLs(t *testing.T) {
	base := serveAs(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(description))
	})

	dev, err := Describe(context.Background(), base+"/desc.xml")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if dev.Name != "RoPieee" || dev.Model != "UPnP Renderer" {
		t.Errorf("device = %+v", dev)
	}
	if want := base + "/ctl/AVTransport"; dev.AVTransportURL != want {
		t.Errorf("AVTransport = %q, want %q", dev.AVTransportURL, want)
	}
	if want := base + "/ctl/RenderingControl"; dev.RenderingControlURL != want {
		t.Errorf("RenderingControl = %q, want %q", dev.RenderingControlURL, want)
	}
	if !strings.HasPrefix(dev.UDN, "uuid:") {
		t.Errorf("UDN = %q", dev.UDN)
	}
}

// A device with no AVTransport cannot play anything, and is refused at setup
// rather than registered and discovered as a failing tap.
func TestDescribeRefusesANonRenderer(t *testing.T) {
	base := serveAs(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<root xmlns="urn:schemas-upnp-org:device-1-0"><device>` +
			`<friendlyName>Some NAS</friendlyName><serviceList><service>` +
			`<serviceType>urn:schemas-upnp-org:service:ContentDirectory:1</serviceType>` +
			`<controlURL>/cd</controlURL></service></serviceList></device></root>`))
	})

	_, err := Describe(context.Background(), base+"/desc.xml")
	if err == nil || !strings.Contains(err.Error(), "media renderer") {
		t.Errorf("error = %v, want a refusal naming what's missing", err)
	}
}

// SetAVTransportURI is the call that makes this bridge worth having: the
// renderer is handed a URL and fetches for itself. The stream URL must arrive
// intact and the metadata alongside it.
func TestSetURISendsTheStreamURL(t *testing.T) {
	var action, body string
	base := serveAs(t, func(w http.ResponseWriter, r *http.Request) {
		action = r.Header.Get("SOAPACTION")
		buf := make([]byte, r.ContentLength)
		_, _ = io.ReadFull(r.Body, buf)
		body = string(buf)
		_, _ = w.Write([]byte(soapEnvelope(`<u:SetAVTransportURIResponse/>`)))
	})

	d := &Device{Name: "R", AVTransportURL: base + "/ctl"}
	stream := "http://10.0.0.2:8080/stream/abc"
	if err := SetURI(context.Background(), d, stream, DIDL("Kind of Blue", "Miles Davis", "audio/wav")); err != nil {
		t.Fatalf("set uri: %v", err)
	}
	if !strings.Contains(action, "SetAVTransportURI") {
		t.Errorf("SOAPACTION = %q", action)
	}
	if !strings.Contains(body, stream) {
		t.Errorf("the stream URL didn't reach the renderer: %s", body)
	}
	// The metadata is nested XML and must arrive escaped, or the renderer
	// parses the DIDL as part of the envelope and rejects the lot.
	if !strings.Contains(body, "&lt;DIDL-Lite") {
		t.Errorf("DIDL should be escaped inside the argument: %s", body)
	}
}

// A SOAP fault carries a UPnP error code, and it is far more useful than the
// HTTP status: 714 is "illegal MIME type" and 701 is "not in that state",
// which point at completely different fixes.
func TestSOAPFaultSurfacesTheUPnPCode(t *testing.T) {
	base := serveAs(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(soapEnvelope(`<s:Fault><detail><UPnPError>` +
			`<errorCode>714</errorCode></UPnPError></detail></s:Fault>`)))
	})

	d := &Device{Name: "R", AVTransportURL: base + "/ctl"}
	err := SetURI(context.Background(), d, "http://x/y", "")
	if err == nil || !strings.Contains(err.Error(), "714") {
		t.Errorf("error = %v, want the UPnP error code", err)
	}
}

func TestStateAndVolumeAreRead(t *testing.T) {
	base := serveAs(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.Header.Get("SOAPACTION"), "GetTransportInfo"):
			_, _ = w.Write([]byte(soapEnvelope(
				`<u:GetTransportInfoResponse><CurrentTransportState>PLAYING</CurrentTransportState>` +
					`</u:GetTransportInfoResponse>`)))
		case strings.Contains(r.Header.Get("SOAPACTION"), "GetVolume"):
			_, _ = w.Write([]byte(soapEnvelope(
				`<u:GetVolumeResponse><CurrentVolume>37</CurrentVolume></u:GetVolumeResponse>`)))
		default:
			_, _ = w.Write([]byte(soapEnvelope(`<ok/>`)))
		}
	})

	d := &Device{Name: "R", AVTransportURL: base + "/av", RenderingControlURL: base + "/rc"}
	st, err := State(context.Background(), d)
	if err != nil || st != StatePlaying {
		t.Errorf("state = %q, %v", st, err)
	}
	v, err := Volume(context.Background(), d)
	if err != nil || v != 37 {
		t.Errorf("volume = %d, %v", v, err)
	}
}

// Volume without a RenderingControl service is a missing control rather than a
// failure to report, and it says so instead of sending a call into nowhere.
func TestVolumeWithoutRenderingControl(t *testing.T) {
	d := &Device{Name: "Bare", AVTransportURL: "http://10.0.0.9/av"}
	if _, err := Volume(context.Background(), d); err == nil {
		t.Error("a renderer with no RenderingControl has no volume to read")
	}
	if err := SetVolume(context.Background(), d, 20); err == nil {
		t.Error("nor one to set")
	}
}

// Protocol info decides whether a renderer is worth serving losslessly, so the
// MIME extraction has to survive the format renderers actually publish.
func TestProtocolsFindsPCMAndWAV(t *testing.T) {
	base := serveAs(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(soapEnvelope(`<u:GetProtocolInfoResponse><Source></Source><Sink>` +
			`http-get:*:audio/mpeg:*,http-get:*:audio/L16;rate=44100;channels=2:*,` +
			`http-get:*:audio/x-wav:*,http-get:*:audio/flac:*` +
			`</Sink></u:GetProtocolInfoResponse>`)))
	})

	d := &Device{Name: "R", AVTransportURL: base + "/av", ConnectionMgrURL: base + "/cm"}
	p, err := Protocols(context.Background(), d)
	if err != nil {
		t.Fatalf("protocols: %v", err)
	}
	if !p.LinearPCM || !p.WAV || !p.FLAC {
		t.Errorf("protocols = %+v, want all three found", p)
	}
	if !p.PlaysPCM() {
		t.Error("a renderer listing WAV can be served losslessly")
	}
}

// A renderer that lists neither is reported as such rather than assumed fine —
// but only as advice, which is why nothing here refuses to play.
func TestProtocolsReportsALossyOnlyRenderer(t *testing.T) {
	base := serveAs(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(soapEnvelope(
			`<u:GetProtocolInfoResponse><Sink>http-get:*:audio/mpeg:*</Sink></u:GetProtocolInfoResponse>`)))
	})

	d := &Device{AVTransportURL: base + "/av", ConnectionMgrURL: base + "/cm"}
	p, err := Protocols(context.Background(), d)
	if err != nil {
		t.Fatalf("protocols: %v", err)
	}
	if p.PlaysPCM() {
		t.Error("an MP3-only renderer cannot be served losslessly")
	}
	if len(p.Raw) != 1 || p.Raw[0] != "audio/mpeg" {
		t.Errorf("raw = %v", p.Raw)
	}
}

// Control traffic stays on the LAN. A device description is fetched from the
// device, but its contents are the device's own claim about where to send
// commands — and a renderer naming an off-LAN host would otherwise make
// HomeHub a proxy for whatever it pointed at.
func TestControlURLsMustBeOnTheLAN(t *testing.T) {
	d := &Device{Name: "Evil", AVTransportURL: "http://198.51.100.7/ctl"}
	if err := Play(context.Background(), d); err == nil {
		t.Error("an off-LAN control URL must be refused")
	}
	if _, err := Describe(context.Background(), "http://198.51.100.7/desc.xml"); err == nil {
		t.Error("an off-LAN description URL must be refused")
	}
}

func TestDIDLEscapesAndCarriesTheTitle(t *testing.T) {
	got := DIDL(`Miles & "Trane"`, "Miles Davis", "audio/wav")
	if strings.Contains(got, `& "`) {
		t.Errorf("unescaped characters in DIDL: %s", got)
	}
	for _, want := range []string{"Miles Davis", "audio/wav", "musicTrack"} {
		if !strings.Contains(got, want) {
			t.Errorf("DIDL should contain %q: %s", want, got)
		}
	}
	// An empty title still has to produce a valid document — some renderers
	// refuse a URI whose metadata is blank.
	if !strings.Contains(DIDL("", "", ""), "HomeHub") {
		t.Error("a title-less item should fall back to a name")
	}
}
