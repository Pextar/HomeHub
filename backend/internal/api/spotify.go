package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Spotify integration: search/browse via the Web API (user's own account,
// PKCE — no client secret), playback via the speakers' linked account (see
// internal/sonos/services.go). All handlers are admin-gated in server.go
// and nil-safe when the Spotify client isn't wired.

const spotifyTimeout = 10 * time.Second

// spotifyRedirectURI computes the OAuth redirect URI for the origin the
// request arrived on. It must be registered verbatim in the Spotify app, so
// the status endpoint surfaces it for the user to copy. Spotify requires
// HTTPS (or a loopback address) for redirect URIs — hence HomeHub's HTTPS
// listener is the natural host for this.
func spotifyRedirectURI(r *http.Request) string {
	scheme := "http"
	if isSecureRequest(r) {
		scheme = "https"
	}
	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}
	return scheme + "://" + host + "/api/spotify/callback"
}

func (s *Server) requireSpotify(w http.ResponseWriter) bool {
	if s.Spotify == nil {
		writeError(w, http.StatusServiceUnavailable, "Spotify integration is not available")
		return false
	}
	return true
}

// spotifyStatus handles GET /api/spotify/status.
func (s *Server) spotifyStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	st := s.Spotify.Status()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured":   st.Configured,
		"connected":    st.Connected,
		"display_name": st.DisplayName,
		"redirect_uri": spotifyRedirectURI(r),
	})
}

// spotifySetConfig handles PUT /api/spotify/config with {"client_id": "..."}.
func (s *Server) spotifySetConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	var body struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(body.ClientID) == "" {
		writeError(w, http.StatusBadRequest, "client_id is required")
		return
	}
	if err := s.Spotify.SetClientID(body.ClientID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// spotifyLogin handles GET /api/spotify/login — returns the authorize URL
// the frontend should navigate to.
func (s *Server) spotifyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	u, err := s.Spotify.AuthURL(spotifyRedirectURI(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": u})
}

// spotifyCallback handles GET /api/spotify/callback — the browser lands
// here from Spotify's consent page. On success it bounces back into the
// Music view; errors are shown by redirecting with a query the view toasts.
func (s *Server) spotifyCallback(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		http.Redirect(w, r, "/#/music?spotify_error="+e, http.StatusFound)
		return
	}
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing code/state")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()
	if err := s.Spotify.HandleCallback(ctx, code, state, spotifyRedirectURI(r)); err != nil {
		http.Redirect(w, r, "/#/music?spotify_error="+err.Error(), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/#/music?spotify=connected", http.StatusFound)
}

// spotifyDisconnect handles POST /api/spotify/disconnect.
func (s *Server) spotifyDisconnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	if err := s.Spotify.Disconnect(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// spotifySearch handles GET /api/spotify/search?q=…&limit=…
func (s *Server) spotifySearch(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()
	res, err := s.Spotify.Search(ctx, q, limit)
	if err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// spotifyPlaylists handles GET /api/spotify/playlists — the connected
// account's own playlists, for browsing without typing.
func (s *Server) spotifyPlaylists(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()
	items, err := s.Spotify.MyPlaylists(ctx, 30)
	if err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// spotifyErrStatus maps "not connected" to 409 so the frontend can prompt
// re-auth, everything else to bad-gateway.
func spotifyErrStatus(err error) int {
	if strings.Contains(err.Error(), "not connected") {
		return http.StatusConflict
	}
	return http.StatusBadGateway
}
