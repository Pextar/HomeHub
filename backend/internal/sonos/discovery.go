package sonos

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

// ssdpAddr is the standard SSDP multicast group.
const ssdpAddr = "239.255.255.250:1900"

// ssdpSearch is the M-SEARCH request targeting Sonos zone players only.
const ssdpSearch = "M-SEARCH * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"MAN: \"ssdp:discover\"\r\n" +
	"MX: 1\r\n" +
	"ST: urn:schemas-upnp-org:device:ZonePlayer:1\r\n\r\n"

// Discover finds Sonos speakers on the LAN. It first multicasts an SSDP
// M-SEARCH; then, because multicast can be flaky across VLANs/Wi-Fi, it
// asks the first responder for the household's full zone topology, which
// enumerates every speaker regardless of whether its own SSDP reply made
// it back. Results are deduplicated by UUID and sorted by room name.
func Discover(ctx context.Context, wait time.Duration) ([]Device, error) {
	ips := ssdpProbe(ctx, wait)

	byUUID := make(map[string]Device)
	// Resolve each SSDP responder to a full identity.
	for _, ip := range ips {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		d, err := Describe(cctx, ip)
		cancel()
		if err == nil {
			byUUID[d.UUID] = *d
		}
	}
	// Expand via topology from any one known speaker.
	for _, seed := range byUUID {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		groups, err := GetTopology(cctx, seed.IP)
		cancel()
		if err != nil {
			continue
		}
		for _, g := range groups {
			for _, m := range g.Members {
				if _, known := byUUID[m.UUID]; known || m.IP == "" {
					continue
				}
				if ValidateHost(m.IP) != nil {
					continue
				}
				dev := Device{IP: m.IP, UUID: m.UUID, Room: m.Name}
				// Model is nice-to-have; fetch but tolerate failure.
				cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				if d, err := Describe(cctx, m.IP); err == nil {
					dev.Model = d.Model
				}
				cancel()
				byUUID[m.UUID] = dev
			}
		}
		break // one topology covers the household
	}

	out := make([]Device, 0, len(byUUID))
	for _, d := range byUUID {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Room < out[j].Room })
	return out, nil
}

// ssdpProbe multicasts the M-SEARCH and collects responder IPs until the
// wait window closes. Errors are swallowed — no network / no permission
// simply yields an empty list and the caller reports "none found".
func ssdpProbe(ctx context.Context, wait time.Duration) []string {
	if wait <= 0 {
		wait = 2 * time.Second
	}
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil
	}
	defer conn.Close()

	dst, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil
	}
	// Three sends spaced out — SSDP is UDP, losing one datagram is routine.
	for i := 0; i < 3; i++ {
		_, _ = conn.WriteTo([]byte(ssdpSearch), dst)
		time.Sleep(100 * time.Millisecond)
	}

	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetReadDeadline(deadline)

	seen := make(map[string]bool)
	var ips []string
	buf := make([]byte, 2048)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break // deadline reached (or socket error) — done collecting
		}
		ip := parseSSDPLocation(string(buf[:n]))
		if ip != "" && !seen[ip] {
			seen[ip] = true
			ips = append(ips, ip)
		}
	}
	return ips
}

// parseSSDPLocation pulls the responder's IP out of an SSDP response's
// LOCATION header. Returns "" for non-Sonos or malformed responses.
func parseSSDPLocation(resp string) string {
	for _, line := range strings.Split(resp, "\r\n") {
		k, _, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "location") {
			continue
		}
		// Everything after the first colon — the URL itself contains colons.
		loc := strings.TrimSpace(line[len(k)+1:])
		ip := ipFromLocation(loc)
		if ip == "" || ValidateHost(ip) != nil {
			return ""
		}
		return ip
	}
	return ""
}

// String implements fmt.Stringer for log lines.
func (d Device) String() string {
	return fmt.Sprintf("%s (%s, %s)", d.Room, d.Model, d.IP)
}
