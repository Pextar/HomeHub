package spotify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// The catalog mapping is what makes a search result worth reading: a track
// row that can say its album and its length, an album card that can say its
// year, an artist card that can say how big the name is. These tests pin the
// fields each surface renders.

func TestSearchEnrichesItems(t *testing.T) {
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		if r.URL.Path != "/v1/search" {
			t.Fatalf("unexpected request to %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{
			"tracks": {"items": [{
				"uri": "spotify:track:t1", "name": "Karma Police",
				"duration_ms": 264000, "explicit": true,
				"artists": [{"name": "Radiohead"}],
				"album": {"name": "OK Computer", "images": [{"url": "https://i/1"}]}
			}]},
			"albums": {"items": [{
				"uri": "spotify:album:a1", "name": "OK Computer",
				"album_type": "album", "release_date": "1997-05-21", "total_tracks": 12,
				"artists": [{"name": "Radiohead"}], "images": [{"url": "https://i/2"}]
			}]},
			"playlists": {"items": [{
				"uri": "spotify:playlist:p1", "name": "Mix",
				"owner": {"display_name": "petter"},
				"tracks": {"total": 87}, "images": [{"url": "https://i/3"}]
			}]},
			"artists": {"items": [{
				"uri": "spotify:artist:ar1", "name": "Radiohead",
				"genres": ["alternative rock", "art rock"], "popularity": 82,
				"followers": {"total": 10567890}, "images": [{"url": "https://i/4"}]
			}]}
		}`)
	}))

	res, err := c.Search(context.Background(), "radiohead", 5)
	if err != nil {
		t.Fatal(err)
	}
	tr := res.Tracks[0]
	if tr.Album != "OK Computer" || tr.DurationMS != 264000 || !tr.Explicit {
		t.Errorf("track missing its record/length/flag: %+v", tr)
	}
	al := res.Albums[0]
	if al.Year != "1997" || al.TotalTracks != 12 {
		t.Errorf("album missing its year/count: %+v", al)
	}
	if res.Playlists[0].TotalTracks != 87 {
		t.Errorf("playlist missing its track count: %+v", res.Playlists[0])
	}
	ar := res.Artists[0]
	if ar.Followers != 10567890 || ar.Popularity != 82 || len(ar.Genres) != 2 {
		t.Errorf("artist missing its stats: %+v", ar)
	}
}

// Spotify tightened /search's limit cap from the documented 50 down to 10
// (anything higher is answered 400 "Invalid limit"), so a caller asking for
// more must never reach the wire with it.
func TestSearchClampsToSpotifyCap(t *testing.T) {
	var sent string
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		sent = r.URL.Query().Get("limit")
		return jsonResponse(http.StatusOK, `{}`)
	}))

	for _, ask := range []int{0, 12, 50} {
		if _, err := c.Search(context.Background(), "adele", ask); err != nil {
			t.Fatal(err)
		}
		if sent != "10" {
			t.Errorf("Search(limit=%d) sent limit=%s, want 10", ask, sent)
		}
	}
}

// TestArtistPageSplitAndDegrade pins the contracts the artist screen leans
// on: the discography arrives split into albums and singles, a refused
// section (related artists 403s for Development Mode apps) costs that
// section, never the page — and the "popular" shelf comes from search,
// since Spotify removed the top-tracks endpoint in February 2026.
func TestArtistPageSplitAndDegrade(t *testing.T) {
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		switch {
		case strings.HasSuffix(r.URL.Path, "/related-artists"):
			return jsonResponse(http.StatusForbidden, `{"error":{"message":"Forbidden"}}`)
		case r.URL.Path == "/v1/search":
			if q := r.URL.Query().Get("q"); q != `artist:"A"` {
				t.Errorf("popular tracks should be an artist-scoped search, got q=%q", q)
			}
			return jsonResponse(http.StatusOK, `{"tracks": {"items": [{
				"uri": "spotify:track:t1", "name": "Song", "duration_ms": 200000,
				"artists": [{"name": "A"}], "album": {"name": "Rec", "images": []}
			}]}}`)
		case strings.HasSuffix(r.URL.Path, "/albums"):
			if l := r.URL.Query().Get("limit"); l != "10" {
				t.Errorf("Spotify caps this endpoint at 10 (400 past that); sent limit=%s", l)
			}
			return jsonResponse(http.StatusOK, `{"items": [
				{"uri": "spotify:album:lp", "name": "LP", "album_type": "album",
				 "release_date": "2020-03-01", "total_tracks": 10, "artists": [{"name": "A"}], "images": []},
				{"uri": "spotify:album:ep", "name": "EP", "album_type": "single",
				 "release_date": "2021", "total_tracks": 4, "artists": [{"name": "A"}], "images": []},
				{"uri": "spotify:album:lp2", "name": "lp", "album_type": "album",
				 "release_date": "2020-06-01", "total_tracks": 10, "artists": [{"name": "A"}], "images": []}
			]}`)
		default:
			// The artist object itself.
			return jsonResponse(http.StatusOK, `{
				"uri": "spotify:artist:ar1", "name": "A",
				"genres": ["rock"], "popularity": 70,
				"followers": {"total": 1234}, "images": [{"url": "https://i/big"}, {"url": "https://i/small"}]
			}`)
		}
	}))
	c.p.Country = "SE" // skip the market backfill round trip

	det, err := c.Artist(context.Background(), "spotify:artist:ar1")
	if err != nil {
		t.Fatal(err)
	}
	if det.Followers != 1234 || det.Popularity != 70 || len(det.Genres) != 1 {
		t.Errorf("header stats missing: %+v", det)
	}
	if det.ArtURL != "https://i/big" {
		t.Errorf("hero should take the largest image, got %q", det.ArtURL)
	}
	if len(det.TopTracks) != 1 || det.TopTracks[0].DurationMS != 200000 {
		t.Errorf("top tracks missing duration: %+v", det.TopTracks)
	}
	// "lp" (case-folded) is the same record re-released — one entry per shelf.
	if len(det.Albums) != 1 || det.Albums[0].Year != "2020" || det.Albums[0].TotalTracks != 10 {
		t.Errorf("albums shelf = %+v", det.Albums)
	}
	if len(det.Singles) != 1 || det.Singles[0].Year != "2021" {
		t.Errorf("singles shelf = %+v", det.Singles)
	}
	if len(det.Related) != 0 {
		t.Errorf("a refused related-artists call must cost the section, not the page: %+v", det.Related)
	}
}

// A discography longer than Spotify's new 10-per-page cap arrives over
// several offset walks, and a page that fails mid-walk keeps what arrived
// rather than costing the section.
func TestDiscographyPaginatesByTen(t *testing.T) {
	page := func(items string) string {
		return `{"items": [` + items + `]}`
	}
	entry := func(uri, name, kind string) string {
		return `{"uri": "spotify:album:` + uri + `", "name": "` + name +
			`", "album_type": "` + kind + `", "release_date": "2020", "total_tracks": 9,
			 "artists": [{"name": "A"}], "images": []}`
	}
	var offsets []string
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		if !strings.HasSuffix(r.URL.Path, "/albums") {
			t.Fatalf("unexpected request to %s", r.URL.Path)
		}
		if l := r.URL.Query().Get("limit"); l != "10" {
			t.Errorf("sent limit=%s, want 10", l)
		}
		offsets = append(offsets, r.URL.Query().Get("offset"))
		switch r.URL.Query().Get("offset") {
		case "0":
			ten := make([]string, 0, 10)
			for i := range 10 {
				ten = append(ten, entry(fmt.Sprintf("a%d", i), fmt.Sprintf("Rec %d", i), "album"))
			}
			return jsonResponse(http.StatusOK, page(strings.Join(ten, ",")))
		case "10":
			return jsonResponse(http.StatusOK, page(entry("b1", "Last", "single")+","+entry("b2", "Last LP", "album")))
		default:
			t.Errorf("a short page ends the walk; got offset=%s", r.URL.Query().Get("offset"))
			return jsonResponse(http.StatusOK, `{"items": []}`)
		}
	}))
	c.p.Country = "SE"

	albums, singles, err := c.For("").artistDiscography(context.Background(), "ar1")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 11 || len(singles) != 1 {
		t.Errorf("walked pages should accumulate: %d albums, %d singles", len(albums), len(singles))
	}
	if len(offsets) != 2 || offsets[0] != "0" || offsets[1] != "10" {
		t.Errorf("offsets walked = %v, want [0 10]", offsets)
	}
}

