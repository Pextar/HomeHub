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
	Activity  *ActivityLog
	DataDir   string
	RF        RFSender
}

const (
	socketsFile   = "sockets.json"
	schedulesFile = "schedules.json"
	groupsFile    = "groups.json"
	scenesFile    = "scenes.json"
	timersFile    = "timers.json"
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
		Activity:  NewActivityLog(200),
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
