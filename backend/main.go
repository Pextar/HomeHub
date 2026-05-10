package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// Socket represents a 433MHz controllable socket
type Socket struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Code     string `json:"code"`     // 433MHz code (e.g., "12345")
	Protocol string `json:"protocol"` // e.g., "nexa", "kaku", "intertechno"
	State    bool   `json:"state"`    // true = on, false = off
	Room     string `json:"room"`     // room/location
}

// Schedule represents a timer for a socket, group, or scene.
//
// Targets:
//   - target_type "socket": fires action ("on"|"off"|"toggle") on a socket
//   - target_type "group":  fires action ("on"|"off"|"toggle") on every member
//   - target_type "scene":  activates the scene (action ignored, treated as "activate")
//
// For backwards compatibility, schedules with socket_id set and no
// target_type are treated as target_type="socket", target_id=socket_id.
type Schedule struct {
	ID          string    `json:"id"`
	SocketID    string    `json:"socket_id,omitempty"`
	TargetType  string    `json:"target_type,omitempty"`
	TargetID    string    `json:"target_id,omitempty"`
	Action      string    `json:"action"` // "on" | "off" | "toggle" | "activate"
	Time        string    `json:"time"`   // "HH:MM" format
	Days        []int     `json:"days"`   // 0=Sun, 1=Mon, etc
	Enabled     bool      `json:"enabled"`
	LastFiredAt time.Time `json:"last_fired_at,omitempty"`
}

// Group is a manually curated collection of sockets that can be controlled
// together. A socket may belong to any number of groups.
type Group struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	SocketIDs []string `json:"socket_ids"`
}

// SceneAction sets one socket to a specific state when its scene fires.
type SceneAction struct {
	SocketID string `json:"socket_id"`
	Action   string `json:"action"` // "on" | "off"
}

// Scene is a named preset that drives a specific set of sockets to specific
// states ("movie night": lamp ON, ceiling OFF).
type Scene struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Actions []SceneAction `json:"actions"`
}

// Timer fires once at FiresAt and is then deleted. Used for "off in 30
// minutes" style quick actions. Persisted so they survive restarts.
type Timer struct {
	ID         string    `json:"id"`
	TargetType string    `json:"target_type"` // "socket" | "group" | "scene"
	TargetID   string    `json:"target_id"`
	Action     string    `json:"action"` // "on" | "off" | "toggle" | "activate"
	FiresAt    time.Time `json:"fires_at"`
	CreatedAt  time.Time `json:"created_at"`
	Note       string    `json:"note,omitempty"` // human-friendly description
}

var (
	mu        sync.RWMutex
	sockets   = make(map[string]*Socket)
	schedules = make(map[string]*Schedule)
	groups    = make(map[string]*Group)
	scenes    = make(map[string]*Scene)
	timers    = make(map[string]*Timer)
	dataDir   = "./data"
)

