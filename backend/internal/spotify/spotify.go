// Package spotify provides catalog search and playlist browsing against the
// Spotify Web API, authorized with the user's own account via the OAuth
// Authorization Code + PKCE flow (no client secret — only a client ID from a
// free Spotify developer app).
//
// Search results are turned into speaker-playable items by the sonos package:
// the speakers stream Spotify themselves through the account linked to the
// Sonos household, so Spotify's cloud is only used to *find* music, never to
// route audio.
//
// KEF speakers are the exception, and the Connect section at the bottom of
// this file exists for them: their local API has transport control but no way
// to hand it something to play, so starting music there means asking Spotify
// to point playback at the speaker's Spotify Connect endpoint. That path does
// route audio through Spotify's cloud — the speaker streams it directly, but
// the *command* goes out and back rather than staying on the LAN.
//
// Tokens persist in spotify.json in the data dir. Like push subscriptions,
// they are credentials — deliberately excluded from the export bundle.
package spotify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	authorizeURL = "https://accounts.spotify.com/authorize"
	tokenURL     = "https://accounts.spotify.com/api/token"
	apiBase      = "https://api.spotify.com/v1"

	stateFile = "spotify.json"

	// Scopes: profile for the "connected as" label, playlist/library reads
	// for browsing, and the two player scopes that Connect playback needs
	// (KEF speakers — see the Connect section). Search itself needs no scope.
	scopes = "user-read-private playlist-read-private user-library-read " +
		scopeReadPlayback + " " + scopeModifyPlayback

	scopeReadPlayback   = "user-read-playback-state"
	scopeModifyPlayback = "user-modify-playback-state"
)

// ErrNotConnected is returned by every call that needs an account when none
// is linked. The API layer maps it to 409 so the frontend can prompt a login.
var ErrNotConnected = errors.New("spotify: not connected")

// ErrPlaybackScope means the stored login was granted before the player
// scopes were asked for — everything else still works, but Connect playback
// needs the user to reconnect once. Refreshing can't widen a grant, so this
// is not something the backend can fix on its own.
var ErrPlaybackScope = errors.New("spotify: reconnect Spotify to let HomeHub start playback — this login was granted before that permission existed")

