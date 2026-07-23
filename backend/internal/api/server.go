// Package api exposes the HTTP surface (REST + SPA host) on top of an
// injected *store.Store. The Server type is the root: configure its
// fields, then call Handler() to build a fully-wired http.Handler.
package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"homehub/internal/llm"
	"homehub/internal/matter"
	"homehub/internal/mqtt"
	"homehub/internal/push"
	"homehub/internal/sonos"
	"homehub/internal/spotify"
	"homehub/internal/store"
)

// maxRequestBody caps API request bodies. Generous for this app's config
// bundles yet small enough to stop a runaway upload.
const maxRequestBody = 1 << 20 // 1 MiB

// Server wires HTTP handlers to a Store.
type Server struct {
	Store         *store.Store
	Matter        *matter.Client  // optional; nil-safe via Matter.Enabled()
	MQTT          *mqtt.Client    // optional; nil-safe via MQTT.Enabled()
	LLM           *llm.Client     // optional; nil-safe via LLM.Enabled(). Powers the assistant.
	Push          *push.Service   // optional; nil means push notifications are disabled
	Spotify       *spotify.Client // optional; nil disables Spotify search in the Music view
	AuthUser      string
	AuthPass      string
	SessionSecret []byte // HMAC key for cookie sessions; see LoadOrCreateSessionSecret
	SPADir        string // path to the built Svelte app (e.g. "./frontend/dist")

	// In-flight Matter commission jobs. Created lazily in Handler() so
	// callers don't need to initialise it. Background commission runs
	// outlive the originating HTTP request; the frontend polls for status.
	matterJobs *commissionJobs

	// events fans live "something changed" signals out to SSE clients.
	// Created lazily in Handler().
	events *sseHub

	// logins throttles repeated failed logins per client IP. Created
	// lazily in Handler().
	logins *loginLimiter

	// sonosAccts caches per-speaker streaming-service account lookups
	// (sid/sn) for the play-item path. Guarded by sonosAcctMu; created
	// lazily on first use.
	sonosAcctMu sync.Mutex
	sonosAccts  map[string]sonosAcctEntry
}

// sonosAcctEntry is one cached service-account resolution.
type sonosAcctEntry struct {
	acct *sonos.ServiceAccount
	at   time.Time
}

