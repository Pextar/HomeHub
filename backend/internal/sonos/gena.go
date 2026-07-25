package sonos

// GENA — General Event Notification Architecture — is the publish/subscribe
// half of UPnP, the counterpart to the SOAP calls in sonos.go. Instead of
// asking a speaker what it is doing, we register a callback URL once and the
// speaker POSTs us a NOTIFY every time its state changes.
//
// The exchange is four HTTP verbs, two of them non-standard:
//
//	SUBSCRIBE   → speaker answers with a SID and the timeout it granted
//	SUBSCRIBE   → again, with just the SID, to renew before expiry
//	NOTIFY      ← speaker POSTs to our callback whenever something changes
//	UNSUBSCRIBE → we let go
//
// SEQ 0 is sent immediately on subscribe and carries the full current state,
// so subscribing doubles as the initial read.
//
// Sonos wraps most of its evented variables in a "LastChange" property whose
// value is itself an escaped XML document, so a notification needs two parse
// passes: ParsePropertySet then ParseLastChange.
//
// This file is protocol only — no lifecycle, no state. The subscription
// manager that uses it lives in monitor.go.

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SubscribeTimeout is how long a subscription is requested for. Speakers may
// grant less; whatever comes back is what the renewal schedule uses.
const SubscribeTimeout = 30 * time.Minute

// minGrantedTimeout floors an implausibly short or unparseable grant, so a
// malformed header can't spin the renewal loop.
const minGrantedTimeout = 60 * time.Second

// EventService is one service whose changes can be subscribed to. Key is a
// short stable name used for subscription bookkeeping and logging.
type EventService struct {
	Key string
	svc service
}

// The three services worth watching. AVTransport carries transport state,
// track metadata and play modes; RenderingControl carries volume and mute;
// ZoneGroupTopology carries grouping. Everything else the bridge reads
// (position, queue contents) is not evented and still needs a SOAP read.
var (
	EventTransport = EventService{Key: "transport", svc: avTransport}
	EventRendering = EventService{Key: "rendering", svc: renderingControl}
	EventTopology  = EventService{Key: "topology", svc: zoneGroupTopo}
)

// EventServices is the set the monitor subscribes to on every speaker.
var EventServices = []EventService{EventTransport, EventRendering, EventTopology}

// eventClient is separate from http.DefaultClient so subscription traffic
// can't be starved by, or starve, the SOAP control path.
var eventClient = &http.Client{Timeout: DefaultTimeout}

// Subscribe registers callback as the destination for svc's change
// notifications on the speaker at ip. It returns the subscription id and the
// timeout the speaker actually granted, which is often shorter than asked.
func Subscribe(ctx context.Context, ip string, svc EventService, callback string, timeout time.Duration) (sid string, granted time.Duration, err error) {
	if err := validCallback(callback); err != nil {
		return "", 0, err
	}
	req, err := eventRequest(ctx, "SUBSCRIBE", ip, svc)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("CALLBACK", "<"+callback+">")
	req.Header.Set("NT", "upnp:event")
	req.Header.Set("TIMEOUT", timeoutHeader(timeout))

	resp, err := doEvent(req, ip, "SUBSCRIBE")
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	sid = strings.TrimSpace(resp.Header.Get("SID"))
	if sid == "" {
		return "", 0, fmt.Errorf("sonos: %s accepted the subscription to %s but returned no SID", ip, svc.Key)
	}
	return sid, parseTimeoutHeader(resp.Header.Get("TIMEOUT")), nil
}

// Renew extends an existing subscription. A speaker that has forgotten the
// SID — because it rebooted, or the subscription lapsed — answers 412, which
// the caller should treat as "subscribe again from scratch".
func Renew(ctx context.Context, ip string, svc EventService, sid string, timeout time.Duration) (granted time.Duration, err error) {
	req, err := eventRequest(ctx, "SUBSCRIBE", ip, svc)
	if err != nil {
		return 0, err
	}
	if err := validSID(sid); err != nil {
		return 0, err
	}
	req.Header.Set("SID", sid)
	req.Header.Set("TIMEOUT", timeoutHeader(timeout))

	resp, err := doEvent(req, ip, "renew")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return parseTimeoutHeader(resp.Header.Get("TIMEOUT")), nil
}

// Unsubscribe releases a subscription. Best-effort: a speaker that has
// already forgotten it answers 412, which is not worth reporting as failure
// since the desired end state — no subscription — already holds.
func Unsubscribe(ctx context.Context, ip string, svc EventService, sid string) error {
	req, err := eventRequest(ctx, "UNSUBSCRIBE", ip, svc)
	if err != nil {
		return err
	}
	if err := validSID(sid); err != nil {
		return err
	}
	req.Header.Set("SID", sid)

	resp, err := doEvent(req, ip, "UNSUBSCRIBE")
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 412") {
			return nil
		}
		return err
	}
	resp.Body.Close()
	return nil
}