// persisted is the on-disk shape. Everything in here survives restarts.
type persisted struct {
	ClientID     string    `json:"client_id,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	DisplayName  string    `json:"display_name,omitempty"`
	// Scope is what Spotify actually granted, as it reports it on every
	// token response. Stored because a grant can be narrower than what we
	// asked for — and because a login made by an older build of HomeHub
	// predates the player scopes entirely, which is worth saying out loud
	// rather than discovering as a 403 mid-tap.
	Scope string `json:"scope,omitempty"`
	// Country is the account's Spotify market, read from /me at connect
	// time. Several catalog endpoints (artist top tracks, artist albums)
	// silently return an empty list rather than an error when a request
	// carries no market — passing this is the difference between an
	// artist page with content and one that's mysteriously empty.
	Country string `json:"country,omitempty"`
}

// pendingAuth is one in-flight PKCE authorization, keyed by state. The
// redirect URI is captured at start so the token exchange always uses
// exactly what the authorize request carried — regardless of which path
// (automatic callback or pasted URL) finishes the flow.
type pendingAuth struct {
	verifier    string
	redirectURI string
	expires     time.Time
}

// Client is the Spotify Web API client. Safe for concurrent use.
type Client struct {
	mu      sync.Mutex
	dataDir string
	p       persisted
	pending map[string]pendingAuth

	// HTTP is swappable for tests; defaults to http.DefaultClient.
	HTTP *http.Client
}

// New loads any persisted credentials and returns a ready client.
func New(dataDir string) (*Client, error) {
	c := &Client{dataDir: dataDir, pending: make(map[string]pendingAuth)}
	raw, err := os.ReadFile(filepath.Join(dataDir, stateFile))
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, fmt.Errorf("spotify: load state: %w", err)
	}
	if err := json.Unmarshal(raw, &c.p); err != nil {
		return nil, fmt.Errorf("spotify: parse state: %w", err)
	}
	return c, nil
}

// save persists credentials. Caller must hold mu. 0600 — it holds tokens.
func (c *Client) save() error {
	raw, err := json.MarshalIndent(c.p, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(c.dataDir, stateFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// Status describes the connection for the frontend.
type Status struct {
	Configured  bool   `json:"configured"` // client ID set
	Connected   bool   `json:"connected"`  // tokens present
	DisplayName string `json:"display_name,omitempty"`
	// Playback reports whether this login can start Connect playback. False
	// on a login made before the player scopes were requested, which is the
	// one thing about the connection that a working search doesn't prove.
	Playback bool `json:"playback"`
}

// Status returns the current connection state.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Status{
		Configured:  c.p.ClientID != "",
		Connected:   c.p.RefreshToken != "",
		DisplayName: c.p.DisplayName,
		Playback:    c.p.RefreshToken != "" && grantsPlayback(c.p.Scope),
	}
}

// grantsPlayback reports whether a granted-scope string carries both player
// scopes. An empty scope means the grant was stored by a build that didn't
// record it, which in practice is a build that never asked for them.
func grantsPlayback(scope string) bool {
	has := func(want string) bool {
		for _, s := range strings.Fields(scope) {
			if s == want {
				return true
			}
		}
		return false
	}
	return has(scopeReadPlayback) && has(scopeModifyPlayback)
}

// SetClientID stores the developer app's client ID. Changing it invalidates
// any existing tokens (they belong to the old app).
func (c *Client) SetClientID(id string) error {
	id = strings.TrimSpace(id)
	c.mu.Lock()
	defer c.mu.Unlock()
	if id != c.p.ClientID {
		c.p = persisted{ClientID: id}
	}
	return c.save()
}

// Disconnect drops the tokens but keeps the client ID.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.p = persisted{ClientID: c.p.ClientID}
	return c.save()
}

// AuthURL starts a PKCE authorization: it returns the Spotify authorize URL
// to send the browser to. The generated state/verifier pair is held for ten
// minutes for HandleCallback to consume.
func (c *Client) AuthURL(redirectURI string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.p.ClientID == "" {
		return "", errors.New("spotify: no client ID configured")
	}
	verifier, err := randomString(64)
	if err != nil {
		return "", err
	}
	state, err := randomString(32)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	// Prune expired entries while we're here.
	now := time.Now()
	for k, v := range c.pending {
		if now.After(v.expires) {
			delete(c.pending, k)
		}
	}
	c.pending[state] = pendingAuth{verifier: verifier, redirectURI: redirectURI, expires: now.Add(10 * time.Minute)}

	q := url.Values{
		"client_id":             {c.p.ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"scope":                 {scopes},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
	}
	return authorizeURL + "?" + q.Encode(), nil
}

// HandleCallback finishes the PKCE flow: verifies state, exchanges the code
// for tokens, fetches the profile for the "connected as" label, persists.
// The redirect URI stored when the flow started is used for the exchange.
func (c *Client) HandleCallback(ctx context.Context, code, state string) error {
	c.mu.Lock()
	pa, ok := c.pending[state]
	if ok {
		delete(c.pending, state)
	}
	clientID := c.p.ClientID
	c.mu.Unlock()
	if !ok || time.Now().After(pa.expires) {
		return errors.New("spotify: login expired or was not started here — try again")
	}

	tok, err := c.tokenRequest(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {pa.redirectURI},
		"client_id":     {clientID},
		"code_verifier": {pa.verifier},
	})
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.p.AccessToken = tok.AccessToken
	c.p.RefreshToken = tok.RefreshToken
	c.p.Scope = tok.Scope
	c.p.Expiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	saveErr := c.save()
	c.mu.Unlock()
	if saveErr != nil {
		return saveErr
	}

	// Best-effort profile fetch; a failure leaves the label empty.
	var me struct {
		DisplayName string `json:"display_name"`
		ID          string `json:"id"`
		Country     string `json:"country"`
	}
	if err := c.apiGet(ctx, "/me", nil, &me); err == nil {
		c.mu.Lock()
		c.p.DisplayName = me.DisplayName
		if c.p.DisplayName == "" {
			c.p.DisplayName = me.ID
		}
		c.p.Country = me.Country
		_ = c.save()
		c.mu.Unlock()
	}
	return nil
}

// ExchangeRedirect finishes the flow from a pasted redirect URL — the
// fallback when HomeHub is served over plain HTTP and the redirect URI is
// a parked loopback address the browser can't load. The user copies the
// address Spotify sent them to and pastes it back; the code and state are
// in its query string.
func (c *Client) ExchangeRedirect(ctx context.Context, rawURL string) error {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return errors.New("spotify: paste the full address from the browser's address bar")
	}
	// Accept a bare query string too ("?code=…" or "code=…").
	if !strings.Contains(raw, "://") {
		raw = "http://127.0.0.1/?" + strings.TrimPrefix(strings.TrimPrefix(raw, "?"), "&")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("spotify: that doesn't look like a web address — paste the full address from the browser's address bar")
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		return fmt.Errorf("spotify: login was refused (%s)", e)
	}
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		return errors.New("spotify: no login code in that address — paste the full address, including everything after the question mark")
	}
	return c.HandleCallback(ctx, code, state)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func (c *Client) tokenRequest(ctx context.Context, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: token request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var e struct {
			Error     string `json:"error"`
			ErrorDesc string `json:"error_description"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.ErrorDesc != "" {
			return nil, fmt.Errorf("spotify: %s", e.ErrorDesc)
		}
		return nil, fmt.Errorf("spotify: token request failed (HTTP %d)", resp.StatusCode)
	}
	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("spotify: parse token response: %w", err)
	}
	return &tok, nil
}

