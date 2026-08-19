// Package app is HomeHub's composition root: the one place that knows which
// subsystems exist, what each of them needs, and in what order they start and
// stop.
//
// Nothing else in the tree assembles anything. A package here builds only
// itself and declares its dependencies as fields or interfaces; this package
// satisfies them. That is what keeps the dependency arrows pointing one way —
// the HTTP layer does not own the scheduler's lifetime, and the store does not
// know a speaker exists.
//
// cmd/homehub is deliberately thin: flags in, one of the three entry points
// below out. Everything a reader needs to understand how the server is put
// together is in this file.
package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"homehub/internal/announce"
	"homehub/internal/api"
	"homehub/internal/audio"
	"homehub/internal/llm"
	"homehub/internal/matter"
	"homehub/internal/media"
	"homehub/internal/mqtt"
	"homehub/internal/music"
	"homehub/internal/push"
	"homehub/internal/qobuz"
	"homehub/internal/reachability"
	"homehub/internal/rf"
	"homehub/internal/rx"
	"homehub/internal/scheduler"
	"homehub/internal/sender"
	"homehub/internal/speakermon"
	"homehub/internal/spotify"
	"homehub/internal/store"
)

// shutdownGrace bounds the graceful HTTP shutdown. Long enough for an
// in-flight zone play to finish answering, short enough that a restart during
// an upgrade doesn't look hung.
const shutdownGrace = 10 * time.Second

// sonosReleaseGrace bounds waiting for the Sonos monitor to hand its event
// subscriptions back. See Run for why that happens before the HTTP server
// stops rather than after.
const sonosReleaseGrace = 5 * time.Second

// App is the assembled server: every subsystem, wired, with one place that
// starts them and one that shuts them down in the right order.
type App struct {
	cfg   Config
	store *store.Store
	api   *api.Server

	monitors *speakermon.Monitors
	music    *music.Service

	matter *matter.Client
	mqtt   *mqtt.Client
	push   *push.Service

	// handler is shared by both listeners: the HTTPS listener is the same
	// application on a second port, not a second application.
	handler http.Handler
	http    *http.Server
	https   *http.Server // nil unless HTTPSPort is set
}

