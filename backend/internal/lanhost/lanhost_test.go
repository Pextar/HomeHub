package lanhost

import "testing"

var (
	speaker = Policy{Noun: "speaker address"}
	device  = Policy{Noun: "device host", AllowPort: true}
)

func TestAcceptsLANAddresses(t *testing.T) {
	for _, h := range []string{
		"192.168.1.50", "10.0.0.7", "172.16.4.9",
		"tasmota-1234.local", "living-room", "SPEAKER.lan",
	} {
		if err := speaker.Validate(h); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", h, err)
		}
	}
}

// A consequence of forbidding the colon rather than parsing it: a policy
// with AllowPort=false cannot express an IPv6 literal at all, so Sonos and
// KEF speakers are reachable by IPv4 or hostname only. That is how these
// bridges have always behaved; it is recorded here rather than changed,
// since fixing it means deciding how a port would be spelled alongside.
func TestIPv6LiteralsNeedAPortAwarePolicy(t *testing.T) {
	if err := speaker.Validate("2001:db8::1"); err == nil {
		t.Error("speaker.Validate accepted an IPv6 literal; the colon rule should reject it")
	}
	if err := device.Validate("2001:db8::1"); err != nil {
		t.Errorf("device.Validate(IPv6) = %v, want nil", err)
	}
}

// These are the cases the check exists for: anything that could point the
// server-side request somewhere other than the device.
func TestRejectsAddressesThatEscapeTheHostPosition(t *testing.T) {
	for _, h := range []string{
		"", "   ",
		"http://evil.test",
		"192.168.1.50/../admin",
		"192.168.1.50?x=1",
		"192.168.1.50#frag",
		"user@evil.test",
		"evil.test\\path",
		"host with spaces",
		"host\nname",
		"under_score",
	} {
		if err := speaker.Validate(h); err == nil {
			t.Errorf("Validate(%q) = nil, want an error", h)
		}
	}
}

// Loopback would target the server itself and link-local covers the cloud
// metadata endpoint, which is the classic SSRF target.
func TestRejectsSensitiveIPLiterals(t *testing.T) {
	for _, h := range []string{
		"127.0.0.1", "::1",
		"169.254.169.254", // cloud metadata
		"fe80::1",
		"0.0.0.0", "::",
		"224.0.0.1", "ff02::1",
	} {
		if err := device.Validate(h); err == nil {
			t.Errorf("Validate(%q) = nil, want an error", h)
		}
	}
}

func TestPortIsOnlyAcceptedWhenThePolicyAllowsIt(t *testing.T) {
	// Tasmota may sit on a non-default port.
	if err := device.Validate("192.168.1.50:8080"); err != nil {
		t.Errorf("device.Validate with a port = %v, want nil", err)
	}
	// Sonos is always :1400 and KEF always :80, so a colon is simply invalid
	// rather than a port to parse.
	if err := speaker.Validate("192.168.1.50:8080"); err == nil {
		t.Error("speaker.Validate accepted a port, want an error")
	}
}

func TestRejectsOutOfRangePorts(t *testing.T) {
	for _, h := range []string{"192.168.1.50:0", "192.168.1.50:70000", "192.168.1.50:abc"} {
		if err := device.Validate(h); err == nil {
			t.Errorf("Validate(%q) = nil, want an error", h)
		}
	}
}

// The noun is what makes one shared implementation serve bridges that phrase
// their errors differently.
func TestErrorsUseThePolicyNoun(t *testing.T) {
	if got := speaker.Validate("").Error(); got != "speaker address is empty" {
		t.Errorf("empty speaker error = %q", got)
	}
	if got := device.Validate("").Error(); got != "device host is empty" {
		t.Errorf("empty device error = %q", got)
	}
	if got := device.Validate("127.0.0.1").Error(); got != `device host "127.0.0.1" is not an allowed address` {
		t.Errorf("loopback error = %q", got)
	}
}
