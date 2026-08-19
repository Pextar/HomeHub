// Package upnp drives a generic UPnP/DLNA MediaRenderer.
//
// This is the bridge that lets HomeHub reach a renderer it knows nothing else
// about, and the reason it exists is worth stating plainly, because the obvious
// question is why a house that already speaks Sonos and AirPlay needs a third
// way to talk to a box.
//
// AirPlay 1 carries 44.1 kHz 16-bit stereo and nothing else. That is the
// protocol, not a HomeHub choice, and it means a receiver reached over AirPlay
// can never be sent hi-res audio however good the box is. Plenty of them are
// much better than that: a Raspberry Pi running RoPieeeXL will happily play
// 24-bit/192 kHz — over Roon's RAAT, which is closed, or over UPnP, which is
// not. So the way to get a lossless master to such a device without reducing it
// is to stop pushing and let it fetch: point it at HomeHub's stream URL with
// SetAVTransportURI and it pulls the audio itself, at whatever rate and word
// length the WAV header declares.
//
// Two things follow that shape everything below.
//
// First, unlike Sonos there is no fixed port and no fixed path. A Sonos speaker
// is always :1400 with known service paths; a generic renderer publishes a
// device description at an arbitrary URL, and the control URLs for its services
// are inside it. So every device is described once and the URLs are remembered.
//
// Second, this package assumes nothing about what the renderer can play. What
// it accepts is advertised in its ConnectionManager's protocol info, and a
// renderer that does not list linear PCM is one HomeHub cannot serve
// losslessly — better found at setup than discovered as silence.
package upnp

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"homehub/internal/lanhost"
)

// ValidateHost keeps control traffic on the LAN, the same policy every other
// device package here applies.
func ValidateHost(host string) error {
	return lanhost.Policy{Noun: "renderer address", AllowPort: true}.Validate(host)
}

// Service URNs. Only these three matter: transport control, volume, and the
// one that says what the device can play.
const (
	AVTransport       = "urn:schemas-upnp-org:service:AVTransport:1"
	RenderingControl  = "urn:schemas-upnp-org:service:RenderingControl:1"
	ConnectionManager = "urn:schemas-upnp-org:service:ConnectionManager:1"
)

// timeout caps a single control call. Renderers are on the LAN and answer
// quickly; a hung one should fail a tap rather than hold a room.
const timeout = 10 * time.Second

// client is the HTTP client every call here uses. A package variable, matching
// the KEF bridge, so tests can point the dialer at a local server while the
// host in the URL stays a real LAN address — validating the address and then
// reaching a test server are otherwise mutually exclusive.
var client = http.DefaultClient

// Device is a described renderer: who it is, and where to send commands.
//
// The control URLs are absolute and resolved against the description's own
// location, because the description gives them relative and a renderer is
// perfectly entitled to serve its services from a different port than its
// description.
type Device struct {
	UDN          string
	Name         string
	Manufacturer string
	Model        string
	// Control URLs, absolute. AVTransport is required to play anything;
	// RenderingControl is optional and its absence only costs volume.
	AVTransportURL      string
	RenderingControlURL string
	ConnectionMgrURL    string
}

// wireDevice is the subset of a UPnP device description that matters.
type wireDevice struct {
	Device struct {
		DeviceType   string `xml:"deviceType"`
		FriendlyName string `xml:"friendlyName"`
		Manufacturer string `xml:"manufacturer"`
		ModelName    string `xml:"modelName"`
		UDN          string `xml:"UDN"`
		ServiceList  struct {
			Services []struct {
				ServiceType string `xml:"serviceType"`
				ControlURL  string `xml:"controlURL"`
			} `xml:"service"`
		} `xml:"serviceList"`
	} `xml:"device"`
}

