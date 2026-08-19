package app

import (
	"os"
	"path/filepath"
	"strings"
)

// Config is everything HomeHub reads from its environment, resolved once.
//
// It exists so that "what can be configured" is answerable by reading one
// struct rather than by grepping for os.Getenv across twenty packages. The
// subsystems take their settings as plain fields; only this file knows the
// values came from the environment, which is also what lets a test build an
// App without one.
//
// Integrations that are entirely optional — Matter, MQTT, the LLM — keep their
// own FromEnv constructors in their own packages. Their configuration is
// theirs, not the application's, and duplicating it here would mean two places
// to change when a bridge grows a setting.
type Config struct {
	// DataDir holds every JSON file, key and certificate. Created on start.
	DataDir string

	// SPADir is the built Svelte app, served at "/".
	SPADir string

	// HTTPPort is the plain-HTTP listener. It is not optional even when TLS
	// is up: speakers post their event callbacks to it and fetch audio from
	// it, and they will not use a self-signed certificate to do either.
	HTTPPort string

	// HTTPSPort, when set, adds a TLS listener on a self-signed certificate.
	// Needed for the QR scanner, which browsers block outside a secure
	// context. Empty means HTTP only.
	HTTPSPort string
	// TLSCert and TLSKey are where that certificate lives. Generated on
	// first start when absent.
	TLSCert string
	TLSKey  string

	// AuthUser and AuthPass seed the first admin on a fresh install, and
	// remain the owner's permanent credential for iOS Shortcuts. Empty
	// leaves the house unauthenticated until a user is created.
	AuthUser string
	AuthPass string

	// NexaScript is the lgpio-backed 433 MHz transmitter helper. Empty runs
	// the Nexa path in simulation mode, which is what a laptop wants.
	NexaScript string
}

// FromEnv resolves the configuration, applying the defaults a fresh checkout
// should just work with.
func FromEnv() Config {
	cfg := Config{
		DataDir:    "./data",
		SPADir:     "./frontend/dist",
		HTTPPort:   envOr("PORT", "8080"),
		HTTPSPort:  os.Getenv("HTTPS_PORT"),
		AuthUser:   os.Getenv("AUTH_USER"),
		AuthPass:   os.Getenv("AUTH_PASS"),
		NexaScript: nexaScriptPath(),
	}
	cfg.TLSCert = envOr("TLS_CERT_FILE", filepath.Join(cfg.DataDir, "tls", "cert.pem"))
	cfg.TLSKey = envOr("TLS_KEY_FILE", filepath.Join(cfg.DataDir, "tls", "key.pem"))
	return cfg
}

// envOr reads an environment variable, falling back to def when it is unset
// or blank. Blank counts as unset: an empty PORT= in a systemd unit is a
// mistake, not a request for an empty port.
func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// nexaScriptPath locates the lgpio-backed Nexa transmitter helper.
// NEXA_TX_SCRIPT overrides it; otherwise we look for nexa_tx.py next to the
// working directory (where deploy-pi.sh places it). An empty result means the
// Nexa path runs in simulation mode — fine for laptop dev.
func nexaScriptPath() string {
	if p := os.Getenv("NEXA_TX_SCRIPT"); p != "" {
		return p
	}
	if _, err := os.Stat("nexa_tx.py"); err == nil {
		if abs, err := filepath.Abs("nexa_tx.py"); err == nil {
			return abs
		}
		return "nexa_tx.py"
	}
	return ""
}
