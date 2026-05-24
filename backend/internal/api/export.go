package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"rf-socket-controller/internal/store"
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
func (s *Server) importConfig(w http.ResponseWriter, r *http.Request) {
	var bundle configBundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

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
