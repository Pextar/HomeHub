package api

import (
	"context"
	"errors"
	"homehub/internal/spotify"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Spotify integration: search/browse via the Web API (the caller's own
// account, PKCE — no client secret), playback via the speakers' linked
// account (see internal/sonos/services.go). Search/browse is open to admins
// and kid profiles: a kid searches as their own connected account, everyone
// else as the household's (see internal/spotify). Connecting an account is
// the same split — a kid's login links their profile's account, an admin's
// the household one — while the developer app's client ID stays admin-only.
// All handlers are nil-safe when the Spotify client isn't wired.

const spotifyTimeout = 10 * time.Second

// spotifyAccount resolves which connected account serves this caller: a kid
// profile always gets its own, everyone else gets the household's.
func (s *Server) spotifyAccount(r *http.Request) *spotify.Account {
	if u := currentUser(r); u != nil && u.Kid {
		return s.Spotify.For(u.ID)
	}
	return s.Spotify.For("")
}

// spotifyRedirectURI computes the OAuth redirect URI to use for the origin
// the request arrived on. It must be registered verbatim in the Spotify
// app, so the status endpoint surfaces it for the user to copy.
//
// Spotify only accepts HTTPS or loopback redirect URIs. Over HTTPS the
// URI points back at this server and the flow completes automatically.
// Over plain HTTP (the common LAN setup) a parked loopback URI is used
// instead: the browser can't load it, but the authorization code is in the
// address bar and the user pastes that address back into the Music view
// (see spotifyExchange). manual reports which of the two flows applies.
func spotifyRedirectURI(r *http.Request) (uri string, manual bool) {
	if isSecureRequest(r) {
		host := r.Host
		if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
			host = xfh
		}
		return "https://" + host + "/api/spotify/callback", false
	}
	// Match the real listen port so that a browser running on the HomeHub
	// host itself still completes automatically.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return "http://127.0.0.1:" + port + "/api/spotify/callback", true
}

func (s *Server) requireSpotify(w http.ResponseWriter) bool {
	if s.Spotify == nil {
		writeError(w, http.StatusServiceUnavailable, "Spotify integration is not available")
		return false
	}
	return true
}

// spotifyStatus handles GET /api/spotify/status — the caller's own
// account's state (a kid's login is a different connection from the
// household's).
func (s *Server) spotifyStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	st := s.spotifyAccount(r).Status()
	uri, manual := spotifyRedirectURI(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured":   st.Configured,
		"connected":    st.Connected,
		"display_name": st.DisplayName,
		"redirect_uri": uri,
		"manual":       manual,
		// Whether this login can start Connect playback (KEF speakers). A
		// login made before HomeHub asked for the player scopes searches
		// fine and can't play, and only a reconnect fixes that — so the
		// frontend has to be able to tell the difference.
		"playback": st.Playback,
		// Whether this login can be asked what the account has been
		// playing — the shelves a search box idles on. Same story: an
		// older grant searches fine and simply has nothing to say here,
		// so the shelves are absent rather than empty, and the offer to
		// reconnect is made once, quietly, where they would have been.
		"listening": st.Listening,
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
	if !decodeBody(w, r, &body) {
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
// the frontend should navigate to. Which account the flow connects is the
// caller's: a kid's login links the kid profile's account, an admin's the
// household one.
func (s *Server) spotifyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	uri, _ := spotifyRedirectURI(r)
	key := ""
	if u := currentUser(r); u != nil && u.Kid {
		key = u.ID
	}
	u, err := s.Spotify.AuthURL(key, uri)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": u})
}

// spotifyCallback handles GET /api/spotify/callback — the browser lands
// here from Spotify's consent page. On success it bounces back into the
// app — the Music view for the household account, the kid app for a kid's
// own; errors are shown by redirecting with a query the view toasts.
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
	accountKey, err := s.Spotify.HandleCallback(ctx, code, state)
	if err != nil {
		http.Redirect(w, r, "/#/music?spotify_error="+err.Error(), http.StatusFound)
		return
	}
	if accountKey != "" {
		// A kid profile connected its own account — land back on the kid
		// app, which has no Music route to toast into.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/#/music?spotify=connected", http.StatusFound)
}

// spotifyExchange handles POST /api/spotify/exchange — the paste-the-URL
// finish for the manual (plain-HTTP) flow. Body: {"url": "<address the
// browser landed on after consent>"}.
func (s *Server) spotifyExchange(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()
	if _, err := s.Spotify.ExchangeRedirect(ctx, body.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// spotifyDisconnect handles POST /api/spotify/disconnect — drops the
// caller's own account's tokens; every other account is untouched.
func (s *Server) spotifyDisconnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	if err := s.spotifyAccount(r).Disconnect(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// spotifySearch handles GET /api/spotify/search?q=…&limit=…&kind=…&offset=…
//
// `kind` narrows the search to one of tracks/albums/playlists/artists and
// `offset` pages into it — together they are what a shelf's "Show more"
// needs, since Spotify caps a search's limit at 10 and paging is the only
// way to an eleventh result. Both are optional: without them this is the
// broad four-kind search every first query makes.
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
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	kind := r.URL.Query().Get("kind")
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()
	res, err := s.spotifyAccount(r).SearchPage(ctx, q, kind, limit, offset)
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
	items, err := s.spotifyAccount(r).MyPlaylists(ctx, 30)
	if err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// spotifyListening handles GET /api/spotify/listening — what this account
// has been playing, for the shelves a search box idles on.
//
// The two halves are read together and **degrade one at a time** (§15.9): a
// refusal costs its own shelf and never the response, so a brand-new
// account with no history still gets its top tracks and vice versa. A login
// that predates the listening scopes is answered 409, which is the frontend's
// cue to offer a reconnect rather than to report a fault — this is a
// convenience, not a capability.
func (s *Server) spotifyListening(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	acc := s.spotifyAccount(r)
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()

	out := spotify.Listening{Recent: []spotify.Item{}, Top: []spotify.Item{}}
	recent, recentErr := acc.RecentTracks(ctx, 12)
	top, topErr := acc.TopTracks(ctx, 12)
	// Both refusing for the same reason is the one case worth reporting:
	// it means the grant, not the account, is the problem.
	if recentErr != nil && topErr != nil {
		writeError(w, spotifyErrStatus(recentErr), recentErr.Error())
		return
	}
	if recentErr == nil {
		out.Recent = recent
	}
	if topErr == nil {
		out.Top = top
	}
	writeJSON(w, http.StatusOK, out)
}

// spotifyArtist handles GET /api/spotify/artist?uri=spotify:artist:… — an
// artist's page: top tracks and albums, for the screen behind a search
// result (DESIGN.md §15).
func (s *Server) spotifyArtist(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	uri := strings.TrimSpace(r.URL.Query().Get("uri"))
	if uri == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()
	det, err := s.spotifyAccount(r).Artist(ctx, uri)
	if err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, det)
}

// spotifyContext handles GET /api/spotify/context?uri=spotify:playlist:…
// or spotify:album:… — the tracks inside a playlist or album, for the
// screen behind a favorite that turns out to be a list rather than one song.
func (s *Server) spotifyContext(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	uri := strings.TrimSpace(r.URL.Query().Get("uri"))
	if uri == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()
	det, err := s.spotifyAccount(r).Context(ctx, uri)
	if err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, det)
}

// spotifyErrStatus maps the two things the user can act on — no account
// linked, and an account linked before HomeHub asked for the scope this
// call needs — to 409 so the frontend can prompt for a (re)connect.
// Everything else is the service's problem, not theirs: bad gateway.
func spotifyErrStatus(err error) int {
	if errors.Is(err, spotify.ErrPlaybackScope) || errors.Is(err, spotify.ErrListeningScope) {
		return http.StatusConflict
	}
	if strings.Contains(err.Error(), "not connected") {
		return http.StatusConflict
	}
	return http.StatusBadGateway
}
