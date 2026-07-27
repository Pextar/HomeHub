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
	var b []byte
	var err error
	s.Store.View(func() {
		out := make([]*store.Group, 0, len(s.Store.Groups))
		for _, g := range s.Store.Groups {
			out = append(out, g)
		}
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
		// Marshalled inside the lock because out holds live pointers.
		b, err = json.Marshal(out)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) getGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var b []byte
	var err error
	var ok bool
	s.Store.View(func() {
		var g *store.Group
		if g, ok = s.Store.Groups[id]; ok {
			b, err = json.Marshal(g)
		}
	})
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
	if !decodeBody(w, r, &g) {
		return
	}

	// Validation runs before the id check, so a body that is both invalid
	// and colliding reports the validation problem.
	if !s.updateOr(w, func() { delete(s.Store.Groups, g.ID) }, func() error {
		if err := s.Store.ValidateGroup(&g); err != nil {
			return errInvalid(err)
		}
		if g.ID == "" {
			g.ID = fmt.Sprintf("group_%d", time.Now().UnixNano())
		} else if _, exists := s.Store.Groups[g.ID]; exists {
			// A client-supplied ID must not silently replace an existing record.
			return errConflict("group")
		}
		s.Store.Groups[g.ID] = &g
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates store.Group
	if !decodeBody(w, r, &updates) {
		return
	}

	var existing *store.Group
	if !s.update(w, func() error {
		var ok bool
		existing, ok = s.Store.Groups[id]
		if !ok {
			return errNotFound("group")
		}
		// Merge into a copy and validate that, so a rejected edit leaves the
		// stored group untouched.
		merged := *existing
		if name := strings.TrimSpace(updates.Name); name != "" {
			merged.Name = name
		}
		if updates.SocketIDs != nil {
			merged.SocketIDs = updates.SocketIDs
		}
		if err := s.Store.ValidateGroup(&merged); err != nil {
			return errInvalid(err)
		}
		*existing = merged
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if !s.update(w, func() error {
		if _, ok := s.Store.Groups[id]; !ok {
			return errNotFound("group")
		}
		delete(s.Store.Groups, id)
		s.Store.CascadeDeleteTarget("group", id)
		return nil
	}) {
		return
	}
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
