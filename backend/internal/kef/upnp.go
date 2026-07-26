package kef

// Playing a stream on a KEF speaker.
//
// The rest of this package speaks KEF's own HTTP JSON API, which has transport
// control and no way at all to accept content — no queue, no URI, no
// favorites. That is why starting music on a KEF has until now meant Spotify
// Connect, and why a KEF could never join a Sonos in playing the same thing.
//
// The speaker also runs a standard UPnP media renderer; discovery.go already
// relies on it answering SSDP as one. That renderer *does* take a URI, which
// is what lets HomeHub hand a KEF the same stream it hands a Sonos. So this
// file is a second protocol to the same box, used for exactly one thing.
//
// The control URL is discovered from the device description rather than
// hardcoded: the port and path have moved between firmware versions, and a
// wrong guess fails as a connection error that names nothing useful. It is
// cached per address, because a play should not cost a description fetch every
// time.

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// avTransportURN is the service we need off the renderer.
const avTransportURN = "urn:schemas-upnp-org:service:AVTransport:1"

// descriptionPorts are where KEF firmware has served its device description.
// 8080 first: that is what the LOCATION header carries on current firmware,
// as recorded in discovery.go.
var descriptionPorts = []int{8080, 80}

// descriptionPaths are the usual names for the description document. Tried
// against each port in turn.
var descriptionPaths = []string{"/description.xml", "/MediaRenderer.xml", "/upnp/description.xml"}

// controlCache remembers where a speaker's AVTransport lives. The renderer
// does not move while the speaker is up, and a stale entry costs one failed
// play before it is re-resolved, so the TTL is generous.
var (
	controlMu    sync.Mutex
	controlCache = map[string]controlEntry{}
)

type controlEntry struct {
	url string
	at  time.Time
}

const controlTTL = time.Hour

// PlayStreamURI points the speaker's UPnP renderer at uri and starts it.
// metadata is a DIDL-Lite document describing what to display and may be
// empty, though Sonos and KEF alike show a friendlier now-playing with it.
//
// The caller is responsible for having woken the speaker: the renderer is
// only reachable once the speaker is awake and on its network input.
func PlayStreamURI(ctx context.Context, ip, uri, metadata string) error {
	control, err := avTransportControl(ctx, ip)
	if err != nil {
		return err
	}
	if err := soapAction(ctx, control, "SetAVTransportURI", map[string]string{
		"InstanceID":         "0",
		"CurrentURI":         uri,
		"CurrentURIMetaData": metadata,
	}, []string{"InstanceID", "CurrentURI", "CurrentURIMetaData"}); err != nil {
		return fmt.Errorf("kef: setting stream URI: %w", err)
	}
	if err := soapAction(ctx, control, "Play", map[string]string{
		"InstanceID": "0",
		"Speed":      "1",
	}, []string{"InstanceID", "Speed"}); err != nil {
		return fmt.Errorf("kef: starting stream: %w", err)
	}
	return nil
}

// StopStream stops the renderer. Used when a zone stops, so the speaker is
// not left holding a connection to a stream nobody is serving any more.
func StopStream(ctx context.Context, ip string) error {
	control, err := avTransportControl(ctx, ip)
	if err != nil {
		return err
	}
	return soapAction(ctx, control, "Stop",
		map[string]string{"InstanceID": "0"}, []string{"InstanceID"})
}

// avTransportControl resolves the absolute control URL for the speaker's
// AVTransport service, using the cache when it can.
func avTransportControl(ctx context.Context, ip string) (string, error) {
	if err := ValidateHost(ip); err != nil {
		return "", err
	}
	controlMu.Lock()
	if e, ok := controlCache[ip]; ok && time.Since(e.at) < controlTTL {
		controlMu.Unlock()
		return e.url, nil
	}
	controlMu.Unlock()

	control, err := discoverControl(ctx, ip)
	if err != nil {
		return "", err
	}
	controlMu.Lock()
	controlCache[ip] = controlEntry{url: control, at: time.Now()}
	controlMu.Unlock()
	return control, nil
}

// forgetControl drops a cached entry, so a speaker whose renderer moved is
// re-resolved rather than failing forever against a stale URL.
func forgetControl(ip string) {
	controlMu.Lock()
	delete(controlCache, ip)
	controlMu.Unlock()
}

