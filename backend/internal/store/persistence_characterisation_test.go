package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Characterisation tests for Load and Save.
//
// Both are currently written out one collection at a time — fourteen reads,
// thirteen writes, and thirteen separate nil-guards afterwards — with
// nothing tying the three lists together. These pin what the pair actually
// does before that is made table-driven, including the asymmetries that a
// naive table would quietly erase: readings are not written by Save, and
// Rooms uses a nil map (not an empty one) to detect a first run.

func loadedStore(t *testing.T, dir string) *Store {
	t.Helper()
	s := New(dir, noopRF{})
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	return s
}

// populate fills every persisted collection so a round-trip can show that
// none of them is dropped.
func populate(s *Store) {
	s.Sockets["sk"] = &Socket{ID: "sk", Name: "Lamp", Code: "1:1", Protocol: "nexa", Room: "Lounge"}
	s.Schedules["sc"] = &Schedule{ID: "sc", TargetType: "socket", TargetID: "sk", Action: "on", Time: "07:00"}
	s.Groups["gr"] = &Group{ID: "gr", Name: "Downstairs", SocketIDs: []string{"sk"}}
	s.Scenes["ce"] = &Scene{ID: "ce", Name: "Movie"}
	s.Timers["tm"] = &Timer{ID: "tm", TargetType: "socket", TargetID: "sk", Action: "off"}
	s.Automations["au"] = &Automation{ID: "au", Name: "Dusk"}
	s.Sensors["sn"] = &Sensor{ID: "sn", Name: "Temp", Kind: "temperature"}
	s.Rooms["rm"] = &Room{ID: "rm", Name: "Lounge"}
	s.Sonos["so"] = &SonosSpeaker{ID: "so", Name: "Living", IP: "192.168.1.10"}
	s.KEF["kf"] = &KEFSpeaker{ID: "kf", Name: "Study", IP: "192.168.1.20"}
	s.Zones["zn"] = &Zone{ID: "zn", Name: "Whole home"}
	s.Users["us"] = &User{ID: "us", Username: "admin", Admin: true}
	s.Settings.Latitude = 59.33
	s.Settings.Longitude = 18.06
}

func TestSaveLoadRoundTripsEveryCollection(t *testing.T) {
	dir := t.TempDir()
	s := loadedStore(t, dir)
	populate(s)
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	got := loadedStore(t, dir)
	for name, n := range map[string]int{
		"sockets": len(got.Sockets), "schedules": len(got.Schedules),
		"groups": len(got.Groups), "scenes": len(got.Scenes),
		"timers": len(got.Timers), "automations": len(got.Automations),
		"sensors": len(got.Sensors), "rooms": len(got.Rooms),
		"sonos": len(got.Sonos), "kef": len(got.KEF),
		"zones": len(got.Zones), "users": len(got.Users),
	} {
		if n != 1 {
			t.Errorf("%s round-tripped %d entries, want 1", name, n)
		}
	}
	if got.Settings.Latitude != 59.33 {
		t.Errorf("settings latitude = %v", got.Settings.Latitude)
	}
	if got.Sockets["sk"].Name != "Lamp" {
		t.Errorf("socket name = %q", got.Sockets["sk"].Name)
	}
}

// Save writes twelve maps plus settings — but deliberately not readings.
// Those are persisted only by the debounced sensor save, so that a stream of
// incoming readings doesn't rewrite the whole store on every sample.
func TestSaveDoesNotWriteReadings(t *testing.T) {
	dir := t.TempDir()
	s := loadedStore(t, dir)
	s.Sensors["sn"] = &Sensor{ID: "sn", Name: "Temp", Kind: "temperature"}
	s.Readings["sn"] = []SensorReading{{Time: time.Now().UTC(), Value: 21}}

	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "readings.json")); !os.IsNotExist(err) {
		t.Error("Save wrote readings.json; only SaveSensors should")
	}

	if err := s.SaveSensors(); err != nil {
		t.Fatalf("save sensors: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "readings.json")); err != nil {
		t.Errorf("SaveSensors did not write readings.json: %v", err)
	}
	if got := loadedStore(t, dir); len(got.Readings["sn"]) != 1 {
		t.Error("readings did not round-trip through SaveSensors")
	}
}

func TestSaveWritesExactlyTheExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	s := loadedStore(t, dir)
	populate(s)
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name()] = true
	}
	want := []string{
		"sockets.json", "schedules.json", "groups.json", "scenes.json",
		"timers.json", "automations.json", "sensors.json", "settings.json",
		"users.json", "rooms.json", "sonos.json", "kef.json", "zones.json",
	}
	for _, f := range want {
		if !got[f] {
			t.Errorf("Save did not write %s", f)
		}
		delete(got, f)
	}
	for extra := range got {
		t.Errorf("Save wrote an unexpected file: %s", extra)
	}
}

