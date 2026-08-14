// Package api exposes the HTTP surface (REST + SPA host) on top of an
// injected *store.Store. The Server type is the root: configure its
// fields, then call Handler() to build a fully-wired http.Handler.
package api

import (
	"context"
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

	"homehub/internal/airplay"
	"homehub/internal/announce"
	"homehub/internal/kef"
	"homehub/internal/llm"
	"homehub/internal/matter"
	"homehub/internal/media"
	"homehub/internal/mqtt"
	"homehub/internal/push"
	"homehub/internal/sonos"
	"homehub/internal/spotify"
	"homehub/internal/store"
	"homehub/internal/stream"
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
	// HTTPPort is the plain-HTTP listener's port. Sonos speakers post their
	// change notifications back to it — they will not use TLS — so it is
	// needed to build the event callback URL even when HTTPS is also up.
	HTTPPort string

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

	// sonosIcons caches where each speaker publishes a picture of itself,
	// keyed by address. Resolving it means reading the device description, so
	// without this every avatar in the speaker list would cost two round
	// trips to the speaker instead of one. Guarded by sonosIconMu.
	sonosIconMu sync.Mutex
	sonosIcons  map[string]sonosIconEntry

	// sonosMon watches speakers over GENA (see internal/sonos/monitor.go)
	// and caches what they report. Created lazily by sonosEvents().
	sonosMonMu sync.Mutex
	sonosMon   *sonos.Monitor

	// autoplay is HomeHub's own "continue with similar music" setting, on
	// top of what the speakers themselves report (DESIGN.md's "Continue play
	// similar" note) — Sonos has no such concept, so the household doesn't
	// either; it's ours to keep, keyed by the coordinator's registered
	// speaker id, and only for as long as this process runs. It is on for
	// every coordinator, so what autoplayOff holds is the opt-*out*: the
	// rooms that were told to fall silent when their queue ends.
	// autoplayAttempt throttles retries when finding similar tracks keeps
	// failing, autoplayRecent remembers what a coordinator was just topped
	// up with so a short discography doesn't loop the same handful of songs,
	// and autoplayHeard is when each coordinator was last seen actually
	// playing its queue — what separates "the queue just ran dry" from "this
	// room has been quiet all evening".
	autoplayMu      sync.Mutex
	autoplayOff     map[string]bool
	autoplayAttempt map[string]time.Time
	autoplayRecent  map[string][]string
	autoplayHeard   map[string]time.Time

	// heardWatches is the listening log's memory between readings: what
	// each room is playing, since when, and whether the log already has it.
	// It exists so that recording what a house is hearing costs a mutex and
	// a string compare on the readings that change nothing — which is
	// almost all of them. See heard.go.
	heardMu      sync.Mutex
	heardWatches map[string]heardWatch

	// kefMon polls the KEF speakers once for the whole process and caches
	// what they report (see internal/kef/monitor.go). KEF's local API has no
	// change notifications to subscribe to, so this is the closest thing to
	// the Sonos monitor it can be. Created lazily by kefEvents().
	kefMonMu sync.Mutex
	kefMon   *kef.Monitor

	// fades holds the cancel func of each room's in-flight volume ramp,
	// keyed by media destination key. One ramp per room: anything starting
	// a new one cancels the old, which is what stops a wake-up fade and a
	// sleep fade from walking the same speakers in opposite directions.
	// See musictimer.go.
	fadeMu sync.Mutex
	fades  map[string]context.CancelFunc

	// zoneSessions tracks live zone playbacks. Only the stream route
	// leaves anything running — a decoder holding the account's Spotify
	// session and an HTTP stream several speakers pull from — so this is
	// what remembers to shut it down. See media_session.go.
	zoneMu       sync.Mutex
	zoneSessions map[string]*media.Session

	// stream serves decoded audio to speakers, and librespot is what
	// decodes it. Both created lazily, both optional: without librespot
	// installed only the cross-vendor route is unavailable.
	streamMu  sync.Mutex
	stream    *stream.Host
	librespot *stream.Librespot
	// librespotBitrate is what the running decoder was built for. Kept so a
	// household changing its stream quality gets a decoder that honours the
	// change rather than one built at the old bitrate — see decoder().
	librespotBitrate int
	// caster pushes audio to AirPlay receivers. Created lazily like the
	// two above, and like them it holds a live session that has to be shut
	// down: a cast that outlives its zone keeps sending to a receiver
	// nobody is listening to. Its own mutex — see airplayCaster().
	casterMu sync.Mutex
	caster   *airplay.Caster

	// announcer serves announcement clips to the speakers (see
	// announce.go). Created lazily, and only ever holds the last few
	// seconds of audio someone asked the house to hear.
	announceMu sync.Mutex
	announcer  *announce.Host
	// announcing is held for as long as an announcement is audible, not
	// just for as long as its request is: a second one starting mid-clip
	// would snapshot the clip as what the rooms were playing.
	announcing bool
}

// sonosAcctEntry is one cached service-account resolution.
type sonosAcctEntry struct {
	acct *sonos.ServiceAccount
	at   time.Time
}

// sonosIconEntry is one cached device-icon lookup. An empty path is cached
// too — a speaker that publishes no picture shouldn't be asked again on every
// render of the list.
type sonosIconEntry struct {
	path string
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

	// Let a scene or an automation reach the speakers. The store holds the
	// actions and knows nothing about how a room is reached; this is the
	// half that does (scene_music.go).
	s.Store.OnMusic = s.runSceneMusic

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

	var authEnabled bool
	s.Store.View(func() { authEnabled = len(s.Store.Users) > 0 })
	if authEnabled {
		log.Printf("HTTP auth enabled (cookie session + basic fallback)")
	} else {
		log.Printf("HTTP auth DISABLED — no users configured; set AUTH_USER and AUTH_PASS to seed an admin")
	}

	s.registerPublicRoutes(r)

	api := r.PathPrefix("/api").Subrouter()
	if authEnabled {
		api.Use(s.authMiddleware)
	}
	s.registerRoutes(api)
	s.registerPushRoutes(r, api)
	s.registerDeviceCallbackRoutes(r)

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

	var authEnabled bool
	s.Store.View(func() { authEnabled = len(s.Store.Users) > 0 })

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
	var socketCount, scheduleCount, groupCount, sceneCount, timerCount int
	s.Store.View(func() {
		socketCount = len(s.Store.Sockets)
		scheduleCount = len(s.Store.Schedules)
		groupCount = len(s.Store.Groups)
		sceneCount = len(s.Store.Scenes)
		timerCount = len(s.Store.Timers)
	})
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
