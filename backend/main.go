package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/handlers"
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
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./frontend/dist/")))
	
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
	
	json.NewEncoder(w).Encode(schedule)
}

func deleteSchedule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	delete(schedules, id)
	saveData()
	
	w.WriteHeader(http.StatusNoContent)
}

// Scheduler goroutine
func runScheduler() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		now := time.Now()
		currentTime := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())
		currentDay := int(now.Weekday())
		
		for _, schedule := range schedules {
			if !schedule.Enabled {
				continue
			}
			
			// Check if schedule should run now
			if schedule.Time == currentTime {
				// Check day
				for _, day := range schedule.Days {
					if day == currentDay {
						// Execute schedule
						executeSchedule(schedule)
						break
					}
				}
			}
		}
	}
}

func executeSchedule(schedule *Schedule) {
	socket, ok := sockets[schedule.SocketID]
	if !ok {
		log.Printf("Schedule %s: socket %s not found", schedule.ID, schedule.SocketID)
		return
	}
	
	var err error
	if schedule.Action == "on" {
		socket.State = true
		err = sendRFCode(socket.Code, socket.Protocol, true)
	} else {
		socket.State = false
		err = sendRFCode(socket.Code, socket.Protocol, false)
	}
	
	if err != nil {
		log.Printf("Schedule %s execution failed: %v", schedule.ID, err)
	} else {
		log.Printf("Schedule %s executed: %s %s -> %s", schedule.ID, socket.Name, schedule.Action)
		saveData()
	}
}

// Data persistence
func saveData() {
	// Save sockets
	socketData, _ := json.MarshalIndent(sockets, "", "  ")
	os.WriteFile(dataDir+"/sockets.json", socketData, 0644)
	
	// Save schedules
	scheduleData, _ := json.MarshalIndent(schedules, "", "  ")
	os.WriteFile(dataDir+"/schedules.json", scheduleData, 0644)
}

func loadData() {
	// Load sockets
	if data, err := os.ReadFile(dataDir + "/sockets.json"); err == nil {
		json.Unmarshal(data, &sockets)
	}
	
	// Load schedules
	if data, err := os.ReadFile(dataDir + "/schedules.json"); err == nil {
		json.Unmarshal(data, &schedules)
	}
}

func showHelpText() {
	fmt.Println(`modbusfinder - The Ultimate Modbus Register Finder for PCS, BMS, and EVSE

USAGE:
  modbusfinder --brand=<brand> --model=<model> [options]

EXAMPLES:
  # Get all registers for a Sinexcel PWS2-100K
  modbusfinder --brand=Sinexcel --model=PWS2-100K

  # Get registers as JSON
  modbusfinder --brand=Sinexcel --model=PWS2-100K --json

  # Search for specific registers (e.g., all SoC related)
  modbusfinder --brand=Sinexcel --model=PWS2-100K --search=SoC

  # Get only alarm/event registers
  modbusfinder --brand=Sinexcel --model=PWS2-100K --registers=alarm

  # Get multiple register types (comma-separated)
  modbusfinder --brand=Sinexcel --model=PWS2-100K --registers=alarm,control,status

  # List all supported brands
  modbusfinder --list-brands

  # List all models for Sungrow
  modbusfinder --brand=Sungrow --list-models

  # Find all PCS devices from SMA
  modbusfinder --type=PCS --brand=SMA

  # Find all EVSE devices
  modbusfinder --type=EVSE

SUPPORTED BRANDS:
  PCS:    Sinexcel, Sungrow, SMA, Fronius, ABB, Delta, Chint Power
  BMS:    BYD, Sungrow, SMA (Sunny Island), Generic SunSpec
  EVSE:   EVB, ABB (Terra), Delta, Generic

DEVICE TYPES:
  PCS    - Power Conversion System (inverters, bidirectional converters)
  BMS    - Battery Management System
  EVSE   - Electric Vehicle Supply Equipment (chargers)
  Hybrid - Combined inverter + battery systems

OUTPUT FORMATS:
  Default: Human-readable table with register details
  --json:  JSON format for programmatic use

REGISTER DETAILS INCLUDE:
  • Address (Modbus register number)
  • Name and Label (human-readable)
  • Type (uint16, float32, enum16, etc.)
  • Size (number of 16-bit registers)
  • Units (V, A, W, %, etc.)
  • Access (R=Read, RW=Read/Write)
  • Scale Factor (if applicable)
  • Description
  • Enum/Bitfield values (for status registers)

REGISTER TYPE FILTERS:
  alarm       - Events, faults, warnings, error registers
  measurement - Voltage, current, power, frequency, temperature
  control     - Writable registers, limits, setpoints, commands
  status      - Operating state, SoC, SoH, mode registers
  energy      - Energy counters, watt-hours, amp-hours
  metadata    - Scale factors, IDs, model info
  all         - All registers (default)

TIPS:
  • Brand and model matching is case-insensitive
  • Partial model names work: PWS500 matches PWS500-KTL-EX
  • Use --search to filter large register maps
  • Use --registers to filter by register category
  • Combine --search and --registers for precise filtering
  • The tool supports SunSpec standard + proprietary extensions`)
}

func listAllBrands(finder *modbusfinder.Finder) {
	brands := finder.ListBrands()
	fmt.Println("Supported Brands:")
	fmt.Println(strings.Repeat("-", 40))
	for _, brand := range brands {
		fmt.Printf("  • %s\n", brand)
	}
	fmt.Printf("\nTotal: %d brands\n", len(brands))
}

func listBrandModels(finder *modbusfinder.Finder, brand string) {
	models := finder.ListModels(brand)
	if len(models) == 0 {
		fmt.Fprintf(os.Stderr, "No models found for brand: %s\n", brand)
		os.Exit(1)
	}
	fmt.Printf("Models for %s:\n", brand)
	fmt.Println(strings.Repeat("-", 50))
	for _, model := range models {
		device := finder.FindDevice(brand, model)
		if device != nil {
			fmt.Printf("  • %-20s | %s | %d registers\n", model, device.Type, len(device.Registers))
		}
	}
}