// A missing data directory is a first run, not an error.
func TestLoadTreatsMissingFilesAsEmpty(t *testing.T) {
	s := loadedStore(t, t.TempDir())
	if len(s.Sockets) != 0 || len(s.Users) != 0 {
		t.Error("a fresh store came back non-empty")
	}
	if s.Settings == nil {
		t.Error("Settings is nil after Load")
	}
}

// Every map must be non-nil after Load: callers index them under the lock
// without checking. A file holding a literal `null` decodes *over* the map
// New() created, so the guard has to run after each read, not just when the
// file is absent.
func TestLoadNeverLeavesANilMap(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"sockets.json", "schedules.json", "groups.json", "scenes.json",
		"timers.json", "automations.json", "sensors.json", "readings.json",
		"users.json", "sonos.json", "kef.json", "zones.json",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("null"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// rooms.json is deliberately absent here — see the migration test.
	s := loadedStore(t, dir)

	for name, isNil := range map[string]bool{
		"Sockets": s.Sockets == nil, "Schedules": s.Schedules == nil,
		"Groups": s.Groups == nil, "Scenes": s.Scenes == nil,
		"Timers": s.Timers == nil, "Automations": s.Automations == nil,
		"Sensors": s.Sensors == nil, "Readings": s.Readings == nil,
		"Users": s.Users == nil, "Sonos": s.Sonos == nil,
		"KEF": s.KEF == nil, "Zones": s.Zones == nil, "Rooms": s.Rooms == nil,
	} {
		if isNil {
			t.Errorf("%s is nil after Load; callers index it without a check", name)
		}
	}
	if s.Settings == nil {
		t.Error("Settings is nil after Load")
	}
}

// Rooms became entities after sockets did, so Load reconciles: any room name
// carried by a socket or sensor that has no matching Room gets one.
//
// This used to be a one-shot first-run migration guarded by a nil map, which
// never fired — see ensureRoomsForNamedDevices. These cover the behaviour
// that replaced it.
func TestRoomsAreDerivedForOrphanedNames(t *testing.T) {
	// dir seeds sockets and sensors carrying room names, plus whatever
	// rooms.json content the case wants.
	seed := func(t *testing.T, roomsJSON string) *Store {
		t.Helper()
		dir := t.TempDir()
		s := New(dir, noopRF{})
		s.Sockets["a"] = &Socket{ID: "a", Name: "A", Room: "Lounge"}
		s.Sockets["b"] = &Socket{ID: "b", Name: "B", Room: "lounge"}
		s.Sensors["c"] = &Sensor{ID: "c", Name: "C", Room: "Kitchen"}
		if err := writeJSON(filepath.Join(dir, "sockets.json"), s.Sockets); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(filepath.Join(dir, "sensors.json"), s.Sensors); err != nil {
			t.Fatal(err)
		}
		if roomsJSON != "" {
			if err := os.WriteFile(filepath.Join(dir, "rooms.json"), []byte(roomsJSON), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return loadedStore(t, dir)
	}

	names := func(s *Store) map[string]bool {
		out := map[string]bool{}
		for _, rm := range s.Rooms {
			out[rm.Name] = true
		}
		return out
	}

	// A genuine first run: no rooms.json at all.
	t.Run("absent rooms.json derives the referenced rooms", func(t *testing.T) {
		got := seed(t, "")
		if len(got.Rooms) != 2 {
			t.Fatalf("derived %d rooms, want 2; got %v", len(got.Rooms), names(got))
		}
		if n := names(got); !n["Kitchen"] || (!n["Lounge"] && !n["lounge"]) {
			t.Errorf("rooms = %v", n)
		}
	})

	// The case that made the original bug permanent: an install that upgraded,
	// missed the migration, then saved an empty rooms.json.
	t.Run("an empty rooms.json still derives them", func(t *testing.T) {
		if got := seed(t, "{}"); len(got.Rooms) != 2 {
			t.Errorf("derived %d rooms, want 2; got %v", len(got.Rooms), names(got))
		}
	})

	// Two spellings of one room collapse to a single entity.
	t.Run("matching is case-insensitive", func(t *testing.T) {
		got := seed(t, "")
		lounge := 0
		for _, rm := range got.Rooms {
			if strings.EqualFold(rm.Name, "lounge") {
				lounge++
			}
		}
		if lounge != 1 {
			t.Errorf("got %d Lounge rooms, want 1", lounge)
		}
	})

	// An existing room is matched, not duplicated — including across case.
	t.Run("existing rooms are left alone", func(t *testing.T) {
		got := seed(t, `{"rm_1":{"id":"rm_1","name":"LOUNGE"},"rm_2":{"id":"rm_2","name":"Kitchen"}}`)
		if len(got.Rooms) != 2 {
			t.Errorf("rooms = %v, want the two existing ones untouched", names(got))
		}
		if got.Rooms["rm_1"].Name != "LOUNGE" {
			t.Errorf("existing room was rewritten: %q", got.Rooms["rm_1"].Name)
		}
	})

	// Deleting a room clears its name from every device, so there is no
	// orphan left for the reconciliation to rebuild from.
	t.Run("a deleted room is not resurrected", func(t *testing.T) {
		dir := t.TempDir()
		s := New(dir, noopRF{})
		s.Sockets["a"] = &Socket{ID: "a", Name: "A", Room: ""}
		if err := writeJSON(filepath.Join(dir, "sockets.json"), s.Sockets); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rooms.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := loadedStore(t, dir); len(got.Rooms) != 0 {
			t.Errorf("derived %d rooms from devices with no room name", len(got.Rooms))
		}
	})

	// Ids must not depend on map iteration order, or a restart could
	// renumber rooms.
	t.Run("derived ids are stable across loads", func(t *testing.T) {
		first := names(seed(t, ""))
		for i := 0; i < 5; i++ {
			if got := names(seed(t, "")); len(got) != len(first) {
				t.Fatalf("run %d derived %v, want %v", i, got, first)
			}
		}
	})

	// A derived id must not collide with one already in use.
	t.Run("derived ids avoid existing ones", func(t *testing.T) {
		got := seed(t, `{"room_1":{"id":"room_1","name":"Hallway"}}`)
		if len(got.Rooms) != 3 {
			t.Fatalf("rooms = %v, want Hallway plus the two derived", names(got))
		}
		if got.Rooms["room_1"].Name != "Hallway" {
			t.Errorf("room_1 was overwritten: %q", got.Rooms["room_1"].Name)
		}
	})
}

// Schedules used to carry a bare socket_id; Load normalises them to the
// target_type/target_id pair everything else uses.
func TestLoadMigratesLegacySchedules(t *testing.T) {
	dir := t.TempDir()
	legacy := map[string]*Schedule{
		"s1": {ID: "s1", SocketID: "sk", Action: "on", Time: "07:00"},
		"s2": {ID: "s2", TargetType: "group", TargetID: "gr", Action: "on", Time: "08:00"},
	}
	if err := writeJSON(filepath.Join(dir, "schedules.json"), legacy); err != nil {
		t.Fatal(err)
	}

	got := loadedStore(t, dir)
	if s1 := got.Schedules["s1"]; s1.TargetType != "socket" || s1.TargetID != "sk" {
		t.Errorf("legacy schedule = %+v, want target_type=socket target_id=sk", s1)
	}
	// An already-migrated schedule is left alone.
	if s2 := got.Schedules["s2"]; s2.TargetType != "group" || s2.TargetID != "gr" {
		t.Errorf("modern schedule was rewritten: %+v", s2)
	}
}

// Scenes predate multi-step, so a flat Actions list is wrapped in a single
// immediate step and the legacy field cleared.
func TestLoadMigratesLegacyScenes(t *testing.T) {
	dir := t.TempDir()
	legacy := map[string]*Scene{
		"c1": {ID: "c1", Name: "Flat", Actions: []SceneAction{{SocketID: "sk", Action: "on"}}},
		"c2": {ID: "c2", Name: "Stepped", Steps: []SceneStep{{DelayMinutes: 5}}},
	}
	if err := writeJSON(filepath.Join(dir, "scenes.json"), legacy); err != nil {
		t.Fatal(err)
	}

	got := loadedStore(t, dir)
	c1 := got.Scenes["c1"]
	if len(c1.Steps) != 1 || c1.Steps[0].DelayMinutes != 0 || len(c1.Steps[0].Actions) != 1 {
		t.Errorf("legacy scene steps = %+v", c1.Steps)
	}
	if c1.Actions != nil {
		t.Errorf("legacy Actions not cleared: %+v", c1.Actions)
	}
	if c2 := got.Scenes["c2"]; len(c2.Steps) != 1 || c2.Steps[0].DelayMinutes != 5 {
		t.Errorf("stepped scene was rewritten: %+v", c2.Steps)
	}
}

// The error names the collection that failed, which is the only clue an
// operator gets from the log.
func TestLoadErrorsNameTheCollection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "groups.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(dir, noopRF{})
	err := s.Load()
	if err == nil {
		t.Fatal("expected a load error")
	}
	if got := err.Error(); got[:16] != "loading groups: " {
		t.Errorf("error = %q, want it to start with \"loading groups: \"", got)
	}
}

func TestSaveErrorsNameTheCollection(t *testing.T) {
	s := loadedStore(t, t.TempDir())
	s.DataDir = filepath.Join(s.DataDir, "missing")
	err := s.Save()
	if err == nil {
		t.Fatal("expected a save error")
	}
	if got := err.Error(); got[:16] != "saving sockets: " {
		t.Errorf("error = %q, want it to start with \"saving sockets: \"", got)
	}
}
