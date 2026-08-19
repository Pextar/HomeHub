package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/control"
	"homehub/internal/store"
)

func (s *Server) getScenes(w http.ResponseWriter, r *http.Request) {
	var b []byte
	var err error
	s.Store.View(func() {
		out := make([]*store.Scene, 0, len(s.Store.Scenes))
		for _, sc := range s.Store.Scenes {
			out = append(out, sc)
		}
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
		b, err = json.Marshal(out)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) getScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var b []byte
	var err error
	var ok bool
	s.Store.View(func() {
		var sc *store.Scene
		if sc, ok = s.Store.Scenes[id]; ok {
			b, err = json.Marshal(sc)
		}
	})
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
	if !decodeBody(w, r, &sc) {
		return
	}

	if !s.updateOr(w, func() { delete(s.Store.Scenes, sc.ID) }, func() error {
		if err := s.Store.ValidateScene(&sc); err != nil {
			return errInvalid(err)
		}
		if sc.ID == "" {
			sc.ID = fmt.Sprintf("scene_%d", time.Now().UnixNano())
		} else if _, exists := s.Store.Scenes[sc.ID]; exists {
			// A client-supplied ID must not silently replace an existing record.
			return errStatus(http.StatusConflict, "a scene with that id already exists")
		}
		s.Store.Scenes[sc.ID] = &sc
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusCreated, sc)
}

func (s *Server) updateScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates store.Scene
	if !decodeBody(w, r, &updates) {
		return
	}

	var existing *store.Scene
	if !s.update(w, func() error {
		var ok bool
		existing, ok = s.Store.Scenes[id]
		if !ok {
			return errStatus(http.StatusNotFound, "scene not found")
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
			return errInvalid(err)
		}
		*existing = merged
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) deleteScene(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if !s.update(w, func() error {
		if _, ok := s.Store.Scenes[id]; !ok {
			return errStatus(http.StatusNotFound, "scene not found")
		}
		delete(s.Store.Scenes, id)
		s.Store.CascadeDeleteTarget("scene", id)
		// Beyond the shared cascade: a scene created by the scene wizard
		// owns its automations outright, so they go with it.
		s.Store.DeleteAutomationsOwnedByScene(id)
		return nil
	}) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) activateScene(w http.ResponseWriter, r *http.Request) {
	res, err := s.Control.Scene(mux.Vars(r)["id"], control.SourceManual)
	if !writeStaged(w, "scene", res, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scene":    res.Label,
		"updated":  res.OK,
		"failures": res.Failures,
	})
}
