package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"homehub/internal/store"
)

// configBundle is a portable snapshot of everything except users/auth.
// Profiles and password hashes are deliberately excluded so a backup file
// can be shared or version-controlled without leaking credentials.
type configBundle struct {
	Version     int                          `json:"version"`
	ExportedAt  time.Time                    `json:"exported_at"`
	Sockets     map[string]*store.Socket     `json:"sockets"`
	Schedules   map[string]*store.Schedule   `json:"schedules"`
	Groups      map[string]*store.Group      `json:"groups"`
	Scenes      map[string]*store.Scene      `json:"scenes"`
	Automations map[string]*store.Automation `json:"automations"`
	Sensors     map[string]*store.Sensor     `json:"sensors"`
	Settings    *store.Settings              `json:"settings"`
}

// exportConfig returns the current configuration as a downloadable JSON
// bundle. Sensor readings, timers and users are omitted — readings are
// transient, timers are one-shot, and users carry credentials.
func (s *Server) exportConfig(w http.ResponseWriter, _ *http.Request) {
	s.Store.Mu.RLock()
	settings := *s.Store.Settings
	bundle := configBundle{
		Version:     1,
		ExportedAt:  time.Now().UTC(),
		Sockets:     s.Store.Sockets,
		Schedules:   s.Store.Schedules,
		Groups:      s.Store.Groups,
		Scenes:      s.Store.Scenes,
		Automations: s.Store.Automations,
		Sensors:     s.Store.Sensors,
		Settings:    &settings,
	}
	body, err := json.MarshalIndent(bundle, "", "  ")
	s.Store.Mu.RUnlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode export: "+err.Error())
		return
	}

	filename := fmt.Sprintf("homehub-backup-%s.json", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// importConfig replaces sockets, schedules, groups, scenes, sensors and
// settings with the contents of an uploaded bundle. Users are never
// touched. Only collections present in the bundle are replaced, so a
// partial bundle leaves the rest intact.
//
// Every item is run through the same ValidateX functions as the regular
// CRUD endpoints (against the post-import view of the data, so
// cross-references resolve correctly) before anything is replaced — a
// malformed bundle is rejected whole with a 400 and the live data is
// untouched.
func (s *Server) importConfig(w http.ResponseWriter, r *http.Request) {
	var bundle configBundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if err := validateBundle(&bundle, s.Store); err != nil {
		writeError(w, http.StatusBadRequest, "invalid bundle: "+err.Error())
		return
	}

	if bundle.Sockets != nil {
		s.Store.Sockets = bundle.Sockets
	}
	if bundle.Schedules != nil {
		s.Store.Schedules = bundle.Schedules
	}
	if bundle.Groups != nil {
		s.Store.Groups = bundle.Groups
	}
	if bundle.Scenes != nil {
		s.Store.Scenes = bundle.Scenes
	}
	if bundle.Automations != nil {
		s.Store.Automations = bundle.Automations
	}
	if bundle.Sensors != nil {
		s.Store.Sensors = bundle.Sensors
	}
	if bundle.Settings != nil {
		s.Store.Settings = bundle.Settings
	}

	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sockets":     len(s.Store.Sockets),
		"schedules":   len(s.Store.Schedules),
		"groups":      len(s.Store.Groups),
		"scenes":      len(s.Store.Scenes),
		"automations": len(s.Store.Automations),
		"sensors":     len(s.Store.Sensors),
	})
}

// validateBundle normalizes and validates every item in the bundle the same
// way the CRUD endpoints would (which also applies legacy migrations, e.g.
// flat scene actions → steps). Validation runs against a scratch store that
// reflects the post-import state — bundle collections where present, live
// collections otherwise — so e.g. a schedule referencing a bundled socket
// resolves. Caller must hold live.Mu. The bundle's items are normalized in
// place; the live store is never modified.
func validateBundle(bundle *configBundle, live *store.Store) error {
	pickSockets := bundle.Sockets
	if pickSockets == nil {
		pickSockets = live.Sockets
	}
	pickGroups := bundle.Groups
	if pickGroups == nil {
		pickGroups = live.Groups
	}
	pickScenes := bundle.Scenes
	if pickScenes == nil {
		pickScenes = live.Scenes
	}
	pickSensors := bundle.Sensors
	if pickSensors == nil {
		pickSensors = live.Sensors
	}
	scratch := &store.Store{
		Sockets:     pickSockets,
		Groups:      pickGroups,
		Scenes:      pickScenes,
		Sensors:     pickSensors,
		Schedules:   map[string]*store.Schedule{},
		Automations: map[string]*store.Automation{},
		Rooms:       live.Rooms, // rooms aren't part of the bundle
	}

	// Maps decoded from JSON can contain explicit nulls and keys that
	// disagree with the item's own ID — both would corrupt the store
	// (a nil entry panics on the next iteration; a mismatched key makes
	// the item unaddressable). normalizeID fills empty IDs from the key.
	normalizeID := func(kind, key string, id *string) error {
		if id == nil {
			return fmt.Errorf("%s %q is null", kind, key)
		}
		if *id == "" {
			*id = key
		} else if *id != key {
			return fmt.Errorf("%s key %q does not match its id %q", kind, key, *id)
		}
		return nil
	}
	for k, v := range bundle.Sockets {
		var id *string
		if v != nil {
			id = &v.ID
		}
		if err := normalizeID("socket", k, id); err != nil {
			return err
		}
		if err := scratch.ValidateSocket(v); err != nil {
			return fmt.Errorf("socket %q: %w", k, err)
		}
	}
	for k, v := range bundle.Groups {
		var id *string
		if v != nil {
			id = &v.ID
		}
		if err := normalizeID("group", k, id); err != nil {
			return err
		}
		if err := scratch.ValidateGroup(v); err != nil {
			return fmt.Errorf("group %q: %w", k, err)
		}
	}
	for k, v := range bundle.Scenes {
		var id *string
		if v != nil {
			id = &v.ID
		}
		if err := normalizeID("scene", k, id); err != nil {
			return err
		}
		if err := scratch.ValidateScene(v); err != nil {
			return fmt.Errorf("scene %q: %w", k, err)
		}
	}
	for k, v := range bundle.Sensors {
		var id *string
		if v != nil {
			id = &v.ID
		}
		if err := normalizeID("sensor", k, id); err != nil {
			return err
		}
		if err := scratch.ValidateSensor(v); err != nil {
			return fmt.Errorf("sensor %q: %w", k, err)
		}
	}
	// Schedules and automations reference the collections above, so they
	// validate last.
	for k, v := range bundle.Schedules {
		var id *string
		if v != nil {
			id = &v.ID
		}
		if err := normalizeID("schedule", k, id); err != nil {
			return err
		}
		if err := scratch.ValidateSchedule(v); err != nil {
			return fmt.Errorf("schedule %q: %w", k, err)
		}
	}
	for k, v := range bundle.Automations {
		var id *string
		if v != nil {
			id = &v.ID
		}
		if err := normalizeID("automation", k, id); err != nil {
			return err
		}
		if err := scratch.ValidateAutomation(v); err != nil {
			return fmt.Errorf("automation %q: %w", k, err)
		}
	}
	if bundle.Settings != nil {
		if err := scratch.ValidateSettings(bundle.Settings); err != nil {
			return fmt.Errorf("settings: %w", err)
		}
	}
	return nil
}