// accessToken returns a valid access token, refreshing when necessary.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.p.RefreshToken == "" {
		c.mu.Unlock()
		return "", ErrNotConnected
	}
	if c.p.AccessToken != "" && time.Until(c.p.Expiry) > 30*time.Second {
		tok := c.p.AccessToken
		c.mu.Unlock()
		return tok, nil
	}
	refresh := c.p.RefreshToken
	clientID := c.p.ClientID
	c.mu.Unlock()

	tok, err := c.tokenRequest(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
		"client_id":     {clientID},
	})
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.p.AccessToken = tok.AccessToken
	// PKCE refreshes rotate the refresh token; keep the old one if the
	// response omitted it.
	if tok.RefreshToken != "" {
		c.p.RefreshToken = tok.RefreshToken
	}
	// A refresh reports the grant too, which is how a login stored by an
	// older build (no recorded scope) learns what it actually has.
	if tok.Scope != "" {
		c.p.Scope = tok.Scope
	}
	c.p.Expiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	_ = c.save()
	token := c.p.AccessToken
	c.mu.Unlock()
	return token, nil
}

// ensureCountry backfills c.p.Country for a login stored before it was
// recorded (HandleCallback now captures it up front, but an account
// connected by an older build never ran that code). Best-effort: a
// failure just leaves market lookups empty, same as before this existed.
func (c *Client) ensureCountry(ctx context.Context) {
	c.mu.Lock()
	known := c.p.Country != ""
	c.mu.Unlock()
	if known {
		return
	}
	var me struct {
		Country string `json:"country"`
	}
	if err := c.apiGet(ctx, "/me", nil, &me); err != nil || me.Country == "" {
		return
	}
	c.mu.Lock()
	c.p.Country = me.Country
	_ = c.save()
	c.mu.Unlock()
}

// market returns a "market" query value for the connected account's
// country, or nil if it isn't known yet (a login stored by an older
// build, before this was recorded). Several catalog endpoints treat a
// request with no market as scoped to no market at all and answer with
// an empty list rather than an error, so this is worth sending whenever
// we have it even though Spotify's docs call the parameter optional.
func (c *Client) market() url.Values {
	c.mu.Lock()
	country := c.p.Country
	c.mu.Unlock()
	if country == "" {
		return nil
	}
	return url.Values{"market": {country}}
}