// Describe fetches and parses a renderer's device description.
//
// location is the full URL of the description document — what an SSDP response
// puts in its LOCATION header, and what a user pastes when adding a device by
// hand. Anything that is not a MediaRenderer is refused here rather than
// half-registered: a UPnP network is full of devices that answer this URL and
// cannot play audio.
func Describe(ctx context.Context, location string) (*Device, error) {
	base, err := url.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("upnp: %q isn't a URL: %w", location, err)
	}
	if err := ValidateHost(base.Host); err != nil {
		return nil, fmt.Errorf("upnp: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, fmt.Errorf("upnp: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upnp: fetching %s: %w", location, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upnp: %s answered %s", location, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return nil, fmt.Errorf("upnp: reading description: %w", err)
	}

	var wd wireDevice
	if err := xml.Unmarshal(raw, &wd); err != nil {
		return nil, fmt.Errorf("upnp: %s isn't a device description: %w", location, err)
	}
	d := &Device{
		UDN:          strings.TrimSpace(wd.Device.UDN),
		Name:         strings.TrimSpace(wd.Device.FriendlyName),
		Manufacturer: strings.TrimSpace(wd.Device.Manufacturer),
		Model:        strings.TrimSpace(wd.Device.ModelName),
	}
	for _, svc := range wd.Device.ServiceList.Services {
		abs, err := base.Parse(strings.TrimSpace(svc.ControlURL))
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(svc.ServiceType, "urn:schemas-upnp-org:service:AVTransport:"):
			d.AVTransportURL = abs.String()
		case strings.HasPrefix(svc.ServiceType, "urn:schemas-upnp-org:service:RenderingControl:"):
			d.RenderingControlURL = abs.String()
		case strings.HasPrefix(svc.ServiceType, "urn:schemas-upnp-org:service:ConnectionManager:"):
			d.ConnectionMgrURL = abs.String()
		}
	}
	if d.AVTransportURL == "" {
		// Every renderer has one. A device without it is something else on
		// the network — a router, a NAS's content directory — and saying so
		// beats registering it and failing on the first tap.
		return nil, fmt.Errorf("upnp: %s has no AVTransport service, so it isn't a media renderer", location)
	}
	if d.Name == "" {
		d.Name = base.Hostname()
	}
	return d, nil
}

// arg is one SOAP argument.
type arg struct{ name, value string }

// soapCall invokes an action on a service and returns the raw response body.
//
// Structurally the same call the Sonos bridge makes, and deliberately not
// shared with it: that one hardcodes Sonos's port and service paths because a
// Sonos always has them, while this one is handed a full control URL because a
// generic renderer's is only knowable from its description. Folding the two
// together would mean teaching the Sonos path about URLs it never needs.
func soapCall(ctx context.Context, controlURL, urn, action string, args []arg) (string, error) {
	u, err := url.Parse(controlURL)
	if err != nil {
		return "", fmt.Errorf("upnp: bad control URL %q: %w", controlURL, err)
	}
	if err := ValidateHost(u.Host); err != nil {
		return "", fmt.Errorf("upnp: %w", err)
	}

	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	body.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	fmt.Fprintf(&body, `<u:%s xmlns:u="%s">`, action, urn)
	for _, a := range args {
		fmt.Fprintf(&body, "<%s>%s</%s>", a.name, xmlEscape(a.value), a.name)
	}
	fmt.Fprintf(&body, `</u:%s></s:Body></s:Envelope>`, action)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL, bytes.NewReader(body.Bytes()))
	if err != nil {
		return "", fmt.Errorf("upnp: %w", err)
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", fmt.Sprintf("%q", urn+"#"+action))

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upnp: %s %s: %w", u.Host, action, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("upnp: reading %s response: %w", action, err)
	}
	if resp.StatusCode >= 400 {
		// A SOAP fault carries a UPnP error code, which is far more useful
		// than the status: 701 is "no such transport state", 714 is "illegal
		// MIME type", and those point at different fixes.
		if code := extractTag(string(raw), "errorCode"); code != "" {
			return "", fmt.Errorf("upnp: %s refused %s (UPnP error %s)", u.Host, action, code)
		}
		return "", fmt.Errorf("upnp: HTTP %d from %s for %s", resp.StatusCode, u.Host, action)
	}
	return string(raw), nil
}

// SetURI points the renderer at a stream and hands it the DIDL metadata to
// display. The renderer fetches the audio itself — which is the whole point:
// nothing is pushed, so nothing is bound to a push protocol's format ceiling.
func SetURI(ctx context.Context, d *Device, streamURL, didl string) error {
	_, err := soapCall(ctx, d.AVTransportURL, AVTransport, "SetAVTransportURI", []arg{
		{"InstanceID", "0"},
		{"CurrentURI", streamURL},
		{"CurrentURIMetaData", didl},
	})
	return err
}

// Play starts playback.
func Play(ctx context.Context, d *Device) error {
	_, err := soapCall(ctx, d.AVTransportURL, AVTransport, "Play", []arg{
		{"InstanceID", "0"}, {"Speed", "1"},
	})
	return err
}

