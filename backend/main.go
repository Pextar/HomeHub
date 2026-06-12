package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"rf-socket-controller/internal/api"
	"rf-socket-controller/internal/matter"
	"rf-socket-controller/internal/mqtt"
	"rf-socket-controller/internal/push"
	"rf-socket-controller/internal/reachability"
	"rf-socket-controller/internal/rf"
	"rf-socket-controller/internal/rx"
	"rf-socket-controller/internal/scheduler"
	"rf-socket-controller/internal/sender"
	"rf-socket-controller/internal/store"
	"rf-socket-controller/internal/tasmota"
)

// lightControl applies scene brightness/colour to smart lights. It satisfies
// store.LightController and routes by protocol to the Tasmota/Matter bridges.
// RF and other protocols are no-ops (on/off only).
type lightControl struct{ matter *matter.Client }

func (l lightControl) SetLight(socket store.Socket, level *int, color string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	switch socket.Protocol {
	case "tasmota":
		return tasmota.SetState(ctx, socket.Code, tasmota.StateUpdate{Dimmer: level, Color: color})
	case "matter", "matter-thread":
		if l.matter == nil || !l.matter.Enabled() {
			return nil
		}
		return l.matter.SetState(ctx, socket.Code, matter.StateUpdate{Level: level, Color: color})
	}
	return nil
}

// nexaScriptPath locates the lgpio-backed Nexa transmitter helper.
// NEXA_TX_SCRIPT overrides it; otherwise we look for nexa_tx.py next to
// the working directory (where deploy-pi.sh places it). An empty result
// means the Nexa path runs in simulation mode — fine for laptop dev.
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

