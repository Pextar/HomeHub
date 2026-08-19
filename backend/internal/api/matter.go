package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/matter"
	"homehub/internal/store"
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
// Body: { "pairing_code": "..." }.
//
// Commissioning a Matter device takes 30–180s (BLE discovery + network
// onboarding + CASE session; Thread adds ~120s mDNS wait) — far longer than the http.Server's
// WriteTimeout and longer than iOS Safari will keep a single fetch
// alive. We start the work in a background goroutine and return a
// job id immediately; the frontend polls
// /matter/commission/jobs/{id} until it reaches "done" or "error".
func (s *Server) matterCommission(w http.ResponseWriter, r *http.Request) {
	if !s.Matter.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "matter bridge is not configured")
		return
	}
	var body struct {
		PairingCode string `json:"pairing_code"`
		Transport   string `json:"transport"` // "wifi" | "thread" | "" (auto)
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.PairingCode == "" {
		writeError(w, http.StatusBadRequest, "pairing_code is required")
		return
	}

	job := s.matterJobs.create()
	pairingCode := body.PairingCode
	transport := body.Transport

	go func() {
		// Detached from r.Context() — the HTTP request will close almost
		// immediately. We give the bridge a generous ceiling of its own.
		// 5 min covers: BLE retry (up to 2×30s) + 6s waits + steps 0-12
		// (~30s) + Thread mDNS discovery (120s) with headroom to spare.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		nodeID, err := s.Matter.Commission(ctx, pairingCode, transport)
		if err != nil {
			log.Printf("matter commission job %s failed: %v", job.ID, err)
		} else {
			log.Printf("matter commission job %s done: node %s", job.ID, nodeID)
		}
		s.matterJobs.complete(job.ID, nodeID, err)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

// matterCommissionJob handles GET /api/matter/commission/jobs/{id}.
// Returns the current state of an async commission attempt.
func (s *Server) matterCommissionJob(w http.ResponseWriter, r *http.Request) {
	if !s.Matter.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "matter bridge is not configured")
		return
	}
	id := mux.Vars(r)["id"]
	job, ok := s.matterJobs.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found (may have expired)")
		return
	}
	writeJSON(w, http.StatusOK, job)
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
	if !decodeBody(w, r, &update) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), matter.DefaultTimeout)
	defer cancel()

	if err := s.Matter.SetState(ctx, nodeID, update); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	// Mirror the on/off back to the stored socket so the dashboard reflects
	// the truth without waiting for the next refresh. MirrorState also fires
	// OnChange/OnStateChange so SSE clients and push subscribers stay live.
	if update.On != nil {
		_ = s.Store.Update(func() error {
			s.Store.MirrorState(mux.Vars(r)["socketId"], *update.On)
			return nil
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// matterTransport handles GET /api/matter/transport.
// Returns all configured network transports as an array. Both "thread" and
// "wifi" can appear at the same time — the commission wizard lets the user
// pick which one to use per device.
func (s *Server) matterTransport(w http.ResponseWriter, r *http.Request) {
	if !s.Matter.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "matter bridge is not configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"transports": s.Matter.Transports()})
}

// matterNodeID resolves the Matter node id for a given Socket id.
func (s *Server) matterNodeID(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !s.Matter.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "matter bridge is not configured")
		return "", false
	}
	id := mux.Vars(r)["socketId"]
	if !s.requireSocketAccess(w, r, id) {
		return "", false
	}
	var code string
	var ok bool
	s.Store.View(func() {
		var sock *store.Socket
		if sock, ok = s.Store.Sockets[id]; ok {
			code = sock.Code
		}
	})

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