func main() {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("failed to create data directory %q: %v", dataDir, err)
	}
	if err := loadData(); err != nil {
		log.Fatalf("failed to load data: %v", err)
	}

	r := mux.NewRouter()
	r.Use(loggingMiddleware)

	authUser := os.Getenv("AUTH_USER")
	authPass := os.Getenv("AUTH_PASS")
	if authUser != "" && authPass != "" {
		r.Use(basicAuthMiddleware(authUser, authPass))
		log.Printf("HTTP basic auth enabled for user %q", authUser)
	} else {
		log.Printf("HTTP basic auth DISABLED — set AUTH_USER and AUTH_PASS to enable")
	}

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/health", getHealth).Methods("GET")

	api.HandleFunc("/sockets", getSockets).Methods("GET")
	api.HandleFunc("/sockets", createSocket).Methods("POST")
	api.HandleFunc("/sockets/all/on", bulkSetState(true)).Methods("POST")
	api.HandleFunc("/sockets/all/off", bulkSetState(false)).Methods("POST")
	api.HandleFunc("/sockets/{id}", getSocket).Methods("GET")
	api.HandleFunc("/sockets/{id}", updateSocket).Methods("PUT")
	api.HandleFunc("/sockets/{id}", deleteSocket).Methods("DELETE")
	api.HandleFunc("/sockets/{id}/toggle", toggleSocket).Methods("POST")
	api.HandleFunc("/sockets/{id}/on", turnOn).Methods("POST")
	api.HandleFunc("/sockets/{id}/off", turnOff).Methods("POST")

	api.HandleFunc("/rooms", getRooms).Methods("GET")
	api.HandleFunc("/rooms/{room}/on", roomSetState(true)).Methods("POST")
	api.HandleFunc("/rooms/{room}/off", roomSetState(false)).Methods("POST")

	api.HandleFunc("/schedules", getSchedules).Methods("GET")
	api.HandleFunc("/schedules", createSchedule).Methods("POST")
	api.HandleFunc("/schedules/{id}", updateSchedule).Methods("PUT")
	api.HandleFunc("/schedules/{id}", deleteSchedule).Methods("DELETE")

	api.HandleFunc("/groups", getGroups).Methods("GET")
	api.HandleFunc("/groups", createGroup).Methods("POST")
	api.HandleFunc("/groups/{id}", getGroup).Methods("GET")
	api.HandleFunc("/groups/{id}", updateGroup).Methods("PUT")
	api.HandleFunc("/groups/{id}", deleteGroup).Methods("DELETE")
	api.HandleFunc("/groups/{id}/on", groupAction("on")).Methods("POST")
	api.HandleFunc("/groups/{id}/off", groupAction("off")).Methods("POST")
	api.HandleFunc("/groups/{id}/toggle", groupAction("toggle")).Methods("POST")

	api.HandleFunc("/scenes", getScenes).Methods("GET")
	api.HandleFunc("/scenes", createScene).Methods("POST")
	api.HandleFunc("/scenes/{id}", getScene).Methods("GET")
	api.HandleFunc("/scenes/{id}", updateScene).Methods("PUT")
	api.HandleFunc("/scenes/{id}", deleteScene).Methods("DELETE")
	api.HandleFunc("/scenes/{id}/activate", activateSceneHandler).Methods("POST")

	api.HandleFunc("/timers", getTimers).Methods("GET")
	api.HandleFunc("/timers", createTimer).Methods("POST")
	api.HandleFunc("/timers/{id}", deleteTimer).Methods("DELETE")
	api.HandleFunc("/sockets/{id}/timer", createSocketTimer).Methods("POST")

	// Static files (Svelte build output). The Svelte SPA is built by `vite
	// build` into ./frontend/dist/. We serve real files when they exist
	// (fingerprinted JS/CSS, the manifest, the service worker, icons) and
	// fall back to index.html for any unknown path so client-side routing
	// works on hard refresh and PWA navigation requests.
	r.PathPrefix("/").Handler(spaHandler("./frontend/dist"))

	cors := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           cors(r),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	schedCtx, stopScheduler := context.WithCancel(context.Background())
	go runScheduler(schedCtx)

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

// spaHandler serves files from `dir`, falling back to index.html for any
// path that doesn't map to an actual file. This is what makes the Svelte
// SPA's hash-free deep links work on a hard refresh.
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	indexPath := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes are matched before this handler, so we never see them
		// here; just guard against a missing build with a clear message.
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			http.Error(w,
				"frontend/dist/index.html is missing — run `npm install && npm run build` in ./frontend.",
				http.StatusServiceUnavailable)
			return
		}

		// Try the literal file first.
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}

		// Fallback: serve the SPA shell.
		http.ServeFile(w, r, indexPath)
	})
}

// basicAuthMiddleware gates the whole app behind HTTP basic auth. It is only
// installed when both AUTH_USER and AUTH_PASS are non-empty. Comparison uses
// constant-time to avoid leaking the credentials via timing. CORS preflight
// (OPTIONS) is allowed through so browsers can negotiate cross-origin in dev.
func basicAuthMiddleware(user, pass string) mux.MiddlewareFunc {
	expectUser := []byte(user)
	expectPass := []byte(pass)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			u, p, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(u), expectUser) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), expectPass) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="rf-socket-controller", charset="UTF-8"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// loggingMiddleware logs each request's method, path, status and duration.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// writeJSONResponse encodes v as JSON with the given status code.
func writeJSONResponse(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError responds with a JSON {"error": "..."} body.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSONResponse(w, status, map[string]string{"error": msg})
}

func getHealth(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	socketCount := len(sockets)
	scheduleCount := len(schedules)
	groupCount := len(groups)
	sceneCount := len(scenes)
	timerCount := len(timers)
	mu.RUnlock()
	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"sockets":   socketCount,
		"schedules": scheduleCount,
		"groups":    groupCount,
		"scenes":    sceneCount,
		"timers":    timerCount,
		"time":      time.Now().UTC().Format(time.RFC3339),
	})
}

