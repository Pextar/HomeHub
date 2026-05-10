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
	s.Store.Mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.Store.Mu.RLock()
	sc, ok := s.Store.Scenes[id]
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}
	writeJSON(w, http.StatusOK, sc)
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
	if updates.Actions != nil {
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
	if err := s.Store.Save(); err != nil {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) activateScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	scene, ok := s.Store.Scenes[id]
	if !ok {
		writeError(w, http.StatusNotFound, "scene not found")
		return
	}

	var okCount int
	failures := make([]map[string]string, 0)
	for _, a := range scene.Actions {
		if err := s.Store.ExecuteAction("socket", a.SocketID, a.Action); err != nil {
			failures = append(failures, map[string]string{
				"socket_id": a.SocketID,
				"error":     err.Error(),
			})
			continue
		}
		okCount++
	}
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scene":    scene.Name,
		"updated":  okCount,
		"failures": failures,
	})
}
