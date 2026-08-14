package qobuz

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Item is one catalogue entry in the neutral shape the media layer wants.
// Deliberately the same field set as the Spotify client's Item: the bridge
// layer maps both onto media.Item, and two clients disagreeing about what a
// search result looks like would push that difference into every caller.
type Item struct {
	Kind   string // track | album | playlist | artist
	ID     string
	URI    string // qobuz:track:12345
	Name   string
	Sub    string // artist, or "artist · album" for a track
	ArtURL string

	// Duration is in seconds, zero when unknown.
	Duration int
	// BitDepth and SampleRate are the *maximum* the catalogue holds for this
	// item, which is what Qobuz reports on a listing. What actually arrives
	// also depends on the subscription, so these are a ceiling and are
	// reported as one.
	BitDepth   int
	SampleRate int // Hz
	// HiRes is Qobuz's own flag for "better than CD". Kept rather than
	// derived so a listing can be trusted about itself.
	HiRes bool
	// Streamable is false for catalogue entries this account can only
	// purchase. Playing one fails, so the UI needs to know before the tap.
	Streamable bool
}

// Results is a search across the kinds the UI shows.
type Results struct {
	Tracks    []Item
	Albums    []Item
	Playlists []Item
	Artists   []Item
}

// URI builds HomeHub's provider URI for a Qobuz object.
func URI(kind, id string) string { return "qobuz:" + kind + ":" + id }

// ParseURI splits a qobuz: URI back into kind and id.
func ParseURI(uri string) (kind, id string, err error) {
	parts := strings.Split(uri, ":")
	if len(parts) != 3 || parts[0] != "qobuz" || parts[1] == "" || parts[2] == "" {
		return "", "", fmt.Errorf("qobuz: %q isn't a Qobuz URI", uri)
	}
	return parts[1], parts[2], nil
}

// wireImage is Qobuz's image object; the sizes are separate URLs.
type wireImage struct {
	Large     string `json:"large"`
	Small     string `json:"small"`
	Thumbnail string `json:"thumbnail"`
}

func (w wireImage) best() string {
	for _, u := range []string{w.Large, w.Small, w.Thumbnail} {
		if u != "" {
			return u
		}
	}
	return ""
}

// wireArtist appears inside albums and tracks.
type wireArtist struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// wireAlbum is an album as the catalogue returns it.
type wireAlbum struct {
	ID                  string     `json:"id"`
	Title               string     `json:"title"`
	Artist              wireArtist `json:"artist"`
	Image               wireImage  `json:"image"`
	Duration            int        `json:"duration"`
	MaximumBitDepth     int        `json:"maximum_bit_depth"`
	MaximumSamplingRate float64    `json:"maximum_sampling_rate"` // kHz
	HiRes               bool       `json:"hires_streamable"`
	Streamable          bool       `json:"streamable"`
	Tracks              struct {
		Items []wireTrack `json:"items"`
	} `json:"tracks"`
}

func (a wireAlbum) item() Item {
	return Item{
		Kind: "album", ID: a.ID, URI: URI("album", a.ID),
		Name: a.Title, Sub: a.Artist.Name, ArtURL: a.Image.best(),
		Duration: a.Duration, BitDepth: a.MaximumBitDepth,
		SampleRate: khzToHz(a.MaximumSamplingRate),
		HiRes:      a.HiRes, Streamable: a.Streamable,
	}
}

// wireTrack is a track as the catalogue returns it. Inside an album listing
// the album object is omitted, which is why the parent is passed in.
type wireTrack struct {
	ID                  int        `json:"id"`
	Title               string     `json:"title"`
	Duration            int        `json:"duration"`
	TrackNumber         int        `json:"track_number"`
	MaximumBitDepth     int        `json:"maximum_bit_depth"`
	MaximumSamplingRate float64    `json:"maximum_sampling_rate"` // kHz
	HiRes               bool       `json:"hires_streamable"`
	Streamable          bool       `json:"streamable"`
	Performer           wireArtist `json:"performer"`
	Album               *wireAlbum `json:"album"`
}

