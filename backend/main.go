package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
	// Create data directory
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("failed to create data directory %q: %v", dataDir, err)
	}

	// Load saved data
	if err := loadData(); err != nil {
		log.Fatalf("failed to load data: %v", err)
	}

	// Setup router
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/sockets", getSockets).Methods("GET")
	api.HandleFunc("/sockets", createSocket).Methods("POST")
	api.HandleFunc("/sockets/{id}", getSocket).Methods("GET")
	api.HandleFunc("/sockets/{id}", updateSocket).Methods("PUT")
	api.HandleFunc("/sockets/{id}", deleteSocket).Methods("DELETE")
	api.HandleFunc("/sockets/{id}/toggle", toggleSocket).Methods("POST")
	api.HandleFunc("/sockets/{id}/on", turnOn).Methods("POST")
	api.HandleFunc("/sockets/{id}/off", turnOff).Methods("POST")

	api.HandleFunc("/schedules", getSchedules).Methods("GET")
	api.HandleFunc("/schedules", createSchedule).Methods("POST")
	api.HandleFunc("/schedules/{id}", deleteSchedule).Methods("DELETE")

	// Static files (frontend)
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./frontend/")))

	// CORS
	cors := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)

	// Start scheduler goroutine
	go runScheduler()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🚀 RF Socket Controller API starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, cors(r)))
}

// 433MHz Control Functions
func sendRFCode(code string, protocol string, state bool) error {
	// Try to use existing tools
	var cmd *exec.Cmd

	// Check for rpi-rf_send (from rpi-rf library)
	if _, err := exec.LookPath("rpi-rf_send"); err == nil {
		cmd = exec.Command("rpi-rf_send", code)
	} else if _, err := exec.LookPath("codesend"); err == nil { // wiringPi
		cmd = exec.Command("codesend", code)
	} else {
		// Fallback: simulate for testing
		fmt.Printf("[SIMULATION] Would send RF code %s (protocol: %s, state: %v)\n", code, protocol, state)
		return nil
	}

	return cmd.Run()
}

// API Handlers
func getSockets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	mu.RLock()
	result := make([]*Socket, 0, len(sockets))
	for _, s := range sockets {
		result = append(result, s)
	}
	mu.RUnlock()

	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })

	json.NewEncoder(w).Encode(result)
}

func createSocket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var socket Socket
	if err := json.NewDecoder(r.Body).Decode(&socket); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	socket.Name = strings.TrimSpace(socket.Name)
	socket.Code = strings.TrimSpace(socket.Code)
	socket.Protocol = strings.TrimSpace(socket.Protocol)
	socket.Room = strings.TrimSpace(socket.Room)

	if socket.Name == "" || socket.Code == "" {
		http.Error(w, "name and code are required", http.StatusBadRequest)
		return
	}

	if socket.ID == "" {
		socket.ID = fmt.Sprintf("socket_%d", time.Now().UnixNano())
	}

	mu.Lock()
	sockets[socket.ID] = &socket
	if err := saveData(); err != nil {
		mu.Unlock()
		http.Error(w, fmt.Sprintf("failed to persist data: %v", err), http.StatusInternalServerError)
		return
	}
	mu.Unlock()

	json.NewEncoder(w).Encode(socket)
}

