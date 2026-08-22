package tasmota

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostsInExcludesNetworkAndBroadcast(t *testing.T) {
	_, ipnet, err := net.ParseCIDR("192.168.1.0/29")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	got := hostsIn(ipnet)
	want := []string{
		"192.168.1.1", "192.168.1.2", "192.168.1.3",
		"192.168.1.4", "192.168.1.5", "192.168.1.6",
	}
	if len(got) != len(want) {
		t.Fatalf("hostsIn returned %d addresses (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("hostsIn()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// A /31 is the two addresses network and broadcast, so subtracting both
// leaves nothing to sweep. Home LANs never look like this; the case is here
// so the enumeration returns empty rather than wrapping around.
func TestHostsInDegenerateSubnet(t *testing.T) {
	for _, cidr := range []string{"10.0.0.0/31", "10.0.0.0/32"} {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatalf("ParseCIDR(%q): %v", cidr, err)
		}
		if got := hostsIn(ipnet); len(got) != 0 {
			t.Errorf("hostsIn(%s) = %v, want no addresses", cidr, got)
		}
	}
}

func TestIPRoundTrip(t *testing.T) {
	for _, s := range []string{"192.168.1.1", "10.0.0.255", "172.16.34.7"} {
		if got := uintToIP(ipToUint(net.ParseIP(s))).String(); got != s {
			t.Errorf("round trip of %q = %q", s, got)
		}
	}
}

func TestProbeDeviceAcceptsTasmota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cmnd") != "Status" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"Status":{"DeviceName":"Hall lamp","FriendlyName":["Hall lamp"],"Topic":"tasmota_A1B2C3"}}`))
	}))
	defer srv.Close()

	dev, ok := probeDevice(context.Background(), strings.TrimPrefix(srv.URL, "http://"))
	if !ok {
		t.Fatal("probeDevice rejected a valid Tasmota status response")
	}
	if dev.Name != "Hall lamp" {
		t.Errorf("Name = %q, want %q", dev.Name, "Hall lamp")
	}
	if dev.Topic != "tasmota_A1B2C3" {
		t.Errorf("Topic = %q, want %q", dev.Topic, "tasmota_A1B2C3")
	}
}

func TestProbeDeviceFallsBackToFriendlyName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Status":{"FriendlyName":["Kitchen plug"],"Topic":"plug1"}}`))
	}))
	defer srv.Close()

	dev, ok := probeDevice(context.Background(), strings.TrimPrefix(srv.URL, "http://"))
	if !ok {
		t.Fatal("probeDevice rejected a valid Tasmota status response")
	}
	if dev.Name != "Kitchen plug" {
		t.Errorf("Name = %q, want the FriendlyName fallback", dev.Name)
	}
}

// Anything on port 80 that isn't Tasmota must be rejected — a home LAN is
// full of things that answer an HTTP GET.
func TestProbeDeviceRejectsNonTasmota(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"html page": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html><body>a router</body></html>"))
		},
		"json without topic": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"Status":{"DeviceName":"something else"}}`))
		},
		"unrelated json api": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"result":"ok","items":[]}`))
		},
		"server error": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			defer srv.Close()
			if _, ok := probeDevice(context.Background(), strings.TrimPrefix(srv.URL, "http://")); ok {
				t.Errorf("probeDevice accepted %s as a Tasmota device", name)
			}
		})
	}
}