// Pause pauses. Not every renderer supports it on a live stream — there is
// nothing to resume into — and those answer with a UPnP fault, which is
// surfaced rather than swallowed.
func Pause(ctx context.Context, d *Device) error {
	_, err := soapCall(ctx, d.AVTransportURL, AVTransport, "Pause", []arg{{"InstanceID", "0"}})
	return err
}

// Stop stops playback and releases the stream.
func Stop(ctx context.Context, d *Device) error {
	_, err := soapCall(ctx, d.AVTransportURL, AVTransport, "Stop", []arg{{"InstanceID", "0"}})
	return err
}

// TransportState is what the renderer says it is doing.
type TransportState string

const (
	StatePlaying       TransportState = "PLAYING"
	StatePaused        TransportState = "PAUSED_PLAYBACK"
	StateStopped       TransportState = "STOPPED"
	StateTransitioning TransportState = "TRANSITIONING"
	StateNoMedia       TransportState = "NO_MEDIA_PRESENT"
)

// State reads the renderer's transport state.
func State(ctx context.Context, d *Device) (TransportState, error) {
	raw, err := soapCall(ctx, d.AVTransportURL, AVTransport, "GetTransportInfo", []arg{{"InstanceID", "0"}})
	if err != nil {
		return "", err
	}
	return TransportState(extractTag(raw, "CurrentTransportState")), nil
}

// Volume reads the renderer's volume as 0-100.
func Volume(ctx context.Context, d *Device) (int, error) {
	if d.RenderingControlURL == "" {
		return 0, fmt.Errorf("upnp: %s has no volume control", d.Name)
	}
	raw, err := soapCall(ctx, d.RenderingControlURL, RenderingControl, "GetVolume", []arg{
		{"InstanceID", "0"}, {"Channel", "Master"},
	})
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(extractTag(raw, "CurrentVolume")))
	if err != nil {
		return 0, fmt.Errorf("upnp: %s reported an unreadable volume", d.Name)
	}
	return n, nil
}

// SetVolume sets the renderer's volume, clamped to 0-100.
func SetVolume(ctx context.Context, d *Device, level int) error {
	if d.RenderingControlURL == "" {
		return fmt.Errorf("upnp: %s has no volume control", d.Name)
	}
	level = max(0, min(100, level))
	_, err := soapCall(ctx, d.RenderingControlURL, RenderingControl, "SetVolume", []arg{
		{"InstanceID", "0"}, {"Channel", "Master"}, {"DesiredVolume", strconv.Itoa(level)},
	})
	return err
}

// SetMute mutes or unmutes.
func SetMute(ctx context.Context, d *Device, muted bool) error {
	if d.RenderingControlURL == "" {
		return fmt.Errorf("upnp: %s has no mute control", d.Name)
	}
	v := "0"
	if muted {
		v = "1"
	}
	_, err := soapCall(ctx, d.RenderingControlURL, RenderingControl, "SetMute", []arg{
		{"InstanceID", "0"}, {"Channel", "Master"}, {"DesiredMute", v},
	})
	return err
}

// Muted reads the mute state.
func Muted(ctx context.Context, d *Device) (bool, error) {
	if d.RenderingControlURL == "" {
		return false, nil
	}
	raw, err := soapCall(ctx, d.RenderingControlURL, RenderingControl, "GetMute", []arg{
		{"InstanceID", "0"}, {"Channel", "Master"},
	})
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(extractTag(raw, "CurrentMute")) == "1", nil
}

// ProtocolInfo is what a renderer says it can play, as a set of the MIME types
// from its ConnectionManager's sink list.
type ProtocolInfo struct {
	// LinearPCM is whether it lists audio/L16 or audio/L24 — raw samples,
	// which is what the AirPlay-free path is for.
	LinearPCM bool
	// WAV is whether it lists audio/wav or audio/x-wav, which is what
	// HomeHub's stream route actually serves.
	WAV bool
	// FLAC is listed for completeness; HomeHub does not serve FLAC today.
	FLAC bool
	// Raw is every sink MIME type, for showing a person when the answer is
	// "it says it can't play what we have".
	Raw []string
}

// PlaysPCM reports whether this renderer accepts what HomeHub's stream route
// serves. WAV is the format actually sent; linear PCM is accepted as
// equivalent because a renderer listing L16/L24 and not WAV is nearly always
// able to read a RIFF header anyway, and refusing on that alone would reject
// working hardware.
func (p ProtocolInfo) PlaysPCM() bool { return p.WAV || p.LinearPCM }

