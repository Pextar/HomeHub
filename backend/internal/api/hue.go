package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/hue"
)

// huePair handles POST /api/hue/pair.
// The user must press the physical link button on their Hue bridge first,
// then POST {"bridge_ip": "..."} within ~30 seconds.
// On success the bridge IP and generated username are written to Settings.
func (s *Server) huePair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BridgeIP string `json:"bridge_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	body.BridgeIP = strings.TrimSpace(body.BridgeIP)
	if body.BridgeIP == "" {
		writeError(w, http.StatusBadRequest, "bridge_ip is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	username, err := hue.Pair(ctx, body.BridgeIP)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	s.Store.Settings.HueBridgeIP = body.BridgeIP
	s.Store.Settings.HueUsername = username
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "paired OK but failed to persist: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"bridge_ip": body.BridgeIP,
		"username":  username,
	})
}

// hueListLights handles GET /api/hue/lights.
// Returns the lights registered on the configured bridge with full state.
func (s *Server) hueListLights(w http.ResponseWriter, r *http.Request) {
	bridgeIP, username, ok := s.hueCreds(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	lights, err := hue.ListLights(ctx, bridgeIP, username)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lights)
}

// hueGetLight handles GET /api/hue/lights/{id}.
func (s *Server) hueGetLight(w http.ResponseWriter, r *http.Request) {
	bridgeIP, username, ok := s.hueCreds(w)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	light, err := hue.GetLight(ctx, bridgeIP, username, id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, light)
}

// hueSetLightState handles PUT /api/hue/lights/{id}/state.
// Accepts a partial state body: {on?, bri?, hue?, sat?, ct?, transitiontime?}.
// Forwards as-is after filtering to allowed keys.
func (s *Server) hueSetLightState(w http.ResponseWriter, r *http.Request) {
	bridgeIP, username, ok := s.hueCreds(w)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]

	var incoming map[string]any
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	allowed := map[string]bool{
		"on": true, "bri": true, "hue": true, "sat": true, "ct": true, "transitiontime": true,
	}
	state := make(map[string]any, len(incoming))
	for k, v := range incoming {
		if allowed[k] {
			state[k] = v
		}
	}
	if len(state) == 0 {
		writeError(w, http.StatusBadRequest, "no recognised state fields")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := hue.SetState(ctx, bridgeIP, username, id, state); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// hueCreds reads the configured bridge IP and username and writes a 400
// response if Hue isn't set up. Returns ok=false in that case.
func (s *Server) hueCreds(w http.ResponseWriter) (bridgeIP, username string, ok bool) {
	s.Store.Mu.RLock()
	bridgeIP = s.Store.Settings.HueBridgeIP
	username = s.Store.Settings.HueUsername
	s.Store.Mu.RUnlock()
	if bridgeIP == "" || username == "" {
		writeError(w, http.StatusBadRequest, "Hue bridge not configured — set bridge IP and username in Settings")
		return "", "", false
	}
	return bridgeIP, username, true
}
