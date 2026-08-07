package api

import (
	"context"
	"net/http"
	"strconv"
	"sync"

	"homehub/internal/spotify"
)

// The shelves a browse screen opens on.
//
// /spotify/listening already answers "what have you been playing" with two
// shelves of tracks. This is the rest of the collection — the albums someone
// saved, the playlists they keep, the artists they actually listen to — and,
// separately, the one shelf that is not about this household at all.
//
// The rule these share with the listening shelves (§15.9): a refused read
// costs its own shelf and nothing else. An account whose grant predates the
// listening scopes still has a library, and answering the whole request with
// that account's one missing permission would empty a screen that had three
// working shelves on it.

// spotifyLibrary handles GET /api/spotify/library?limit= — the account's own
// collection, three shelves at once.
//
// Three reads in parallel rather than in sequence: they are independent, and
// a browse screen that waits for the slowest of three round trips laid end to
// end is a browse screen someone walks away from.
func (s *Server) spotifyLibrary(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	limit := 20
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 50 {
		limit = n
	}
	acc := s.spotifyAccount(r)
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()

	var (
		mu                        sync.Mutex
		albums, playlists, artist []spotify.Item
		failures                  int
		firstErr                  error
	)
	shelf := func(into *[]spotify.Item, read func() ([]spotify.Item, error)) func() {
		return func() {
			items, err := read()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			*into = items
		}
	}

	var wg sync.WaitGroup
	for _, fn := range []func(){
		shelf(&albums, func() ([]spotify.Item, error) { return acc.SavedAlbums(ctx, limit) }),
		shelf(&playlists, func() ([]spotify.Item, error) { return acc.MyPlaylists(ctx, limit) }),
		shelf(&artist, func() ([]spotify.Item, error) { return acc.TopArtists(ctx, limit) }),
	} {
		wg.Add(1)
		go func() { defer wg.Done(); fn() }()
	}
	wg.Wait()

	// All three refusing means the account, not one permission, is the
	// problem — the same call the listening shelves make when both halves
	// fail. Anything less is a partial answer, which is a real answer.
	if failures == 3 {
		writeError(w, spotifyErrStatus(firstErr), firstErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"albums":    emptyItems(albums),
		"playlists": emptyItems(playlists),
		"artists":   emptyItems(artist),
	})
}

// spotifyNewReleases handles GET /api/spotify/new-releases?limit= — what came
// out lately. The only shelf here that is about the catalog rather than about
// this household, and so the only answer available on an evening when nobody
// wants to hear anything they already know.
func (s *Server) spotifyNewReleases(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	limit := 20
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 50 {
		limit = n
	}
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()

	items, err := s.spotifyAccount(r).NewReleases(ctx, limit)
	if err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptyItems(items))
}

// emptyItems substitutes an empty slice for a nil one, so a shelf that read
// nothing encodes as [] rather than null and the frontend can map over it
// without a guard per shelf.
func emptyItems(in []spotify.Item) []spotify.Item {
	if in == nil {
		return []spotify.Item{}
	}
	return in
}