// Protocols asks the renderer what it can play.
//
// Advisory rather than authoritative: some renderers under-report badly, so a
// "no" here is worth showing a person at setup and is not used to block a play.
// Better to let someone try their box than to refuse it on its own bad
// paperwork.
func Protocols(ctx context.Context, d *Device) (ProtocolInfo, error) {
	var out ProtocolInfo
	if d.ConnectionMgrURL == "" {
		return out, nil
	}
	raw, err := soapCall(ctx, d.ConnectionMgrURL, ConnectionManager, "GetProtocolInfo", nil)
	if err != nil {
		return out, err
	}
	sink := extractTag(raw, "Sink")
	for _, entry := range strings.Split(sink, ",") {
		// Entries are protocol:network:mime:extras — the MIME is third.
		parts := strings.Split(entry, ":")
		if len(parts) < 3 {
			continue
		}
		mime := strings.ToLower(strings.TrimSpace(parts[2]))
		if mime == "" {
			continue
		}
		out.Raw = append(out.Raw, mime)
		// Linear PCM is almost always published with its parameters
		// attached — "audio/L16;rate=44100;channels=2" — so the type has to
		// be compared without them. Matching the whole string would report
		// every real renderer as unable to play PCM, which is exactly
		// backwards for the ones this bridge exists to serve.
		if i := strings.IndexByte(mime, ';'); i >= 0 {
			mime = strings.TrimSpace(mime[:i])
		}
		switch mime {
		case "audio/l16", "audio/l24":
			out.LinearPCM = true
		case "audio/wav", "audio/x-wav", "audio/vnd.wave":
			out.WAV = true
		case "audio/flac", "audio/x-flac":
			out.FLAC = true
		}
	}
	return out, nil
}

// DIDL builds the one-item metadata document a renderer expects alongside a
// stream URL.
//
// Sent because renderers with a display show it and some refuse a URI with no
// metadata at all. The object class is the generic audio item: HomeHub's stream
// is a live thing with no known length, and claiming a duration it does not
// have makes a renderer's progress bar lie.
func DIDL(title, artist, mime string) string {
	if title == "" {
		title = "HomeHub"
	}
	if mime == "" {
		mime = "audio/wav"
	}
	var b strings.Builder
	b.WriteString(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" `)
	b.WriteString(`xmlns:dc="http://purl.org/dc/elements/1.1/" `)
	b.WriteString(`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">`)
	b.WriteString(`<item id="0" parentID="-1" restricted="1">`)
	fmt.Fprintf(&b, `<dc:title>%s</dc:title>`, xmlEscape(title))
	if artist != "" {
		fmt.Fprintf(&b, `<upnp:artist>%s</upnp:artist>`, xmlEscape(artist))
	}
	b.WriteString(`<upnp:class>object.item.audioItem.musicTrack</upnp:class>`)
	fmt.Fprintf(&b, `<res protocolInfo="http-get:*:%s:*"></res>`, xmlEscape(mime))
	b.WriteString(`</item></DIDL-Lite>`)
	return b.String()
}

// HostPort splits a control URL into host and port for display and for the
// stored record's address field.
func HostPort(controlURL string) (host string, port int) {
	u, err := url.Parse(controlURL)
	if err != nil {
		return "", 0
	}
	host = u.Hostname()
	if p := u.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	}
	if port == 0 {
		port = 80
	}
	return host, port
}

// xmlEscape escapes a value for embedding in XML.
func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// extractTag pulls the text of the first <name>…</name>, ignoring namespaces.
//
// A real XML walk would be tidier, but UPnP responses nest the payload inside
// an escaped string inside the envelope often enough that a parser has to be
// run twice anyway; matching the tag is what every implementation of this ends
// up doing.
func extractTag(doc, name string) string {
	open, closeTag := "<"+name, "</"+name+">"
	i := strings.Index(doc, open)
	if i < 0 {
		return ""
	}
	// Skip to the end of the opening tag, which may carry attributes.
	j := strings.Index(doc[i:], ">")
	if j < 0 {
		return ""
	}
	start := i + j + 1
	end := strings.Index(doc[start:], closeTag)
	if end < 0 {
		return ""
	}
	return unescape(doc[start : start+end])
}

// unescape reverses the entity escaping UPnP applies to nested documents.
func unescape(s string) string {
	r := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&amp;", "&")
	return r.Replace(s)
}