func TestContextDetailEnrichment(t *testing.T) {
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		switch {
		case strings.HasSuffix(r.URL.Path, "/playlists/p1"):
			// The post-February-2026 shape: the listing is "items", and each
			// entry's track is "item".
			if !strings.Contains(r.URL.RawQuery, "duration_ms") {
				t.Errorf("playlist fields must ask for track durations, got %s", r.URL.RawQuery)
			}
			return jsonResponse(http.StatusOK, `{
				"name": "Mix", "description": "For late nights",
				"images": [{"url": "https://i/p"}],
				"owner": {"display_name": "petter"},
				"followers": {"total": 42},
				"items": {"total": 2, "items": [
					{"item": {"uri": "spotify:track:x", "name": "One", "duration_ms": 1000,
						"artists": [{"name": "A"}], "album": {"name": "R1", "images": []}}},
					{"item": null}
				]}
			}`)
		case strings.HasSuffix(r.URL.Path, "/playlists/p2"):
			// Extended-quota apps are still answered in the old shape.
			return jsonResponse(http.StatusOK, `{
				"name": "Old Mix", "images": [], "owner": {"display_name": "petter"},
				"followers": {"total": 1},
				"tracks": {"total": 1, "items": [
					{"track": {"uri": "spotify:track:z", "name": "Two", "duration_ms": 2000,
						"artists": [{"name": "A"}], "album": {"name": "R2", "images": []}}}
				]}
			}`)
		case strings.Contains(r.URL.Path, "/albums/"):
			return jsonResponse(http.StatusOK, `{
				"name": "OK Computer", "release_date": "1997-05-21", "total_tracks": 12,
				"images": [{"url": "https://i/a"}],
				"artists": [{"uri": "spotify:artist:ar1", "name": "Radiohead"}],
				"tracks": {"items": [
					{"uri": "spotify:track:y", "name": "Airbag", "duration_ms": 284000,
						"artists": [{"name": "Radiohead"}]}
				]}
			}`)
		default:
			t.Fatalf("unexpected request to %s", r.URL.Path)
			return nil
		}
	}))

	pl, err := c.Context(context.Background(), "spotify:playlist:p1")
	if err != nil {
		t.Fatal(err)
	}
	if pl.Followers != 42 || pl.TotalTracks != 2 || pl.Description == "" {
		t.Errorf("playlist header = %+v", pl)
	}
	if len(pl.Tracks) != 1 || pl.Tracks[0].DurationMS != 1000 || pl.Tracks[0].Album != "R1" {
		t.Errorf("a null entry is skipped, the real one keeps its meta: %+v", pl.Tracks)
	}

	old, err := c.Context(context.Background(), "spotify:playlist:p2")
	if err != nil {
		t.Fatal(err)
	}
	if len(old.Tracks) != 1 || old.Tracks[0].DurationMS != 2000 || old.TotalTracks != 1 {
		t.Errorf("the pre-rename shape still parses: %+v", old.Tracks)
	}

	al, err := c.Context(context.Background(), "spotify:album:a1")
	if err != nil {
		t.Fatal(err)
	}
	if al.Year != "1997" || al.TotalTracks != 12 || al.ArtistURI != "spotify:artist:ar1" {
		t.Errorf("album header = %+v", al)
	}
	// Simplified album tracks carry no album object; the record's own art
	// stands in so the row never renders blank.
	if len(al.Tracks) != 1 || al.Tracks[0].ArtURL != "https://i/a" || al.Tracks[0].DurationMS != 284000 {
		t.Errorf("album track = %+v", al.Tracks)
	}
}