// apiGet performs an authenticated GET against the Web API.
func (c *Client) apiGet(ctx context.Context, path string, q url.Values, out interface{}) error {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	u := apiBase + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("spotify: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		return apiError(resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, out)
}

// apiError turns an error response into a message worth showing. Spotify puts
// a human-readable reason in the body for most failures; the status code alone
// is the fallback.
func apiError(status int, raw []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	if e.Error.Message != "" {
		return fmt.Errorf("spotify: %s", e.Error.Message)
	}
	return fmt.Errorf("spotify: HTTP %d", status)
}

// Item is one playable search/browse result, flattened for the frontend.
// URI is the canonical Spotify URI (spotify:track:… / spotify:album:… /
// spotify:playlist:… / spotify:artist:…) that the Sonos mapping consumes.
//
// The rest is whatever the endpoint it came from answered for, so a row can
// be as informative as Spotify's own: a track carries its album and length,
// an album its year and track count, an artist its following and genres.
// Absent means "the service didn't say", and the UI drops the field rather
// than fabricating it.
type Item struct {
	Kind   string `json:"kind"` // track | album | playlist | artist
	URI    string `json:"uri"`
	Name   string `json:"name"`
	Sub    string `json:"sub,omitempty"`     // artist / owner line
	ArtURL string `json:"art_url,omitempty"` // https CDN image

	Album       string   `json:"album,omitempty"`        // tracks: the record it sits on
	DurationMS  int      `json:"duration_ms,omitempty"`  // tracks
	Explicit    bool     `json:"explicit,omitempty"`     // tracks
	Year        string   `json:"year,omitempty"`         // albums: release year
	TotalTracks int      `json:"total_tracks,omitempty"` // albums + playlists
	Followers   int      `json:"followers,omitempty"`    // artists + playlists
	Genres      []string `json:"genres,omitempty"`       // artists
	Popularity  int      `json:"popularity,omitempty"`   // artists, 0–100
}

// Results groups items by kind, in Spotify's relevance order.
type Results struct {
	Tracks    []Item `json:"tracks"`
	Albums    []Item `json:"albums"`
	Playlists []Item `json:"playlists"`
	Artists   []Item `json:"artists"`
}

// Raw wire shapes — only what we read.
type wireImage struct {
	URL string `json:"url"`
}
type wireArtist struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}
type wireArtistFull struct {
	URI        string      `json:"uri"`
	Name       string      `json:"name"`
	Images     []wireImage `json:"images"`
	Genres     []string    `json:"genres"`
	Popularity int         `json:"popularity"`
	Followers  struct {
		Total int `json:"total"`
	} `json:"followers"`
}
type wireTrack struct {
	URI        string       `json:"uri"`
	Name       string       `json:"name"`
	DurationMS int          `json:"duration_ms"`
	Explicit   bool         `json:"explicit"`
	Artists    []wireArtist `json:"artists"`
	Album      struct {
		Name   string      `json:"name"`
		Images []wireImage `json:"images"`
	} `json:"album"`
}
type wireAlbum struct {
	URI         string       `json:"uri"`
	Name        string       `json:"name"`
	AlbumType   string       `json:"album_type"` // album | single | compilation
	ReleaseDate string       `json:"release_date"`
	TotalTracks int          `json:"total_tracks"`
	Artists     []wireArtist `json:"artists"`
	Images      []wireImage  `json:"images"`
}
type wirePlaylist struct {
	URI       string      `json:"uri"`
	Name      string      `json:"name"`
	Images    []wireImage `json:"images"`
	Followers struct {
		Total int `json:"total"`
	} `json:"followers"`
	Tracks struct {
		Total int `json:"total"`
	} `json:"tracks"`
	Owner struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
}

func artOf(images []wireImage) string {
	// Spotify orders images largest-first; the last is the smallest.
	// Middle sizes (~300px) suit our tiles; fall back to whatever exists.
	if len(images) == 0 {
		return ""
	}
	return images[len(images)/2].URL
}

// artOfLarge is for hero surfaces (artist page, album header), where the
// biggest image Spotify sent is the one worth painting full-bleed.
func artOfLarge(images []wireImage) string {
	if len(images) == 0 {
		return ""
	}
	return images[0].URL
}

// yearOf keeps just the year of a release date — Spotify answers
// "2019-11-29", "2019-11" or "2019" depending on what the label gave it.
func yearOf(releaseDate string) string {
	if len(releaseDate) < 4 {
		return ""
	}
	return releaseDate[:4]
}

// trackItem flattens a full track object once — every surface that lists
// tracks (search, top tracks, a playlist's own listing) gets the same fields.
func trackItem(t wireTrack) Item {
	return Item{
		Kind: "track", URI: t.URI, Name: t.Name,
		Sub: artistLine(t.Artists), ArtURL: artOf(t.Album.Images),
		Album: t.Album.Name, DurationMS: t.DurationMS, Explicit: t.Explicit,
	}
}

// albumItem flattens an album listing entry: its year and size are what tell
// a reissue from the original in a discography.
func albumItem(a wireAlbum) Item {
	return Item{
		Kind: "album", URI: a.URI, Name: a.Name,
		Sub: artistLine(a.Artists), ArtURL: artOf(a.Images),
		Year: yearOf(a.ReleaseDate), TotalTracks: a.TotalTracks,
	}
}

// artistItem flattens a full artist object — search results and related
// artists both arrive in this shape.
func artistItem(a wireArtistFull) Item {
	return Item{
		Kind: "artist", URI: a.URI, Name: a.Name, ArtURL: artOf(a.Images),
		Followers: a.Followers.Total, Genres: a.Genres, Popularity: a.Popularity,
	}
}