// New builds every subsystem and wires them together. It reads and writes
// disk (state, keys, certificates) but starts nothing: Run does that.
func New(cfg Config) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("creating data directory %q: %w", cfg.DataDir, err)
	}

	a := &App{cfg: cfg}

	// ── Device transports ────────────────────────────────────────────────
	// Each reports whether it is configured rather than failing: a house
	// with no Matter bridge is a supported house, not a broken one.
	a.matter = matter.FromEnv()
	if a.matter.Enabled() {
		log.Printf("Matter bridge enabled at %s", a.matter.BaseURL)
	} else {
		log.Printf("Matter bridge disabled — set MATTER_BRIDGE_URL to enable")
	}

	a.mqtt = mqtt.FromEnv()
	if a.mqtt.Enabled() {
		if err := a.mqtt.Connect(); err != nil {
			log.Printf("MQTT: initial connect to %s failed: %v (retrying in background)", a.mqtt.BrokerURL, err)
		} else {
			log.Printf("MQTT broker connected at %s", a.mqtt.BrokerURL)
		}
	} else {
		log.Printf("MQTT disabled — set MQTT_BROKER_URL to enable")
	}

	llmClient := llm.FromEnv()
	if llmClient.Enabled() {
		log.Printf("LLM assistant enabled (%s at %s)", llmClient.Model, llmClient.BaseURL)
	} else {
		log.Printf("LLM assistant disabled — set LLM_ENABLED=true to enable")
	}

	// ── State ────────────────────────────────────────────────────────────
	// One dispatcher carries both directions the store sends in — on/off
	// and brightness/colour — so a protocol is wired up once.
	devices := &sender.Multi{
		RF:     rf.Sender{NexaScript: cfg.NexaScript},
		Matter: a.matter,
		MQTT:   a.mqtt,
	}
	a.store = store.New(cfg.DataDir, devices)
	a.store.Light = devices
	if err := a.store.Load(); err != nil {
		return nil, fmt.Errorf("loading data: %w", err)
	}

	// ── Notifications ────────────────────────────────────────────────────
	var err error
	if a.push, err = newPushService(cfg.DataDir, a.store); err != nil {
		return nil, err
	}

	// ── Music services ───────────────────────────────────────────────────
	// Both are configured from the UI and keep their credentials in
	// dataDir, so both can fail to load and neither can fail to exist.
	spotifyClient, err := spotify.New(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("loading spotify state: %w", err)
	}
	qobuzClient, err := qobuz.New(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("loading qobuz state: %w", err)
	}

	// ── Live audio ───────────────────────────────────────────────────────
	// The engine reads nothing for itself: the two things that can change
	// while the house runs — where the speakers are, and how hard to
	// compress — arrive as functions, and the rest as settled values.
	audioEngine := audio.New(audio.Config{
		BaseURL:       cfg.StreamURL,
		HTTPPort:      cfg.HTTPPort,
		StreamPath:    api.StreamPath,
		StartDelays:   cfg.StreamDelays,
		SpeakerAddr:   a.store.AnySpeakerAddr,
		Quality:       func() media.StreamQuality { return streamQuality(a.store) },
		LibrespotBin:  cfg.LibrespotBin,
		LibrespotName: cfg.LibrespotName,
		CacheDir:      cfg.LibrespotCache,
		Qobuz:         qobuzCatalog(qobuzClient),
		Logf:          log.Printf,
	})

	// ── Speakers ─────────────────────────────────────────────────────────
	// One cached view of what every speaker is doing, asked once for the
	// whole process however many phones are watching it.
	a.monitors = speakermon.New(speakermon.Config{
		Store:     a.store,
		OnChange:  func() { a.api.SpeakersChanged() },
		HTTPPort:  cfg.HTTPPort,
		EventPath: api.SonosEventPath,
		Logf:      log.Printf,
	})

	// ── Music ────────────────────────────────────────────────────────────
	// Where a household means when it names a room, what can play there,
	// and what stays running once it does. Everything above it — the HTTP
	// handlers, the sleep timer, a scene's music step — asks here rather
	// than assembling the answer from the store and the bridges itself.
	a.music = music.New(music.Config{
		Store:    a.store,
		Speakers: a.monitors,
		Audio:    audioEngine,
		Spotify:  spotifyClient,
		Qobuz:    qobuzClient,
		Logf:     log.Printf,
	})

	// Announcements share the audio engine's address discovery: both are
	// "somewhere a speaker can fetch from", and resolving it twice would
	// mean two ways to be wrong on a multi-homed box.
	announcer := &announce.Service{
		BaseURL:    audioEngine.BaseURL,
		PathPrefix: api.AnnouncePath,
	}

	// ── HTTP ─────────────────────────────────────────────────────────────
	secret, err := api.LoadOrCreateSessionSecret(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("loading session secret: %w", err)
	}

	a.api = &api.Server{
		Store:         a.store,
		Audio:         audioEngine,
		Announce:      announcer,
		Speakers:      a.monitors,
		Music:         a.music,
		Matter:        a.matter,
		MQTT:          a.mqtt,
		LLM:           llmClient,
		Push:          a.push,
		Spotify:       spotifyClient,
		Qobuz:         qobuzClient,
		AuthUser:      cfg.AuthUser,
		AuthPass:      cfg.AuthPass,
		SessionSecret: secret,
		SPADir:        cfg.SPADir,
		HTTPPort:      cfg.HTTPPort,
	}

	// Seed an admin from AUTH_USER/AUTH_PASS on first run (no-op once any
	// user exists). Keeps legacy single-credential setups working.
	if err := a.api.Bootstrap(); err != nil {
		return nil, fmt.Errorf("bootstrapping the admin user: %w", err)
	}

	a.handler = a.api.Handler()
	if err := a.buildListeners(); err != nil {
		return nil, err
	}
	return a, nil
}

// buildListeners prepares the HTTP server, and the HTTPS one when configured.
func (a *App) buildListeners() error {
	a.http = newHTTPServer(":"+a.cfg.HTTPPort, a.handler)
	if a.cfg.HTTPSPort == "" {
		return nil
	}
	// Optional HTTPS listener. Required if the household wants the QR
	// scanner to work in mobile browsers — getUserMedia is blocked outside
	// secure contexts. A self-signed cert is generated on first start and
	// reused across restarts.
	cert, err := api.LoadOrCreateTLSCert(a.cfg.TLSCert, a.cfg.TLSKey, nil)
	if err != nil {
		return fmt.Errorf("tls: %w", err)
	}
	a.https = newHTTPServer(":"+a.cfg.HTTPSPort, a.handler)
	a.https.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	return nil
}