func TestYearOf(t *testing.T) {
	for in, want := range map[string]string{
		"1997-05-21": "1997",
		"2021":       "2021",
		"2019-11":    "2019",
		"":           "",
		"ab":         "",
	} {
		if got := yearOf(in); got != want {
			t.Errorf("yearOf(%q) = %q, want %q", in, got, want)
		}
	}
}

// A shelf that can't go past its tenth result makes its own count ("Songs
// 10") a number nobody can act on, so "show more" pages with an offset —
// the only way past Spotify's limit cap. One kind at a time, since that is
// what a narrowed shelf is asking for.
func TestSearchPagePagesOneKind(t *testing.T) {
	var sent url.Values
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		sent = r.URL.Query()
		return jsonResponse(http.StatusOK, `{}`)
	}))

	if _, err := c.For("").SearchPage(context.Background(), "adele", "tracks", 10, 10); err != nil {
		t.Fatal(err)
	}
	if got := sent.Get("type"); got != "track" {
		t.Errorf("type = %q, want the one kind asked for", got)
	}
	if got := sent.Get("offset"); got != "10" {
		t.Errorf("offset = %q, want 10", got)
	}
}

// An unknown kind is not a failure: a caller asking for something this
// version doesn't have gets the broad search rather than an error, and a
// zero offset is left off the wire entirely.
func TestSearchPageFallsBackToTheBroadSearch(t *testing.T) {
	var sent url.Values
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		sent = r.URL.Query()
		return jsonResponse(http.StatusOK, `{}`)
	}))

	if _, err := c.For("").SearchPage(context.Background(), "adele", "podcasts", 10, 0); err != nil {
		t.Fatal(err)
	}
	if got := sent.Get("type"); got != "track,album,playlist,artist" {
		t.Errorf("type = %q, want every kind", got)
	}
	if _, ok := sent["offset"]; ok {
		t.Errorf("offset was sent for the first page: %q", sent.Get("offset"))
	}
}