func artistLine(artists []wireArtist) string {
	names := make([]string, 0, len(artists))
	for _, a := range artists {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

// Search queries the catalog for tracks, albums, playlists and artists.
func (c *Client) Search(ctx context.Context, query string, limit int) (*Results, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	var raw struct {
		Tracks struct {
			Items []wireTrack `json:"items"`
		} `json:"tracks"`
		Albums struct {
			Items []wireAlbum `json:"items"`
		} `json:"albums"`
		Playlists struct {
			Items []*wirePlaylist `json:"items"` // entries can be null
		} `json:"playlists"`
		Artists struct {
			Items []wireArtistFull `json:"items"`
		} `json:"artists"`
	}
	err := c.apiGet(ctx, "/search", url.Values{
		"q":     {query},
		"type":  {"track,album,playlist,artist"},
		"limit": {fmt.Sprint(limit)},
	}, &raw)
	if err != nil {
		return nil, err
	}
	res := &Results{Tracks: []Item{}, Albums: []Item{}, Playlists: []Item{}, Artists: []Item{}}
	for _, t := range raw.Tracks.Items {
		res.Tracks = append(res.Tracks, trackItem(t))
	}
	for _, a := range raw.Albums.Items {
		res.Albums = append(res.Albums, albumItem(a))
	}
	for _, p := range raw.Playlists.Items {
		if p == nil {
			continue
		}
		res.Playlists = append(res.Playlists, Item{
			Kind: "playlist", URI: p.URI, Name: p.Name,
			Sub: p.Owner.DisplayName, ArtURL: artOf(p.Images),
			TotalTracks: p.Tracks.Total,
		})
	}
	for _, a := range raw.Artists.Items {
		res.Artists = append(res.Artists, artistItem(a))
	}
	return res, nil
}

// artistIDFromURI pulls the bare id out of a canonical artist URI.
func artistIDFromURI(uri string) (string, error) {
	const prefix = "spotify:artist:"
	if !strings.HasPrefix(uri, prefix) {
		return "", fmt.Errorf("spotify: %q is not an artist URI", uri)
	}
	return strings.TrimPrefix(uri, prefix), nil
}

// ArtistDetail is an artist's page, as informative as Spotify's own: the
// following and genres up top, the most-played tracks, the discography split
// the way Spotify splits it, and the artists their listeners also play.
type ArtistDetail struct {
	URI        string   `json:"uri"`
	Name       string   `json:"name"`
	ArtURL     string   `json:"art_url,omitempty"`
	Genres     []string `json:"genres,omitempty"`
	Followers  int      `json:"followers,omitempty"`
	Popularity int      `json:"popularity,omitempty"` // 0–100
	TopTracks  []Item   `json:"top_tracks"`
	Albums     []Item   `json:"albums"`
	Singles    []Item   `json:"singles"`
	Related    []Item   `json:"related"`
}

// Artist fetches one artist's page. Its sections degrade independently — an
// artist with no answer for one still has a name, a picture and the other
// sections worth showing, the same "render only what the service answered
// for" discipline the Sonos side follows for its own capability probes.
func (c *Client) Artist(ctx context.Context, uri string) (*ArtistDetail, error) {
	id, err := artistIDFromURI(uri)
	if err != nil {
		return nil, err
	}
	var raw wireArtistFull
	if err := c.apiGet(ctx, "/artists/"+url.PathEscape(id), nil, &raw); err != nil {
		return nil, err
	}
	c.ensureCountry(ctx)
	det := &ArtistDetail{
		URI: uri, Name: raw.Name, ArtURL: artOfLarge(raw.Images),
		Genres: raw.Genres, Followers: raw.Followers.Total, Popularity: raw.Popularity,
		TopTracks: []Item{}, Albums: []Item{}, Singles: []Item{}, Related: []Item{},
	}
	// The three section reads run together — the page is only as fast as its
	// slowest section either way — and each keeps its own failure, so a
	// refused one costs a section, never the page.
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		if tt, err := c.artistTopTracks(ctx, id); err == nil {
			det.TopTracks = tt
		}
	}()
	go func() {
		defer wg.Done()
		if al, si, err := c.artistDiscography(ctx, id); err == nil {
			det.Albums, det.Singles = al, si
		}
	}()
	go func() {
		defer wg.Done()
		if rel, err := c.relatedArtists(ctx, id); err == nil {
			det.Related = rel
		}
	}()
	wg.Wait()
	return det, nil
}

func (c *Client) artistTopTracks(ctx context.Context, id string) ([]Item, error) {
	var raw struct {
		Tracks []wireTrack `json:"tracks"`
	}
	if err := c.apiGet(ctx, "/artists/"+url.PathEscape(id)+"/top-tracks", c.market(), &raw); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(raw.Tracks))
	for _, t := range raw.Tracks {
		out = append(out, trackItem(t))
	}
	return out, nil
}