// sendRFCode emits a 433MHz code via whichever userspace tool is installed.
// In simulation (no tool installed) it just logs — useful in dev / non-Pi
// environments. The exec calls have a hard timeout so a stuck transmitter
// driver cannot block an HTTP handler indefinitely.
func sendRFCode(code string, protocol string, state bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch {
	case lookPath("rpi-rf_send"):
		cmd = exec.CommandContext(ctx, "rpi-rf_send", code)
	case lookPath("codesend"):
		cmd = exec.CommandContext(ctx, "codesend", code)
	default:
		log.Printf("[simulation] code=%s protocol=%s state=%v", code, protocol, state)
		return nil
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v: %s", cmd.Path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// API Handlers

func getSockets(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	result := make([]*Socket, 0, len(sockets))
	for _, s := range sockets {
		result = append(result, s)
	}
	mu.RUnlock()

	sort.Slice(result, func(i, j int) bool {
		if result[i].Room != result[j].Room {
			return strings.ToLower(result[i].Room) < strings.ToLower(result[j].Room)
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	writeJSONResponse(w, http.StatusOK, result)
}

func createSocket(w http.ResponseWriter, r *http.Request) {
	var socket Socket
	if err := json.NewDecoder(r.Body).Decode(&socket); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	socket.Name = strings.TrimSpace(socket.Name)
	socket.Code = strings.TrimSpace(socket.Code)
	socket.Protocol = strings.TrimSpace(socket.Protocol)
	socket.Room = strings.TrimSpace(socket.Room)

	if socket.Name == "" || socket.Code == "" {
		writeError(w, http.StatusBadRequest, "name and code are required")
		return
	}

	if socket.ID == "" {
		socket.ID = fmt.Sprintf("socket_%d", time.Now().UnixNano())
	}

	mu.Lock()
	sockets[socket.ID] = &socket
	if err := saveData(); err != nil {
		delete(sockets, socket.ID)
		mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	mu.Unlock()

	writeJSONResponse(w, http.StatusCreated, socket)
}

func getSocket(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	mu.RLock()
	socket, ok := sockets[id]
	mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	writeJSONResponse(w, http.StatusOK, socket)
}

func updateSocket(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates Socket
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	socket, ok := sockets[id]
	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}

	if name := strings.TrimSpace(updates.Name); name != "" {
		socket.Name = name
	}
	if code := strings.TrimSpace(updates.Code); code != "" {
		socket.Code = code
	}
	if protocol := strings.TrimSpace(updates.Protocol); protocol != "" {
		socket.Protocol = protocol
	}
	// Room can be cleared explicitly by sending empty string. Detect intent
	// by checking whether the Room field was *present* in the JSON via a
	// trim-aware compare against the existing value: callers that want to
	// clear pass a single space or a sentinel; otherwise Room stays.
	if room := strings.TrimSpace(updates.Room); room != "" {
		socket.Room = room
	}

	if err := saveData(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, socket)
}

func deleteSocket(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	mu.Lock()
	if _, ok := sockets[id]; !ok {
		mu.Unlock()
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	delete(sockets, id)
	cascadeDeleteSocket(id)
	if err := saveData(); err != nil {
		mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// cascadeDeleteSocket removes a socket from every group/scene and deletes
// schedules and timers that target it directly. Caller must hold mu.
func cascadeDeleteSocket(socketID string) {
	for sid, s := range schedules {
		if s.TargetType == "socket" && s.TargetID == socketID {
			delete(schedules, sid)
		}
	}
	for tid, t := range timers {
		if t.TargetType == "socket" && t.TargetID == socketID {
			delete(timers, tid)
		}
	}
	for _, g := range groups {
		g.SocketIDs = filterStrings(g.SocketIDs, socketID)
	}
	for _, sc := range scenes {
		out := sc.Actions[:0]
		for _, a := range sc.Actions {
			if a.SocketID != socketID {
				out = append(out, a)
			}
		}
		sc.Actions = out
	}
}

func filterStrings(in []string, drop string) []string {
	out := in[:0]
	for _, s := range in {
		if s != drop {
			out = append(out, s)
		}
	}
	return out
}

// applyState changes a single socket's state and fires the RF command. The
// caller must hold mu (write lock). On RF failure the previous state is
// restored. saveData is intentionally NOT called here — callers batch.
func applyState(socket *Socket, target *bool) error {
	previous := socket.State
	if target == nil {
		socket.State = !socket.State
	} else {
		socket.State = *target
	}
	if err := sendRFCode(socket.Code, socket.Protocol, socket.State); err != nil {
		socket.State = previous
		return err
	}
	return nil
}

func setSocketState(w http.ResponseWriter, r *http.Request, target *bool) {
	id := mux.Vars(r)["id"]

	mu.Lock()
	defer mu.Unlock()

	socket, ok := sockets[id]
	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	if err := applyState(socket, target); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send RF command: "+err.Error())
		return
	}
	if err := saveData(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, socket)
}

func toggleSocket(w http.ResponseWriter, r *http.Request) { setSocketState(w, r, nil) }
func turnOn(w http.ResponseWriter, r *http.Request) {
	on := true
	setSocketState(w, r, &on)
}
func turnOff(w http.ResponseWriter, r *http.Request) {
	off := false
	setSocketState(w, r, &off)
}

// bulkSetState returns a handler that switches every socket on or off.
// It returns the number of successes and a list of failures.
func bulkSetState(target bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		var ok int
		failures := make([]map[string]string, 0)
		for _, s := range sockets {
			if err := applyState(s, &target); err != nil {
				failures = append(failures, map[string]string{
					"socket_id": s.ID,
					"error":     err.Error(),
				})
				continue
			}
			ok++
		}
		if err := saveData(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"updated":  ok,
			"failures": failures,
		})
	}
}

// roomSetState returns a handler that switches every socket in a single room.
func roomSetState(target bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		room := strings.TrimSpace(mux.Vars(r)["room"])
		if room == "" {
			writeError(w, http.StatusBadRequest, "room is required")
			return
		}

		mu.Lock()
		defer mu.Unlock()

		var ok int
		failures := make([]map[string]string, 0)
		var matched bool
		for _, s := range sockets {
			if !strings.EqualFold(s.Room, room) {
				continue
			}
			matched = true
			if err := applyState(s, &target); err != nil {
				failures = append(failures, map[string]string{
					"socket_id": s.ID,
					"error":     err.Error(),
				})
				continue
			}
			ok++
		}
		if !matched {
			writeError(w, http.StatusNotFound, "no sockets in that room")
			return
		}
		if err := saveData(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"room":     room,
			"updated":  ok,
			"failures": failures,
		})
	}
}

// getRooms returns rooms with their socket counts and on-counts.
func getRooms(w http.ResponseWriter, r *http.Request) {
	type roomSummary struct {
		Name    string `json:"name"`
		Sockets int    `json:"sockets"`
		On      int    `json:"on"`
	}
	mu.RLock()
	byName := make(map[string]*roomSummary)
	for _, s := range sockets {
		name := s.Room
		if name == "" {
			name = "Unassigned"
		}
		rs, ok := byName[name]
		if !ok {
			rs = &roomSummary{Name: name}
			byName[name] = rs
		}
		rs.Sockets++
		if s.State {
			rs.On++
		}
	}
	mu.RUnlock()

	out := make([]*roomSummary, 0, len(byName))
	for _, rs := range byName {
		out = append(out, rs)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSONResponse(w, http.StatusOK, out)
}

// Schedule handlers

func getSchedules(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	result := make([]*Schedule, 0, len(schedules))
	for _, s := range schedules {
		result = append(result, s)
	}
	mu.RUnlock()

	sort.Slice(result, func(i, j int) bool {
		if result[i].Time != result[j].Time {
			return result[i].Time < result[j].Time
		}
		return result[i].ID < result[j].ID
	})

	writeJSONResponse(w, http.StatusOK, result)
}

// validateSchedule normalizes and validates a schedule. Caller must hold mu
// (read lock at minimum) so target existence can be checked.
func validateSchedule(s *Schedule) error {
	s.SocketID = strings.TrimSpace(s.SocketID)
	s.TargetType = strings.ToLower(strings.TrimSpace(s.TargetType))
	s.TargetID = strings.TrimSpace(s.TargetID)
	s.Action = strings.ToLower(strings.TrimSpace(s.Action))
	s.Time = strings.TrimSpace(s.Time)

	// Backwards compat: socket_id alone implies a socket target.
	if s.TargetType == "" && s.SocketID != "" {
		s.TargetType = "socket"
		s.TargetID = s.SocketID
	}
	if s.TargetType == "socket" {
		s.SocketID = s.TargetID
	} else {
		s.SocketID = ""
	}

	switch s.TargetType {
	case "socket":
		if s.TargetID == "" {
			return errors.New("socket_id (or target_id) is required")
		}
		if _, ok := sockets[s.TargetID]; !ok {
			return errors.New("target socket does not exist")
		}
		if s.Action != "on" && s.Action != "off" && s.Action != "toggle" {
			return errors.New("socket action must be on/off/toggle")
		}
	case "group":
		if s.TargetID == "" {
			return errors.New("target_id is required for group schedules")
		}
		if _, ok := groups[s.TargetID]; !ok {
			return errors.New("target group does not exist")
		}
		if s.Action != "on" && s.Action != "off" && s.Action != "toggle" {
			return errors.New("group action must be on/off/toggle")
		}
	case "scene":
		if s.TargetID == "" {
			return errors.New("target_id is required for scene schedules")
		}
		if _, ok := scenes[s.TargetID]; !ok {
			return errors.New("target scene does not exist")
		}
		s.Action = "activate"
	default:
		return errors.New("target_type must be socket, group, or scene")
	}

	if _, err := time.Parse("15:04", s.Time); err != nil {
		return errors.New("time must be in HH:MM format")
	}
	for _, d := range s.Days {
		if d < 0 || d > 6 {
			return errors.New("days values must be 0-6 (Sun-Sat)")
		}
	}
	return nil
}

func createSchedule(w http.ResponseWriter, r *http.Request) {
	var schedule Schedule
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if err := validateSchedule(&schedule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if schedule.ID == "" {
		schedule.ID = fmt.Sprintf("schedule_%d", time.Now().UnixNano())
	}

	schedules[schedule.ID] = &schedule
	if err := saveData(); err != nil {
		delete(schedules, schedule.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}

	writeJSONResponse(w, http.StatusCreated, schedule)
}

func updateSchedule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates Schedule
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	existing, ok := schedules[id]
	if !ok {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}

	// Build merged schedule and validate it whole.
	merged := *existing
	if v := strings.TrimSpace(updates.SocketID); v != "" {
		merged.SocketID = v
	}
	if v := strings.TrimSpace(updates.TargetType); v != "" {
		merged.TargetType = v
	}
	if v := strings.TrimSpace(updates.TargetID); v != "" {
		merged.TargetID = v
	}
	if v := strings.TrimSpace(updates.Action); v != "" {
		merged.Action = v
	}
	if v := strings.TrimSpace(updates.Time); v != "" {
		merged.Time = v
	}
	if updates.Days != nil {
		merged.Days = updates.Days
	}
	merged.Enabled = updates.Enabled

	if err := validateSchedule(&merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	*existing = merged
	if err := saveData(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, existing)
}

func deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	mu.Lock()
	if _, ok := schedules[id]; !ok {
		mu.Unlock()
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	delete(schedules, id)
	if err := saveData(); err != nil {
		mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// runScheduler ticks every 5 seconds: fires any due one-shot timers, and
// (every 30s minimum) any enabled schedules whose HH:MM + weekday match
// the current local time. Deduplicated within a minute.
func runScheduler(ctx context.Context) {
	lastFired := make(map[string]string)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		now := time.Now()
		stamp := now.Format("2006-01-02 15:04")
		hhmm := now.Format("15:04")
		weekday := int(now.Weekday())

		// Collect due schedules and timers under a read lock.
		var dueSchedules []Schedule
		var dueTimers []Timer
		mu.RLock()
		for _, s := range schedules {
			if !s.Enabled || s.Time != hhmm {
				continue
			}
			if !dayMatches(s.Days, weekday) {
				continue
			}
			if lastFired[s.ID] == stamp {
				continue
			}
			dueSchedules = append(dueSchedules, *s)
		}
		for _, t := range timers {
			if !now.Before(t.FiresAt) {
				dueTimers = append(dueTimers, *t)
			}
		}
		mu.RUnlock()

		for _, s := range dueSchedules {
			lastFired[s.ID] = stamp
			if err := executeSchedule(s); err != nil {
				log.Printf("scheduler: schedule %s failed: %v", s.ID, err)
			}
		}
		for _, t := range dueTimers {
			if err := executeTimer(t); err != nil {
				log.Printf("scheduler: timer %s failed: %v", t.ID, err)
			}
		}
	}
}

// executeTimer fires a one-shot timer and removes it from the persistent
// store regardless of success — the user already saw it scheduled and will
// see the resulting state on the next refresh.
func executeTimer(t Timer) error {
	mu.Lock()
	defer mu.Unlock()

	delete(timers, t.ID)
	err := executeAction(t.TargetType, t.TargetID, t.Action)
	if saveErr := saveData(); err == nil && saveErr != nil {
		err = saveErr
	}
	if err == nil {
		log.Printf("timer fired: %s on %s/%s", t.Action, t.TargetType, t.TargetID)
	}
	return err
}

func dayMatches(days []int, weekday int) bool {
	if len(days) == 0 {
		return true
	}
	for _, d := range days {
		if d == weekday {
			return true
		}
	}
	return false
}

// executeAction runs the given action against the given target. Caller must
// hold mu (write lock). Returns an error per-target failure but does NOT
// persist — callers batch saveData.
func executeAction(targetType, targetID, action string) error {
	switch targetType {
	case "socket":
		socket, ok := sockets[targetID]
		if !ok {
			return fmt.Errorf("socket %q no longer exists", targetID)
		}
		var target *bool
		switch action {
		case "on":
			t := true
			target = &t
		case "off":
			t := false
			target = &t
		case "toggle":
			target = nil
		default:
			return fmt.Errorf("unsupported socket action %q", action)
		}
		return applyState(socket, target)

	case "group":
		group, ok := groups[targetID]
		if !ok {
			return fmt.Errorf("group %q no longer exists", targetID)
		}
		var firstErr error
		for _, sid := range group.SocketIDs {
			if err := executeAction("socket", sid, action); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr

	case "scene":
		scene, ok := scenes[targetID]
		if !ok {
			return fmt.Errorf("scene %q no longer exists", targetID)
		}
		var firstErr error
		for _, a := range scene.Actions {
			if err := executeAction("socket", a.SocketID, a.Action); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr

	default:
		return fmt.Errorf("unsupported target type %q", targetType)
	}
}

func executeSchedule(s Schedule) error {
	mu.Lock()
	defer mu.Unlock()

	tt, tid, action := s.TargetType, s.TargetID, s.Action
	if tt == "" && s.SocketID != "" {
		tt, tid = "socket", s.SocketID
	}
	if err := executeAction(tt, tid, action); err != nil {
		return err
	}

	if existing, ok := schedules[s.ID]; ok {
		existing.LastFiredAt = time.Now().UTC()
	}
	if err := saveData(); err != nil {
		return err
	}
	log.Printf("scheduler: %s %s (%s/%s)", action, s.ID, tt, tid)
	return nil
}

// ---- Groups ----

func getGroups(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	out := make([]*Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, g)
	}
	mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSONResponse(w, http.StatusOK, out)
}

func getGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	mu.RLock()
	g, ok := groups[id]
	mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	writeJSONResponse(w, http.StatusOK, g)
}

// validateGroup normalizes a group, dedupes its socket IDs and verifies
// every member exists. Caller must hold mu.
func validateGroup(g *Group) error {
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" {
		return errors.New("name is required")
	}
	seen := make(map[string]bool, len(g.SocketIDs))
	out := make([]string, 0, len(g.SocketIDs))
	for _, sid := range g.SocketIDs {
		sid = strings.TrimSpace(sid)
		if sid == "" || seen[sid] {
			continue
		}
		if _, ok := sockets[sid]; !ok {
			return fmt.Errorf("unknown socket %q", sid)
		}
		seen[sid] = true
		out = append(out, sid)
	}
	g.SocketIDs = out
	return nil
}

func createGroup(w http.ResponseWriter, r *http.Request) {
	var g Group
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if err := validateGroup(&g); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.ID == "" {
		g.ID = fmt.Sprintf("group_%d", time.Now().UnixNano())
	}
	groups[g.ID] = &g
	if err := saveData(); err != nil {
		delete(groups, g.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusCreated, g)
}

func updateGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates Group
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	existing, ok := groups[id]
	if !ok {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	merged := *existing
	if name := strings.TrimSpace(updates.Name); name != "" {
		merged.Name = name
	}
	if updates.SocketIDs != nil {
		merged.SocketIDs = updates.SocketIDs
	}
	if err := validateGroup(&merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	*existing = merged
	if err := saveData(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, existing)
}

func deleteGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	mu.Lock()
	if _, ok := groups[id]; !ok {
		mu.Unlock()
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	delete(groups, id)
	for sid, s := range schedules {
		if s.TargetType == "group" && s.TargetID == id {
			delete(schedules, sid)
		}
	}
	for tid, t := range timers {
		if t.TargetType == "group" && t.TargetID == id {
			delete(timers, tid)
		}
	}
	if err := saveData(); err != nil {
		mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// groupAction returns a handler that applies an action to every member of a
// group.
func groupAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]

		mu.Lock()
		defer mu.Unlock()

		g, ok := groups[id]
		if !ok {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		var ok2 int
		failures := make([]map[string]string, 0)
		for _, sid := range g.SocketIDs {
			if err := executeAction("socket", sid, action); err != nil {
				failures = append(failures, map[string]string{
					"socket_id": sid,
					"error":     err.Error(),
				})
				continue
			}
			ok2++
		}
		if err := saveData(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"group":    g.Name,
			"updated":  ok2,
			"failures": failures,
		})
	}
}

// ---- Scenes ----

func getScenes(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	out := make([]*Scene, 0, len(scenes))
	for _, s := range scenes {
		out = append(out, s)
	}
	mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSONResponse(w, http.StatusOK, out)
}

func getScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	mu.RLock()
	s, ok := scenes[id]
	mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}
	writeJSONResponse(w, http.StatusOK, s)
}

// validateScene checks that every socket referenced by the scene exists and
// that each action is on/off. Caller must hold mu.
func validateScene(s *Scene) error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return errors.New("name is required")
	}
	seen := make(map[string]bool, len(s.Actions))
	out := make([]SceneAction, 0, len(s.Actions))
	for _, a := range s.Actions {
		a.SocketID = strings.TrimSpace(a.SocketID)
		a.Action = strings.ToLower(strings.TrimSpace(a.Action))
		if a.SocketID == "" || seen[a.SocketID] {
			continue
		}
		if a.Action != "on" && a.Action != "off" {
			return errors.New("scene action must be on or off")
		}
		if _, ok := sockets[a.SocketID]; !ok {
			return fmt.Errorf("unknown socket %q", a.SocketID)
		}
		seen[a.SocketID] = true
		out = append(out, a)
	}
	s.Actions = out
	return nil
}

func createScene(w http.ResponseWriter, r *http.Request) {
	var s Scene
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if err := validateScene(&s); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.ID == "" {
		s.ID = fmt.Sprintf("scene_%d", time.Now().UnixNano())
	}
	scenes[s.ID] = &s
	if err := saveData(); err != nil {
		delete(scenes, s.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusCreated, s)
}

func updateScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates Scene
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	existing, ok := scenes[id]
	if !ok {
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}
	merged := *existing
	if name := strings.TrimSpace(updates.Name); name != "" {
		merged.Name = name
	}
	if updates.Actions != nil {
		merged.Actions = updates.Actions
	}
	if err := validateScene(&merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	*existing = merged
	if err := saveData(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, existing)
}

func deleteScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	mu.Lock()
	if _, ok := scenes[id]; !ok {
		mu.Unlock()
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}
	delete(scenes, id)
	for sid, s := range schedules {
		if s.TargetType == "scene" && s.TargetID == id {
			delete(schedules, sid)
		}
	}
	for tid, t := range timers {
		if t.TargetType == "scene" && t.TargetID == id {
			delete(timers, tid)
		}
	}
	if err := saveData(); err != nil {
		mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func activateSceneHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	mu.Lock()
	defer mu.Unlock()

	scene, ok := scenes[id]
	if !ok {
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}

	var okCount int
	failures := make([]map[string]string, 0)
	for _, a := range scene.Actions {
		if err := executeAction("socket", a.SocketID, a.Action); err != nil {
			failures = append(failures, map[string]string{
				"socket_id": a.SocketID,
				"error":     err.Error(),
			})
			continue
		}
		okCount++
	}
	if err := saveData(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"scene":    scene.Name,
		"updated":  okCount,
		"failures": failures,
	})
}

// ---- Timers (one-shot) ----

func getTimers(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	out := make([]*Timer, 0, len(timers))
	for _, t := range timers {
		out = append(out, t)
	}
	mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].FiresAt.Before(out[j].FiresAt) })
	writeJSONResponse(w, http.StatusOK, out)
}