// eventRequest builds a subscription-verb request against svc's event URL.
func eventRequest(ctx context.Context, method, ip string, svc EventService) (*http.Request, error) {
	if err := ValidateHost(ip); err != nil {
		return nil, fmt.Errorf("sonos: %w", err)
	}
	if svc.svc.event == "" {
		return nil, fmt.Errorf("sonos: service %s is not evented", svc.Key)
	}
	u := fmt.Sprintf("http://%s:%d%s", ip, Port, svc.svc.event)
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, fmt.Errorf("sonos: build %s request: %w", method, err)
	}
	return req, nil
}

// doEvent runs a subscription request and turns a non-2xx into an error.
func doEvent(req *http.Request, ip, what string) (*http.Response, error) {
	resp, err := eventClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sonos: %s %s: %w", ip, what, err)
	}
	if resp.StatusCode >= 300 {
		// Drain a little so the connection can be reused, then report.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()
		return nil, fmt.Errorf("sonos: HTTP %d from %s for %s", resp.StatusCode, ip, what)
	}
	return resp, nil
}

// validCallback rejects anything that could smuggle a header break into the
// CALLBACK value, or point a speaker somewhere other than back at us.
func validCallback(callback string) error {
	if callback == "" || strings.ContainsAny(callback, "\r\n<> \t") {
		return fmt.Errorf("sonos: invalid event callback %q", callback)
	}
	// Speakers will not do TLS, so the callback is always plain HTTP.
	if !strings.HasPrefix(callback, "http://") {
		return fmt.Errorf("sonos: event callback must be http://, got %q", callback)
	}
	return nil
}

// validSID guards the SID header the same way, since it round-trips from the
// speaker and back out again.
func validSID(sid string) error {
	if sid == "" || strings.ContainsAny(sid, "\r\n \t") {
		return fmt.Errorf("sonos: invalid subscription id %q", sid)
	}
	return nil
}

func timeoutHeader(d time.Duration) string {
	return "Second-" + strconv.Itoa(int(d.Seconds()))
}

// parseTimeoutHeader reads "Second-1800". Anything else — including the
// legal-but-unhelpful "infinite" — falls back to the floor, so the renewal
// loop always has a finite interval to work from.
func parseTimeoutHeader(h string) time.Duration {
	n, ok := strings.CutPrefix(strings.TrimSpace(h), "Second-")
	if !ok {
		return minGrantedTimeout
	}
	secs, err := strconv.Atoi(strings.TrimSpace(n))
	if err != nil || secs <= 0 {
		return minGrantedTimeout
	}
	d := time.Duration(secs) * time.Second
	if d < minGrantedTimeout {
		return minGrantedTimeout
	}
	return d
}

// ── Notification bodies ──────────────────────────────────────────────────

// ParsePropertySet flattens a GENA NOTIFY body into name → value:
//
//	<e:propertyset><e:property><LastChange>…</LastChange></e:property>…
//
// Values arrive XML-unescaped, so a LastChange value comes out as the inner
// document ready for ParseLastChange.
func ParsePropertySet(body string) map[string]string {
	out := make(map[string]string, 4)
	dec := xml.NewDecoder(strings.NewReader(body))
	// Be permissive: speakers are the only source, and a strict decoder that
	// bails on an unknown entity would drop the whole notification.
	dec.Strict = false
	depth := 0
	var cur string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			// propertyset > property > <Name>
			if depth == 3 {
				cur = t.Name.Local
			}
		case xml.CharData:
			// Values can arrive in several chunks for a long document.
			if cur != "" {
				out[cur] += string(t)
			}
		case xml.EndElement:
			if depth == 3 {
				cur = ""
			}
			depth--
		}
	}
	return out
}

// lastChangeXML mirrors the LastChange document: one InstanceID element
// whose children are the changed variables, each carrying its value in a
// val attribute rather than as text.
type lastChangeXML struct {
	Instances []struct {
		ID     string `xml:"val,attr"`
		Values []struct {
			XMLName xml.Name
			Val     string `xml:"val,attr"`
			Channel string `xml:"channel,attr"`
		} `xml:",any"`
	} `xml:"InstanceID"`
}

// ParseLastChange flattens the InstanceID 0 block of a LastChange document
// into name → value. RenderingControl reports several variables per channel;
// only Master is kept, since that is the one the bridge reads and sets.
func ParseLastChange(doc string) map[string]string {
	var parsed lastChangeXML
	dec := xml.NewDecoder(strings.NewReader(doc))
	dec.Strict = false
	if err := dec.Decode(&parsed); err != nil {
		return nil
	}
	out := make(map[string]string, 8)
	for _, inst := range parsed.Instances {
		// Sonos only ever uses instance 0; an empty attr means the same.
		if inst.ID != "" && inst.ID != instance0 {
			continue
		}
		for _, v := range inst.Values {
			if v.Channel != "" && v.Channel != "Master" {
				continue
			}
			out[v.XMLName.Local] = v.Val
		}
	}
	return out
}
