package api

import (
	"net/http"
)

// The HTTP face of "continue with similar music". The engine that acts on it
// lives in internal/autoplay; this is the switch, and the one thing about it
// worth saying here is that the setting is per *coordinator* — the same
// requirement every other group-level control on this surface has.

// sonosSetAutoplay handles PUT /api/sonos/{id}/autoplay with
// {"enabled": bool}. id must be a group's coordinator — the same requirement
// every other group-level control here has.
func (s *Server) sonosSetAutoplay(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	s.Autoplay.SetEnabled(sp.ID, body.Enabled)
	w.WriteHeader(http.StatusNoContent)
}