// timerRequest is the JSON shape clients use to schedule a one-shot timer.
// Either FiresAt (RFC3339) or InSeconds must be set.
type timerRequest struct {
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Action     string    `json:"action"`
	FiresAt    time.Time `json:"fires_at,omitempty"`
	InSeconds  int       `json:"in_seconds,omitempty"`
	Note       string    `json:"note,omitempty"`
}

func (req *timerRequest) toTimer() (*Timer, error) {
	tt := strings.ToLower(strings.TrimSpace(req.TargetType))
	tid := strings.TrimSpace(req.TargetID)
	action := strings.ToLower(strings.TrimSpace(req.Action))

	if tt == "" || tid == "" {
		return nil, errors.New("target_type and target_id are required")
	}
	switch tt {
	case "socket", "group":
		if action != "on" && action != "off" && action != "toggle" {
			return nil, errors.New("action must be on/off/toggle")
		}
	case "scene":
		action = "activate"
	default:
		return nil, errors.New("target_type must be socket, group, or scene")
	}

	var firesAt time.Time
	switch {
	case !req.FiresAt.IsZero():
		firesAt = req.FiresAt
	case req.InSeconds > 0:
		firesAt = time.Now().Add(time.Duration(req.InSeconds) * time.Second)
	default:
		return nil, errors.New("either fires_at or in_seconds is required")
	}
	if !firesAt.After(time.Now().Add(-time.Second)) {
		return nil, errors.New("fires_at must be in the future")
	}

	now := time.Now()
	return &Timer{
		ID:         fmt.Sprintf("timer_%d", now.UnixNano()),
		TargetType: tt,
		TargetID:   tid,
		Action:     action,
		FiresAt:    firesAt,
		CreatedAt:  now,
		Note:       strings.TrimSpace(req.Note),
	}, nil
}

