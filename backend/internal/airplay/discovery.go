package airplay

import (
	"context"
	"strings"
	"sync"
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

	// RAOP is the service that carries the classic session, so it is what a
	// device is built from first. The _airplay._tcp side is read after, and
	// unlike an earlier version of this it can create a device of its own —
	// see the loop below.
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

	// Now the _airplay._tcp side, which is where an AirPlay 2 receiver
	// describes itself. Two jobs: improve a device already found — the
	// friendly name lives here and is often better — and create one for a box
	// that advertises no classic audio service at all.
	//
	// That second job is the fix for the case this scan used to miss. A
	// receiver in AirPlay 2 mode may publish only this service, and skipping
	// it meant a box the user can see working from their phone simply never
	// appeared. Whether it will take a classic session is then settled by
	// asking it, below, rather than by its absence from a service list.
	for _, instance := range recs.instances[airplayService] {
		name := strings.TrimSpace(trimService(instance, airplayService))
		txt := recs.txt[instance]
		id := NormalizeID(txt["deviceid"])
		ip := recs.address(instance)

		matched := false
		for key, d := range byKey {
			if !sameDevice(d, key, id, ip) {
				continue
			}
			matched = true
			if name != "" {
				d.Name = name
			}
			d.fromAirPlayTXT(txt)
		}
		if matched || ip == "" || ValidateHost(ip) != nil {
			continue
		}

		d := &Device{Name: name, IP: ip, Port: DefaultPort, ID: id}
		if s, ok := recs.srv[instance]; ok && s.port > 0 {
			d.Port = s.port
		}
		// No classic keys to read: the audio parameters default to what
		// AirPlay 1 carries, and the codec and cipher lists stay empty, which
		// attempts() reads as "offer it the universal pair".
		d.fromTXT(nil)
		d.fromAirPlayTXT(txt)
		d.AirPlay2 = true
		if d.Name == "" {
			d.Name = d.IP
		}
		key := d.ID
		if key == "" {
			key = d.Addr()
		}
		byKey[key] = d
	}

	out := make([]Device, 0, len(byKey))
	for _, d := range byKey {
		out = append(out, *d)
	}
	verifyClassic(ctx, out)
	sortDevices(out)
	return out, nil
}

// verifyProbes caps how many receivers are asked at once, and how long each
// gets. Small and short: this runs inside a scan the user is watching, and an
// OPTIONS exchange with a box on the same LAN is a couple of milliseconds.
const (
	verifyProbes  = 8
	verifyTimeout = 2 * time.Second
)

// verifyClassic asks every discovered receiver whether it takes the session
// this package sends, and records the answer on the device.
//
// This is the same shape as the KEF scan: SSDP narrows the subnet down and the
// API probe settles it. Here the advertisement narrows it down and the
// receiver's own OPTIONS reply settles it — which matters most for the AirPlay
// 2 boxes, where the advertisement genuinely does not answer the question.
//
// Asking costs nothing the user would notice and takes nothing away from
// anyone: OPTIONS is stateless, so unlike ANNOUNCE it does not claim the
// receiver from whatever is currently playing to it.
func verifyClassic(ctx context.Context, devices []Device) {
	sem := make(chan struct{}, verifyProbes)
	var wg sync.WaitGroup
	for i := range devices {
		wg.Add(1)
		go func(d *Device) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cctx, cancel := context.WithTimeout(ctx, verifyTimeout)
			defer cancel()
			d.Classic = probeClassic(cctx, d.Addr())
		}(&devices[i])
	}
	wg.Wait()
}

// probeClassic asks one receiver whether it will take a classic session.
//
// The question is RTSP OPTIONS, and the answer is whether ANNOUNCE appears in
// the methods it lists. A box that cannot be reached at all is left Unknown
// rather than marked as refusing: it may simply be asleep, and "this receiver
// said no" is a different claim from "nothing answered".
func probeClassic(ctx context.Context, addr string) Classic {
	c, err := dial(ctx, addr)
	if err != nil {
		return ClassicUnknown
	}
	defer c.Close()

	resp, err := c.do(ctx, request{Method: "OPTIONS", URI: "*"})
	if err != nil {
		return ClassicUnknown
	}
	if resp.Status != 200 {
		return ClassicNo
	}
	pub := strings.ToUpper(resp.Header("Public"))
	if pub == "" {
		// Answered 200 and listed nothing. Every receiver worth the name
		// lists its methods, but a terse one is not a refusal — the ANNOUNCE
		// will settle it.
		return ClassicUnknown
	}
	if strings.Contains(pub, "ANNOUNCE") {
		return ClassicYes
	}
	return ClassicNo
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
	// A receiver that answers RTSP and lists methods without ANNOUNCE among
	// them is refusing the classic session — an AirPlay 2 speaker that wants
	// pairing, or something else entirely on this port. Caught here rather
	// than at the first play.
	pub := strings.ToUpper(resp.Header("Public"))
	switch {
	case pub == "":
		d.Classic = ClassicUnknown
	case strings.Contains(pub, "ANNOUNCE"):
		d.Classic = ClassicYes
	default:
		return nil, &UnsupportedError{
			Reason: "this address answers RTSP but won't take the AirPlay audio session " +
				"HomeHub opens — an AirPlay 2 speaker that needs pairing looks like this",
		}
	}
	return d, nil
}