// Spotify refuses offset+limit past 1000. A caller paging forever must hit
// the end of the list, not a 400.
func TestSearchPageClampsTheDeepEnd(t *testing.T) {
	var sent url.Values
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		sent = r.URL.Query()
		return jsonResponse(http.StatusOK, `{}`)
	}))

	if _, err := c.For("").SearchPage(context.Background(), "adele", "tracks", 10, 5000); err != nil {
		t.Fatal(err)
	}
	if got := sent.Get("offset"); got != "990" {
		t.Errorf("offset = %q, want it clamped to 990", got)
	}
}

// The listening shelves exist so that putting music on doesn't have to
// begin with typing. Both are gated on scopes a login made by an older
// build never asked for — and since a grant can't be widened by refreshing,
// the answer has to be "reconnect", not Spotify's 403.
func TestListeningNeedsItsOwnGrant(t *testing.T) {
	c := connected(t, "", roundTripFunc(func(*http.Request) *http.Response {
		t.Error("a request went out on a grant that doesn't carry the scope")
		return jsonResponse(http.StatusOK, `{}`)
	}))
	// connected() stores whatever scope the token response carried; an old
	// login carries none of these.
	if _, err := c.For("").RecentTracks(context.Background(), 10); !errors.Is(err, ErrListeningScope) {
		t.Errorf("RecentTracks on an old grant = %v, want ErrListeningScope", err)
	}
	if _, err := c.For("").TopTracks(context.Background(), 10); !errors.Is(err, ErrListeningScope) {
		t.Errorf("TopTracks on an old grant = %v, want ErrListeningScope", err)
	}
	if st := c.For("").Status(); st.Listening {
		t.Error("Status claims listening on a grant without the scopes")
	}
}

