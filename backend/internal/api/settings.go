package api

import (
	"net/http"

	"homehub/internal/store"
)

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	var out store.Settings
	s.Store.View(func() { out = *s.Store.Settings })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var incoming store.Settings
	if !decodeBody(w, r, &incoming) {
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
	if !s.saveStoreOr(w, func() { *s.Store.Settings = previous }) {
		return
	}
	writeJSON(w, http.StatusOK, s.Store.Settings)
}