// Handler returns the configured router with logging, optional basic
// auth, the API routes, the SPA fallback and CORS — in that order.
func (s *Server) Handler() http.Handler {
	if s.matterJobs == nil {
		s.matterJobs = newCommissionJobs()
	}
	if s.events == nil {
		s.events = newSSEHub()
	}
	if s.logins == nil {
		s.logins = newLoginLimiter()
	}
	// Push a live signal to connected clients whenever a socket's state
	// changes — including scheduler- and timer-driven changes, since those
	// also flow through Store.ApplyState.
	s.Store.OnChange = s.events.broadcast

	// Wire push notification callbacks when the push service is available.
	if s.Push != nil {
		s.Store.OnStateChange = func(socket store.Socket, newState bool) {
			action := "off"
			if newState {
				action = "on"
			}
			go s.Push.NotifyEvent(push.CategoryStateChanges, socket.ID, push.PushPayload{
				Title: fmt.Sprintf("💡 %s turned %s", socket.Name, action),
				URL:   "/#/sockets",
				Tag:   "state-" + socket.ID,
			})
		}
		s.Store.OnSensorAlert = func(sensor store.Sensor, value float64, direction string) {
			go s.Push.NotifyEvent(push.CategorySensorAlerts, sensor.ID, push.PushPayload{
				Title: fmt.Sprintf("⚠️ %s alert", sensor.Name),
				Body:  fmt.Sprintf("%.1f%s (%s threshold)", value, sensor.Unit, direction),
				URL:   "/#/sensors",
				Tag:   "sensor-" + sensor.ID,
			})
		}
	}

	r := mux.NewRouter()
	r.Use(loggingMiddleware)
	r.Use(maxBodyBytes(maxRequestBody))
	r.Use(csrfMiddleware)

	s.Store.Mu.RLock()
	authEnabled := len(s.Store.Users) > 0
	s.Store.Mu.RUnlock()
	if authEnabled {
		log.Printf("HTTP auth enabled (cookie session + basic fallback)")
	} else {
		log.Printf("HTTP auth DISABLED — no users configured; set AUTH_USER and AUTH_PASS to seed an admin")
	}

	// Auth endpoints are public — the SPA needs to reach /api/login without
	// being authenticated, and /api/logout just clears the cookie.
	r.HandleFunc("/api/login", s.handleLogin).Methods("POST")
	r.HandleFunc("/api/logout", s.handleLogout).Methods("POST")

	// Invite endpoints are also public: a new admin sets their own password
	// via a one-time link before they have a session cookie.
	r.HandleFunc("/api/invite", s.lookupInvite).Methods("GET")
	r.HandleFunc("/api/invite", s.acceptInvite).Methods("POST")

	api := r.PathPrefix("/api").Subrouter()
	if authEnabled {
		api.Use(s.authMiddleware)
	}
	api.HandleFunc("/health", s.getHealth).Methods("GET")
	api.HandleFunc("/events", s.handleEvents).Methods("GET")

	api.HandleFunc("/me", s.getMe).Methods("GET")
	api.HandleFunc("/users", s.requireAdmin(s.listUsers)).Methods("GET")
	api.HandleFunc("/users", s.requireAdmin(s.createUser)).Methods("POST")
	api.HandleFunc("/users/{id}", s.requireAdmin(s.updateUser)).Methods("PUT")
	api.HandleFunc("/users/{id}", s.requireAdmin(s.deleteUser)).Methods("DELETE")

	// Sockets: lists are filtered to the caller's allowed set, control
	// endpoints are gated per-socket, and create/edit/delete are admin-only.
	api.HandleFunc("/sockets", s.getSockets).Methods("GET")
	api.HandleFunc("/sockets", s.requireAdmin(s.createSocket)).Methods("POST")
	api.HandleFunc("/sockets/learn", s.requireAdmin(s.learnSocket)).Methods("POST")
	api.HandleFunc("/sockets/all/on", s.bulkSetState(true)).Methods("POST")
	api.HandleFunc("/sockets/all/off", s.bulkSetState(false)).Methods("POST")
	api.HandleFunc("/sockets/{id}", s.getSocket).Methods("GET")
	api.HandleFunc("/sockets/{id}", s.requireAdmin(s.updateSocket)).Methods("PUT")
	api.HandleFunc("/sockets/{id}", s.requireAdmin(s.deleteSocket)).Methods("DELETE")
	api.HandleFunc("/sockets/{id}/toggle", s.toggleSocket).Methods("POST")
	api.HandleFunc("/sockets/{id}/on", s.turnOn).Methods("POST")
	api.HandleFunc("/sockets/{id}/off", s.turnOff).Methods("POST")
	api.HandleFunc("/sockets/{id}/timer", s.createSocketTimer).Methods("POST")
	api.HandleFunc("/sockets/{id}/favorite", s.toggleFavorite).Methods("POST")

	api.HandleFunc("/rooms", s.getRooms).Methods("GET")
	api.HandleFunc("/rooms", s.requireAdmin(s.createRoom)).Methods("POST")
	api.HandleFunc("/rooms/{id}", s.requireAdmin(s.updateRoom)).Methods("PUT")
	api.HandleFunc("/rooms/{id}", s.requireAdmin(s.deleteRoom)).Methods("DELETE")
	api.HandleFunc("/rooms/{room}/on", s.roomSetState(true)).Methods("POST")
	api.HandleFunc("/rooms/{room}/off", s.roomSetState(false)).Methods("POST")

	// Everything below is admin-only: a non-admin's app is just their
	// devices and the dashboard, so groups/scenes/schedules/sensors/
	// settings management never reaches them.
	// Schedule read/write is open to all authenticated users; handlers filter
	// results to the caller's own sockets for non-admins. The bulk
	// enable/disable ("vacation mode") remains admin-only.
	api.HandleFunc("/schedules", s.getSchedules).Methods("GET")
	api.HandleFunc("/schedules", s.createSchedule).Methods("POST")
	api.HandleFunc("/schedules/all/enable", s.requireAdmin(s.setAllSchedules(true))).Methods("POST")
	api.HandleFunc("/schedules/all/disable", s.requireAdmin(s.setAllSchedules(false))).Methods("POST")
	api.HandleFunc("/schedules/{id}", s.updateSchedule).Methods("PUT")
	api.HandleFunc("/schedules/{id}", s.deleteSchedule).Methods("DELETE")

	api.HandleFunc("/automations", s.requireAdmin(s.getAutomations)).Methods("GET")
	api.HandleFunc("/automations", s.requireAdmin(s.createAutomation)).Methods("POST")
	api.HandleFunc("/automations/{id}", s.requireAdmin(s.updateAutomation)).Methods("PUT")
	api.HandleFunc("/automations/{id}", s.requireAdmin(s.deleteAutomation)).Methods("DELETE")
	api.HandleFunc("/automations/{id}/run", s.requireAdmin(s.runAutomation)).Methods("POST")
	api.HandleFunc("/automations/{id}/rules/{idx}/run", s.requireAdmin(s.runAutomationRule)).Methods("POST")

	api.HandleFunc("/groups", s.requireAdmin(s.getGroups)).Methods("GET")
	api.HandleFunc("/groups", s.requireAdmin(s.createGroup)).Methods("POST")
	api.HandleFunc("/groups/{id}", s.requireAdmin(s.getGroup)).Methods("GET")
	api.HandleFunc("/groups/{id}", s.requireAdmin(s.updateGroup)).Methods("PUT")
	api.HandleFunc("/groups/{id}", s.requireAdmin(s.deleteGroup)).Methods("DELETE")
	api.HandleFunc("/groups/{id}/on", s.requireAdmin(s.groupAction("on"))).Methods("POST")
	api.HandleFunc("/groups/{id}/off", s.requireAdmin(s.groupAction("off"))).Methods("POST")
	api.HandleFunc("/groups/{id}/toggle", s.requireAdmin(s.groupAction("toggle"))).Methods("POST")

	api.HandleFunc("/scenes", s.requireAdmin(s.getScenes)).Methods("GET")
	api.HandleFunc("/scenes", s.requireAdmin(s.createScene)).Methods("POST")
	api.HandleFunc("/scenes/{id}", s.requireAdmin(s.getScene)).Methods("GET")
	api.HandleFunc("/scenes/{id}", s.requireAdmin(s.updateScene)).Methods("PUT")
	api.HandleFunc("/scenes/{id}", s.requireAdmin(s.deleteScene)).Methods("DELETE")
	api.HandleFunc("/scenes/{id}/activate", s.requireAdmin(s.activateScene)).Methods("POST")

	api.HandleFunc("/timers", s.requireAdmin(s.getTimers)).Methods("GET")
	api.HandleFunc("/timers", s.requireAdmin(s.createTimer)).Methods("POST")
	api.HandleFunc("/timers/{id}", s.requireAdmin(s.deleteTimer)).Methods("DELETE")

	api.HandleFunc("/sensors", s.requireAdmin(s.getSensors)).Methods("GET")
	api.HandleFunc("/sensors", s.requireAdmin(s.createSensor)).Methods("POST")
	api.HandleFunc("/sensors/pair/start", s.requireAdmin(s.startSensorPair)).Methods("POST")
	api.HandleFunc("/sensors/discover", s.requireAdmin(s.listDiscoveryCandidates)).Methods("GET")
	api.HandleFunc("/sensors/{id}", s.requireAdmin(s.updateSensor)).Methods("PUT")
	api.HandleFunc("/sensors/{id}", s.requireAdmin(s.deleteSensor)).Methods("DELETE")
	api.HandleFunc("/sensors/{id}/readings", s.requireAdmin(s.getSensorReadings)).Methods("GET")
	api.HandleFunc("/sensors/{id}/readings", s.requireAdmin(s.postSensorReading)).Methods("POST")

	api.HandleFunc("/activity", s.requireAdmin(s.getActivity)).Methods("GET")
	api.HandleFunc("/shortcut-auth", s.requireAdmin(s.getShortcutAuth)).Methods("GET")

	api.HandleFunc("/settings", s.getSettings).Methods("GET")
	api.HandleFunc("/settings", s.requireAdmin(s.updateSettings)).Methods("PUT")

	api.HandleFunc("/export", s.requireAdmin(s.exportConfig)).Methods("GET")
	api.HandleFunc("/import", s.requireAdmin(s.importConfig)).Methods("POST")

	api.HandleFunc("/tasmota/probe", s.requireAdmin(s.tasmotaProbe)).Methods("GET")
	api.HandleFunc("/tasmota/{socketId}", s.tasmotaGetState).Methods("GET")
	api.HandleFunc("/tasmota/{socketId}/state", s.tasmotaSetState).Methods("PUT")

	// Sonos speakers: local UPnP control (playback, volume, grouping,
	// favorites). Admin-gated like the other whole-home surfaces.
	api.HandleFunc("/sonos/status", s.requireAdmin(s.sonosStatus)).Methods("GET")
	api.HandleFunc("/sonos/discover", s.requireAdmin(s.sonosDiscover)).Methods("GET")
	api.HandleFunc("/sonos/speakers", s.requireAdmin(s.sonosCreateSpeaker)).Methods("POST")
	api.HandleFunc("/sonos/speakers/{id}", s.requireAdmin(s.sonosUpdateSpeaker)).Methods("PUT")
	api.HandleFunc("/sonos/speakers/{id}", s.requireAdmin(s.sonosDeleteSpeaker)).Methods("DELETE")
	api.HandleFunc("/sonos/{id}/play", s.requireAdmin(s.sonosTransport(sonos.Play))).Methods("POST")
	api.HandleFunc("/sonos/{id}/pause", s.requireAdmin(s.sonosTransport(sonos.Pause))).Methods("POST")
	api.HandleFunc("/sonos/{id}/next", s.requireAdmin(s.sonosTransport(sonos.Next))).Methods("POST")
	api.HandleFunc("/sonos/{id}/previous", s.requireAdmin(s.sonosTransport(sonos.Previous))).Methods("POST")
	api.HandleFunc("/sonos/{id}/leave", s.requireAdmin(s.sonosTransport(sonos.Leave))).Methods("POST")
	api.HandleFunc("/sonos/{id}/join", s.requireAdmin(s.sonosJoin)).Methods("POST")
	api.HandleFunc("/sonos/{id}/volume", s.requireAdmin(s.sonosSetVolume)).Methods("PUT")
	api.HandleFunc("/sonos/{id}/mute", s.requireAdmin(s.sonosSetMute)).Methods("PUT")
	api.HandleFunc("/sonos/{id}/favorites", s.requireAdmin(s.sonosFavorites)).Methods("GET")
	api.HandleFunc("/sonos/{id}/favorites/play", s.requireAdmin(s.sonosPlayFavorite)).Methods("POST")
	api.HandleFunc("/sonos/{id}/art", s.requireAdmin(s.sonosArt)).Methods("GET")
	api.HandleFunc("/sonos/{id}/play-item", s.requireAdmin(s.sonosPlayItem)).Methods("POST")

	// Spotify search/browse for the Music view. OAuth is the user's own
	// account (PKCE); playback stays local via the play-item route above.
	api.HandleFunc("/spotify/status", s.requireAdmin(s.spotifyStatus)).Methods("GET")
	api.HandleFunc("/spotify/config", s.requireAdmin(s.spotifySetConfig)).Methods("PUT")
	api.HandleFunc("/spotify/login", s.requireAdmin(s.spotifyLogin)).Methods("GET")
	api.HandleFunc("/spotify/callback", s.requireAdmin(s.spotifyCallback)).Methods("GET")
	api.HandleFunc("/spotify/exchange", s.requireAdmin(s.spotifyExchange)).Methods("POST")
	api.HandleFunc("/spotify/disconnect", s.requireAdmin(s.spotifyDisconnect)).Methods("POST")
	api.HandleFunc("/spotify/search", s.requireAdmin(s.spotifySearch)).Methods("GET")
	api.HandleFunc("/spotify/playlists", s.requireAdmin(s.spotifyPlaylists)).Methods("GET")

	api.HandleFunc("/matter/transport", s.requireAdmin(s.matterTransport)).Methods("GET")
	api.HandleFunc("/matter/devices", s.requireAdmin(s.matterListDevices)).Methods("GET")
	api.HandleFunc("/matter/commission", s.requireAdmin(s.matterCommission)).Methods("POST")
	api.HandleFunc("/matter/commission/jobs/{id}", s.requireAdmin(s.matterCommissionJob)).Methods("GET")
	api.HandleFunc("/matter/{socketId}", s.matterGetState).Methods("GET")
	api.HandleFunc("/matter/{socketId}/state", s.matterSetState).Methods("PUT")

	api.HandleFunc("/mqtt/status", s.requireAdmin(s.mqttStatus)).Methods("GET")
	api.HandleFunc("/mqtt/publish", s.requireAdmin(s.mqttPublish)).Methods("POST")

	// Local LLM assistant. Admin-gated: it can drive bulk control and reads
	// across every device, matching the posture of the groups/scenes routes.
	// When the LLM client is disabled the handlers return 503.
	api.HandleFunc("/assistant/status", s.requireAdmin(s.assistantStatus)).Methods("GET")
	api.HandleFunc("/assistant/chat", s.requireAdmin(s.assistantChat)).Methods("POST")
	api.HandleFunc("/assistant/confirm", s.requireAdmin(s.assistantConfirm)).Methods("POST")

	// Push notifications. vapid-key is public (no auth) so the frontend can
	// subscribe before the user is authenticated. Subscribe/unsubscribe require
	// a session; prefs require auth but not admin.
	r.HandleFunc("/api/push/vapid-key", s.getPushVAPIDKey).Methods("GET")
	api.HandleFunc("/push/subscribe", s.subscribePush).Methods("POST")
	api.HandleFunc("/push/unsubscribe", s.unsubscribePush).Methods("DELETE")
	api.HandleFunc("/push/prefs", s.updatePushPrefs).Methods("PUT")
	api.HandleFunc("/push/test", s.testPush).Methods("POST")

	r.PathPrefix("/").Handler(spaHandler(s.SPADir))

	// CORS is locked down by default: the SPA is served from the same
	// origin as the API, so cross-origin access isn't needed. Operators who
	// want to call the API from another origin opt specific ones in via
	// CORS_ALLOWED_ORIGINS.
	if cors := corsFromEnv(); cors != nil {
		return cors(r)
	}
	return r
}

