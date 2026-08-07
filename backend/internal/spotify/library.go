package spotify

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// The account's own shelves, and the one shelf that isn't the account's.
//
// Search answers a question someone already has. Everything here exists for
// the case where they don't: a wall panel is walked up to, not typed at, and
// the shelves that fill a browse screen before a single key is pressed are
// what decide whether music gets put on at all (DESIGN.md §16).
//
// MyPlaylists, RecentTracks and TopTracks already cover part of that and stay
// where they are. What was missing is the rest of a record collection — the
// albums someone saved, the artists they actually listen to — plus the one
// thing no account can supply: what came out this week.

// SavedAlbums is the account's own album library, most recently added first
// (Spotify's own order for this endpoint, which is the useful one: a record
// saved this week is the one someone means to play).
//
// Needs no scope beyond user-library-read, which every login has carried from
// the start — so unlike the listening shelves this one works on the oldest
// grant in the house.
func (a *Account) SavedAlbums(ctx context.Context, limit int) ([]Item, error) {
	limit = clampPage(limit, 30)
	var raw struct {
		Items []struct {
			Album wireAlbum `json:"album"`
		} `json:"items"`
	}
	if err := a.apiGet(ctx, "/me/albums", url.Values{"limit": {fmt.Sprint(limit)}}, &raw); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(raw.Items))
	for _, e := range raw.Items {
		if e.Album.URI == "" {
			continue
		}
		out = append(out, albumItem(e.Album))
	}
	return out, nil
}

// TopArtists is who this account actually listens to. `short_term` — roughly
// the last month — for the same reason TopTracks uses it: the shelf answers
// "put something on" today, and a lifetime ranking barely moves week to week.
//
// An artist row is worth more than a track row on a browse screen, because
// tapping it opens a page with a discography behind it rather than starting
// three minutes of music and then needing another decision.
func (a *Account) TopArtists(ctx context.Context, limit int) ([]Item, error) {
	if err := a.requireListening(); err != nil {
		return nil, err
	}
	limit = clampPage(limit, 20)
	var raw struct {
		Items []wireArtistFull `json:"items"`
	}
	if err := a.apiGet(ctx, "/me/top/artists", url.Values{
		"limit":      {fmt.Sprint(limit)},
		"time_range": {"short_term"},
	}, &raw); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(raw.Items))
	for _, ar := range raw.Items {
		if ar.URI == "" {
			continue
		}
		out = append(out, artistItem(ar))
	}
	return out, nil
}

// NewReleases is what came out lately — the one shelf that is about the
// catalog rather than about this household, and the only answer available on
// an evening when nobody wants to hear anything they already know.
//
// Market matters here more than anywhere else: release calendars are
// regional, and a request with no market answers a global list containing
// records the account cannot play. A login too old to know its country still
// gets a list, which is better than an empty screen.
func (a *Account) NewReleases(ctx context.Context, limit int) ([]Item, error) {
	limit = clampPage(limit, 20)
	q := url.Values{"limit": {fmt.Sprint(limit)}}
	// /browse/new-releases spells the market parameter "country".
	if m := a.market(); m.Get("market") != "" {
		q.Set("country", m.Get("market"))
	}
	var raw struct {
		Albums struct {
			Items []wireAlbum `json:"items"`
		} `json:"albums"`
	}
	if err := a.apiGet(ctx, "/browse/new-releases", q, &raw); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(raw.Albums.Items))
	for _, al := range raw.Albums.Items {
		if al.URI == "" {
			continue
		}
		out = append(out, albumItem(al))
	}
	return out, nil
}

// clampPage keeps a caller's page size inside what Spotify's list endpoints
// accept, substituting a sensible default for nonsense.
func clampPage(limit, def int) int {
	if limit <= 0 || limit > 50 {
		return def
	}
	return limit
}

// ── Saving ───────────────────────────────────────────────────────────────
//
// The library is per kind: tracks live under /me/tracks, albums under
// /me/albums, and the two are separate collections with separate contains
// checks. The heart on an album card and the heart on a track row therefore
// mean different things to Spotify even though they are the same gesture, and
// savedPath is the whole of that difference.

// savedPath maps a URI to the library collection it belongs to and its bare
// id. Only the kinds Spotify's library actually holds are accepted: following
// a playlist or an artist is a different API with a different scope, and
// answering it here would mean a heart that silently does nothing.
func savedPath(uri string) (path, id string, err error) {
	for _, k := range []struct{ prefix, path string }{
		{"spotify:track:", "/me/tracks"},
		{"spotify:album:", "/me/albums"},
	} {
		if rest := strings.TrimPrefix(uri, k.prefix); rest != uri && rest != "" {
			return k.path, rest, nil
		}
	}
	return "", "", fmt.Errorf("spotify: %q cannot be saved to a library — only tracks and albums can", uri)
}