func createTimer(w http.ResponseWriter, r *http.Request) {
	var req timerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	t, err := req.toTimer()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if err := verifyTarget(t.TargetType, t.TargetID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	timers[t.ID] = t
	if err := saveData(); err != nil {
		delete(timers, t.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusCreated, t)
}

// createSocketTimer is a convenience for "off in N seconds" from a socket
// card. The path supplies the socket id; the body supplies action and
// in_seconds (or fires_at).
func createSocketTimer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var req timerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	req.TargetType = "socket"
	req.TargetID = id

	t, err := req.toTimer()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, ok := sockets[id]; !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	timers[t.ID] = t
	if err := saveData(); err != nil {
		delete(timers, t.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSONResponse(w, http.StatusCreated, t)
}

func deleteTimer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	mu.Lock()
	if _, ok := timers[id]; !ok {
		mu.Unlock()
		writeError(w, http.StatusNotFound, "timer not found")
		return
	}
	delete(timers, id)
	if err := saveData(); err != nil {
		mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// verifyTarget checks that a target_type/target_id pair refers to an
// existing entity. Caller must hold mu.
func verifyTarget(tt, tid string) error {
	switch tt {
	case "socket":
		if _, ok := sockets[tid]; !ok {
			return errors.New("target socket does not exist")
		}
	case "group":
		if _, ok := groups[tid]; !ok {
			return errors.New("target group does not exist")
		}
	case "scene":
		if _, ok := scenes[tid]; !ok {
			return errors.New("target scene does not exist")
		}
	default:
		return errors.New("invalid target_type")
	}
	return nil
}

// Persistence

const (
	socketsFile   = "sockets.json"
	schedulesFile = "schedules.json"
	groupsFile    = "groups.json"
	scenesFile    = "scenes.json"
	timersFile    = "timers.json"
)

// loadData reads everything from disk. A missing file is not an error — it
// simply means we are starting fresh. After loading, legacy schedules
// (socket_id-only, no target_type) are normalized into the new shape.
func loadData() error {
	if err := readJSON(filepath.Join(dataDir, socketsFile), &sockets); err != nil {
		return fmt.Errorf("loading sockets: %w", err)
	}
	if err := readJSON(filepath.Join(dataDir, schedulesFile), &schedules); err != nil {
		return fmt.Errorf("loading schedules: %w", err)
	}
	if err := readJSON(filepath.Join(dataDir, groupsFile), &groups); err != nil {
		return fmt.Errorf("loading groups: %w", err)
	}
	if err := readJSON(filepath.Join(dataDir, scenesFile), &scenes); err != nil {
		return fmt.Errorf("loading scenes: %w", err)
	}
	if err := readJSON(filepath.Join(dataDir, timersFile), &timers); err != nil {
		return fmt.Errorf("loading timers: %w", err)
	}
	if sockets == nil {
		sockets = make(map[string]*Socket)
	}
	if schedules == nil {
		schedules = make(map[string]*Schedule)
	}
	if groups == nil {
		groups = make(map[string]*Group)
	}
	if scenes == nil {
		scenes = make(map[string]*Scene)
	}
	if timers == nil {
		timers = make(map[string]*Timer)
	}

	// Normalize legacy schedules.
	for _, s := range schedules {
		if s.TargetType == "" {
			if s.SocketID != "" {
				s.TargetType = "socket"
				s.TargetID = s.SocketID
			}
		}
	}
	return nil
}

// saveData writes every file atomically. Callers must hold mu.
func saveData() error {
	if err := writeJSON(filepath.Join(dataDir, socketsFile), sockets); err != nil {
		return fmt.Errorf("saving sockets: %w", err)
	}
	if err := writeJSON(filepath.Join(dataDir, schedulesFile), schedules); err != nil {
		return fmt.Errorf("saving schedules: %w", err)
	}
	if err := writeJSON(filepath.Join(dataDir, groupsFile), groups); err != nil {
		return fmt.Errorf("saving groups: %w", err)
	}
	if err := writeJSON(filepath.Join(dataDir, scenesFile), scenes); err != nil {
		return fmt.Errorf("saving scenes: %w", err)
	}
	if err := writeJSON(filepath.Join(dataDir, timersFile), timers); err != nil {
		return fmt.Errorf("saving timers: %w", err)
	}
	return nil
}

func readJSON(path string, v interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}

// writeJSON writes via a temp file + rename so a crash mid-write cannot
// corrupt the on-disk state.
func writeJSON(path string, v interface{}) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
