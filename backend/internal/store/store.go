// Package store owns the in-memory state for sockets, schedules,
// groups, scenes and timers, plus the on-disk persistence and the
// business operations (apply state, execute action, validators) that
// callers need under a single lock.
//
// Locking convention: Mu is exposed directly. Callers acquire it for the
// duration of multi-step operations and pass through methods whose
// docstring says "caller must hold Mu". Self-locking helpers are
// avoided so cross-package code (api, scheduler) can compose atomic
// reads + writes without giving up the lock between them.
package store

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RFSender abstracts the radio transmitter so the store doesn't import
// the rf package directly. The real implementation is rf.Sender.
type RFSender interface {
	Send(code, protocol string, state bool) error
}

// LightController applies brightness/colour to a smart light (Tasmota/Matter).
// Implemented outside the store (it talks to the bridges), wired in at
// startup. Nil-safe: when unset, scenes still switch lights on/off but skip
// level/colour. Implementations must not touch the store (it is called while
// Mu is held).
type LightController interface {
	SetLight(socket Socket, level *int, color string) error
}

// Store is the single source of truth for application state at runtime.
type Store struct {
	Mu          sync.RWMutex
	Sockets     map[string]*Socket
	Schedules   map[string]*Schedule
	Groups      map[string]*Group
	Scenes      map[string]*Scene
	Timers      map[string]*Timer
	Automations map[string]*Automation
	Sensors     map[string]*Sensor
	Rooms       map[string]*Room
	Sonos       map[string]*SonosSpeaker
	// Readings is a rolling window of recent values per sensor id.
	// Trimmed to ReadingsHistorySize on each append.
	Readings  map[string][]SensorReading
	Users     map[string]*User
	Settings  *Settings
	Activity  *ActivityLog
	Discovery *Discovery
	DataDir   string
	RF        RFSender
	Light     LightController

	// OnChange, if set, is invoked whenever a socket's state changes via
	// ApplyState (manual control, scheduler, or timer). It must be cheap and
	// non-blocking — it runs while Mu is held. Used to push live updates to
	// connected clients over SSE.
	OnChange func()

	// OnStateChange, if set, is called after a socket's state changes
	// successfully (same conditions as OnChange). It receives a copy of the
	// socket and its new state. Runs while Mu is held — keep it non-blocking.
	OnStateChange func(socket Socket, newState bool)

	// OnSensorAlert, if set, is called when a sensor reading crosses a
	// threshold for the first time (rising edge only — not on every reading
	// while already alerting). direction is "above" or "below".
	// Runs while Mu is held — keep it non-blocking.
	OnSensorAlert func(sensor Sensor, value float64, direction string)

	// SuppressStateChange, when true, prevents ApplyState from firing
	// OnStateChange. Bulk operations (all-off, room, group, scene,
	// scheduler) set this so they can emit a single summary notification
	// instead of one per affected socket. OnChange (SSE) still fires so the
	// UI stays live. Caller must hold Mu (write lock).
	SuppressStateChange bool

	// pendingLights buffers smart-light brightness/colour commands produced
	// while executing a scene under Mu. They are drained by FlushLights after
	// the lock is released, so the (network) bridge calls never block the lock.
	pendingLights []lightCmd

	// sensorsDirty + sensorSaveTimer implement the readings-persistence
	// debounce (see scheduleSensorSave). Guarded by Mu.
	sensorsDirty    bool
	sensorSaveTimer *time.Timer

	// txMu serializes 433 MHz transmissions (see Transmit). Concurrent RF
	// sends would overlap on air and garble both frames; network protocols
	// (Tasmota/Matter/MQTT) don't take it. Never acquired while waiting on Mu,
	// so the Mu → txMu order in ApplyState cannot deadlock.
	txMu sync.Mutex
}

type lightCmd struct {
	socket Socket
	level  *int
	color  string
}

