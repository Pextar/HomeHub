package main

import (
	"context"
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

// Schedule represents a timer for a socket
type Schedule struct {
	ID       string `json:"id"`
	SocketID string `json:"socket_id"`
	Action   string `json:"action"` // "on" or "off"
	Time     string `json:"time"`   // "HH:MM" format
	Days     []int  `json:"days"`   // 0=Sun, 1=Mon, etc
	Enabled  bool   `json:"enabled"`
}

var (
	mu        sync.RWMutex
	sockets   = make(map[string]*Socket)
	schedules = make(map[string]*Schedule)
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

	// Static files (frontend). Use a custom handler so /healthz-style probes
	// against unknown paths still get the SPA shell rather than directory
	// listings of the frontend folder.
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./frontend/")))

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
	mu.RUnlock()
	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"sockets":   socketCount,
		"schedules": scheduleCount,
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
	// Cascade: drop schedules pointing at this socket.
	for sid, s := range schedules {
		if s.SocketID == id {
			delete(schedules, sid)
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

func validateSchedule(s *Schedule) error {
	s.SocketID = strings.TrimSpace(s.SocketID)
	s.Action = strings.ToLower(strings.TrimSpace(s.Action))
	s.Time = strings.TrimSpace(s.Time)

	if s.SocketID == "" {
		return errors.New("socket_id is required")
	}
	if s.Action != "on" && s.Action != "off" {
		return errors.New("action must be 'on' or 'off'")
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
	if err := validateSchedule(&schedule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, ok := sockets[schedule.SocketID]; !ok {
		writeError(w, http.StatusBadRequest, "socket_id refers to unknown socket")
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
	if _, ok := sockets[merged.SocketID]; !ok {
		writeError(w, http.StatusBadRequest, "socket_id refers to unknown socket")
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

// runScheduler ticks every 30 seconds and fires any enabled schedules whose
// HH:MM and weekday match the current local time. It deduplicates within a
// minute by tracking the last time each schedule fired. It exits when ctx
// is cancelled.
func runScheduler(ctx context.Context) {
	lastFired := make(map[string]string)
	ticker := time.NewTicker(30 * time.Second)
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

		var due []Schedule
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
			due = append(due, *s)
		}
		mu.RUnlock()

		for _, s := range due {
			lastFired[s.ID] = stamp
			if err := executeSchedule(s); err != nil {
				log.Printf("scheduler: %s failed: %v", s.ID, err)
			}
		}
	}
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

func executeSchedule(s Schedule) error {
	mu.Lock()
	defer mu.Unlock()

	socket, ok := sockets[s.SocketID]
	if !ok {
		return fmt.Errorf("socket %q no longer exists", s.SocketID)
	}

	desired := s.Action == "on"
	previous := socket.State
	socket.State = desired

	if err := sendRFCode(socket.Code, socket.Protocol, desired); err != nil {
		socket.State = previous
		return err
	}

	if err := saveData(); err != nil {
		return err
	}
	log.Printf("scheduler: socket %s turned %s by schedule %s", socket.ID, s.Action, s.ID)
	return nil
}

// Persistence

const (
	socketsFile   = "sockets.json"
	schedulesFile = "schedules.json"
)

// loadData reads sockets and schedules from disk. A missing file is not an
// error — it simply means we are starting fresh.
func loadData() error {
	if err := readJSON(filepath.Join(dataDir, socketsFile), &sockets); err != nil {
		return fmt.Errorf("loading sockets: %w", err)
	}
	if err := readJSON(filepath.Join(dataDir, schedulesFile), &schedules); err != nil {
		return fmt.Errorf("loading schedules: %w", err)
	}
	if sockets == nil {
		sockets = make(map[string]*Socket)
	}
	if schedules == nil {
		schedules = make(map[string]*Schedule)
	}
	return nil
}

// saveData writes both files atomically. Callers must hold mu.
func saveData() error {
	if err := writeJSON(filepath.Join(dataDir, socketsFile), sockets); err != nil {
		return fmt.Errorf("saving sockets: %w", err)
	}
	if err := writeJSON(filepath.Join(dataDir, schedulesFile), schedules); err != nil {
		return fmt.Errorf("saving schedules: %w", err)
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