// artistDiscography lists the artist's records, split the way Spotify splits
// them: full albums on one shelf, singles and EPs on another. Spotify lists
// regional re-releases as separate entries with the same name; one is enough,
// so later duplicates by name are dropped — per shelf, since a single and
// the album it was lifted from legitimately share one.
func (c *Client) artistDiscography(ctx context.Context, id string) (albums, singles []Item, err error) {
	var raw struct {
		Items []wireAlbum `json:"items"`
	}
	q := url.Values{
		"include_groups": {"album,single"},
		"limit":          {"50"},
	}
	if m := c.market(); m != nil {
		q.Set("market", m.Get("market"))
	}
	if err := c.apiGet(ctx, "/artists/"+url.PathEscape(id)+"/albums", q, &raw); err != nil {
		return nil, nil, err
	}
	albums, singles = []Item{}, []Item{}
	seen := make(map[string]bool, len(raw.Items))
	for _, a := range raw.Items {
		key := a.AlbumType + ":" + strings.ToLower(strings.TrimSpace(a.Name))
		if seen[key] {
			continue
		}
		seen[key] = true
		if a.AlbumType == "single" {
			singles = append(singles, albumItem(a))
		} else {
			albums = append(albums, albumItem(a))
		}
	}
	return albums, singles, nil
}

// relatedArtists answers "fans also like". Spotify retired the endpoint for
// apps created after November 2024 (they are answered with a 403), so a
// refusal costs the section rather than the page — the caller keeps the
// error to itself and renders without it.
func (c *Client) relatedArtists(ctx context.Context, id string) ([]Item, error) {
	var raw struct {
		Artists []wireArtistFull `json:"artists"`
	}
	if err := c.apiGet(ctx, "/artists/"+url.PathEscape(id)+"/related-artists", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(raw.Artists))
	for _, a := range raw.Artists {
		out = append(out, artistItem(a))
	}
	return out, nil
}