// newHTTPServer applies the timeouts both listeners share. They exist to stop
// a stalled client from holding a connection open indefinitely; the write
// timeout is the one that matters most, since a zone play can be slow.
func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// Run starts every background subsystem and both listeners, then blocks until
// ctx is cancelled or a listener fails, and shuts everything down.
func (a *App) Run(ctx context.Context) error {
	// Background subsystems share one context, cancelled first at shutdown
	// so nothing new is started while the listeners drain. It is not derived
	// from ctx: shutdown has an order, and deriving would cancel everything
	// at once.
	bg, stopBackground := context.WithCancel(context.Background())
	a.startBackground(bg)

	// Sonos change notifications get their own context, cancelled before the
	// HTTP server: the subscriptions have to be released while the speakers
	// can still reach us, or they keep posting to a dead callback until
	// their grants expire.
	sonosCtx, stopSonosEvents := context.WithCancel(context.Background())
	sonosDone := make(chan struct{})
	go func() {
		defer close(sonosDone)
		a.monitors.RunSonos(sonosCtx)
	}()

	// A listener that cannot bind is fatal — the alternative is a server
	// that logs a failure and then sits there answering nothing.
	fatal := make(chan error, 2)
	go func() {
		log.Printf("HomeHub listening on http://:%s", a.cfg.HTTPPort)
		if err := a.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal <- fmt.Errorf("http server: %w", err)
		}
	}()
	if a.https != nil {
		go func() {
			log.Printf("HTTPS also listening on https://:%s (self-signed)", a.cfg.HTTPSPort)
			// Empty cert/key paths make ListenAndServeTLS use the
			// certificate already in TLSConfig.
			if err := a.https.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				fatal <- fmt.Errorf("https server: %w", err)
			}
		}()
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-fatal:
	}

	log.Println("shutting down...")
	a.shutdown(stopBackground, stopSonosEvents, sonosDone)
	log.Println("bye")
	return runErr
}

// startBackground launches everything that runs on its own clock. All of it
// rides the same context, so one cancel stops the lot.
func (a *App) startBackground(ctx context.Context) {
	// Schedules, timers and the automation engine, on a 5-second tick.
	go scheduler.Run(ctx, a.store, a.push)

	// 433 MHz sensor readings, over the air and over serial.
	go rx.FromEnv().Run(ctx, a.store)
	if serial := rx.SerialFromEnv(); serial != nil {
		go serial.Run(ctx, a.store)
	}
	go mqtt.SensorListener{Client: a.mqtt}.Run(ctx, a.store)

	// Wi-Fi/Matter device reachability, with push on drop and recovery.
	go reachability.Run(ctx, a.store, a.matter, a.push)

	// KEF speakers are polled rather than subscribed to — their local API
	// has no callback — so unlike Sonos this one has nothing to release.
	go a.monitors.RunKEF(ctx)

	// "Continue play similar": tops a group's queue up as it reaches the
	// last queued track, and picks a room back up if it fell silent anyway.
	go a.api.RunAutoplay(ctx)

	// Music that starts and stops on its own: wake-ups and sleep timers.
	// Cancelling the context also stops any volume ramp in flight — a fade
	// left running past shutdown would leave a room at an interim volume
	// with nothing left to move it.
	go a.api.RunMusicTimers(ctx)
}

// shutdown reverses New, in the order the pieces depend on each other.
func (a *App) shutdown(stopBackground, stopSonosEvents context.CancelFunc, sonosDone <-chan struct{}) {
	stopBackground()

	// Before the HTTP server stops: the speakers must be able to reach our
	// callback while their subscriptions are being released.
	stopSonosEvents()
	select {
	case <-sonosDone:
	case <-time.After(sonosReleaseGrace):
		log.Println("sonos: timed out releasing event subscriptions")
	}

	a.mqtt.Close()

	// Release any live zone playback and stop the decoder. The stream route
	// holds the account's Spotify session, and leaving it behind would keep
	// the user's Spotify pointed at a HomeHub that has stopped.
	a.music.Close()

	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := a.http.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	if a.https != nil {
		if err := a.https.Shutdown(ctx); err != nil {
			log.Printf("https graceful shutdown failed: %v", err)
		}
	}

	// Last, because a reading can still arrive while the listeners drain:
	// persist anything still sitting in the debounce window.
	a.store.FlushSensorSaves()
}
