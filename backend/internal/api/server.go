// Package api exposes the HTTP surface (REST + SPA host) on top of an
// injected *store.Store. The Server type is the root: configure its
// fields, then call Handler() to build a fully-wired http.Handler.
package api

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

// Server wires HTTP handlers to a Store.
type Server struct {
	Store         *store.Store
	AuthUser      string
	AuthPass      string
	SessionSecret []byte // HMAC key for cookie sessions; see LoadOrCreateSessionSecret
	SPADir        string // path to the built Svelte app (e.g. "./frontend/dist")
}

// Handler returns the configured router with logging, optional basic
// auth, the API routes, the SPA fallback and CORS — in that order.
func (s *Server) Handler() http.Handler {
	r := mux.NewRouter()
	r.Use(loggingMiddleware)

	authEnabled := s.AuthUser != "" && s.AuthPass != ""
	if authEnabled {
		log.Printf("HTTP auth enabled for user %q (cookie session + basic fallback)", s.AuthUser)
	} else {
		log.Printf("HTTP auth DISABLED — set AUTH_USER and AUTH_PASS to enable")
	}

	// Auth endpoints are public — the SPA needs to reach /api/login without
	// being authenticated, and /api/logout just clears the cookie.
	r.HandleFunc("/api/login", s.handleLogin).Methods("POST")
	r.HandleFunc("/api/logout", s.handleLogout).Methods("POST")

	api := r.PathPrefix("/api").Subrouter()
	if authEnabled {
		api.Use(authMiddleware(s.AuthUser, s.AuthPass, s.SessionSecret))
	}
	api.HandleFunc("/health", s.getHealth).Methods("GET")

	api.HandleFunc("/sockets", s.getSockets).Methods("GET")
	api.HandleFunc("/sockets", s.createSocket).Methods("POST")
	api.HandleFunc("/sockets/learn", s.learnSocket).Methods("POST")
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

	api.HandleFunc("/sensors", s.getSensors).Methods("GET")
	api.HandleFunc("/sensors", s.createSensor).Methods("POST")
	api.HandleFunc("/sensors/pair/start", s.startSensorPair).Methods("POST")
	api.HandleFunc("/sensors/discover", s.listDiscoveryCandidates).Methods("GET")
	api.HandleFunc("/sensors/{id}", s.updateSensor).Methods("PUT")
	api.HandleFunc("/sensors/{id}", s.deleteSensor).Methods("DELETE")
	api.HandleFunc("/sensors/{id}/readings", s.getSensorReadings).Methods("GET")
	api.HandleFunc("/sensors/{id}/readings", s.postSensorReading).Methods("POST")

	api.HandleFunc("/activity", s.getActivity).Methods("GET")
	api.HandleFunc("/shortcut-auth", s.getShortcutAuth).Methods("GET")

	api.HandleFunc("/settings", s.getSettings).Methods("GET")
	api.HandleFunc("/settings", s.updateSettings).Methods("PUT")

	api.HandleFunc("/hue/pair", s.huePair).Methods("POST")
	api.HandleFunc("/hue/lights", s.hueListLights).Methods("GET")

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

// handleLogin verifies credentials and sets the session cookie. Empty
// AUTH_USER/AUTH_PASS means "auth is off" — we still accept the call so
// the frontend's flow works uniformly, but the cookie is meaningless.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if s.AuthUser != "" && s.AuthPass != "" {
		if subtle.ConstantTimeCompare([]byte(body.Username), []byte(s.AuthUser)) != 1 ||
			subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.AuthPass)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
	}
	setSessionCookie(w, s.SessionSecret, body.Username)
	writeJSON(w, http.StatusOK, map[string]string{"username": body.Username})
}

func (s *Server) handleLogout(w http.ResponseWriter, _ *http.Request) {
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// getShortcutAuth returns the HTTP Basic auth header value for the
// configured credentials, so the frontend's "iOS Shortcuts" helper can
// hand the user a ready-to-paste Authorization header.
//
// This sits behind authMiddleware, so only an already-authenticated
// client can reach it — and it grants nothing the caller's session
// cookie didn't already grant. Returns an empty header when auth is off.
func (s *Server) getShortcutAuth(w http.ResponseWriter, _ *http.Request) {
	header := ""
	if s.AuthUser != "" && s.AuthPass != "" {
		token := base64.StdEncoding.EncodeToString([]byte(s.AuthUser + ":" + s.AuthPass))
		header = "Basic " + token
	}
	writeJSON(w, http.StatusOK, map[string]string{"header": header})
}

// getActivity returns the most recent activity log entries (newest first).
// Supports ?limit=N (default 50, max 200).
func (s *Server) getActivity(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	writeJSON(w, http.StatusOK, s.Store.Activity.Recent(limit))
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
