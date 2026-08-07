package spotify

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// A saved album arrives wrapped in an "added at" envelope rather than as a
// bare album, which is the one thing that separates this from every other
// album listing in the file.
func TestSavedAlbumsUnwrapsTheLibraryEnvelope(t *testing.T) {
	var path, limit string
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		path, limit = r.URL.Path, r.URL.Query().Get("limit")
		return jsonResponse(http.StatusOK, `{"items": [
			{"added_at": "2026-01-02T00:00:00Z", "album": {
				"uri": "spotify:album:a1", "name": "Spaces",
				"release_date": "2013-11-15", "total_tracks": 11,
				"artists": [{"name": "Nils Frahm"}], "images": [{"url": "https://i/1"}]
			}},
			{"added_at": "2026-01-01T00:00:00Z", "album": {}}
		]}`)
	}))

	got, err := c.SavedAlbums(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/me/albums" || limit != "5" {
		t.Errorf("requested %s?limit=%s, want /v1/me/albums?limit=5", path, limit)
	}
	if len(got) != 1 {
		t.Fatalf("got %d albums, want 1 — the envelope with no album in it is not a row", len(got))
	}
	if got[0].Kind != "album" || got[0].Name != "Spaces" || got[0].Year != "2013" || got[0].Sub != "Nils Frahm" {
		t.Errorf("album not flattened like every other album listing: %+v", got[0])
	}
}

// The library needs no scope beyond the one every login has carried, so it
// must work on the oldest grant in the house — unlike the listening shelves.
func TestSavedAlbumsWorksOnAGrantTooOldForTheListeningShelves(t *testing.T) {
	c := connected(t, "user-read-private playlist-read-private user-library-read",
		roundTripFunc(func(*http.Request) *http.Response {
			return jsonResponse(http.StatusOK, `{"items": []}`)
		}))
	if _, err := c.SavedAlbums(context.Background(), 5); err != nil {
		t.Errorf("SavedAlbums on an old grant = %v, want it to work", err)
	}
	if _, err := c.TopArtists(context.Background(), 5); !errors.Is(err, ErrListeningScope) {
		t.Errorf("TopArtists on the same grant = %v, want ErrListeningScope", err)
	}
}

func TestTopArtistsAsksForTheRecentWindow(t *testing.T) {
	var q string
	c := connected(t, fullScope+" "+scopeTop+" "+scopeRecent,
		roundTripFunc(func(r *http.Request) *http.Response {
			q = r.URL.Path + "?" + r.URL.RawQuery
			return jsonResponse(http.StatusOK, `{"items": [{
				"uri": "spotify:artist:ar1", "name": "Nils Frahm",
				"genres": ["ambient"], "popularity": 61,
				"followers": {"total": 900000}, "images": [{"url": "https://i/1"}]
			}, {"uri": "", "name": "nameless"}]}`)
		}))

	got, err := c.TopArtists(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "time_range=short_term") {
		t.Errorf("requested %s, want the short_term window TopTracks uses", q)
	}
	if len(got) != 1 {
		t.Fatalf("got %d artists, want the one with a URI", len(got))
	}
	if got[0].Kind != "artist" || got[0].Followers != 900000 || got[0].Popularity != 61 {
		t.Errorf("artist missing its stats: %+v", got[0])
	}
}

// Release calendars are regional, and /browse/new-releases spells the market
// parameter "country" rather than "market" like everything else does.
func TestNewReleasesSendsTheAccountsCountry(t *testing.T) {
	var q string
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		q = r.URL.RawQuery
		return jsonResponse(http.StatusOK, `{"albums": {"items": [
			{"uri": "spotify:album:a1", "name": "New", "release_date": "2026-08-01",
			 "artists": [{"name": "Someone"}]}
		]}}`)
	}))
	c.p.Household.Country = "SE"

	got, err := c.NewReleases(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "country=SE") {
		t.Errorf("requested ?%s, want country=SE", q)
	}
	if len(got) != 1 || got[0].Year != "2026" {
		t.Errorf("new releases = %+v, want one album dated 2026", got)
	}
}

// A login stored before HomeHub recorded the account's country still gets a
// list — a global one is better than an empty screen.
func TestNewReleasesWithoutACountryStillAsks(t *testing.T) {
	var q string
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		q = r.URL.RawQuery
		return jsonResponse(http.StatusOK, `{"albums": {"items": []}}`)
	}))
	if _, err := c.NewReleases(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q, "country=") {
		t.Errorf("requested ?%s, want no country when the account's is unknown", q)
	}
}

// Saving is per collection: the heart on an album card and the heart on a
// track row are the same gesture and two different Spotify endpoints.
func TestSavingRoutesTracksAndAlbumsToTheirOwnCollections(t *testing.T) {
	var method, path, ids string
	c := connected(t, scopeLibraryWrite, roundTripFunc(func(r *http.Request) *http.Response {
		method, path, ids = r.Method, r.URL.Path, r.URL.Query().Get("ids")
		if r.Method == http.MethodGet {
			return jsonResponse(http.StatusOK, `[true]`)
		}
		return jsonResponse(http.StatusOK, ``)
	}))
	ctx := context.Background()

	if err := c.SetSaved(ctx, "spotify:album:a1", true); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPut || path != "/v1/me/albums" || ids != "a1" {
		t.Errorf("saving an album sent %s %s?ids=%s, want PUT /v1/me/albums?ids=a1", method, path, ids)
	}
	if err := c.SetSaved(ctx, "spotify:track:t1", false); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/v1/me/tracks" || ids != "t1" {
		t.Errorf("unsaving a track sent %s %s?ids=%s, want DELETE /v1/me/tracks?ids=t1", method, path, ids)
	}
	if _, err := c.IsSaved(ctx, "spotify:album:a1"); err != nil {
		t.Fatal(err)
	}
	if path != "/v1/me/albums/contains" {
		t.Errorf("checking an album read %s, want /v1/me/albums/contains", path)
	}
}

// Following a playlist or an artist is a different API with a different
// scope. Accepting one here would mean a heart that silently does nothing.
func TestSavingRefusesKindsTheLibraryDoesNotHold(t *testing.T) {
	var called bool
	c := connected(t, scopeLibraryWrite, roundTripFunc(func(*http.Request) *http.Response {
		called = true
		return jsonResponse(http.StatusOK, `[true]`)
	}))
	for _, uri := range []string{"spotify:playlist:p1", "spotify:artist:ar1", "spotify:track:", "nonsense"} {
		if err := c.SetSaved(context.Background(), uri, true); err == nil {
			t.Errorf("SetSaved(%q) was accepted, want a refusal", uri)
		}
		if _, err := c.IsSaved(context.Background(), uri); err == nil {
			t.Errorf("IsSaved(%q) was accepted, want a refusal", uri)
		}
	}
	if called {
		t.Error("an unsavable URI reached the wire; it must be refused before the request")
	}
}
