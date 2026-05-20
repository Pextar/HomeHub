package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

// roomSetState returns a handler that switches every socket in a single
// room.
func (s *Server) roomSetState(target bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		room := strings.TrimSpace(mux.Vars(r)["room"])
		if room == "" {
			writeError(w, http.StatusBadRequest, "room is required")
			return
		}

		user := currentUser(r)
		s.Store.Mu.Lock()
		defer s.Store.Mu.Unlock()

		var ok int
		failures := make([]map[string]string, 0)
		var matched bool
		for _, sock := range s.Store.Sockets {
			if !strings.EqualFold(sock.Room, room) || !canAccess(user, sock.ID) {
				continue
			}
			matched = true
			if err := s.Store.ApplyState(sock, &target); err != nil {
				failures = append(failures, map[string]string{
					"socket_id": sock.ID,
					"error":     err.Error(),
				})
				continue
			}
			ok++
		}
		if !matched {
			writeError(w, http.StatusNotFound, "no sockets in that room")
			return
		}
		action := "off"
		if target {
			action = "on"
		}
		entry := store.ActivityEntry{Kind: "room", Source: "manual", Action: action, Label: room}
		if len(failures) > 0 {
			entry.Status = "error"
			entry.Error = fmt.Sprintf("%d of %d failed", len(failures), ok+len(failures))
		}
		s.Store.Activity.Add(entry)
		if err := s.Store.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"room":     room,
			"updated":  ok,
			"failures": failures,
		})
	}
}

// getRooms returns rooms with their socket counts and on-counts.
func (s *Server) getRooms(w http.ResponseWriter, r *http.Request) {
	type roomSummary struct {
		Name    string `json:"name"`
		Sockets int    `json:"sockets"`
		On      int    `json:"on"`
	}
	user := currentUser(r)
	s.Store.Mu.RLock()
	byName := make(map[string]*roomSummary)
	for _, sock := range s.Store.Sockets {
		if !canAccess(user, sock.ID) {
			continue
		}
		name := sock.Room
		if name == "" {
			name = "Unassigned"
		}
		rs, ok := byName[name]
		if !ok {
			rs = &roomSummary{Name: name}
			byName[name] = rs
		}
		rs.Sockets++
		if sock.State {
			rs.On++
		}
	}
	s.Store.Mu.RUnlock()

	out := make([]*roomSummary, 0, len(byName))
	for _, rs := range byName {
		out = append(out, rs)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSON(w, http.StatusOK, out)
}
