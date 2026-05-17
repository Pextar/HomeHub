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
	"sync"
)

// RFSender abstracts the radio transmitter so the store doesn't import
// the rf package directly. The real implementation is rf.Sender.
type RFSender interface {
	Send(code, protocol string, state bool) error
}

// Store is the single source of truth for application state at runtime.
type Store struct {
	Mu        sync.RWMutex
	Sockets   map[string]*Socket
	Schedules map[string]*Schedule
	Groups    map[string]*Group
	Scenes    map[string]*Scene
	Timers    map[string]*Timer
	Sensors   map[string]*Sensor
	// Readings is a rolling window of recent values per sensor id.
	// Trimmed to ReadingsHistorySize on each append.
	Readings  map[string][]SensorReading
	Settings  *Settings
	Activity  *ActivityLog
	Discovery *Discovery
	DataDir   string
	RF        RFSender
}

const (
	socketsFile   = "sockets.json"
	schedulesFile = "schedules.json"
	groupsFile    = "groups.json"
	scenesFile    = "scenes.json"
	timersFile    = "timers.json"
	sensorsFile   = "sensors.json"
	readingsFile  = "readings.json"
	settingsFile  = "settings.json"

	// ReadingsHistorySize caps how many readings are kept per sensor.
	// At one sample per minute that's ~16 hours; at one per five minutes
	// it's ~3.5 days. Plenty for the chart ranges the UI exposes.
	ReadingsHistorySize = 1000
)

// New returns an empty Store wired to dataDir and rf. Call Load to read
// previously persisted data into it.
func New(dataDir string, rf RFSender) *Store {
	return &Store{
		Sockets:   make(map[string]*Socket),
		Schedules: make(map[string]*Schedule),
		Groups:    make(map[string]*Group),
		Scenes:    make(map[string]*Scene),
		Timers:    make(map[string]*Timer),
		Sensors:   make(map[string]*Sensor),
		Readings:  make(map[string][]SensorReading),
		Settings:  &Settings{},
		Activity:  NewActivityLog(200),
		Discovery: &Discovery{Candidates: make(map[string]*DiscoveryCandidate)},
		DataDir:   dataDir,
		RF:        rf,
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
	if err := readJSON(filepath.Join(s.DataDir, sensorsFile), &s.Sensors); err != nil {
		return fmt.Errorf("loading sensors: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, readingsFile), &s.Readings); err != nil {
		return fmt.Errorf("loading readings: %w", err)
	}
	if err := readJSON(filepath.Join(s.DataDir, settingsFile), &s.Settings); err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}
	if s.Settings == nil {
		s.Settings = &Settings{}
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
	if err := writeJSON(filepath.Join(s.DataDir, sensorsFile), s.Sensors); err != nil {
		return fmt.Errorf("saving sensors: %w", err)
	}
	if err := writeJSON(filepath.Join(s.DataDir, settingsFile), s.Settings); err != nil {
		return fmt.Errorf("saving settings: %w", err)
	}
	return nil
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
// the sensor's LastValue/LastReadingAt, and persists. Caller must hold Mu.
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
