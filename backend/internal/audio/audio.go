// Package audio owns the pieces of HomeHub that hold live sound: the HTTP
// stream speakers pull from, the decoders that fill it, and the AirPlay sender
// that pushes to receivers which cannot pull.
//
// These are the only long-lived, closeable things in the media path. Most
// routes leave nothing running — the speaker holds the content and HomeHub is
// out of the loop the moment the command lands — but a decoded stream holds a
// subprocess, a Spotify session and a set of connected listeners, and something
// has to own that and shut it down.
//
// Everything here is built on first use and reused after. That is not
// laziness: librespot is a subprocess that a house with no cross-vendor zone
// should never start, and the stream host cannot be built until a speaker is
// registered, because its address depends on which network a speaker is on.
//
// The package reads no environment and no store. What it needs from either
// arrives through Config — as a value when it is fixed at startup, as a
// function when it can change while running.
package audio

import (
	"net/http"
	"sync"
	"time"

	"homehub/internal/airplay"
	"homehub/internal/media"
	"homehub/internal/mediabridge"
	"homehub/internal/platform/lanaddr"
	"homehub/internal/stream"
)

// Config is everything the engine needs from outside it.
type Config struct {
	// BaseURL fixes the address speakers fetch from. Empty means resolve one
	// against a registered speaker, which is right whenever the speakers
	// share a subnet with us — the normal case. Set it when they do not.
	BaseURL string

	// HTTPPort is the plain-HTTP listener the resolved address names.
	// Speakers will not fetch over a self-signed TLS listener.
	HTTPPort string

	// StreamPath is where the stream handler is mounted.
	StreamPath string

	// StartDelays spaces out when each vendor is told to play, compensating
	// for speakers that fill their buffers at different rates.
	StartDelays map[media.Vendor]time.Duration

	// SpeakerAddr returns the address of any registered speaker, for working
	// out which of our interfaces faces the LAN. Empty means nothing is
	// registered yet — in which case there is nothing to stream to either.
	SpeakerAddr func() string

	// Quality is the household's chosen decode quality. Read fresh on every
	// use rather than captured, because a household that changes it expects
	// the next thing they play to honour the change.
	Quality func() media.StreamQuality

	// Librespot configures the Spotify decoder. Absent binary is not an
	// error: it makes exactly one route unavailable and says so.
	LibrespotBin  string
	LibrespotName string
	// CacheDir is where librespot keeps its credentials and audio cache,
	// which is what makes the second start much faster than the first.
	CacheDir string

	// Qobuz resolves and signs lossless downloads. Nil disables the one
	// provider HomeHub can stream losslessly.
	Qobuz stream.Catalog

	Logf func(format string, args ...any)
}

// Engine holds the live audio runtime.
type Engine struct {
	cfg Config

	// mu guards the stream host and both decoders. They are grouped because
	// they are created and closed together, and because the quality setting
	// reaches two of the three.
	mu      sync.Mutex
	host    *stream.Host
	spot    *stream.Librespot
	qobuz   *stream.Qobuz
	bitrate int // what spot was built for; see Decoder

	// casterMu is separate on purpose. The caster is reached from inside a
	// call that already holds the store lock, while the decoder reads
	// settings under that same lock — one mutex for both would mean two
	// locks acquired in two orders. Building a caster touches nothing else.
	casterMu sync.Mutex
	caster   *airplay.Caster
}

// New returns an engine. It starts nothing; every part is built on first use.
func New(cfg Config) *Engine {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.StreamPath == "" {
		cfg.StreamPath = "/stream"
	}
	return &Engine{cfg: cfg}
}

// BaseURL is the plain-HTTP origin speakers should fetch from, or empty when
// no such address can be found.
//
// A stream has a single URL serving every listener, so unlike a Sonos event
// callback it cannot be resolved per speaker. It is resolved toward one
// registered speaker instead, which yields the LAN-facing interface.
func (e *Engine) BaseURL() string {
	if e.cfg.BaseURL != "" {
		return e.cfg.BaseURL
	}
	if e.cfg.SpeakerAddr == nil {
		return ""
	}
	speaker := e.cfg.SpeakerAddr()
	if speaker == "" {
		return "" // nothing registered yet; nothing to stream to either
	}
	base, err := lanaddr.BaseURL(speaker, e.cfg.HTTPPort)
	if err != nil {
		e.cfg.Logf("media: %v", err)
		return ""
	}
	return base
}

// StreamHost returns the HTTP stream host, creating it on first use.
//
// It returns a nil interface — not a host — when no reachable address can be
// found, because a host that handed speakers a URL they cannot fetch would
// fail later and less clearly. The media layer reports "no stream host
// configured", which is the truth.
func (e *Engine) StreamHost() media.StreamHost {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.host != nil {
		return e.host
	}
	base := e.BaseURL()
	if base == "" {
		return nil
	}
	e.host = stream.NewHost(stream.Config{
		BaseURL:     base,
		PathPrefix:  e.cfg.StreamPath,
		StartDelays: e.cfg.StartDelays,
		Logf:        e.cfg.Logf,
	})
	return e.host
}

