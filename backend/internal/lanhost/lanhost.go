// Package lanhost validates the addresses of devices on the local network
// before they are interpolated into a server-side URL.
//
// Three bridges — Sonos, KEF and Tasmota — each take a host from the user
// and build http://<host>/… from it, so each has to reject anything that
// could redirect that request somewhere else. They had three copies of the
// same check; the Sonos and KEF ones were identical but for a comment.
//
// The check is deliberately conservative:
//
//   - Nothing that lets the value escape the host position of the URL: an
//     embedded scheme, path, query, fragment or userinfo.
//   - No IP literal pointing somewhere sensitive — loopback, link-local
//     (which covers the 169.254.169.254 cloud-metadata endpoint),
//     unspecified or multicast.
//   - Otherwise a LAN hostname of letters, digits, hyphens and dots, e.g.
//     "tasmota-1234.local".
//
// Private/RFC-1918 ranges are intentionally allowed: that is where these
// devices live. Names are not resolved here — the range checks bind IP
// literals only, and resolution stays the network's responsibility.
package lanhost

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Policy is one bridge's variant of the check.
type Policy struct {
	// Noun names the thing being validated in error messages, e.g.
	// "speaker address" or "device host".
	Noun string
	// AllowPort permits a trailing ":port". Tasmota devices may be on a
	// non-default port; Sonos is always :1400 and KEF always :80, so for
	// those a colon is simply invalid.
	AllowPort bool
}

// Validate reports whether host is safe to use as the host part of a URL.
func (p Policy) Validate(host string) error {
	h := strings.TrimSpace(host)
	if h == "" {
		return fmt.Errorf("%s is empty", p.Noun)
	}

	// Anything that lets the value escape the host position of the URL.
	unsafe := "/?#@\\ \t\r\n"
	if !p.AllowPort {
		unsafe += ":"
	}
	if strings.ContainsAny(h, unsafe) {
		return fmt.Errorf("invalid %s %q", p.Noun, host)
	}

	hostPart := h
	if p.AllowPort {
		// This never matches a bare IPv6 literal, which SplitHostPort
		// rejects as "too many colons".
		if hp, port, err := net.SplitHostPort(h); err == nil {
			hostPart = hp
			if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("invalid port in %s %q", p.Noun, host)
			}
		}
	}

	if parsed := net.ParseIP(hostPart); parsed != nil {
		if parsed.IsLoopback() || parsed.IsLinkLocalUnicast() ||
			parsed.IsLinkLocalMulticast() || parsed.IsUnspecified() || parsed.IsMulticast() {
			return fmt.Errorf("%s %q is not an allowed address", p.Noun, host)
		}
		return nil
	}

	for _, c := range hostPart {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '.'
		if !ok {
			return fmt.Errorf("invalid %s %q", p.Noun, host)
		}
	}
	return nil
}
