package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"rf-socket-controller/internal/api"
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

	// Build an empty store first so we can pass its Settings pointer into the
	// multi-protocol sender; the pointer stays valid after Load populates it.
	st := store.New(dataDir, nil)
	if err := st.Load(); err != nil {
		log.Fatalf("failed to load data: %v", err)
	}
	st.RF = &sender.Multi{
		RF:       rf.Sender{NexaScript: nexaScriptPath()},
		Settings: st.Settings,
	}

	secret, err := api.LoadOrCreateSessionSecret(dataDir)
	if err != nil {
		log.Fatalf("failed to load session secret: %v", err)
	}

	server := &api.Server{
		Store:         st,
		AuthUser:      os.Getenv("AUTH_USER"),
		AuthPass:      os.Getenv("AUTH_PASS"),
		SessionSecret: secret,
		SPADir:        "./frontend/dist",
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	schedCtx, stopScheduler := context.WithCancel(context.Background())
	go scheduler.Run(schedCtx, st)
	go rx.FromEnv().Run(schedCtx, st)

	go func() {
		log.Printf("RF Socket Controller listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	stopScheduler()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	log.Println("bye")
}
