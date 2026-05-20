package api

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

func (s *Server) getSockets(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	s.Store.Mu.RLock()
	result := make([]*store.Socket, 0, len(s.Store.Sockets))
	for _, sock := range s.Store.Sockets {
		if !canAccess(user, sock.ID) {
			continue
		}
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
	if !s.requireSocketAccess(w, r, id) {
		return
	}

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
	// Emoji is set unconditionally so an admin can also clear it.
	socket.Emoji = strings.TrimSpace(updates.Emoji)

	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, socket)
}

// toggleFavorite flips the socket's `favorite` flag. Used by the dashboard
// star button — keeps the toggle as one round-trip without forcing the UI
// to send a full PUT payload.
func (s *Server) toggleFavorite(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !s.requireSocketAccess(w, r, id) {
		return
	}
	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()
	socket, ok := s.Store.Sockets[id]
	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	socket.Favorite = !socket.Favorite
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
	if !s.requireSocketAccess(w, r, id) {
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	socket, ok := s.Store.Sockets[id]
	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	action := "toggle"
	if target != nil {
		if *target {
			action = "on"
		} else {
			action = "off"
		}
	}
	err := s.Store.ApplyState(socket, target)
	entry := store.ActivityEntry{Kind: "socket", Source: "manual", Action: action, Label: socket.Name}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
	}
	s.Store.Activity.Add(entry)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send RF command: "+err.Error())
		return
	}
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, socket)
}

// learnSocket picks a random unused code and broadcasts an ON signal so
// a 433MHz socket in learn mode pairs to it. The caller then saves the
// socket via the regular POST /sockets with the returned code.
//
// Workflow:
//   1. User long-presses the physical socket's button (learn mode).
//   2. Frontend hits this endpoint.
//   3. Socket associates with the code; user verifies the socket clicked
//      and then saves it.
//
// For the Nexa self-learning protocol the code is "<houseID>:<unit>" —
// each socket gets its own random 26-bit house id, so they never
// collide and there is no per-controller 16-unit limit. Other protocols
// keep the legacy random 7-digit code.
func (s *Server) learnSocket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Protocol string `json:"protocol"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	protocol := strings.TrimSpace(body.Protocol)
	if protocol == "" {
		protocol = "nexa"
	}

	s.Store.Mu.RLock()
	used := make(map[string]bool, len(s.Store.Sockets))
	for _, sock := range s.Store.Sockets {
		used[sock.Code] = true
	}
	s.Store.Mu.RUnlock()

	// 32 attempts is plenty given how wide both code spaces are.
	var code string
	for i := 0; i < 32; i++ {
		var c string
		if strings.EqualFold(protocol, "nexa") {
			c = fmt.Sprintf("%d:0", rand.Intn(1<<26))
		} else {
			c = strconv.Itoa(1_000_000 + rand.Intn(9_000_000))
		}
		if !used[c] {
			code = c
			break
		}
	}
	if code == "" {
		writeError(w, http.StatusInternalServerError, "could not find an unused code")
		return
	}

	if err := s.Store.RF.Send(code, protocol, true); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send learn signal: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": code, "protocol": protocol})
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
		user := currentUser(r)
		s.Store.Mu.Lock()
		defer s.Store.Mu.Unlock()

		var ok int
		failures := make([]map[string]string, 0)
		for _, sock := range s.Store.Sockets {
			if !canAccess(user, sock.ID) {
				continue
			}
			if err := s.Store.ApplyState(sock, &target); err != nil {
				failures = append(failures, map[string]string{
					"socket_id": sock.ID,
					"error":     err.Error(),
				})
				continue
			}
			ok++
		}
		action := "off"
		if target {
			action = "on"
		}
		entry := store.ActivityEntry{Kind: "bulk", Source: "manual", Action: action, Label: "All sockets"}
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
			"updated":  ok,
			"failures": failures,
		})
	}
}
