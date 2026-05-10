package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rf-socket-controller/internal/api"
	"rf-socket-controller/internal/rf"
	"rf-socket-controller/internal/scheduler"
	"rf-socket-controller/internal/store"
)

func main() {
	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("failed to create data directory %q: %v", dataDir, err)
	}

	st := store.New(dataDir, rf.Sender{})
	if err := st.Load(); err != nil {
		log.Fatalf("failed to load data: %v", err)
	}

	server := &api.Server{
		Store:    st,
		AuthUser: os.Getenv("AUTH_USER"),
		AuthPass: os.Getenv("AUTH_PASS"),
		SPADir:   "./frontend/dist",
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
