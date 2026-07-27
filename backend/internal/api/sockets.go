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

	"homehub/internal/store"
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
	sort.Slice(result, func(i, j int) bool {
		if result[i].Room != result[j].Room {
			return strings.ToLower(result[i].Room) < strings.ToLower(result[j].Room)
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	// Marshal under the lock so we snapshot the sockets consistently rather
	// than handing live *store.Socket pointers to the encoder after unlocking
	// (writers mutate those structs in place).
	b, err := json.Marshal(result)
	s.Store.Mu.RUnlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}

	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) createSocket(w http.ResponseWriter, r *http.Request) {
	var socket store.Socket
	if !decodeBody(w, r, &socket) {
		return
	}

	if err := s.Store.ValidateSocket(&socket); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hadID := socket.ID != ""
	if !hadID {
		socket.ID = fmt.Sprintf("socket_%d", time.Now().UnixNano())
	}

	if !s.updateOr(w, func() { delete(s.Store.Sockets, socket.ID) }, func() error {
		if _, exists := s.Store.Sockets[socket.ID]; exists && hadID {
			// A client-supplied ID must not silently replace an existing record.
			return errStatus(http.StatusConflict, "a socket with that id already exists")
		}
		s.Store.Sockets[socket.ID] = &socket
		return nil
	}) {
		return
	}

	writeJSON(w, http.StatusCreated, socket)
}

func (s *Server) getSocket(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !s.requireSocketAccess(w, r, id) {
		return
	}

	var b []byte
	var err error
	var ok bool
	s.Store.View(func() {
		var socket *store.Socket
		if socket, ok = s.Store.Sockets[id]; ok {
			b, err = json.Marshal(socket)
		}
	})
	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) updateSocket(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates store.Socket
	if !decodeBody(w, r, &updates) {
		return
	}

	var socket *store.Socket
	if !s.update(w, func() error {
		var ok bool
		socket, ok = s.Store.Sockets[id]
		if !ok {
			return errStatus(http.StatusNotFound, "socket not found")
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

		// Re-validate after applying updates; catches e.g. switching an existing
		// socket to the Nexa protocol with a code that isn't in houseID:unit form.
		if err := s.Store.ValidateSocket(socket); err != nil {
			return errInvalid(err)
		}
		return nil
	}) {
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
	var socket *store.Socket
	if !s.update(w, func() error {
		var ok bool
		socket, ok = s.Store.Sockets[id]
		if !ok {
			return errStatus(http.StatusNotFound, "socket not found")
		}
		socket.Favorite = !socket.Favorite
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, socket)
}

func (s *Server) deleteSocket(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if !s.update(w, func() error {
		if _, ok := s.Store.Sockets[id]; !ok {
			return errStatus(http.StatusNotFound, "socket not found")
		}
		delete(s.Store.Sockets, id)
		s.Store.CascadeDeleteSocket(id)
		return nil
	}) {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setSocketState(w http.ResponseWriter, r *http.Request, target *bool) {
	id := mux.Vars(r)["id"]
	if !s.requireSocketAccess(w, r, id) {
		return
	}

	var socket *store.Socket
	if !s.update(w, func() error {
		var ok bool
		socket, ok = s.Store.Sockets[id]
		if !ok {
			return errStatus(http.StatusNotFound, "socket not found")
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
			return errStatus(http.StatusInternalServerError, "failed to send RF command: %s", err)
		}
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, socket)
}

// doControlSocket applies on/off/toggle to a single socket by id, transmitting
// synchronously (like the toggle/on/off REST handlers) so a device error is
// reported directly. Shared by the assistant's control_device tool. found is
// false when no socket has the id. Caller must NOT hold Mu.
func (s *Server) doControlSocket(id, action string) (sock store.Socket, found bool, err error) {
	var target *bool
	switch action {
	case "on":
		t := true
		target = &t
	case "off":
		t := false
		target = &t
	case "toggle":
		target = nil
	default:
		return store.Socket{}, true, fmt.Errorf("unsupported action %q (use on, off, or toggle)", action)
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()
	socket, ok := s.Store.Sockets[id]
	if !ok {
		return store.Socket{}, false, nil
	}
	applyErr := s.Store.ApplyState(socket, target)
	entry := store.ActivityEntry{Kind: "socket", Source: "assistant", Action: action, Label: socket.Name}
	if applyErr != nil {
		entry.Status = "error"
		entry.Error = applyErr.Error()
	}
	s.Store.Activity.Add(entry)
	if applyErr != nil {
		return *socket, true, applyErr
	}
	if err := s.Store.Save(); err != nil {
		return *socket, true, err
	}
	return *socket, true, nil
}

// learnSocket picks a random unused code and broadcasts an ON signal so
// a 433MHz socket in learn mode pairs to it. The caller then saves the
// socket via the regular POST /sockets with the returned code.
//
// Workflow:
//  1. User long-presses the physical socket's button (learn mode).
//  2. Frontend hits this endpoint.
//  3. Socket associates with the code; user verifies the socket clicked
//     and then saves it.
//
// For the Nexa self-learning protocol the code is "<houseID>:<unit>" —
// each socket gets its own random 26-bit house id, so they never
// collide and there is no per-controller 16-unit limit. Other protocols
// keep the legacy random 7-digit code.
//
// If the caller supplies a non-empty "code" field in the request body, that
// code is re-used instead of generating a new one. This lets the user retry
// pairing a stubborn socket (e.g. Telldus 312530) without the code changing
// between attempts: put the socket back into learn mode, tap Pair again, and
// the same code is broadcast so the socket can learn it on a later attempt.
func (s *Server) learnSocket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Protocol string `json:"protocol"`
		Code     string `json:"code"` // optional: resend this code instead of generating a new one
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	protocol := strings.TrimSpace(body.Protocol)
	if protocol == "" {
		protocol = "nexa"
	}

	var code string

	if existing := strings.TrimSpace(body.Code); existing != "" {
		// Caller wants to resend an existing code. Validate format for Nexa;
		// other protocols accept any non-empty string.
		if strings.EqualFold(protocol, "nexa") {
			if err := store.ValidateNexaCode(existing); err != nil {
				writeError(w, http.StatusBadRequest, "invalid code: "+err.Error())
				return
			}
		}
		code = existing
	} else {
		// Generate a fresh unused code.
		s.Store.Mu.RLock()
		used := make(map[string]bool, len(s.Store.Sockets))
		for _, sock := range s.Store.Sockets {
			used[sock.Code] = true
		}
		s.Store.Mu.RUnlock()

		// 32 attempts is plenty given how wide both code spaces are.
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
	}

	// Send the pairing signal twice with a short pause between bursts.
	// Doubling the on-air time significantly improves reliability for sockets
	// whose receive windows are short or whose decoders need a clean frame
	// after the radio settles (observed with some Telldus/Proove models).
	if err := s.Store.Transmit(code, protocol, true); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send learn signal: "+err.Error())
		return
	}
	time.Sleep(200 * time.Millisecond)
	_ = s.Store.Transmit(code, protocol, true) // best-effort second burst

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
func (s *Server) bulkSetState(target bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, failures, err := s.doBulkSetState(currentUser(r), target)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"updated":  ok,
			"failures": failures,
		})
	}
}

// doBulkSetState switches every socket the user may access on or off and
// returns the success count plus the per-socket failure list. Device I/O
// happens between two lock acquisitions (staged flow) so one slow device
// can't stall the rest of the API. Shared by the bulk REST handler and the
// assistant's all_devices tool. Caller must NOT hold Mu.
func (s *Server) doBulkSetState(user *store.User, target bool) (ok int, failures []map[string]string, err error) {
	action := "off"
	if target {
		action = "on"
	}
	_, ok, failures, _, err = s.runStaged(stagedAction{
		Kind: "bulk", Action: action, Source: "manual",
		Stage: func() (string, []store.StagedSend, bool) {
			staged := make([]store.StagedSend, 0, len(s.Store.Sockets))
			for _, sock := range s.Store.Sockets {
				if !canAccess(user, sock.ID) {
					continue
				}
				staged = append(staged, s.Store.StageSocketSend(sock.ID, action))
			}
			// Always "found": switching an empty set is a success, not a 404.
			return "All sockets", staged, true
		},
		Notify: func(_ string, _ int) string {
			return fmt.Sprintf("All devices turned %s", action)
		},
	})
	return ok, failures, err
}

// stagedFailures splits staged results into a success count and the
// per-socket failure list shape shared by all bulk endpoints.
func stagedFailures(staged []store.StagedSend) (ok int, failures []map[string]string) {
	failures = make([]map[string]string, 0)
	for _, c := range staged {
		if c.Err != nil {
			failures = append(failures, map[string]string{
				"socket_id": c.SocketID,
				"error":     c.Err.Error(),
			})
			continue
		}
		ok++
	}
	return ok, failures
}
