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

func (s *Server) getGroups(w http.ResponseWriter, r *http.Request) {
	s.Store.Mu.RLock()
	out := make([]*store.Group, 0, len(s.Store.Groups))
	for _, g := range s.Store.Groups {
		out = append(out, g)
	}
	s.Store.Mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.Store.Mu.RLock()
	g, ok := s.Store.Groups[id]
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var g store.Group
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if err := s.Store.ValidateGroup(&g); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.ID == "" {
		g.ID = fmt.Sprintf("group_%d", time.Now().UnixNano())
	}
	s.Store.Groups[g.ID] = &g
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Groups, g.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates store.Group
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	existing, ok := s.Store.Groups[id]
	if !ok {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	merged := *existing
	if name := strings.TrimSpace(updates.Name); name != "" {
		merged.Name = name
	}
	if updates.SocketIDs != nil {
		merged.SocketIDs = updates.SocketIDs
	}
	if err := s.Store.ValidateGroup(&merged); err != nil {
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

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	if _, ok := s.Store.Groups[id]; !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	delete(s.Store.Groups, id)
	for sid, sch := range s.Store.Schedules {
		if sch.TargetType == "group" && sch.TargetID == id {
			delete(s.Store.Schedules, sid)
		}
	}
	for tid, t := range s.Store.Timers {
		if t.TargetType == "group" && t.TargetID == id {
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

// groupAction returns a handler that applies an action to every member
// of a group.
func (s *Server) groupAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]

		s.Store.Mu.Lock()
		defer s.Store.Mu.Unlock()

		g, ok := s.Store.Groups[id]
		if !ok {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		var ok2 int
		failures := make([]map[string]string, 0)
		for _, sid := range g.SocketIDs {
			if err := s.Store.ExecuteAction("socket", sid, action); err != nil {
				failures = append(failures, map[string]string{
					"socket_id": sid,
					"error":     err.Error(),
				})
				continue
			}
			ok2++
		}
		entry := store.ActivityEntry{Kind: "group", Source: "manual", Action: action, Label: g.Name}
		if len(failures) > 0 {
			entry.Status = "error"
			entry.Error = fmt.Sprintf("%d of %d failed", len(failures), ok2+len(failures))
		}
		s.Store.Activity.Add(entry)
		if err := s.Store.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"group":    g.Name,
			"updated":  ok2,
			"failures": failures,
		})
	}
}
