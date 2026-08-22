package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/store"
	"homehub/internal/tasmota"
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
	if !decodeBody(w, r, &update) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), tasmota.DefaultTimeout)
	defer cancel()

	if err := tasmota.SetState(ctx, ip, update); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	// Mirror the on/off back to the stored socket (like the Matter handler)
	// so the dashboard reflects the truth without waiting for a refresh.
	if update.On != nil {
		_ = s.Store.Update(func() error {
			s.Store.MirrorState(mux.Vars(r)["socketId"], *update.On)
			return nil
		})
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

// tasmotaDiscoverTimeout bounds the whole LAN sweep. Generous enough for a
// /24 at the sweep's concurrency, short enough that the wizard's discovery
// step doesn't feel stalled.
const tasmotaDiscoverTimeout = 20 * time.Second

// tasmotaDiscover handles GET /api/tasmota/discover — sweeps the host's own
// subnets for devices answering the Tasmota HTTP API. Used by the add-device
// wizard so a user never has to read an IP off their router.
func (s *Server) tasmotaDiscover(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), tasmotaDiscoverTimeout)
	defer cancel()

	devices, err := tasmota.Discover(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

// tasmotaIP resolves the Tasmota device IP for a socket.
func (s *Server) tasmotaIP(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := mux.Vars(r)["socketId"]
	if !s.requireSocketAccess(w, r, id) {
		return "", false
	}
	var ip string
	var ok bool
	s.Store.View(func() {
		var sock *store.Socket
		if sock, ok = s.Store.Sockets[id]; ok {
			ip = sock.Code
		}
	})

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
