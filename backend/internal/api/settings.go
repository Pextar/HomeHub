package api

import (
	"encoding/json"
	"net/http"

	"homehub/internal/store"
)

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	s.Store.Mu.RLock()
	out := *s.Store.Settings
	s.Store.Mu.RUnlock()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var incoming store.Settings
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if err := s.Store.ValidateSettings(&incoming); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	previous := *s.Store.Settings
	*s.Store.Settings = incoming
	if err := s.Store.Save(); err != nil {
		*s.Store.Settings = previous
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Store.Settings)
}
