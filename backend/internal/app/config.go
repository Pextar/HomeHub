package app

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"homehub/internal/media"
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

	// StreamURL fixes the address speakers fetch audio from. Empty resolves
	// one against a registered speaker, which is right whenever they share a
	// subnet with us — the normal case. Set it when they do not.
	StreamURL string

	// StreamDelays spaces out when each vendor is told to start playing, so
	// speakers that fill their buffers at different rates come in together.
	//
	// Empty by default. The right values depend on the speakers, the network
	// and the firmware; inventing them would be worse than leaving a zone a
	// few hundred milliseconds apart for someone who can actually hear it to
	// tune. See docs/MEDIA-PROTOCOL.md.
	StreamDelays map[media.Vendor]time.Duration

	// LibrespotBin is the Spotify decoder's executable, resolved on PATH
	// when empty. LibrespotName is what it calls itself in Spotify's device
	// list, and LibrespotCache is where it keeps credentials and audio —
	// which is what makes the second start much faster than the first.
	LibrespotBin   string
	LibrespotName  string
	LibrespotCache string
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

	cfg.StreamURL = strings.TrimRight(strings.TrimSpace(os.Getenv("HOMEHUB_STREAM_URL")), "/")
	cfg.StreamDelays = streamDelaysFromEnv()
	cfg.LibrespotBin = strings.TrimSpace(os.Getenv("HOMEHUB_LIBRESPOT_BIN"))
	cfg.LibrespotName = strings.TrimSpace(os.Getenv("HOMEHUB_LIBRESPOT_NAME"))
	cfg.LibrespotCache = filepath.Join(cfg.DataDir, "librespot")
	return cfg
}

// streamDelaysFromEnv reads per-vendor start compensation. An unparseable
// value is logged and ignored rather than fatal: it is a tuning knob, and a
// house should not fail to boot over a typo in one.
func streamDelaysFromEnv() map[media.Vendor]time.Duration {
	out := map[media.Vendor]time.Duration{}
	for _, v := range []struct {
		vendor media.Vendor
		env    string
	}{
		{media.VendorSonos, "HOMEHUB_STREAM_DELAY_SONOS"},
		{media.VendorKEF, "HOMEHUB_STREAM_DELAY_KEF"},
	} {
		raw := strings.TrimSpace(os.Getenv(v.env))
		if raw == "" {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 {
			log.Printf("media: ignoring %s=%q: not a valid duration", v.env, raw)
			continue
		}
		out[v.vendor] = d
	}
	return out
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
