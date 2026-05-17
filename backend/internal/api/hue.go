package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

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
// Returns the lights registered on the configured bridge.
func (s *Server) hueListLights(w http.ResponseWriter, _ *http.Request) {
	s.Store.Mu.RLock()
	bridgeIP := s.Store.Settings.HueBridgeIP
	username := s.Store.Settings.HueUsername
	s.Store.Mu.RUnlock()

	if bridgeIP == "" || username == "" {
		writeError(w, http.StatusBadRequest, "Hue bridge not configured — set bridge IP and username in Settings")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lights, err := hue.ListLights(ctx, bridgeIP, username)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lights)
}