func (t wireTrack) item(parent *wireAlbum) Item {
	album := t.Album
	if album == nil {
		album = parent
	}
	sub := t.Performer.Name
	if album != nil {
		if sub == "" {
			sub = album.Artist.Name
		}
		if album.Title != "" {
			sub = strings.TrimSpace(sub + " · " + album.Title)
			sub = strings.TrimPrefix(sub, "· ")
		}
	}
	it := Item{
		Kind: "track", ID: strconv.Itoa(t.ID), URI: URI("track", strconv.Itoa(t.ID)),
		Name: t.Title, Sub: sub, Duration: t.Duration,
		BitDepth: t.MaximumBitDepth, SampleRate: khzToHz(t.MaximumSamplingRate),
		HiRes: t.HiRes, Streamable: t.Streamable,
	}
	if album != nil {
		it.ArtURL = album.Image.best()
	}
	return it
}

// khzToHz converts Qobuz's sampling rate, which is quoted in kHz as a decimal
// ("44.1", "96", "192"), into the Hz every other layer speaks.
func khzToHz(khz float64) int {
	if khz <= 0 {
		return 0
	}
	return int(khz*1000 + 0.5)
}

// Search queries the catalogue.
func (c *Client) Search(ctx context.Context, query string, limit int) (*Results, error) {
	if strings.TrimSpace(query) == "" {
		return &Results{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var out struct {
		Albums struct {
			Items []wireAlbum `json:"items"`
		} `json:"albums"`
		Tracks struct {
			Items []wireTrack `json:"items"`
		} `json:"tracks"`
		Artists struct {
			Items []struct {
				ID    int       `json:"id"`
				Name  string    `json:"name"`
				Image wireImage `json:"image"`
			} `json:"items"`
		} `json:"artists"`
		Playlists struct {
			Items []struct {
				ID          int                   `json:"id"`
				Name        string                `json:"name"`
				Owner       struct{ Name string } `json:"owner"`
				Images      []string              `json:"images300"`
				Image       wireImage             `json:"image"`
				TracksCount int                   `json:"tracks_count"`
			} `json:"items"`
		} `json:"playlists"`
	}
	q := url.Values{"query": {query}, "limit": {strconv.Itoa(limit)}}
	if err := c.get(ctx, "/catalog/search", q, false, &out); err != nil {
		return nil, err
	}

	res := &Results{}
	for _, t := range out.Tracks.Items {
		res.Tracks = append(res.Tracks, t.item(nil))
	}
	for _, a := range out.Albums.Items {
		res.Albums = append(res.Albums, a.item())
	}
	for _, a := range out.Artists.Items {
		res.Artists = append(res.Artists, Item{
			Kind: "artist", ID: strconv.Itoa(a.ID), URI: URI("artist", strconv.Itoa(a.ID)),
			Name: a.Name, ArtURL: a.Image.best(), Streamable: true,
		})
	}
	for _, p := range out.Playlists.Items {
		art := p.Image.best()
		if art == "" && len(p.Images) > 0 {
			art = p.Images[0]
		}
		res.Playlists = append(res.Playlists, Item{
			Kind: "playlist", ID: strconv.Itoa(p.ID), URI: URI("playlist", strconv.Itoa(p.ID)),
			Name: p.Name, Sub: p.Owner.Name, ArtURL: art, Streamable: true,
		})
	}
	return res, nil
}

// Favorites is the account's saved albums, for the browse view.
func (c *Client) Favorites(ctx context.Context, limit int) ([]Item, error) {
	if _, _, token := c.creds(); token == "" {
		return nil, ErrNotLoggedIn
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var out struct {
		Albums struct {
			Items []wireAlbum `json:"items"`
		} `json:"albums"`
	}
	q := url.Values{"type": {"albums"}, "limit": {strconv.Itoa(limit)}}
	if err := c.get(ctx, "/favorite/getUserFavorites", q, false, &out); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(out.Albums.Items))
	for _, a := range out.Albums.Items {
		items = append(items, a.item())
	}
	return items, nil
}

// Tracks expands a playable URI into the tracks to play, in order.
//
// A track is itself; an album and a playlist are their listings. This is what
// lets the stream route play more than one thing: HomeHub holds the audio for
// this provider, so unlike a Connect device it has to know what comes next.
func (c *Client) Tracks(ctx context.Context, uri string) ([]Item, error) {
	kind, id, err := ParseURI(uri)
	if err != nil {
		return nil, err
	}
	switch kind {
	case "track":
		it, err := c.Track(ctx, id)
		if err != nil {
			return nil, err
		}
		return []Item{*it}, nil
	case "album":
		return c.AlbumTracks(ctx, id)
	case "playlist":
		return c.PlaylistTracks(ctx, id)
	}
	return nil, fmt.Errorf("qobuz: %s isn't something HomeHub can play", kind)
}

// Track fetches one track's metadata.
func (c *Client) Track(ctx context.Context, id string) (*Item, error) {
	var t wireTrack
	if err := c.get(ctx, "/track/get", url.Values{"track_id": {id}}, false, &t); err != nil {
		return nil, err
	}
	it := t.item(nil)
	return &it, nil
}

// AlbumTracks lists an album in running order.
func (c *Client) AlbumTracks(ctx context.Context, id string) ([]Item, error) {
	var a wireAlbum
	if err := c.get(ctx, "/album/get", url.Values{"album_id": {id}}, false, &a); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(a.Tracks.Items))
	for _, t := range a.Tracks.Items {
		items = append(items, t.item(&a))
	}
	if len(items) == 0 {
		return nil, errors.New("qobuz: that album came back with no tracks")
	}
	return items, nil
}

// PlaylistTracks lists a playlist in order.
func (c *Client) PlaylistTracks(ctx context.Context, id string) ([]Item, error) {
	var out struct {
		Tracks struct {
			Items []wireTrack `json:"items"`
		} `json:"tracks"`
	}
	q := url.Values{"playlist_id": {id}, "extra": {"tracks"}, "limit": {"500"}}
	if err := c.get(ctx, "/playlist/get", q, false, &out); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(out.Tracks.Items))
	for _, t := range out.Tracks.Items {
		items = append(items, t.item(nil))
	}
	if len(items) == 0 {
		return nil, errors.New("qobuz: that playlist came back with no tracks")
	}
	return items, nil
}