// corsOrigins parses CORS_ALLOWED_ORIGINS (a comma-separated list) into the
// origins the operator has opted in. Empty when unset. Shared by the CORS
// middleware and the CSRF origin check so the two can't drift.
func corsOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if raw == "" {
		return nil
	}
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// csrfMiddleware rejects state-changing requests whose Origin header shows a
// browser context on a different origin than ours (or an operator-allowed
// CORS origin). SameSite=Lax on the session cookie is the first line of
// defense; this covers the gaps — login CSRF, older browsers, and overly
// broad CORS_ALLOWED_ORIGINS entries combined with credentials. Requests
// without an Origin header (curl, iOS Shortcuts) pass through untouched.
func csrfMiddleware(next http.Handler) http.Handler {
	allowed := make(map[string]bool)
	for _, o := range corsOrigins() {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		// Same-origin check against the Host the request arrived with, and
		// against X-Forwarded-Host for reverse proxies that rewrite Host.
		// (Trusting XFH only loosens toward same-origin: a cross-site
		// browser request can't carry a custom XFH header — forms can't set
		// headers and fetch would need a preflight that CORS denies.)
		if u, err := url.Parse(origin); err == nil {
			if strings.EqualFold(u.Host, r.Host) ||
				(r.Header.Get("X-Forwarded-Host") != "" && strings.EqualFold(u.Host, r.Header.Get("X-Forwarded-Host"))) {
				next.ServeHTTP(w, r) // same-origin
				return
			}
		}
		if allowed["*"] || allowed[origin] {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
	})
}

