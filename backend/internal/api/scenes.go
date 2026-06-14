package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

func (s *Server) getScenes(w http.ResponseWriter, r *http.Request) {
	s.Store.Mu.RLock()
	out := make([]*store.Scene, 0, len(s.Store.Scenes))
	for _, sc := range s.Store.Scenes {
		out = append(out, sc)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	b, err := json.Marshal(out)
	s.Store.Mu.RUnlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) getScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.Store.Mu.RLock()
	sc, ok := s.Store.Scenes[id]
	var b []byte
	var err error
	if ok {
		b, err = json.Marshal(sc)
	}
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) createScene(w http.ResponseWriter, r *http.Request) {
	var sc store.Scene
	if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if err := s.Store.ValidateScene(&sc); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if sc.ID == "" {
		sc.ID = fmt.Sprintf("scene_%d", time.Now().UnixNano())
	} else if _, exists := s.Store.Scenes[sc.ID]; exists {
		// A client-supplied ID must not silently replace an existing record.
		writeError(w, http.StatusConflict, "a scene with that id already exists")
		return
	}
	s.Store.Scenes[sc.ID] = &sc
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Scenes, sc.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sc)
}

func (s *Server) updateScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates store.Scene
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	existing, ok := s.Store.Scenes[id]
	if !ok {
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}
	merged := *existing
	if name := strings.TrimSpace(updates.Name); name != "" {
		merged.Name = name
	}
	merged.Room = strings.TrimSpace(updates.Room)
	merged.Icon = strings.TrimSpace(updates.Icon)
	merged.Color = strings.TrimSpace(updates.Color)
	if updates.Steps != nil {
		merged.Steps = updates.Steps
		merged.Actions = nil // clear legacy field when steps are provided
	} else if updates.Actions != nil {
		// Legacy clients that still send flat Actions; let ValidateScene migrate.
		merged.Actions = updates.Actions
	}
	if err := s.Store.ValidateScene(&merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	*existing = merged
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) deleteScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	if _, ok := s.Store.Scenes[id]; !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}
	delete(s.Store.Scenes, id)
	for sid, sch := range s.Store.Schedules {
		if sch.TargetType == "scene" && sch.TargetID == id {
			delete(s.Store.Schedules, sid)
		}
	}
	for tid, t := range s.Store.Timers {
		if t.TargetType == "scene" && t.TargetID == id {
			delete(s.Store.Timers, tid)
		}
	}
	s.Store.PruneAutomationsForTarget("scene", id)
	s.Store.DeleteAutomationsOwnedByScene(id)
	if err := s.Store.Save(); err != nil {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) activateScene(w http.ResponseWriter, r *http.Request) {
	name, okCount, failures, found, err := s.doActivateScene(mux.Vars(r)["id"])
	if !found {
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scene":    name,
		"updated":  okCount,
		"failures": failures,
	})
}

// doActivateScene runs the scene's first step through the staged flow (delayed
// steps are scheduled as background goroutines by StageAction), records
// activation telemetry, and sends a single summary notification. Shared by the
// activate REST handler and the assistant's activate_scene tool. found is false
// when no scene has the given id. Caller must NOT hold Mu.
func (s *Server) doActivateScene(id string) (name string, okCount int, failures []map[string]string, found bool, err error) {
	// Stage the first step, transmit off-lock, then fold the results back in.
	// Smart-light brightness/colour is queued during staging and drained by
	// FlushLights at the end.
	s.Store.Mu.Lock()
	scene, ok := s.Store.Scenes[id]
	var staged []store.StagedSend
	if ok {
		name = scene.Name
		staged, _ = s.Store.StageAction("scene", id, "activate")
	}
	s.Store.Mu.Unlock()
	if !ok {
		return "", 0, nil, false, nil
	}

	s.Store.SendStaged(staged)

	s.Store.Mu.Lock()
	// Per-socket notifications suppressed so we send a single summary.
	s.Store.SuppressStateChange = true
	_ = s.Store.ApplyStaged(staged)
	s.Store.SuppressStateChange = false
	okCount, failures = stagedFailures(staged)
	// Record activation telemetry so the UI can show "ran N× · 2h ago".
	// Re-fetch: the scene may have been deleted while the sends were in flight.
	if sc, still := s.Store.Scenes[id]; still {
		sc.LastActivatedAt = time.Now().UTC()
		sc.ActivateCount++
	}
	entry := store.ActivityEntry{Kind: "scene", Source: "manual", Action: "activate", Label: name}
	if len(failures) > 0 {
		entry.Status = "error"
		entry.Error = fmt.Sprintf("%d of %d failed", len(failures), okCount+len(failures))
	}
	s.Store.Activity.Add(entry)
	err = s.Store.Save()
	s.Store.Mu.Unlock()
	s.Store.FlushLights()
	if err != nil {
		return name, okCount, failures, true, err
	}
	s.notifyBulkState(fmt.Sprintf("Scene activated: %s", name), okCount)
	return name, okCount, failures, true, nil
}