const (
	socketsFile     = "sockets.json"
	schedulesFile   = "schedules.json"
	groupsFile      = "groups.json"
	scenesFile      = "scenes.json"
	timersFile      = "timers.json"
	automationsFile = "automations.json"
	sensorsFile     = "sensors.json"
	readingsFile    = "readings.json"
	settingsFile    = "settings.json"
	usersFile       = "users.json"
	roomsFile       = "rooms.json"
	sonosFile       = "sonos.json"

	// ReadingsHistorySize caps how many readings are kept per sensor.
	// At one sample per minute that's ~16 hours; at one per five minutes
	// it's ~3.5 days. Plenty for the chart ranges the UI exposes.
	ReadingsHistorySize = 1000
)

// New returns an empty Store wired to dataDir and rf. Call Load to read
// previously persisted data into it.
func New(dataDir string, rf RFSender) *Store {
	return &Store{
		Sockets:     make(map[string]*Socket),
		Schedules:   make(map[string]*Schedule),
		Groups:      make(map[string]*Group),
		Scenes:      make(map[string]*Scene),
		Timers:      make(map[string]*Timer),
		Automations: make(map[string]*Automation),
		Sensors:     make(map[string]*Sensor),
		Rooms:       make(map[string]*Room),
		Sonos:       make(map[string]*SonosSpeaker),
		Readings:    make(map[string][]SensorReading),
		Users:       make(map[string]*User),
		Settings:    &Settings{},
		Activity:    NewActivityLog(200),
		Discovery:   &Discovery{Candidates: make(map[string]*DiscoveryCandidate)},
		DataDir:     dataDir,
		RF:          rf,
	}
}

