package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/store"
)

func (s *Server) getGroups(w http.ResponseWriter, r *http.Request) {
	s.Store.Mu.RLock()
	out := make([]*store.Group, 0, len(s.Store.Groups))
	for _, g := range s.Store.Groups {
		out = append(out, g)
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

func (s *Server) getGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.Store.Mu.RLock()
	g, ok := s.Store.Groups[id]
	var b []byte
	var err error
	if ok {
		b, err = json.Marshal(g)
	}
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	writeJSONBytes(w, http.StatusOK, b)
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
	} else if _, exists := s.Store.Groups[g.ID]; exists {
		// A client-supplied ID must not silently replace an existing record.
		writeError(w, http.StatusConflict, "a group with that id already exists")
		return
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
	s.Store.PruneAutomationsForTarget("group", id)
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
		name, ok, failures, found, err := s.doGroupAction(mux.Vars(r)["id"], action)
		if !found {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"group":    name,
			"updated":  ok,
			"failures": failures,
		})
	}
}

// doGroupAction applies an action to every member of a group through the
// staged flow and sends a single summary notification. Shared by the group
// REST handler and the assistant's control_group tool. found is false when no
// group has the given id. Caller must NOT hold Mu.
func (s *Server) doGroupAction(id, action string) (name string, ok int, failures []map[string]string, found bool, err error) {
	return s.runStaged(stagedAction{
		Kind: "group", Action: action, Source: "manual",
		Stage: func() (string, []store.StagedSend, bool) {
			g, exists := s.Store.Groups[id]
			if !exists {
				return "", nil, false
			}
			staged, _ := s.Store.StageAction("group", id, action)
			return g.Name, staged, true
		},
		Notify: func(label string, _ int) string {
			return fmt.Sprintf("%s turned %s", label, action)
		},
	})
}