// Handler serves the published streams to speakers.
//
// It resolves the host per request rather than capturing it when routes are
// built, because the host is created on first play and the routes are built at
// startup. Before anything has been published there is nothing to serve, and a
// 404 is the honest answer — the same one an expired stream id gets.
func (e *Engine) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		host := e.host
		e.mu.Unlock()
		if host == nil {
			http.NotFound(w, r)
			return
		}
		host.Handler().ServeHTTP(w, r)
	})
}

// Decoder returns the Spotify decoder, creating it on first use.
//
// The bitrate is baked into the subprocess's command line, so a household that
// changes its stream quality needs a new decoder. Rebuilding here rather than
// when the setting is saved keeps the decision in one place, and means a
// running decode is not cut off mid-song to improve it: the change lands on
// the next thing started.
func (e *Engine) Decoder() mediabridge.Decoder {
	bitrate := e.quality().Bitrate()

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.spot != nil && e.bitrate == bitrate {
		return e.spot
	}
	if e.spot != nil {
		if err := e.spot.Close(); err != nil {
			e.cfg.Logf("media: stopping the old decoder: %v", err)
		}
	}
	e.spot = stream.NewLibrespot(stream.LibrespotConfig{
		Binary:     e.cfg.LibrespotBin,
		DeviceName: e.cfg.LibrespotName,
		CacheDir:   e.cfg.CacheDir,
		Bitrate:    bitrate,
		Logf:       e.cfg.Logf,
	})
	e.bitrate = bitrate
	return e.spot
}

// DecoderName is what HomeHub's own decoder calls itself in Spotify's device
// list. Empty when no decoder has been created, which is also when it cannot
// be on the list.
func (e *Engine) DecoderName() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.spot == nil {
		return ""
	}
	return e.spot.DeviceName()
}

// DecoderBitrate is what the running decoder was built for, in kbps, or zero
// when there is no decoder. Diagnostic: it is the one way to see that a
// quality change actually reached the process.
func (e *Engine) DecoderBitrate() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.spot == nil {
		return 0
	}
	return e.bitrate
}

// QobuzDecoder returns the lossless decoder, building it on first use.
//
// Simpler than Decoder by exactly the thing that makes librespot awkward:
// there is no subprocess and no bitrate baked into a command line, so the
// household's stream-quality setting does not reach it and nothing ever needs
// rebuilding. No catalogue yields a nil decoder, which the provider reports as
// unconfigured rather than failing at the tap.
func (e *Engine) QobuzDecoder() mediabridge.Decoder {
	if e.cfg.Qobuz == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.qobuz == nil {
		e.qobuz = stream.NewQobuz(stream.QobuzConfig{
			Catalog: e.cfg.Qobuz,
			Logf:    e.cfg.Logf,
		})
	}
	return e.qobuz
}

// Caster returns the AirPlay sender, creating it on first use.
func (e *Engine) Caster() *airplay.Caster {
	e.casterMu.Lock()
	defer e.casterMu.Unlock()
	if e.caster == nil {
		e.caster = airplay.NewCaster(e.cfg.Logf)
	}
	return e.caster
}

// Deps is everything executing a media plan needs from this engine. One place,
// so a route added to the media layer cannot be half-wired: a call site that
// forgot the AirPlay host would fail at the moment someone plays to a
// receiver, which is the worst time to find out.
func (e *Engine) Deps() media.Deps {
	return media.Deps{
		Stream:  e.StreamHost(),
		AirPlay: e.Caster(),
		Logf:    e.cfg.Logf,
	}
}

// Close stops the decoder and any live cast. Called at shutdown.
//
// The stream host needs nothing: its listeners are HTTP responses that end
// when the server drains. A cast does, and it is the one session that keeps
// *sending* after HomeHub stops — a receiver holds no state to notice the
// silence, so it would sit on an open RTSP session that nothing will ever
// feed.
func (e *Engine) Close() {
	e.mu.Lock()
	spot := e.spot
	e.mu.Unlock()
	if spot != nil {
		if err := spot.Close(); err != nil {
			e.cfg.Logf("media: stopping the decoder: %v", err)
		}
	}

	e.casterMu.Lock()
	caster := e.caster
	e.casterMu.Unlock()
	if caster != nil {
		caster.Close()
	}
}

// Quality is the quality this engine decodes at: the household's setting,
// normalised. Callers that describe the audio chain to a user read it from
// here rather than from the store, so that what is reported and what is
// decoded cannot drift apart.
func (e *Engine) Quality() media.StreamQuality { return e.quality() }

// quality is the household's choice, defaulted for a configuration that did
// not supply one.
func (e *Engine) quality() media.StreamQuality {
	if e.cfg.Quality == nil {
		return media.StreamQuality("").Normalize()
	}
	return e.cfg.Quality().Normalize()
}