// discoverControl fetches the device description and reads the AVTransport
// control URL out of it, trying the known locations in turn.
func discoverControl(ctx context.Context, ip string) (string, error) {
	var lastErr error
	for _, port := range descriptionPorts {
		for _, path := range descriptionPaths {
			base := fmt.Sprintf("http://%s%s", net.JoinHostPort(ip, fmt.Sprint(port)), path)
			control, err := controlFromDescription(ctx, base)
			if err == nil {
				return control, nil
			}
			lastErr = err
			// A cancelled context means the caller gave up; keep trying
			// only while there is time left.
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
		}
	}
	return "", fmt.Errorf("kef: %s has no reachable UPnP renderer for streaming: %w", ip, lastErr)
}

// deviceDescription is the subset of the UPnP description we read.
type deviceDescription struct {
	URLBase string `xml:"URLBase"`
	Device  struct {
		ServiceList struct {
			Services []struct {
				ServiceType string `xml:"serviceType"`
				ControlURL  string `xml:"controlURL"`
			} `xml:"service"`
		} `xml:"serviceList"`
		// Some renderers nest the interesting services one level down in an
		// embedded device list rather than at the top.
		DeviceList struct {
			Devices []struct {
				ServiceList struct {
					Services []struct {
						ServiceType string `xml:"serviceType"`
						ControlURL  string `xml:"controlURL"`
					} `xml:"service"`
				} `xml:"serviceList"`
			} `xml:"device"`
		} `xml:"deviceList"`
	} `xml:"device"`
}

// controlFromDescription fetches one candidate description URL and extracts
// the AVTransport control URL, resolved against the document's own base.
func controlFromDescription(ctx context.Context, descURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, descURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("description at %s: HTTP %d", descURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", err
	}

	var desc deviceDescription
	if err := xml.Unmarshal(body, &desc); err != nil {
		return "", fmt.Errorf("description at %s: %w", descURL, err)
	}

	raw := findAVTransport(&desc)
	if raw == "" {
		return "", fmt.Errorf("description at %s has no %s", descURL, avTransportURN)
	}
	return resolveURL(descURL, desc.URLBase, raw)
}

// findAVTransport looks for the AVTransport control URL at both levels the
// description can carry it.
func findAVTransport(desc *deviceDescription) string {
	for _, svc := range desc.Device.ServiceList.Services {
		if strings.EqualFold(svc.ServiceType, avTransportURN) {
			return svc.ControlURL
		}
	}
	for _, dev := range desc.Device.DeviceList.Devices {
		for _, svc := range dev.ServiceList.Services {
			if strings.EqualFold(svc.ServiceType, avTransportURN) {
				return svc.ControlURL
			}
		}
	}
	return ""
}

// resolveURL turns a possibly relative controlURL into an absolute one.
// URLBase wins when the document supplies it, per the UPnP spec; otherwise
// the description's own URL is the base.
func resolveURL(descURL, urlBase, control string) (string, error) {
	base := descURL
	if strings.TrimSpace(urlBase) != "" {
		base = urlBase
	}
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	c, err := url.Parse(strings.TrimSpace(control))
	if err != nil {
		return "", err
	}
	return b.ResolveReference(c).String(), nil
}

// soapAction sends one AVTransport action. Argument order matters to strict
// renderers, so it is passed explicitly rather than taken from map iteration.
func soapAction(ctx context.Context, control, action string, args map[string]string, order []string) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	body.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"`)
	body.WriteString(` s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	body.WriteString(`<u:` + action + ` xmlns:u="` + avTransportURN + `">`)
	for _, k := range order {
		body.WriteString("<" + k + ">")
		body.WriteString(escapeXML(args[k]))
		body.WriteString("</" + k + ">")
	}
	body.WriteString(`</u:` + action + `></s:Body></s:Envelope>`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, control,
		bytes.NewReader([]byte(body.String())))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+avTransportURN+"#"+action+`"`)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	// A 404 means the control URL is wrong — most likely a firmware update
	// moved it — so drop the cache entry and let the next attempt re-resolve
	// instead of failing identically forever.
	if resp.StatusCode == http.StatusNotFound {
		if u, err := url.Parse(control); err == nil {
			forgetControl(u.Hostname())
		}
	}
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("%s: HTTP %d: %s", action, resp.StatusCode, upnpFault(string(detail)))
}

// upnpFault pulls the human-readable part out of a SOAP fault, so an error
// reads as a reason rather than as forty lines of envelope.
func upnpFault(body string) string {
	if s := between(body, "<errorDescription>", "</errorDescription>"); s != "" {
		return s
	}
	if s := between(body, "<faultstring>", "</faultstring>"); s != "" {
		return s
	}
	body = strings.TrimSpace(body)
	if len(body) > 200 {
		return body[:200] + "…"
	}
	return body
}

func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:j])
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}
