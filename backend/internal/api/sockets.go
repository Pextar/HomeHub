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

func (s *Server) getSockets(w http.ResponseWriter, r *http.Request) {
	s.Store.Mu.RLock()
	result := make([]*store.Socket, 0, len(s.Store.Sockets))
	for _, sock := range s.Store.Sockets {
		result = append(result, sock)
	}
	s.Store.Mu.RUnlock()

	sort.Slice(result, func(i, j int) bool {
		if result[i].Room != result[j].Room {
			return strings.ToLower(result[i].Room) < strings.ToLower(result[j].Room)
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createSocket(w http.ResponseWriter, r *http.Request) {
	var socket store.Socket
	if err := json.NewDecoder(r.Body).Decode(&socket); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	socket.Name = strings.TrimSpace(socket.Name)
	socket.Code = strings.TrimSpace(socket.Code)
	socket.Protocol = strings.TrimSpace(socket.Protocol)
	socket.Room = strings.TrimSpace(socket.Room)

	if socket.Name == "" || socket.Code == "" {
		writeError(w, http.StatusBadRequest, "name and code are required")
		return
	}

	if socket.ID == "" {
		socket.ID = fmt.Sprintf("socket_%d", time.Now().UnixNano())
	}

	s.Store.Mu.Lock()
	s.Store.Sockets[socket.ID] = &socket
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Sockets, socket.ID)
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()

	writeJSON(w, http.StatusCreated, socket)
}

func (s *Server) getSocket(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.RLock()
	socket, ok := s.Store.Sockets[id]
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	writeJSON(w, http.StatusOK, socket)
}

func (s *Server) updateSocket(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates store.Socket
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	socket, ok := s.Store.Sockets[id]
	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}

	if name := strings.TrimSpace(updates.Name); name != "" {
		socket.Name = name
	}
	if code := strings.TrimSpace(updates.Code); code != "" {
		socket.Code = code
	}
	if protocol := strings.TrimSpace(updates.Protocol); protocol != "" {
		socket.Protocol = protocol
	}
	if room := strings.TrimSpace(updates.Room); room != "" {
		socket.Room = room
	}

	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, socket)
}

func (s *Server) deleteSocket(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	if _, ok := s.Store.Sockets[id]; !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	delete(s.Store.Sockets, id)
	s.Store.CascadeDeleteSocket(id)
	if err := s.Store.Save(); err != nil {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setSocketState(w http.ResponseWriter, r *http.Request, target *bool) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	socket, ok := s.Store.Sockets[id]
	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	if err := s.Store.ApplyState(socket, target); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send RF command: "+err.Error())
		return
	}
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, socket)
}

func (s *Server) toggleSocket(w http.ResponseWriter, r *http.Request) { s.setSocketState(w, r, nil) }
func (s *Server) turnOn(w http.ResponseWriter, r *http.Request) {
	on := true
	s.setSocketState(w, r, &on)
}
func (s *Server) turnOff(w http.ResponseWriter, r *http.Request) {
	off := false
	s.setSocketState(w, r, &off)
}

// bulkSetState returns a handler that switches every socket on or off.
// It returns the number of successes and a list of failures.
func (s *Server) bulkSetState(target bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.Store.Mu.Lock()
		defer s.Store.Mu.Unlock()

		var ok int
		failures := make([]map[string]string, 0)
		for _, sock := range s.Store.Sockets {
			if err := s.Store.ApplyState(sock, &target); err != nil {
				failures = append(failures, map[string]string{
					"socket_id": sock.ID,
					"error":     err.Error(),
				})
				continue
			}
			ok++
		}
		if err := s.Store.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"updated":  ok,
			"failures": failures,
		})
	}
}