// Load reads everything from disk. A missing file is not an error — it
// simply means we are starting fresh. After loading, legacy schedules
// (socket_id-only, no target_type) are normalized into the new shape.
func (s *Store) Load() error {
	if err := readJSON(filepath.Join(s.DataDir, socketsFile), &s.Sockets); err != nil {
		return fmt.Errorf("loading sockets: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, schedulesFile), &s.Schedules); err != nil {
		return fmt.Errorf("loading schedules: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, groupsFile), &s.Groups); err != nil {
		return fmt.Errorf("loading groups: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, scenesFile), &s.Scenes); err != nil {
		return fmt.Errorf("loading scenes: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, timersFile), &s.Timers); err != nil {
		return fmt.Errorf("loading timers: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, automationsFile), &s.Automations); err != nil {
		return fmt.Errorf("loading automations: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, sensorsFile), &s.Sensors); err != nil {
		return fmt.Errorf("loading sensors: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, readingsFile), &s.Readings); err != nil {
		return fmt.Errorf("loading readings: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, settingsFile), &s.Settings); err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, usersFile), &s.Users); err != nil {
		return fmt.Errorf("loading users: %w", err)
	}
	// Rooms: nil means rooms.json doesn't exist yet (first run).
	// In that case derive Room entities from the room strings already on sockets
	// and sensors so existing installations get a clean migration automatically.
	if err := readJSON(filepath.Join(s.DataDir, roomsFile), &s.Rooms); err != nil {
		return fmt.Errorf("loading rooms: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, sonosFile), &s.Sonos); err != nil {
		return fmt.Errorf("loading sonos speakers: %w", err)
	}
	if s.Settings == nil {
		s.Settings = &Settings{}
	}
	if s.Users == nil {
		s.Users = make(map[string]*User)
	}
	if s.Sockets == nil {
		s.Sockets = make(map[string]*Socket)
	}
	if s.Schedules == nil {
		s.Schedules = make(map[string]*Schedule)
	}
	if s.Groups == nil {
		s.Groups = make(map[string]*Group)
	}
	if s.Scenes == nil {
		s.Scenes = make(map[string]*Scene)
	}
	if s.Timers == nil {
		s.Timers = make(map[string]*Timer)
	}
	if s.Automations == nil {
		s.Automations = make(map[string]*Automation)
	}
	if s.Sensors == nil {
		s.Sensors = make(map[string]*Sensor)
	}
	if s.Readings == nil {
		s.Readings = make(map[string][]SensorReading)
	}
	if s.Sonos == nil {
		s.Sonos = make(map[string]*SonosSpeaker)
	}
	if s.Rooms == nil {
		// First run: rooms.json absent — derive rooms from socket/sensor strings.
		s.Rooms = make(map[string]*Room)
		seen := make(map[string]bool)
		counter := 1
		for _, sock := range s.Sockets {
			name := strings.TrimSpace(sock.Room)
			if name != "" && !seen[strings.ToLower(name)] {
				seen[strings.ToLower(name)] = true
				id := fmt.Sprintf("room_%d", counter)
				counter++
				s.Rooms[id] = &Room{ID: id, Name: name}
			}
		}
		for _, sn := range s.Sensors {
			name := strings.TrimSpace(sn.Room)
			if name != "" && !seen[strings.ToLower(name)] {
				seen[strings.ToLower(name)] = true
				id := fmt.Sprintf("room_%d", counter)
				counter++
				s.Rooms[id] = &Room{ID: id, Name: name}
			}
		}
	}

	for _, sch := range s.Schedules {
		if sch.TargetType == "" && sch.SocketID != "" {
			sch.TargetType = "socket"
			sch.TargetID = sch.SocketID
		}
	}

	// Migrate legacy flat-actions scenes to the multi-step format.
	// A scene that only has Actions (pre-multi-step) gets wrapped in a
	// single step with DelayMinutes=0, then Actions is cleared.
	for _, sc := range s.Scenes {
		if len(sc.Steps) == 0 && len(sc.Actions) > 0 {
			sc.Steps = []SceneStep{{DelayMinutes: 0, Actions: sc.Actions}}
			sc.Actions = nil
		}
	}

	return nil
}

// Save writes every file atomically. Caller must hold Mu.
func (s *Store) Save() error {
	if err := writeJSON(filepath.Join(s.DataDir, socketsFile), s.Sockets); err != nil {
		return fmt.Errorf("saving sockets: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, schedulesFile), s.Schedules); err != nil {
		return fmt.Errorf("saving schedules: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, groupsFile), s.Groups); err != nil {
		return fmt.Errorf("saving groups: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, scenesFile), s.Scenes); err != nil {
		return fmt.Errorf("saving scenes: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, timersFile), s.Timers); err != nil {
		return fmt.Errorf("saving timers: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, automationsFile), s.Automations); err != nil {
		return fmt.Errorf("saving automations: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, sensorsFile), s.Sensors); err != nil {
		return fmt.Errorf("saving sensors: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, settingsFile), s.Settings); err != nil {
		return fmt.Errorf("saving settings: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, usersFile), s.Users); err != nil {
		return fmt.Errorf("saving users: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, roomsFile), s.Rooms); err != nil {
		return fmt.Errorf("saving rooms: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, sonosFile), s.Sonos); err != nil {
		return fmt.Errorf("saving sonos speakers: %w", err)
	}
	return nil
}

// UserByID returns the user with the given ID, or nil. Caller must hold Mu.
func (s *Store) UserByID(id string) *User {
	return s.Users[id]
}

// UserByUsername returns the user with the given (case-insensitive)
// username, or nil. Caller must hold Mu.
func (s *Store) UserByUsername(username string) *User {
	for _, u := range s.Users {
		if strings.EqualFold(u.Username, username) {
			return u
		}
	}
	return nil
}

// UserByLoginCode returns the user whose login code exactly matches, or
// nil. An empty code never matches. The comparison is constant-time and
// every user is checked even after a match, so response timing doesn't
// help an attacker guess codes character by character. Caller must hold Mu.
func (s *Store) UserByLoginCode(code string) *User {
	if code == "" {
		return nil
	}
	var found *User
	for _, u := range s.Users {
		if u.LoginCode == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(u.LoginCode), []byte(code)) == 1 && found == nil {
			found = u
		}
	}
	return found
}

// UserByInviteToken returns the user whose pending invite token matches,
// or nil. An empty token never matches. Caller must hold Mu.
func (s *Store) UserByInviteToken(token string) *User {
	if token == "" {
		return nil
	}
	for _, u := range s.Users {
		if u.InviteToken == token {
			return u
		}
	}
	return nil
}

// AdminCount returns how many admin users exist. Caller must hold Mu.
func (s *Store) AdminCount() int {
	n := 0
	for _, u := range s.Users {
		if u.Admin {
			n++
		}
	}
	return n
}

// SaveSensors persists only the sensors and readings files. Called from
// the RX hot path so a steady stream of incoming readings doesn't rewrite
// every JSON file on disk. Caller must hold Mu.
func (s *Store) SaveSensors() error {
	if err := writeJSON(filepath.Join(s.DataDir, sensorsFile), s.Sensors); err != nil {
		return fmt.Errorf("saving sensors: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, readingsFile), s.Readings); err != nil {
		return fmt.Errorf("saving readings: %w", err)
	}
	return nil
}

// AppendReading adds one reading to a sensor's rolling window, updates
// the sensor's LastValue/LastReadingAt, and fires OnSensorAlert if the
// reading crosses a configured threshold for the first time. Persistence
// is debounced (see scheduleSensorSave) — a chatty rtl_433 sensor can
// deliver several packets a second, and rewriting sensors.json plus the
// full readings window per packet was the RX path's dominant cost.
// Caller must hold Mu.
func (s *Store) AppendReading(sensorID string, r SensorReading) error {
	sensor, ok := s.Sensors[sensorID]
	if !ok {
		return fmt.Errorf("sensor %q not found", sensorID)
	}
	hist := append(s.Readings[sensorID], r)
	if len(hist) > ReadingsHistorySize {
		// Drop oldest, keep the tail.
		hist = hist[len(hist)-ReadingsHistorySize:]
	}
	s.Readings[sensorID] = hist
	v := r.Value
	t := r.Time
	sensor.LastValue = &v
	sensor.LastReadingAt = &t

	// Rising-edge alert detection: fire OnSensorAlert only when the sensor
	// transitions from OK → alerting, not on every reading while breached.
	wasAlerting := sensor.Alerting
	nowAlerting := (sensor.AlertMax != nil && r.Value > *sensor.AlertMax) ||
		(sensor.AlertMin != nil && r.Value < *sensor.AlertMin)
	sensor.Alerting = nowAlerting
	if nowAlerting && !wasAlerting && s.OnSensorAlert != nil {
		direction := "above"
		if sensor.AlertMin != nil && r.Value < *sensor.AlertMin {
			direction = "below"
		}
		s.OnSensorAlert(*sensor, r.Value, direction)
	}

	s.scheduleSensorSave()
	return nil
}

// sensorSaveDelay is the debounce window for persisting readings. A crash
// loses at most this much sensor history — readings are telemetry, not
// commands, so that's an easy trade for not hammering the SD card.
const sensorSaveDelay = 5 * time.Second

// scheduleSensorSave arms a one-shot deferred SaveSensors. Multiple
// readings inside the window coalesce into a single write. Caller must
// hold Mu.
func (s *Store) scheduleSensorSave() {
	s.sensorsDirty = true
	if s.sensorSaveTimer != nil {
		return // already armed; the pending flush picks this reading up
	}
	s.sensorSaveTimer = time.AfterFunc(sensorSaveDelay, func() {
		s.Mu.Lock()
		defer s.Mu.Unlock()
		s.sensorSaveTimer = nil
		if !s.sensorsDirty {
			return
		}
		s.sensorsDirty = false
		if err := s.SaveSensors(); err != nil {
			log.Printf("store: deferred sensor save failed: %v", err)
		}
	})
}

// FlushSensorSaves persists any pending sensor/readings writes right away.
// Called on shutdown so the debounce window's readings aren't lost.
func (s *Store) FlushSensorSaves() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.sensorSaveTimer != nil {
		s.sensorSaveTimer.Stop()
		s.sensorSaveTimer = nil
	}
	if !s.sensorsDirty {
		return
	}
	s.sensorsDirty = false
	if err := s.SaveSensors(); err != nil {
		log.Printf("store: final sensor save failed: %v", err)
	}
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
