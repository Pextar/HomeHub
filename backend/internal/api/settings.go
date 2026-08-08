package api

import (
	"net/http"

	"homehub/internal/store"
)

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	var out store.Settings
	s.Store.View(func() { out = *s.Store.Settings })
	// The announce presets answer resolved rather than raw: nil on disk
	// means "nobody has set these", and an editor that had to know about
	// that would be a second place the defaults live. The panel reads them
	// through the same resolver on /api/announce, so both surfaces are
	// looking at one list. Saving from the editor then makes the household's
	// choice explicit on disk, including the choice to have none.
	out.AnnouncePresets = out.Presets()
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
	var out store.Settings
	s.Store.View(func() { out = *s.Store.Settings })
	out.AnnouncePresets = out.Presets()
	writeJSON(w, http.StatusOK, out)
}
