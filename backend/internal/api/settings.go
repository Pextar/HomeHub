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

	// Captured so the undo can restore the previous settings if the write fails.
	var previous store.Settings
	if !s.updateOr(w, func() { *s.Store.Settings = previous }, func() error {
		if err := s.Store.ValidateSettings(&incoming); err != nil {
			return errInvalid(err)
		}
		previous = *s.Store.Settings
		*s.Store.Settings = incoming
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, s.Store.Settings)
}
