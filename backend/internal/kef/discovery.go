package kef

// LAN discovery. KEF's wireless speakers answer SSDP as UPnP media
// renderers, but nothing in that reply identifies them as KEF reliably
// across firmware versions — so the M-SEARCH is only used to narrow the
// subnet down to a handful of addresses, and each one is then asked the
// question that actually settles it: does it answer the KEF JSON API?
// Anything that does is a KEF speaker; anything that doesn't is somebody
// else's renderer and is dropped.
//
// Multicast is blocked on plenty of home Wi-Fi setups, which is why the UI
// always offers typing the address by hand as an equal path rather than as
// a fallback for when the scan "fails".

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// ssdpAddr is the standard SSDP multicast group.
const ssdpAddr = "239.255.255.250:1900"

// ssdpSearch targets UPnP media renderers. KEF speakers answer it; so do
// other renderers on the LAN, which the KEF API probe then filters out.
const ssdpSearch = "M-SEARCH * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"MAN: \"ssdp:discover\"\r\n" +
	"MX: 1\r\n" +
	"ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n\r\n"

// probeConcurrency caps how many candidate addresses are asked at once.
// Small on purpose: the point is to identify a handful of SSDP responders,
// not to sweep a subnet.
const probeConcurrency = 8

// Discover finds KEF speakers on the LAN. Results are deduplicated by MAC
// (a speaker that answers SSDP on two interfaces is still one speaker) and
// sorted by name.
func Discover(ctx context.Context, wait time.Duration) ([]Device, error) {
	ips := ssdpProbe(ctx, wait)

	var mu sync.Mutex
	byMAC := make(map[string]Device)

	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			d, err := Describe(cctx, ip)
			if err != nil {
				return // not a KEF, or not answering right now
			}
			mu.Lock()
			defer mu.Unlock()
			byMAC[d.MAC] = *d
		}(ip)
	}
	wg.Wait()

	out := make([]Device, 0, len(byMAC))
	for _, d := range byMAC {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].IP < out[j].IP
	})
	return out, nil
}

// ssdpProbe multicasts the M-SEARCH and collects responder IPs until the
// wait window closes. Errors are swallowed — no network, no multicast
// permission — and the caller reports "none found".
func ssdpProbe(ctx context.Context, wait time.Duration) []string {
	if wait <= 0 {
		wait = 2 * time.Second
	}
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil
	}
	defer func() { _ = conn.Close() }()

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
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			break // deadline reached (or socket error) — done collecting
		}
		ip := parseSSDPLocation(string(buf[:n]))
		if ip == "" {
			// No usable LOCATION header; the sender's own address is still
			// a candidate worth probing.
			if udp, ok := from.(*net.UDPAddr); ok && udp.IP != nil {
				ip = udp.IP.String()
				if ValidateHost(ip) != nil {
					ip = ""
				}
			}
		}
		if ip != "" && !seen[ip] {
			seen[ip] = true
			ips = append(ips, ip)
		}
	}
	return ips
}

// parseSSDPLocation pulls the responder's host out of an SSDP response's
// LOCATION header. Returns "" when absent or unusable.
func parseSSDPLocation(resp string) string {
	for _, line := range strings.Split(resp, "\r\n") {
		k, _, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "location") {
			continue
		}
		// Everything after the first colon — the URL itself contains colons.
		host := hostFromLocation(strings.TrimSpace(line[len(k)+1:]))
		if host == "" || ValidateHost(host) != nil {
			return ""
		}
		return host
	}
	return ""
}

// hostFromLocation extracts the host from a LOCATION URL
// (http://192.168.1.60:8080/description.xml → 192.168.1.60).
func hostFromLocation(loc string) string {
	s := strings.TrimPrefix(loc, "http://")
	s = strings.TrimPrefix(s, "https://")
	if i := strings.IndexAny(s, ":/"); i >= 0 {
		s = s[:i]
	}
	return s
}

// String implements fmt.Stringer for log lines.
func (d Device) String() string {
	return fmt.Sprintf("%s (%s, %s)", d.Name, d.Model, d.IP)
}
