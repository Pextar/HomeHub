package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/tasmota"
)

// tasmotaGetState handles GET /api/tasmota/{socketId}.
// Looks up the socket's IP (stored in Code) and proxies a state request.
func (s *Server) tasmotaGetState(w http.ResponseWriter, r *http.Request) {
	ip, ok := s.tasmotaIP(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), tasmota.DefaultTimeout)
	defer cancel()

	state, err := tasmota.GetState(ctx, ip)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// tasmotaSetState handles PUT /api/tasmota/{socketId}/state.
// Accepts {on?, dimmer?, color?, ct?} and sends the appropriate command(s).
func (s *Server) tasmotaSetState(w http.ResponseWriter, r *http.Request) {
	ip, ok := s.tasmotaIP(w, r)
	if !ok {
		return
	}

	var update tasmota.StateUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), tasmota.DefaultTimeout)
	defer cancel()

	if err := tasmota.SetState(ctx, ip, update); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// tasmotaProbe handles GET /api/tasmota/probe?ip=<ip>.
// Used by the socket editor's "Test connection" button.
func (s *Server) tasmotaProbe(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		writeError(w, http.StatusBadRequest, "ip query parameter is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), tasmota.DefaultTimeout)
	defer cancel()

	if err := tasmota.Probe(ctx, ip); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "ip": ip})
}

// tasmotaIP resolves the Tasmota device IP for a socket.
func (s *Server) tasmotaIP(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := mux.Vars(r)["socketId"]
	if !s.requireSocketAccess(w, r, id) {
		return "", false
	}
	s.Store.Mu.RLock()
	sock, ok := s.Store.Sockets[id]
	var ip string
	if ok {
		ip = sock.Code
	}
	s.Store.Mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return "", false
	}
	if ip == "" {
		writeError(w, http.StatusBadRequest, "socket has no device IP configured")
		return "", false
	}
	return ip, true
}
