package api

import (
	"encoding/json"
	"net/http"

	"homehub/internal/qobuz"
)

// Qobuz setup and account handlers.
//
// Deliberately shorter than the Spotify ones, because the flow is. Spotify
// needs a PKCE round trip through a browser and a redirect back; Qobuz takes an
// email and password once and returns a token. What it needs that Spotify does
// not is an app id and secret issued to the *application* — HomeHub ships none,
// so the two are separate steps with separate error messages, and the UI can
// tell a household which of the two they are missing.

// qobuzStatus handles GET /api/qobuz/status.
func (s *Server) qobuzStatus(w http.ResponseWriter, r *http.Request) {
	if s.Qobuz == nil {
		writeJSON(w, http.StatusOK, qobuz.Status{})
		return
	}
	writeJSON(w, http.StatusOK, s.Qobuz.Status())
}

// qobuzSetConfig handles PUT /api/qobuz/config — the app credentials.
func (s *Server) qobuzSetConfig(w http.ResponseWriter, r *http.Request) {
	if s.Qobuz == nil {
		writeError(w, http.StatusServiceUnavailable, "Qobuz isn't set up on this server")
		return
	}
	var body struct {
		AppID     string `json:"app_id"`
		AppSecret string `json:"app_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.Qobuz.SetApp(body.AppID, body.AppSecret); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Qobuz.Status())
}

// qobuzLogin handles POST /api/qobuz/login.
//
// The password is read, forwarded and dropped — it is never stored and never
// logged. What persists is the token Qobuz returns for it, which is what every
// later call actually uses.
func (s *Server) qobuzLogin(w http.ResponseWriter, r *http.Request) {
	if s.Qobuz == nil {
		writeError(w, http.StatusServiceUnavailable, "Qobuz isn't set up on this server")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.Qobuz.Login(r.Context(), body.Email, body.Password); err != nil {
		status := http.StatusBadGateway
		switch err {
		case qobuz.ErrNoApp:
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Qobuz.Status())
}

// qobuzDisconnect handles POST /api/qobuz/disconnect. It forgets the listener
// and keeps the app credentials, which belong to the installation.
func (s *Server) qobuzDisconnect(w http.ResponseWriter, r *http.Request) {
	if s.Qobuz == nil {
		writeError(w, http.StatusServiceUnavailable, "Qobuz isn't set up on this server")
		return
	}
	if err := s.Qobuz.Disconnect(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Qobuz.Status())
}