// File is a signed, time-limited URL to one track's audio, with the format
// Qobuz actually granted.
//
// The granted format is not always the one asked for: a hi-res request on a
// CD-only subscription, or for a track the catalogue only holds at 16-bit,
// comes back at what exists. That is why this reports back rather than
// echoing the request — every quality claim downstream is built on it.
type File struct {
	URL        string
	FormatID   FormatID
	MimeType   string
	SampleRate int // Hz
	BitDepth   int
	Duration   int
}

// Lossless reports whether this file is FLAC rather than the MP3 fallback.
func (f File) Lossless() bool { return f.FormatID.Lossless() }

// FileURL asks for one track's audio at the best format this account allows.
//
// This is the one signed call in the API. It is also the one that can fail for
// a reason a listener can act on — a track that is purchase-only, or a
// subscription that has lapsed — so those come back as ErrNotStreamable rather
// than as an empty URL nobody checked.
func (c *Client) FileURL(ctx context.Context, trackID string, want FormatID) (*File, error) {
	if _, _, token := c.creds(); token == "" {
		return nil, ErrNotLoggedIn
	}
	if want == 0 {
		want = c.MaxFormat()
	}
	var out struct {
		TrackID      int     `json:"track_id"`
		FormatID     int     `json:"format_id"`
		MimeType     string  `json:"mime_type"`
		URL          string  `json:"url"`
		SamplingRate float64 `json:"sampling_rate"` // kHz
		BitDepth     int     `json:"bit_depth"`
		Duration     int     `json:"duration"`
		Streamable   *bool   `json:"streamable"`
	}
	// intent is part of the signature, so it is set explicitly rather than
	// left to the server's default — an unsigned default would not match.
	q := url.Values{
		"track_id":  {trackID},
		"format_id": {strconv.Itoa(int(want))},
		"intent":    {"stream"},
	}
	if err := c.get(ctx, "/track/getFileUrl", q, true, &out); err != nil {
		return nil, err
	}
	if out.URL == "" || (out.Streamable != nil && !*out.Streamable) {
		return nil, ErrNotStreamable
	}
	return &File{
		URL:        out.URL,
		FormatID:   FormatID(out.FormatID),
		MimeType:   out.MimeType,
		SampleRate: khzToHz(out.SamplingRate),
		BitDepth:   out.BitDepth,
		Duration:   out.Duration,
	}, nil
}
