package airplay

import (
	"context"
	"strings"
	"time"
)

// Discover finds AirPlay receivers on the LAN.
//
// Multicast is blocked or filtered on plenty of home Wi-Fi setups, so — as
// with the Sonos and KEF scans — the UI offers typing an address by hand as an
// equal path rather than as a fallback for when the scan "fails". Probe is
// what that path calls.
func Discover(ctx context.Context, wait time.Duration) ([]Device, error) {
	recs, err := browse(ctx, wait, raopService, airplayService)
	if err != nil {
		return nil, err
	}

	// RAOP is the service that can actually take audio, so it is what a
	// device is built from. The _airplay._tcp advertisement is only read for
	// the friendly name and model, which are sometimes better there.
	byKey := make(map[string]*Device)
	for _, instance := range recs.instances[raopService] {
		id, name := splitInstance(trimService(instance, raopService))
		ip := recs.address(instance)
		if ip == "" || ValidateHost(ip) != nil {
			continue // a responder we cannot address is not a device
		}
		d := &Device{Name: strings.TrimSpace(name), IP: ip, Port: DefaultPort, ID: id}
		if s, ok := recs.srv[instance]; ok && s.port > 0 {
			d.Port = s.port
		}
		d.fromTXT(recs.txt[instance])
		if d.Name == "" {
			d.Name = d.IP
		}
		// Keyed by identity where there is one, by address where there
		// isn't: a receiver answering on two interfaces is one device, and
		// two receivers sharing a name are not.
		key := d.ID
		if key == "" {
			key = d.Addr()
		}
		byKey[key] = d
	}

	// Fill in from the _airplay._tcp side. It never creates a device — a box
	// advertising AirPlay video with no RAOP service has no audio sink to
	// send to — it only improves the name and model of one already found.
	for _, instance := range recs.instances[airplayService] {
		name := strings.TrimSpace(trimService(instance, airplayService))
		txt := recs.txt[instance]
		id := NormalizeID(txt["deviceid"])
		ip := recs.address(instance)
		for key, d := range byKey {
			if !sameDevice(d, key, id, ip) {
				continue
			}
			if name != "" {
				d.Name = name
			}
			if d.Model == "" {
				d.Model = txt["model"]
			}
		}
	}

	out := make([]Device, 0, len(byKey))
	for _, d := range byKey {
		out = append(out, *d)
	}
	sortDevices(out)
	return out, nil
}

// sameDevice decides whether an _airplay._tcp advertisement describes a RAOP
// device already found: same identity, or failing that, same address.
func sameDevice(d *Device, key, id, ip string) bool {
	if id != "" && (id == d.ID || id == key) {
		return true
	}
	return ip != "" && ip == d.IP
}

// trimService strips the service suffix from an instance FQDN, leaving the
// instance name: "Kitchen._raop._tcp.local." → "Kitchen".
func trimService(instance, service string) string {
	trimmed := strings.TrimSuffix(instance, "."+strings.TrimSuffix(service, "."))
	trimmed = strings.TrimSuffix(trimmed, "."+service)
	return strings.TrimSuffix(trimmed, ".")
}

// Probe asks one address whether it is an AirPlay receiver, and what kind.
//
// This is the typed-in path, and it cannot learn as much as a scan: an RTSP
// OPTIONS exchange proves something is listening and answering RAOP, but the
// codec and encryption lists live in the mDNS advertisement, which a direct
// connection never sees. So a probed device is described conservatively — PCM
// and ALAC, cleartext or RSA, which is what every shairport-sync build offers
// — and the first ANNOUNCE is where a receiver that wanted something else says
// so, with an error naming what it refused.
func Probe(ctx context.Context, host string, port int) (*Device, error) {
	if err := ValidateHost(host); err != nil {
		return nil, err
	}
	if port <= 0 {
		port = DefaultPort
	}
	d := &Device{
		Name:       host,
		IP:         host,
		Port:       port,
		Codecs:     []Codec{CodecPCM, CodecALAC},
		Encryption: []Encryption{EncryptionNone, EncryptionRSA},
		Audio:      Audio{SampleRate: SampleRate, BitDepth: BitsPerSample, Channels: Channels},
		Metadata:   true,
	}

	c, err := dial(ctx, d.Addr())
	if err != nil {
		return nil, err
	}
	defer c.Close()

	resp, err := c.do(ctx, request{Method: "OPTIONS", URI: "*"})
	if err != nil {
		return nil, err
	}
	if resp.Status != 200 {
		return nil, &StatusError{Method: "OPTIONS", Status: resp.Status, Reason: resp.Reason}
	}
	// A receiver that lists no methods is answering something other than
	// RAOP on this port — worth catching here rather than at ANNOUNCE.
	if pub := resp.Header("Public"); pub != "" && !strings.Contains(strings.ToUpper(pub), "ANNOUNCE") {
		return nil, &UnsupportedError{Reason: "this address answers RTSP but not AirPlay audio"}
	}
	return d, nil
}