func getSocket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	mu.RLock()
	socket, ok := sockets[id]
	mu.RUnlock()
	if !ok {
		http.Error(w, "Socket not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(socket)
}

func updateSocket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	var updates Socket
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	socket, ok := sockets[id]
	if !ok {
		http.Error(w, "Socket not found", http.StatusNotFound)
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
	if room := strings.TrimSpace(updates.Room); room != "" {
		socket.Room = room
	}

	if err := saveData(); err != nil {
		http.Error(w, fmt.Sprintf("failed to persist data: %v", err), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(socket)
}

func deleteSocket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	mu.Lock()
	if _, ok := sockets[id]; !ok {
		mu.Unlock()
		http.Error(w, "Socket not found", http.StatusNotFound)
		return
	}
	delete(sockets, id)
	// Also drop schedules referencing this socket so the scheduler does not try
	// to fire commands against a missing target.
	for sid, s := range schedules {
		if s.SocketID == id {
			delete(schedules, sid)
		}
	}
	if err := saveData(); err != nil {
		mu.Unlock()
		http.Error(w, fmt.Sprintf("failed to persist data: %v", err), http.StatusInternalServerError)
		return
	}
	mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func setSocketState(w http.ResponseWriter, r *http.Request, target *bool) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	mu.Lock()
	defer mu.Unlock()

	socket, ok := sockets[id]
	if !ok {
		http.Error(w, "Socket not found", http.StatusNotFound)
		return
	}

	previous := socket.State
	if target == nil {
		socket.State = !socket.State
	} else {
		socket.State = *target
	}

	if err := sendRFCode(socket.Code, socket.Protocol, socket.State); err != nil {
		socket.State = previous
		http.Error(w, fmt.Sprintf("Failed to send RF command: %v", err), http.StatusInternalServerError)
		return
	}

	if err := saveData(); err != nil {
		http.Error(w, fmt.Sprintf("failed to persist data: %v", err), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(socket)
}

func toggleSocket(w http.ResponseWriter, r *http.Request) {
	setSocketState(w, r, nil)
}

func turnOn(w http.ResponseWriter, r *http.Request) {
	on := true
	setSocketState(w, r, &on)
}

func turnOff(w http.ResponseWriter, r *http.Request) {
	off := false
	setSocketState(w, r, &off)
}

// Schedule handlers
func getSchedules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	mu.RLock()
	result := make([]*Schedule, 0, len(schedules))
	for _, s := range schedules {
		result = append(result, s)
	}
	mu.RUnlock()

	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })

	json.NewEncoder(w).Encode(result)
}

func createSchedule(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var schedule Schedule
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	schedule.SocketID = strings.TrimSpace(schedule.SocketID)
	schedule.Action = strings.ToLower(strings.TrimSpace(schedule.Action))
	schedule.Time = strings.TrimSpace(schedule.Time)

	if schedule.SocketID == "" {
		http.Error(w, "socket_id is required", http.StatusBadRequest)
		return
	}
	if schedule.Action != "on" && schedule.Action != "off" {
		http.Error(w, "action must be 'on' or 'off'", http.StatusBadRequest)
		return
	}
	if _, err := time.Parse("15:04", schedule.Time); err != nil {
		http.Error(w, "time must be in HH:MM format", http.StatusBadRequest)
		return
	}
	for _, d := range schedule.Days {
		if d < 0 || d > 6 {
			http.Error(w, "days values must be 0-6 (Sun-Sat)", http.StatusBadRequest)
			return
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if _, ok := sockets[schedule.SocketID]; !ok {
		http.Error(w, "socket_id refers to unknown socket", http.StatusBadRequest)
		return
	}

	if schedule.ID == "" {
		schedule.ID = fmt.Sprintf("schedule_%d", time.Now().UnixNano())
	}

	schedules[schedule.ID] = &schedule
	if err := saveData(); err != nil {
		delete(schedules, schedule.ID)
		http.Error(w, fmt.Sprintf("failed to persist data: %v", err), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(schedule)
}

func deleteSchedule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	mu.Lock()
	if _, ok := schedules[id]; !ok {
		mu.Unlock()
		http.Error(w, "Schedule not found", http.StatusNotFound)
		return
	}
	delete(schedules, id)
	if err := saveData(); err != nil {
		mu.Unlock()
		http.Error(w, fmt.Sprintf("failed to persist data: %v", err), http.StatusInternalServerError)
		return
	}
	mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// runScheduler ticks every 30 seconds and fires any enabled schedules whose
// HH:MM and weekday match the current local time. It deduplicates within a
// minute by tracking the last time each schedule fired.
func runScheduler() {
	lastFired := make(map[string]string)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
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
