// Package api exposes the HTTP surface (REST + SPA host) on top of an
// injected *store.Store. The Server type is the root: configure its
// fields, then call Handler() to build a fully-wired http.Handler.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

// Server wires HTTP handlers to a Store.
type Server struct {
	Store    *store.Store
	AuthUser string
	AuthPass string
	SPADir   string // path to the built Svelte app (e.g. "./frontend/dist")
}

// Handler returns the configured router with logging, optional basic
// auth, the API routes, the SPA fallback and CORS — in that order.
func (s *Server) Handler() http.Handler {
	r := mux.NewRouter()
	r.Use(loggingMiddleware)

	if s.AuthUser != "" && s.AuthPass != "" {
		r.Use(basicAuthMiddleware(s.AuthUser, s.AuthPass))
		log.Printf("HTTP basic auth enabled for user %q", s.AuthUser)
	} else {
		log.Printf("HTTP basic auth DISABLED — set AUTH_USER and AUTH_PASS to enable")
	}

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/health", s.getHealth).Methods("GET")

	api.HandleFunc("/sockets", s.getSockets).Methods("GET")
	api.HandleFunc("/sockets", s.createSocket).Methods("POST")
	api.HandleFunc("/sockets/all/on", s.bulkSetState(true)).Methods("POST")
	api.HandleFunc("/sockets/all/off", s.bulkSetState(false)).Methods("POST")
	api.HandleFunc("/sockets/{id}", s.getSocket).Methods("GET")
	api.HandleFunc("/sockets/{id}", s.updateSocket).Methods("PUT")
	api.HandleFunc("/sockets/{id}", s.deleteSocket).Methods("DELETE")
	api.HandleFunc("/sockets/{id}/toggle", s.toggleSocket).Methods("POST")
	api.HandleFunc("/sockets/{id}/on", s.turnOn).Methods("POST")
	api.HandleFunc("/sockets/{id}/off", s.turnOff).Methods("POST")
	api.HandleFunc("/sockets/{id}/timer", s.createSocketTimer).Methods("POST")

	api.HandleFunc("/rooms", s.getRooms).Methods("GET")
	api.HandleFunc("/rooms/{room}/on", s.roomSetState(true)).Methods("POST")
	api.HandleFunc("/rooms/{room}/off", s.roomSetState(false)).Methods("POST")

	api.HandleFunc("/schedules", s.getSchedules).Methods("GET")
	api.HandleFunc("/schedules", s.createSchedule).Methods("POST")
	api.HandleFunc("/schedules/{id}", s.updateSchedule).Methods("PUT")
	api.HandleFunc("/schedules/{id}", s.deleteSchedule).Methods("DELETE")

	api.HandleFunc("/groups", s.getGroups).Methods("GET")
	api.HandleFunc("/groups", s.createGroup).Methods("POST")
	api.HandleFunc("/groups/{id}", s.getGroup).Methods("GET")
	api.HandleFunc("/groups/{id}", s.updateGroup).Methods("PUT")
	api.HandleFunc("/groups/{id}", s.deleteGroup).Methods("DELETE")
	api.HandleFunc("/groups/{id}/on", s.groupAction("on")).Methods("POST")
	api.HandleFunc("/groups/{id}/off", s.groupAction("off")).Methods("POST")
	api.HandleFunc("/groups/{id}/toggle", s.groupAction("toggle")).Methods("POST")

	api.HandleFunc("/scenes", s.getScenes).Methods("GET")
	api.HandleFunc("/scenes", s.createScene).Methods("POST")
	api.HandleFunc("/scenes/{id}", s.getScene).Methods("GET")
	api.HandleFunc("/scenes/{id}", s.updateScene).Methods("PUT")
	api.HandleFunc("/scenes/{id}", s.deleteScene).Methods("DELETE")
	api.HandleFunc("/scenes/{id}/activate", s.activateScene).Methods("POST")

	api.HandleFunc("/timers", s.getTimers).Methods("GET")
	api.HandleFunc("/timers", s.createTimer).Methods("POST")
	api.HandleFunc("/timers/{id}", s.deleteTimer).Methods("DELETE")

	r.PathPrefix("/").Handler(spaHandler(s.SPADir))

	cors := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)
	return cors(r)
}

// writeJSON encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError responds with a JSON {"error": "..."} body.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) getHealth(w http.ResponseWriter, r *http.Request) {
	s.Store.Mu.RLock()
	socketCount := len(s.Store.Sockets)
	scheduleCount := len(s.Store.Schedules)
	groupCount := len(s.Store.Groups)
	sceneCount := len(s.Store.Scenes)
	timerCount := len(s.Store.Timers)
	s.Store.Mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"sockets":   socketCount,
		"schedules": scheduleCount,
		"groups":    groupCount,
		"scenes":    sceneCount,
		"timers":    timerCount,
		"time":      time.Now().UTC().Format(time.RFC3339),
	})
}