func main() {
	resetAdmin := flag.Bool("reset-admin", false, "reset the first admin's password from AUTH_PASS and exit")
	flag.Parse()

	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("failed to create data directory %q: %v", dataDir, err)
	}

	if *resetAdmin {
		newPass := os.Getenv("AUTH_PASS")
		if newPass == "" {
			log.Fatal("AUTH_PASS is not set — export it before running --reset-admin")
		}
		st := store.New(dataDir, nil)
		if err := st.Load(); err != nil {
			log.Fatalf("failed to load data: %v", err)
		}
		st.Mu.Lock()
		var admin *store.User
		for _, u := range st.Users {
			if u.Admin {
				admin = u
				break
			}
		}
		if admin == nil {
			st.Mu.Unlock()
			log.Fatal("no admin user found — delete data/users.json and restart to re-seed from AUTH_USER/AUTH_PASS")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
		if err != nil {
			st.Mu.Unlock()
			log.Fatalf("failed to hash password: %v", err)
		}
		admin.PasswordHash = string(hash)
		admin.TokenVersion++ // invalidate any sessions minted with the old password
		if err := st.Save(); err != nil {
			st.Mu.Unlock()
			log.Fatalf("failed to save: %v", err)
		}
		st.Mu.Unlock()
		fmt.Printf("Password reset for admin %q — you can now log in with the new AUTH_PASS.\n", admin.Username)
		return
	}

	matterClient := matter.FromEnv()
	if matterClient.Enabled() {
		log.Printf("Matter bridge enabled at %s", matterClient.BaseURL)
	} else {
		log.Printf("Matter bridge disabled — set MATTER_BRIDGE_URL to enable")
	}

	mqttClient := mqtt.FromEnv()
	if mqttClient.Enabled() {
		if err := mqttClient.Connect(); err != nil {
			log.Printf("MQTT: initial connect to %s failed: %v (retrying in background)", mqttClient.BrokerURL, err)
		} else {
			log.Printf("MQTT broker connected at %s", mqttClient.BrokerURL)
		}
	} else {
		log.Printf("MQTT disabled — set MQTT_BROKER_URL to enable")
	}

	st := store.New(dataDir, &sender.Multi{
		RF:     rf.Sender{NexaScript: nexaScriptPath()},
		Matter: matterClient,
		MQTT:   mqttClient,
	})
	st.Light = lightControl{matter: matterClient}
	if err := st.Load(); err != nil {
		log.Fatalf("failed to load data: %v", err)
	}

	secret, err := api.LoadOrCreateSessionSecret(dataDir)
	if err != nil {
		log.Fatalf("failed to load session secret: %v", err)
	}

	// Set up Web Push notifications. VAPID keys are generated on first run
	// and reused across restarts.
	vapidKeys, err := push.LoadOrGenerateVAPIDKeys(dataDir)
	if err != nil {
		log.Fatalf("failed to load/generate VAPID keys: %v", err)
	}
	subStore, err := push.NewSubscriptionStore(dataDir)
	if err != nil {
		log.Fatalf("failed to load push subscriptions: %v", err)
	}
	pushSvc := &push.Service{
		VAPIDPublicKey:  vapidKeys.PublicKey,
		VAPIDPrivateKey: vapidKeys.PrivateKey,
		Subs:            subStore,
		// GetUserPrefs reads user prefs under a read lock so it is safe to
		// call from goroutines spawned by the push callbacks.
		GetUserPrefs: func() []push.UserPrefs {
			st.Mu.RLock()
			defer st.Mu.RUnlock()
			out := make([]push.UserPrefs, 0, len(st.Users))
			for _, u := range st.Users {
				muted := make(map[string]bool, len(u.NotifPrefs.MutedSocketIDs)+len(u.NotifPrefs.MutedSensorIDs))
				for _, id := range u.NotifPrefs.MutedSocketIDs {
					muted[id] = true
				}
				for _, id := range u.NotifPrefs.MutedSensorIDs {
					muted[id] = true
				}
				out = append(out, push.UserPrefs{
					ID:            u.ID,
					SensorAlerts:  u.NotifPrefs.SensorAlerts,
					StateChanges:  u.NotifPrefs.StateChanges,
					ScheduleFired: u.NotifPrefs.ScheduleFired,
					DeviceOffline: u.NotifPrefs.DeviceOffline,
					QuietHours:    u.NotifPrefs.QuietHours,
					QuietStart:    u.NotifPrefs.QuietStart,
					QuietEnd:      u.NotifPrefs.QuietEnd,
					MutedIDs:      muted,
				})
			}
			return out
		},
	}
	log.Printf("Web Push notifications enabled (VAPID public key: %s...)", vapidKeys.PublicKey[:min(12, len(vapidKeys.PublicKey))])

	server := &api.Server{
		Store:         st,
		Matter:        matterClient,
		MQTT:          mqttClient,
		Push:          pushSvc,
		AuthUser:      os.Getenv("AUTH_USER"),
		AuthPass:      os.Getenv("AUTH_PASS"),
		SessionSecret: secret,
		SPADir:        "./frontend/dist",
	}

	// Seed an admin from AUTH_USER/AUTH_PASS on first run (no-op once any
	// user exists). Keeps legacy single-credential setups working.
	if err := server.Bootstrap(); err != nil {
		log.Fatalf("failed to bootstrap admin user: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := server.Handler()
	httpSrv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Optional HTTPS listener. Required if the user wants the QR scanner
	// to work in mobile browsers — getUserMedia is blocked outside secure
	// contexts. Enable by setting HTTPS_PORT (e.g. 8443); a self-signed
	// cert is generated on first start and reused across restarts.
	var httpsSrv *http.Server
	if httpsPort := os.Getenv("HTTPS_PORT"); httpsPort != "" {
		certPath := filepath.Join(dataDir, "tls", "cert.pem")
		keyPath := filepath.Join(dataDir, "tls", "key.pem")
		if p := os.Getenv("TLS_CERT_FILE"); p != "" {
			certPath = p
		}
		if p := os.Getenv("TLS_KEY_FILE"); p != "" {
			keyPath = p
		}
		cert, err := api.LoadOrCreateTLSCert(certPath, keyPath, nil)
		if err != nil {
			log.Fatalf("tls: %v", err)
		}
		httpsSrv = &http.Server{
			Addr:              ":" + httpsPort,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			},
		}
	}

	schedCtx, stopScheduler := context.WithCancel(context.Background())
	go scheduler.Run(schedCtx, st, pushSvc)
	go rx.FromEnv().Run(schedCtx, st)
	if serial := rx.SerialFromEnv(); serial != nil {
		go serial.Run(schedCtx, st)
	}
	go mqtt.SensorListener{Client: mqttClient}.Run(schedCtx, st)
	go reachability.Run(schedCtx, st, matterClient, pushSvc)

	go func() {
		log.Printf("RF Socket Controller listening on http://:%s", port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()
	if httpsSrv != nil {
		go func() {
			log.Printf("HTTPS also listening on https://:%s (self-signed)", httpsSrv.Addr[1:])
			// ListenAndServeTLS with empty cert/key paths picks the cert
			// from TLSConfig.Certificates that we already populated.
			if err := httpsSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("https server: %v", err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	stopScheduler()
	mqttClient.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	if httpsSrv != nil {
		if err := httpsSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("https graceful shutdown failed: %v", err)
		}
	}
	// Persist any readings still sitting in the debounce window.
	st.FlushSensorSaves()
	log.Println("bye")
}