// Spotify's history is one entry per *play*, so a song left on repeat comes
// back five times. A shelf of the same track five times is not a shelf.
func TestRecentTracksDropsRepeats(t *testing.T) {
	c := connected(t, scopeTop+" "+scopeRecent, roundTripFunc(func(r *http.Request) *http.Response {
		if got := r.URL.Path; got != "/v1/me/player/recently-played" {
			t.Fatalf("unexpected path %s", got)
		}
		return jsonResponse(http.StatusOK, `{"items":[
			{"track":{"uri":"spotify:track:a","name":"A"}},
			{"track":{"uri":"spotify:track:a","name":"A"}},
			{"track":{"uri":"spotify:track:b","name":"B"}},
			{"track":{"uri":"spotify:track:a","name":"A"}}
		]}`)
	}))

	got, err := c.For("").RecentTracks(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "A" || got[1].Name != "B" {
		t.Errorf("recent = %+v, want one A then one B, newest first", got)
	}
}

// "Most played" is meant to answer "put something on" today, so it asks for
// the recent window rather than a lifetime ranking that barely moves.
func TestTopTracksAsksForTheRecentWindow(t *testing.T) {
	var sent url.Values
	c := connected(t, scopeTop+" "+scopeRecent, roundTripFunc(func(r *http.Request) *http.Response {
		sent = r.URL.Query()
		return jsonResponse(http.StatusOK, `{"items":[{"uri":"spotify:track:a","name":"A"}]}`)
	}))

	if _, err := c.For("").TopTracks(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if got := sent.Get("time_range"); got != "short_term" {
		t.Errorf("time_range = %q, want short_term", got)
	}
}

// Saving is the one library call split across two grants: reading whether a
// track is saved has always been in the scope set, writing was added later.
// An old login must therefore still answer the heart's *state* and refuse
// only the tap.
func TestSavedReadsOnAnyGrantAndWritesOnlyOnTheNewOne(t *testing.T) {
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/me/tracks/contains" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("ids"); got != "abc" {
			t.Errorf("ids = %q, want the bare id", got)
		}
		return jsonResponse(http.StatusOK, `[true]`)
	}))
	saved, err := c.For("").IsSaved(context.Background(), "spotify:track:abc")
	if err != nil || !saved {
		t.Errorf("IsSaved on an old grant = %v, %v; want true, nil", saved, err)
	}
	if err := c.For("").SetSaved(context.Background(), "spotify:track:abc", true); !errors.Is(err, ErrLibraryScope) {
		t.Errorf("SetSaved on an old grant = %v, want ErrLibraryScope", err)
	}
	if st := c.For("").Status(); st.Library {
		t.Error("Status claims library writes on a grant without the scope")
	}
}

func TestSetSavedSendsTheRightVerb(t *testing.T) {
	var seen []string
	c := connected(t, scopeLibraryWrite, roundTripFunc(func(r *http.Request) *http.Response {
		seen = append(seen, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		return jsonResponse(http.StatusOK, ``)
	}))
	if err := c.For("").SetSaved(context.Background(), "spotify:track:abc", true); err != nil {
		t.Fatal(err)
	}
	if err := c.For("").SetSaved(context.Background(), "spotify:track:abc", false); err != nil {
		t.Fatal(err)
	}
	want := []string{"PUT /v1/me/tracks?ids=abc", "DELETE /v1/me/tracks?ids=abc"}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Errorf("calls = %v, want %v", seen, want)
	}
}

// A URI the library cannot hold never reaches Spotify. Tracks and albums are
// the two collections it has (see savedPath); following a playlist or an
// artist is a different API with a different scope, and sending one here
// would save nothing and report success.
func TestSavedRejectsURIsTheLibraryCannotHold(t *testing.T) {
	c := connected(t, scopeLibraryWrite, roundTripFunc(func(*http.Request) *http.Response {
		t.Error("an unsavable URI reached the service")
		return jsonResponse(http.StatusOK, `[]`)
	}))
	for _, uri := range []string{"spotify:playlist:x", "spotify:artist:x", "spotify:track:", "", "nonsense"} {
		if _, err := c.For("").IsSaved(context.Background(), uri); err == nil {
			t.Errorf("IsSaved(%q) = nil, want error", uri)
		}
		if err := c.For("").SetSaved(context.Background(), uri, true); err == nil {
			t.Errorf("SetSaved(%q) = nil, want error", uri)
		}
	}
}