// SimilarTracks finds tracks to continue with once a group's queue runs out
// — same-artist top tracks, seeded from whatever is currently playing.
// Spotify retired the recommendations and related-artists endpoints for apps
// created after November 2024, so this integration can't assume access to
// either; same-artist top tracks stay available to every app and are an
// honest (if narrower) answer to "more like this". exclude drops URIs
// queued recently, so a short discography doesn't loop the same handful of
// songs every time the queue runs out.
func (c *Client) SimilarTracks(ctx context.Context, artistName string, exclude map[string]bool, limit int) ([]Item, error) {
	artistName = strings.TrimSpace(artistName)
	if artistName == "" {
		return nil, errors.New("spotify: no artist to seed similar tracks from")
	}
	id, err := c.searchArtistID(ctx, artistName)
	if err != nil {
		return nil, err
	}
	tracks, err := c.artistTopTracks(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, limit)
	for _, t := range tracks {
		if exclude[t.URI] {
			continue
		}
		out = append(out, t)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (c *Client) searchArtistID(ctx context.Context, name string) (string, error) {
	var raw struct {
		Artists struct {
			Items []struct {
				URI string `json:"uri"`
			} `json:"items"`
		} `json:"artists"`
	}
	err := c.apiGet(ctx, "/search", url.Values{
		"q": {name}, "type": {"artist"}, "limit": {"1"},
	}, &raw)
	if err != nil {
		return "", err
	}
	if len(raw.Artists.Items) == 0 {
		return "", fmt.Errorf("spotify: no artist matched %q", name)
	}
	return artistIDFromURI(raw.Artists.Items[0].URI)
}

// ContextDetail is a playlist or album's own track listing — the drill-in
// behind a favorite or a search result that turns out to be a list rather
// than one song (DESIGN.md §15). The header fields are what make the page
// worth opening: whose it is, when it came out, how much is on it.
type ContextDetail struct {
	Kind        string `json:"kind"` // playlist | album
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Sub         string `json:"sub,omitempty"` // album: artists · playlist: owner
	ArtURL      string `json:"art_url,omitempty"`
	Year        string `json:"year,omitempty"`      // album
	Followers   int    `json:"followers,omitempty"` // playlist
	TotalTracks int    `json:"total_tracks,omitempty"`
	Description string `json:"description,omitempty"` // playlist
	ArtistURI   string `json:"artist_uri,omitempty"`  // album: first artist, for "More by"
	Tracks      []Item `json:"tracks"`
}

// Context browses a playlist or album's tracks.
func (c *Client) Context(ctx context.Context, uri string) (*ContextDetail, error) {
	switch {
	case strings.HasPrefix(uri, "spotify:playlist:"):
		return c.playlistContext(ctx, uri)
	case strings.HasPrefix(uri, "spotify:album:"):
		return c.albumContext(ctx, uri)
	default:
		return nil, fmt.Errorf("spotify: %q is not a playlist or album URI", uri)
	}
}

func (c *Client) playlistContext(ctx context.Context, uri string) (*ContextDetail, error) {
	id := strings.TrimPrefix(uri, "spotify:playlist:")
	var raw struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Images      []wireImage `json:"images"`
		Owner       struct {
			DisplayName string `json:"display_name"`
		} `json:"owner"`
		Followers struct {
			Total int `json:"total"`
		} `json:"followers"`
		Tracks struct {
			Total int `json:"total"`
			Items []struct {
				Track *wireTrack `json:"track"` // null for a removed or local track
			} `json:"items"`
		} `json:"tracks"`
	}
	err := c.apiGet(ctx, "/playlists/"+url.PathEscape(id), url.Values{
		"fields": {"name,description,images,owner.display_name,followers.total,tracks.total,tracks.items(track(uri,name,duration_ms,explicit,artists,album(name,images)))"},
	}, &raw)
	if err != nil {
		return nil, err
	}
	det := &ContextDetail{
		Kind: "playlist", URI: uri, Name: raw.Name,
		Sub: raw.Owner.DisplayName, ArtURL: artOfLarge(raw.Images),
		Description: raw.Description, Followers: raw.Followers.Total,
		TotalTracks: raw.Tracks.Total, Tracks: []Item{},
	}
	for _, it := range raw.Tracks.Items {
		if it.Track == nil || it.Track.URI == "" {
			continue
		}
		det.Tracks = append(det.Tracks, trackItem(*it.Track))
	}
	return det, nil
}

func (c *Client) albumContext(ctx context.Context, uri string) (*ContextDetail, error) {
	id := strings.TrimPrefix(uri, "spotify:album:")
	var raw struct {
		Name        string       `json:"name"`
		ReleaseDate string       `json:"release_date"`
		TotalTracks int          `json:"total_tracks"`
		Images      []wireImage  `json:"images"`
		Artists     []wireArtist `json:"artists"`
		Tracks      struct {
			Items []wireTrack `json:"items"`
		} `json:"tracks"`
	}
	if err := c.apiGet(ctx, "/albums/"+url.PathEscape(id), nil, &raw); err != nil {
		return nil, err
	}
	det := &ContextDetail{
		Kind: "album", URI: uri, Name: raw.Name,
		Sub: artistLine(raw.Artists), ArtURL: artOfLarge(raw.Images),
		Year: yearOf(raw.ReleaseDate), TotalTracks: raw.TotalTracks, Tracks: []Item{},
	}
	if len(raw.Artists) > 0 {
		det.ArtistURI = raw.Artists[0].URI
	}
	for _, t := range raw.Tracks.Items {
		if t.URI == "" {
			continue
		}
		// An album's own tracks arrive simplified — no album object — so the
		// record's own art stands in, and the album name is the page's, not
		// the row's.
		it := trackItem(t)
		it.ArtURL = det.ArtURL
		it.Album = ""
		det.Tracks = append(det.Tracks, it)
	}
	return det, nil
}

// MyPlaylists lists the connected account's playlists.
func (c *Client) MyPlaylists(ctx context.Context, limit int) ([]Item, error) {
	if limit <= 0 || limit > 50 {
		limit = 30
	}
	var raw struct {
		Items []*wirePlaylist `json:"items"`
	}
	err := c.apiGet(ctx, "/me/playlists", url.Values{"limit": {fmt.Sprint(limit)}}, &raw)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(raw.Items))
	for _, p := range raw.Items {
		if p == nil {
			continue
		}
		out = append(out, Item{
			Kind: "playlist", URI: p.URI, Name: p.Name,
			Sub: p.Owner.DisplayName, ArtURL: artOf(p.Images),
		})
	}
	return out, nil
}

// ── Spotify Connect ──────────────────────────────────────────────────────
//
// Everything above this line only *finds* music. This part starts it, and it
// exists for one reason: a KEF speaker's local API can play, pause and skip
// but has no way to be handed something to play. Its Spotify Connect endpoint
// does, so HomeHub asks Spotify to point playback at the speaker.
//
// Two consequences worth knowing before building on it. The speaker has to
// have been signed in to this account once from the Spotify app — Connect
// devices are registered account-side, not discovered by us — and the account
// has to be Premium, because that is what the player endpoints require.
// Both failures are reported in those words rather than as an HTTP code.

// Device is one Spotify Connect endpoint the account can play to.
type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // Speaker | Computer | Smartphone | …
	// Active is Spotify's own "this is where playback is right now".
	Active bool `json:"active"`
	// Restricted devices accept no Web API commands at all (some car and
	// TV integrations). Naming one is better than a silent no-op.
	Restricted bool `json:"restricted"`
	Volume     int  `json:"volume,omitempty"`
}

