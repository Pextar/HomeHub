package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/store"
)

type roomSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Sockets int    `json:"sockets"`
	On      int    `json:"on"`
}

// getRooms returns all rooms with their socket counts and on-counts.
func (s *Server) getRooms(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	s.Store.Mu.RLock()

	// Count sockets per room name (case-insensitive key → canonical name from Room entity).
	type counts struct{ total, on int }
	byName := make(map[string]*counts)
	for _, sock := range s.Store.Sockets {
		if !canAccess(user, sock.ID) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(sock.Room))
		if key == "" {
			continue
		}
		if byName[key] == nil {
			byName[key] = &counts{}
		}
		byName[key].total++
		if sock.State {
			byName[key].on++
		}
	}

	out := make([]*roomSummary, 0, len(s.Store.Rooms))
	for _, rm := range s.Store.Rooms {
		c := byName[strings.ToLower(rm.Name)]
		rs := &roomSummary{ID: rm.ID, Name: rm.Name}
		if c != nil {
			rs.Sockets = c.total
			rs.On = c.on
		}
		out = append(out, rs)
	}
	s.Store.Mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSON(w, http.StatusOK, out)
}

// createRoom creates a new named room.
func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	var rm store.Room
	if !decodeBody(w, r, &rm) {
		return
	}

	if !s.updateOr(w, func() { delete(s.Store.Rooms, rm.ID) }, func() error {
		if rm.ID == "" {
			rm.ID = fmt.Sprintf("room_%d", time.Now().UnixNano())
		} else if _, exists := s.Store.Rooms[rm.ID]; exists {
			// A client-supplied ID must not silently replace an existing record.
			return errStatus(http.StatusConflict, "a room with that id already exists")
		}
		if err := s.Store.ValidateRoom(&rm); err != nil {
			return errInvalid(err)
		}
		s.Store.Rooms[rm.ID] = &rm
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusCreated, roomSummary{ID: rm.ID, Name: rm.Name})
}

// updateRoom renames a room and cascades the new name to all sockets and sensors.
func (s *Server) updateRoom(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates store.Room
	if !decodeBody(w, r, &updates) {
		return
	}

	var existing *store.Room
	if !s.update(w, func() error {
		var ok bool
		existing, ok = s.Store.Rooms[id]
		if !ok {
			return errStatus(http.StatusNotFound, "room not found")
		}

		oldName := existing.Name
		merged := *existing
		merged.ID = id
		if name := strings.TrimSpace(updates.Name); name != "" {
			merged.Name = name
		}
		if err := s.Store.ValidateRoom(&merged); err != nil {
			return errInvalid(err)
		}

		// Cascade rename to sockets and sensors that carried the old name.
		if !strings.EqualFold(oldName, merged.Name) {
			for _, sock := range s.Store.Sockets {
				if strings.EqualFold(sock.Room, oldName) {
					sock.Room = merged.Name
				}
			}
			for _, sn := range s.Store.Sensors {
				if strings.EqualFold(sn.Room, oldName) {
					sn.Room = merged.Name
				}
			}
			for _, sc := range s.Store.Scenes {
				if strings.EqualFold(sc.Room, oldName) {
					sc.Room = merged.Name
				}
			}
		}

		*existing = merged
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, roomSummary{ID: existing.ID, Name: existing.Name})
}

// deleteRoom removes a room and clears its name from all sockets and sensors.
func (s *Server) deleteRoom(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	// Captured so the undo can put the room back if the write fails, and
	// memory keeps matching disk.
	var existing *store.Room
	if !s.updateOr(w, func() { s.Store.Rooms[id] = existing }, func() error {
		var ok bool
		existing, ok = s.Store.Rooms[id]
		if !ok {
			return errStatus(http.StatusNotFound, "room not found")
		}

		name := existing.Name
		delete(s.Store.Rooms, id)
		// Cascade: drop schedules/timers targeting the room and prune room
		// actions from automations.
		s.Store.CascadeDeleteRoom(id)

		// Cascade: clear room name from sockets, sensors, and scenes.
		for _, sock := range s.Store.Sockets {
			if strings.EqualFold(sock.Room, name) {
				sock.Room = ""
			}
		}
		for _, sn := range s.Store.Sensors {
			if strings.EqualFold(sn.Room, name) {
				sn.Room = ""
			}
		}
		for _, sc := range s.Store.Scenes {
			if strings.EqualFold(sc.Room, name) {
				sc.Room = ""
			}
		}

		return nil
	}) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// roomSetState returns a handler that switches every socket in a named room.
func (s *Server) roomSetState(target bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		room := strings.TrimSpace(mux.Vars(r)["room"])
		if room == "" {
			writeError(w, http.StatusBadRequest, "room is required")
			return
		}
		ok, failures, found, err := s.doRoomSetState(currentUser(r), room, target)
		if !found {
			writeError(w, http.StatusNotFound, "no sockets in that room")
			return
		}
		if err != nil {
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

// doRoomSetState switches every accessible socket in a named room on or off
// through the staged flow and sends a single summary notification. Shared by
// the room REST handler and the assistant's control_room tool. found is false
// when the room has no accessible sockets. Caller must NOT hold Mu.
func (s *Server) doRoomSetState(user *store.User, room string, target bool) (ok int, failures []map[string]string, found bool, err error) {
	action := "off"
	if target {
		action = "on"
	}
	_, ok, failures, found, err = s.runStaged(stagedAction{
		Kind: "room", Action: action, Source: "manual",
		Stage: func() (string, []store.StagedSend, bool) {
			var staged []store.StagedSend
			for _, sock := range s.Store.Sockets {
				if !strings.EqualFold(sock.Room, room) || !canAccess(user, sock.ID) {
					continue
				}
				staged = append(staged, s.Store.StageSocketSend(sock.ID, action))
			}
			// An empty room is a 404 rather than a no-op success.
			return room, staged, len(staged) > 0
		},
		Notify: func(label string, _ int) string {
			return fmt.Sprintf("%s turned %s", label, action)
		},
	})
	return ok, failures, found, err
}
