package tasmota

import "testing"

func TestValidateHost(t *testing.T) {
	allowed := []string{
		"192.168.1.50",
		"192.168.1.50:8080",
		"10.0.0.5",
		"172.16.3.4",
		"tasmota-1234.local",
		"plug.lan",
		"tasmota-1234.local:80",
	}
	for _, h := range allowed {
		if err := ValidateHost(h); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", h, err)
		}
	}

	rejected := []string{
		"",
		"127.0.0.1",            // loopback
		"169.254.169.254",      // cloud metadata (link-local)
		"0.0.0.0",              // unspecified
		"224.0.0.1",            // multicast
		"192.168.1.5/../admin", // path escape
		"192.168.1.5?x=1",      // query escape
		"evil.com/redirect",    // path escape via hostname
		"user@192.168.1.5",     // userinfo
		"192.168.1.5:99999",    // port out of range
		"192.168.1.5:abc",      // non-numeric port
		"http://192.168.1.5",   // embedded scheme
		"192.168.1.5 8080",     // whitespace
	}
	for _, h := range rejected {
		if err := ValidateHost(h); err == nil {
			t.Errorf("ValidateHost(%q) = nil, want error", h)
		}
	}
}
