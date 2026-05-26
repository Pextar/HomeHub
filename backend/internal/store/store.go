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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
// nil. An empty code never matches. Caller must hold Mu.
func (s *Store) UserByLoginCode(code string) *User {
	if code == "" {
		return nil
	}
	for _, u := range s.Users {
		if u.LoginCode == code {
			return u
		}
	}
	return nil
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
// the sensor's LastValue/LastReadingAt, persists, and fires OnSensorAlert
// if the reading crosses a configured threshold for the first time.
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

	return s.SaveSensors()
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
