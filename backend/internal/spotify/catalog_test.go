package spotify

import (
	"context"
	"fmt"
	"net/http"
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

	albums, singles, err := c.artistDiscography(context.Background(), "ar1")
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
