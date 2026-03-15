package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
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
	Action   string `json:"action"`   // "on" or "off"
	Time     string `json:"time"`     // "HH:MM" format
	Days     []int  `json:"days"`     // 0=Sun, 1=Mon, etc
	Enabled  bool   `json:"enabled"`
}

var (
	sockets   = make(map[string]*Socket)
	schedules = make(map[string]*Schedule)
	dataDir   = "./data"
)

func main() {
	// Create data directory
	os.MkdirAll(dataDir, 0755)

	// Load saved data
	loadData()

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

	var result []*Socket
	for _, s := range sockets {
		result = append(result, s)
	}

	json.NewEncoder(w).Encode(result)
}

func createSocket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var socket Socket
	if err := json.NewDecoder(r.Body).Decode(&socket); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if socket.ID == "" {
		socket.ID = fmt.Sprintf("socket_%d", time.Now().Unix())
	}

	sockets[socket.ID] = &socket
	saveData()

	json.NewEncoder(w).Encode(socket)
}

func getSocket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	socket, ok := sockets[id]
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

	socket, ok := sockets[id]
	if !ok {
		http.Error(w, "Socket not found", http.StatusNotFound)
		return
	}

	var updates Socket
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update fields
	if updates.Name != "" {
		socket.Name = updates.Name
	}
	if updates.Code != "" {
		socket.Code = updates.Code
	}
	if updates.Protocol != "" {
		socket.Protocol = updates.Protocol
	}
	if updates.Room != "" {
		socket.Room = updates.Room
	}

	saveData()
	json.NewEncoder(w).Encode(socket)
}

func deleteSocket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	delete(sockets, id)
	saveData()

	w.WriteHeader(http.StatusNoContent)
}

func toggleSocket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	socket, ok := sockets[id]
	if !ok {
		http.Error(w, "Socket not found", http.StatusNotFound)
		return
	}

	// Toggle state
	socket.State = !socket.State

	// Send RF command
	if err := sendRFCode(socket.Code, socket.Protocol, socket.State); err != nil {
		// Revert state on error
		socket.State = !socket.State
		http.Error(w, fmt.Sprintf("Failed to send RF command: %v", err), http.StatusInternalServerError)
		return
	}

	saveData()
	json.NewEncoder(w).Encode(socket)
}

func turnOn(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	socket, ok := sockets[id]
	if !ok {
		http.Error(w, "Socket not found", http.StatusNotFound)
		return
	}

	socket.State = true

	if err := sendRFCode(socket.Code, socket.Protocol, true); err != nil {
		socket.State = false
		http.Error(w, fmt.Sprintf("Failed to send RF command: %v", err), http.StatusInternalServerError)
		return
	}

	saveData()
	json.NewEncoder(w).Encode(socket)
}

func turnOff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	socket, ok := sockets[id]
	if !ok {
		http.Error(w, "Socket not found", http.StatusNotFound)
		return
	}

	socket.State = false

	if err := sendRFCode(socket.Code, socket.Protocol, false); err != nil {
		socket.State = true
		http.Error(w, fmt.Sprintf("Failed to send RF command: %v", err), http.StatusInternalServerError)
		return
	}

	saveData()
	json.NewEncoder(w).Encode(socket)
}

// Schedule handlers
func getSchedules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var result []*Schedule
	for _, s := range schedules {
		result = append(result, s)
	}

	json.NewEncoder(w).Encode(result)
}

func createSchedule(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var schedule Schedule
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if schedule.ID == "" {
		schedule.ID = fmt.Sprintf("schedule_%d", time.Now().Unix())
	}

	schedules[schedule.ID] = &schedule
	saveData()