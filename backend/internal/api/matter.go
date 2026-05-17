package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/matter"
)

// matterListDevices handles GET /api/matter/devices — returns every node
// the bridge has commissioned (whether or not it's saved as a Socket).
func (s *Server) matterListDevices(w http.ResponseWriter, r *http.Request) {
	if !s.Matter.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "matter bridge is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), matter.DefaultTimeout)
	defer cancel()
	devices, err := s.Matter.List(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if devices == nil {
		devices = []matter.State{}
	}
	writeJSON(w, http.StatusOK, devices)
}

// matterCommission handles POST /api/matter/commission.
// Body: { "pairing_code": "..." }. Returns the assigned node id so the
// frontend can immediately save it as a Socket with protocol="matter".
func (s *Server) matterCommission(w http.ResponseWriter, r *http.Request) {
	if !s.Matter.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "matter bridge is not configured")
		return
	}
	var body struct {
		PairingCode string `json:"pairing_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.PairingCode == "" {
		writeError(w, http.StatusBadRequest, "pairing_code is required")
		return
	}
	// Commissioning can take 30s+ as we discover via BLE + bring the
	// device onto Wi-Fi, so use a wider timeout than read/write calls.
	ctx, cancel := context.WithTimeout(r.Context(), matter.DefaultTimeout*6)
	defer cancel()

	nodeID, err := s.Matter.Commission(ctx, body.PairingCode)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"node_id": nodeID})
}

// matterGetState handles GET /api/matter/{socketId}.
func (s *Server) matterGetState(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := s.matterNodeID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), matter.DefaultTimeout)
	defer cancel()

	state, err := s.Matter.GetState(ctx, nodeID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// matterSetState handles PUT /api/matter/{socketId}/state.
// Accepts {on?, level?, color?, ct?} as a partial update.
func (s *Server) matterSetState(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := s.matterNodeID(w, r)
	if !ok {
		return
	}
	var update matter.StateUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), matter.DefaultTimeout)
	defer cancel()

	if err := s.Matter.SetState(ctx, nodeID, update); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	// Mirror the on/off back to the stored socket so the dashboard reflects
	// the truth without waiting for the next refresh.
	if update.On != nil {
		s.Store.Mu.Lock()
		if sock, found := s.Store.Sockets[mux.Vars(r)["socketId"]]; found {
			sock.State = *update.On
			_ = s.Store.Save()
		}
		s.Store.Mu.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}

// matterNodeID resolves the Matter node id for a given Socket id.
func (s *Server) matterNodeID(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !s.Matter.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "matter bridge is not configured")
		return "", false
	}
	id := mux.Vars(r)["socketId"]
	s.Store.Mu.RLock()
	sock, ok := s.Store.Sockets[id]
	var code string
	if ok {
		code = sock.Code
	}
	s.Store.Mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return "", false
	}
	if code == "" {
		writeError(w, http.StatusBadRequest, "socket has no Matter node id configured")
		return "", false
	}
	return code, true
}