// corsFromEnv builds CORS middleware from CORS_ALLOWED_ORIGINS. It returns
// nil when the var is unset, leaving the API same-origin only. Explicit
// origins also get credentialed requests enabled; a "*" entry can't, since
// credentials and wildcard are mutually exclusive per the CORS spec.
func corsFromEnv() func(http.Handler) http.Handler {
	origins := corsOrigins()
	if len(origins) == 0 {
		return nil
	}
	wildcard := false
	for _, o := range origins {
		if o == "*" {
			wildcard = true
		}
	}
	opts := []handlers.CORSOption{
		handlers.AllowedOrigins(origins),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	}
	if !wildcard {
		opts = append(opts, handlers.AllowCredentials())
	}
	return handlers.CORS(opts...)
}

// writeJSON encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeJSONBytes writes an already-encoded JSON body. Used together with a
// json.Marshal performed under the store lock: the marshal produces a
// consistent snapshot while the lock is held (it does no network I/O), and the
// potentially slow client write happens here after the lock is released, so the
// store is never held across client I/O.
func writeJSONBytes(w http.ResponseWriter, status int, b []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// writeError responds with a JSON {"error": "..."} body.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleLogin verifies credentials against the stored users and sets the
// session cookie. When no users exist auth is off — we still accept the
// call so the frontend's flow works uniformly, but the cookie is unused.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	s.Store.Mu.RLock()
	authEnabled := len(s.Store.Users) > 0
	s.Store.Mu.RUnlock()

	if authEnabled {
		ip := clientIP(r)
		if ok, retryAfter := s.logins.allowed(ip); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "too many failed attempts — try again later")
			return
		}
		// Cross-IP cap: an attacker rotating source addresses still runs
		// into this one. Existing sessions are unaffected by the pause.
		if ok, retryAfter := s.logins.allowed(globalLoginKey); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "too many failed attempts — try again later")
			return
		}
		// A login code is the single credential for limited profiles;
		// admins use username + password. Try whichever was supplied.
		var user *store.User
		if strings.TrimSpace(body.Code) != "" {
			user = s.verifyLoginCode(body.Code)
		} else {
			user = s.verifyCredentials(body.Username, body.Password)
		}
		if user == nil {
			s.logins.recordFailure(ip)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		s.logins.recordSuccess(ip)
		setSessionCookie(w, s.SessionSecret, user.ID, user.TokenVersion, isSecureRequest(r))
		writeJSON(w, http.StatusOK, map[string]string{"username": user.Username})
		return
	}
	setSessionCookie(w, s.SessionSecret, body.Username, 0, isSecureRequest(r))
	writeJSON(w, http.StatusOK, map[string]string{"username": body.Username})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w, isSecureRequest(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// getShortcutAuth returns the HTTP Basic auth header value for the
// configured credentials, so the frontend's "iOS Shortcuts" helper can
// hand the user a ready-to-paste Authorization header.
//
// Only the owner gets the credential back: AUTH_USER/AUTH_PASS is the
// owner's permanent password equivalent, so returning it to any admin
// session would let a non-owner admin escalate to the (undemotable)
// owner account. Other callers get an empty header, the same shape the
// frontend already handles for the auth-off case.
func (s *Server) getShortcutAuth(w http.ResponseWriter, r *http.Request) {
	header := ""
	u := currentUser(r)
	ownerOrAuthOff := u == nil || u.Owner
	if ownerOrAuthOff && s.AuthUser != "" && s.AuthPass != "" {
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