// Devices lists the account's currently visible Connect endpoints. A speaker
// only appears while it is awake and on the network, which is why the KEF
// bridge wakes the speaker before asking.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	if err := c.requirePlayback(); err != nil {
		return nil, err
	}
	var raw struct {
		Devices []struct {
			ID            string `json:"id"`
			IsActive      bool   `json:"is_active"`
			IsRestricted  bool   `json:"is_restricted"`
			Name          string `json:"name"`
			Type          string `json:"type"`
			VolumePercent *int   `json:"volume_percent"`
		} `json:"devices"`
	}
	if err := c.apiGet(ctx, "/me/player/devices", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(raw.Devices))
	for _, d := range raw.Devices {
		dev := Device{
			ID: d.ID, Name: strings.TrimSpace(d.Name), Type: d.Type,
			Active: d.IsActive, Restricted: d.IsRestricted,
		}
		if d.VolumePercent != nil {
			dev.Volume = *d.VolumePercent
		}
		out = append(out, dev)
	}
	return out, nil
}

// PlayOn starts a Spotify URI on one Connect device, moving playback there if
// it was somewhere else. A track plays on its own; an album, playlist or
// artist plays as a context, so the rest of it follows.
func (c *Client) PlayOn(ctx context.Context, deviceID, uri string) error {
	if strings.TrimSpace(deviceID) == "" {
		return errors.New("spotify: no Connect device to play on")
	}
	body, err := playBody(uri)
	if err != nil {
		return err
	}
	if err := c.requirePlayback(); err != nil {
		return err
	}
	// 202 means the device was reachable but not ready yet — a speaker that
	// has just woken says this. One retry is what the difference between
	// "didn't work" and "took a second" costs.
	err = c.apiPut(ctx, "/me/player/play", url.Values{"device_id": {deviceID}}, body)
	if errors.Is(err, errDeviceNotReady) {
		select {
		case <-time.After(700 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		err = c.apiPut(ctx, "/me/player/play", url.Values{"device_id": {deviceID}}, body)
		if errors.Is(err, errDeviceNotReady) {
			return errors.New("spotify: the speaker didn't pick it up — wake it and try again")
		}
	}
	return err
}

// playBody builds the /me/player/play payload for one URI.
func playBody(uri string) ([]byte, error) {
	uri = strings.TrimSpace(uri)
	switch {
	case strings.HasPrefix(uri, "spotify:track:"):
		return json.Marshal(map[string]any{"uris": []string{uri}})
	case strings.HasPrefix(uri, "spotify:album:"),
		strings.HasPrefix(uri, "spotify:playlist:"),
		strings.HasPrefix(uri, "spotify:artist:"):
		return json.Marshal(map[string]string{"context_uri": uri})
	default:
		return nil, fmt.Errorf("spotify: %q is not a playable Spotify URI", uri)
	}
}

// requirePlayback fails early when the stored grant can't reach the player
// endpoints, so the user gets "reconnect" instead of Spotify's 403.
func (c *Client) requirePlayback() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.p.RefreshToken == "" {
		return ErrNotConnected
	}
	if !grantsPlayback(c.p.Scope) {
		return ErrPlaybackScope
	}
	return nil
}

// errDeviceNotReady is the internal marker for Spotify's 202: the command
// arrived, the device hasn't taken it yet.
var errDeviceNotReady = errors.New("spotify: device not ready")

// apiPut performs an authenticated PUT with a JSON body. The player endpoints
// answer 204 with no body on success.
func (c *Client) apiPut(ctx context.Context, path string, q url.Values, body []byte) error {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	u := apiBase + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("spotify: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusAccepted:
		return errDeviceNotReady
	case resp.StatusCode == http.StatusNotFound:
		// The device list is a snapshot; a speaker that went to sleep
		// between listing and playing lands here.
		return errors.New("spotify: that speaker is no longer available to Spotify — wake it and try again")
	case resp.StatusCode == http.StatusForbidden:
		// Premium is the usual reason the player endpoints refuse.
		err := apiError(resp.StatusCode, raw)
		if strings.Contains(strings.ToLower(err.Error()), "premium") {
			return errors.New("spotify: starting playback needs Spotify Premium")
		}
		return err
	case resp.StatusCode >= 400:
		return apiError(resp.StatusCode, raw)
	}
	return nil
}

// randomString returns n bytes of randomness, base64url-encoded (unpadded) —
// valid for both PKCE verifiers (RFC 7636 charset) and state values.
func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
