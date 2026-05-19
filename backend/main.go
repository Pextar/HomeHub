package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"rf-socket-controller/internal/api"
	"rf-socket-controller/internal/matter"
	"rf-socket-controller/internal/rf"
	"rf-socket-controller/internal/rx"
	"rf-socket-controller/internal/scheduler"
	"rf-socket-controller/internal/sender"
	"rf-socket-controller/internal/store"
)

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
	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("failed to create data directory %q: %v", dataDir, err)
	}

	matterClient := matter.FromEnv()
	if matterClient.Enabled() {
		log.Printf("Matter bridge enabled at %s", matterClient.BaseURL)
	} else {
		log.Printf("Matter bridge disabled — set MATTER_BRIDGE_URL to enable")
	}

	st := store.New(dataDir, &sender.Multi{
		RF:     rf.Sender{NexaScript: nexaScriptPath()},
		Matter: matterClient,
	})
	if err := st.Load(); err != nil {
		log.Fatalf("failed to load data: %v", err)
	}

	secret, err := api.LoadOrCreateSessionSecret(dataDir)
	if err != nil {
		log.Fatalf("failed to load session secret: %v", err)
	}

	server := &api.Server{
		Store:         st,
		Matter:        matterClient,
		AuthUser:      os.Getenv("AUTH_USER"),
		AuthPass:      os.Getenv("AUTH_PASS"),
		SessionSecret: secret,
		SPADir:        "./frontend/dist",
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
	go scheduler.Run(schedCtx, st)
	go rx.FromEnv().Run(schedCtx, st)

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
	log.Println("bye")
}
