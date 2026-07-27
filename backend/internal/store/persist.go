package store

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// The persisted collections, described once.
//
// Load and Save used to spell every collection out: fourteen reads, thirteen
// writes and thirteen nil-guards, in three separate lists that nothing kept
// in step. Adding a collection meant remembering all three, and the lists had
// already drifted — the guard for Rooms is absent from the third, which is
// why the first-run room migration never runs (see the note on Load).
//
// Now each collection is described once and Load, Save and SaveSensors walk
// the same table, in the order written here. That order is part of the
// contract: it decides which error an operator sees first when several files
// are unwritable.

// collection binds one in-memory collection to its file on disk.
type collection struct {
	// label names the collection in errors: "loading sockets: …".
	label string
	// field is the Store field this collection persists. Only used by the
	// coverage test, which reflects over Store and fails if a persisted
	// field is missing from the table below — the drift this table exists
	// to prevent.
	field string
	file  string
	// load reads the file into the store. A missing file is not an error.
	load func(*Store, string) error
	// save writes the collection out.
	save func(*Store, string) error
	// inFullSave is false for collections Save() deliberately skips.
	// Readings are the only one: they are written by the debounced
	// SaveSensors instead, so a stream of incoming samples doesn't rewrite
	// the whole store on each reading.
	inFullSave bool
	// ensure runs after load and substitutes an empty map for a nil one. A
	// file containing the literal `null` decodes over the map New() created,
	// and callers index these maps under the lock without checking.
	//
	// Nil only for Rooms, whose nil-ness is meaningful: Load uses it to
	// decide whether to run the first-run migration.
	ensure func(*Store)
}

// mapCollection describes a map[string]T held in one file.
func mapCollection[T any](label, storeField, file string, field func(*Store) *map[string]T) collection {
	return collection{
		label:      label,
		field:      storeField,
		file:       file,
		inFullSave: true,
		load:       func(s *Store, path string) error { return readJSON(path, field(s)) },
		save:       func(s *Store, path string) error { return writeJSON(path, *field(s)) },
		ensure: func(s *Store) {
			if p := field(s); *p == nil {
				*p = make(map[string]T)
			}
		},
	}
}

// with applies tweaks to a collection, so the table below reads as the
// default plus its exceptions.
func with(c collection, tweak func(*collection)) collection {
	tweak(&c)
	return c
}

// collections is the whole persisted surface, in load/save order.
var collections = []collection{
	mapCollection("sockets", "Sockets", socketsFile, func(s *Store) *map[string]*Socket { return &s.Sockets }),
	mapCollection("schedules", "Schedules", schedulesFile, func(s *Store) *map[string]*Schedule { return &s.Schedules }),
	mapCollection("groups", "Groups", groupsFile, func(s *Store) *map[string]*Group { return &s.Groups }),
	mapCollection("scenes", "Scenes", scenesFile, func(s *Store) *map[string]*Scene { return &s.Scenes }),
	mapCollection("timers", "Timers", timersFile, func(s *Store) *map[string]*Timer { return &s.Timers }),
	mapCollection("automations", "Automations", automationsFile, func(s *Store) *map[string]*Automation { return &s.Automations }),
	mapCollection("sensors", "Sensors", sensorsFile, func(s *Store) *map[string]*Sensor { return &s.Sensors }),
	with(mapCollection("readings", "Readings", readingsFile,
		func(s *Store) *map[string][]SensorReading { return &s.Readings }),
		func(c *collection) { c.inFullSave = false }),
	{
		label:      "settings",
		field:      "Settings",
		file:       settingsFile,
		inFullSave: true,
		load:       func(s *Store, path string) error { return readJSON(path, &s.Settings) },
		save:       func(s *Store, path string) error { return writeJSON(path, s.Settings) },
		ensure: func(s *Store) {
			if s.Settings == nil {
				s.Settings = &Settings{}
			}
		},
	},
	mapCollection("users", "Users", usersFile, func(s *Store) *map[string]*User { return &s.Users }),
	mapCollection("rooms", "Rooms", roomsFile, func(s *Store) *map[string]*Room { return &s.Rooms }),
	mapCollection("sonos speakers", "Sonos", sonosFile, func(s *Store) *map[string]*SonosSpeaker { return &s.Sonos }),
	mapCollection("kef speakers", "KEF", kefFile, func(s *Store) *map[string]*KEFSpeaker { return &s.KEF }),
	mapCollection("zones", "Zones", zonesFile, func(s *Store) *map[string]*Zone { return &s.Zones }),
}

// loadAll reads every collection in table order.
func (s *Store) loadAll() error {
	for _, c := range collections {
		if err := c.load(s, filepath.Join(s.DataDir, c.file)); err != nil {
			return fmt.Errorf("loading %s: %w", c.label, err)
		}
		if c.ensure != nil {
			c.ensure(s)
		}
	}
	return nil
}

// saveAll writes every collection Save owns, in table order.
func (s *Store) saveAll() error {
	return s.saveMatching(func(c collection) bool { return c.inFullSave })
}

// saveMatching writes the collections pick selects, in table order.
func (s *Store) saveMatching(pick func(collection) bool) error {
	for _, c := range collections {
		if !pick(c) {
			continue
		}
		if err := c.save(s, filepath.Join(s.DataDir, c.file)); err != nil {
			return fmt.Errorf("saving %s: %w", c.label, err)
		}
	}
	return nil
}

// ensureRoomsForNamedDevices creates a Room for every room name carried by a
// socket or sensor that has no matching entity.
//
// Rooms became entities after sockets did, and Load has always carried a
// migration to derive them from the room strings already in place. It never
// ran: it was guarded by `s.Rooms == nil`, but New() assigns an empty map and
// readJSON leaves it alone when the file is absent, so on a real first run
// the guard was false. Installations that predate rooms were left with
// sockets naming a room that did not exist, and once anything called Save the
// empty rooms.json made the intended one-shot window unreachable for good.
//
// Reconciling on every load rather than only on the first fixes those
// installations, and cannot resurrect a deliberately deleted room: deleting a
// room clears its name from the sockets, sensors and scenes that carried it,
// so no orphan name is left to rebuild from.
//
// Caller must hold Mu (Load runs before the store is shared).
func (s *Store) ensureRoomsForNamedDevices() {
	known := make(map[string]bool, len(s.Rooms))
	for _, rm := range s.Rooms {
		known[strings.ToLower(strings.TrimSpace(rm.Name))] = true
	}

	// First spelling encountered wins, matched case-insensitively, so
	// "Lounge" and "lounge" produce one room rather than two.
	missing := make(map[string]string)
	note := func(name string) {
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		if name == "" || known[key] {
			return
		}
		if _, dup := missing[key]; !dup {
			missing[key] = name
		}
	}
	for _, sock := range s.Sockets {
		note(sock.Room)
	}
	for _, sn := range s.Sensors {
		note(sn.Room)
	}
	if len(missing) == 0 {
		return
	}

	// Sorted so the ids assigned don't depend on map iteration order.
	keys := make([]string, 0, len(missing))
	for k := range missing {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	next := 1
	for _, k := range keys {
		var id string
		for {
			id = fmt.Sprintf("room_%d", next)
			next++
			if _, taken := s.Rooms[id]; !taken {
				break
			}
		}
		s.Rooms[id] = &Room{ID: id, Name: missing[k]}
	}
}
