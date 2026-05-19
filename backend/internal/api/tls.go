// TLS helper for the optional HTTPS listener.
//
// Self-hosted home automation often runs over a LAN where a real CA-signed
// certificate isn't an option. We auto-generate a long-lived self-signed
// cert on first start and reuse it on every restart. Browsers will warn
// about the cert the first time, but the rest of the stack — fetch from
// the PWA, getUserMedia for QR scanning, service workers — only cares
// that the page was served over HTTPS at all.
package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// LoadOrCreateTLSCert returns a tls.Certificate from certPath/keyPath if
// they exist, otherwise generates a new self-signed cert valid for ten
// years and writes it to those paths so the next start is idempotent.
//
// extraHosts is added to the cert's SAN entries alongside the system
// hostname, "localhost", and 127.0.0.1 / ::1, so the cert works regardless
// of which name or IP the user hits the server with.
func LoadOrCreateTLSCert(certPath, keyPath string, extraHosts []string) (tls.Certificate, error) {
	if fileExists(certPath) && fileExists(keyPath) {
		return tls.LoadX509KeyPair(certPath, keyPath)
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return tls.Certificate{}, fmt.Errorf("create cert dir: %w", err)
	}
	cert, key, err := generateSelfSigned(extraHosts)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		return tls.Certificate{}, fmt.Errorf("write cert: %w", err)
	}
	// Private key is readable only by the owner — defence in depth on
	// shared boxes; the Pi runs as a single user anyway.
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("write key: %w", err)
	}
	return tls.X509KeyPair(cert, key)
}

func generateSelfSigned(extraHosts []string) (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}
	hosts := map[string]struct{}{
		"localhost": {},
	}
	for _, h := range extraHosts {
		if h != "" {
			hosts[h] = struct{}{}
		}
	}
	if name, err := os.Hostname(); err == nil && name != "" {
		hosts[name] = struct{}{}
		hosts[name+".local"] = struct{}{} // mDNS / Bonjour
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "rf-socket-controller"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	for h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	// Discover this host's outbound IP and add it as a SAN so connecting
	// via e.g. https://192.168.1.50:8443 doesn't trigger a name-mismatch
	// warning on top of the self-signed warning.
	for _, ip := range localIPs() {
		tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create cert: %w", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal key: %w", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// localIPs returns the non-loopback IPv4/IPv6 addresses bound on this host.
// Best-effort: any error simply returns an empty list, leaving the cert
// without an IP SAN — the user can still reach it by hostname.
func localIPs() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		out = append(out, ipNet.IP)
	}
	return out
}
