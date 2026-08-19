// Package lanaddr answers one question: which of this host's addresses can a
// given device on the LAN reach us at?
//
// It matters because a home server is usually multi-homed — a Pi on Wi-Fi and
// Ethernet, a VPN, Docker's bridge — and all but one of its addresses are
// unreachable from any given speaker. Three subsystems need the answer and
// would each get it wrong differently: Sonos event callbacks, the audio stream
// speakers pull from, and announcement clips.
package lanaddr

import (
	"fmt"
	"net"
	"strconv"
)

// probePort is the port the route lookup dials. Any port would do — dialling
// UDP sends no packet, it only asks the kernel which route it would take — so
// this is simply a plausible number to hand net.Dial. 1400 is the port Sonos
// speakers listen on, which is where the lookup was first needed.
const probePort = 1400

// defaultPort is the listener a device is pointed at when none is given.
const defaultPort = "8080"

// For returns the local address the kernel would use to reach ip.
//
// Unlike "the first non-loopback interface" this is correct on a multi-homed
// host, and it is free: no packet leaves the machine.
func For(ip string) (string, error) {
	conn, err := net.Dial("udp", net.JoinHostPort(ip, strconv.Itoa(probePort)))
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil || addr.IP.IsUnspecified() {
		return "", fmt.Errorf("no route to %s", ip)
	}
	return addr.IP.String(), nil
}

// BaseURL is For rendered as the origin a device should fetch from, e.g.
// "http://192.168.1.10:8080".
//
// Plain HTTP, and not negotiable: speakers will not present a client
// certificate and many will not accept a self-signed one. Everything a device
// fetches from us names the HTTP listener, even when TLS is also up — which is
// why the port is passed in rather than inferred from the request that
// happened to ask.
func BaseURL(deviceIP, port string) (string, error) {
	local, err := For(deviceIP)
	if err != nil {
		return "", fmt.Errorf("no local address can reach %s: %w", deviceIP, err)
	}
	if port == "" {
		port = defaultPort
	}
	return "http://" + net.JoinHostPort(local, port), nil
}
