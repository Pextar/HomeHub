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
// Tokens persist in spotify.json in the data dir. Like push subscriptions,
// they are credentials — deliberately excluded from the export bundle.
package spotify

import (
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
	// for browsing. Search itself needs no scope.
	scopes = "user-read-private playlist-read-private user-library-read"
)

// persisted is the on-disk shape. Everything in here survives restarts.
type persisted struct {
	ClientID     string    `json:"client_id,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	DisplayName  string    `json:"display_name,omitempty"`
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
}

// Status returns the current connection state.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Status{
		Configured:  c.p.ClientID != "",
		Connected:   c.p.RefreshToken != "",
		DisplayName: c.p.DisplayName,
	}
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
	}
	if err := c.apiGet(ctx, "/me", nil, &me); err == nil {
		c.mu.Lock()
		c.p.DisplayName = me.DisplayName
		if c.p.DisplayName == "" {
			c.p.DisplayName = me.ID
		}
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
		return "", errors.New("spotify: not connected")
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
	c.p.Expiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	_ = c.save()
	token := c.p.AccessToken
	c.mu.Unlock()
	return token, nil
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
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error.Message != "" {
			return fmt.Errorf("spotify: %s", e.Error.Message)
		}
		return fmt.Errorf("spotify: HTTP %d", resp.StatusCode)
	}
	return json.Unmarshal(raw, out)
}

// Item is one playable search/browse result, flattened for the frontend.
// URI is the canonical Spotify URI (spotify:track:… / spotify:album:… /
// spotify:playlist:…) that the Sonos mapping consumes.
type Item struct {
	Kind   string `json:"kind"` // track | album | playlist
	URI    string `json:"uri"`
	Name   string `json:"name"`
	Sub    string `json:"sub,omitempty"`     // artist / owner line
	ArtURL string `json:"art_url,omitempty"` // https CDN image
}

// Results groups items by kind, in Spotify's relevance order.
type Results struct {
	Tracks    []Item `json:"tracks"`
	Albums    []Item `json:"albums"`
	Playlists []Item `json:"playlists"`
}

// Raw wire shapes — only what we read.
type wireImage struct {
	URL string `json:"url"`
}
type wireArtist struct {
	Name string `json:"name"`
}
type wireTrack struct {
	URI     string       `json:"uri"`
	Name    string       `json:"name"`
	Artists []wireArtist `json:"artists"`
	Album   struct {
		Name   string      `json:"name"`
		Images []wireImage `json:"images"`
	} `json:"album"`
}
type wireAlbum struct {
	URI     string       `json:"uri"`
	Name    string       `json:"name"`
	Artists []wireArtist `json:"artists"`
	Images  []wireImage  `json:"images"`
}
type wirePlaylist struct {
	URI    string      `json:"uri"`
	Name   string      `json:"name"`
	Images []wireImage `json:"images"`
	Owner  struct {
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

func artistLine(artists []wireArtist) string {
	names := make([]string, 0, len(artists))
	for _, a := range artists {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

// Search queries the catalog for tracks, albums and playlists.
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
	}
	err := c.apiGet(ctx, "/search", url.Values{
		"q":     {query},
		"type":  {"track,album,playlist"},
		"limit": {fmt.Sprint(limit)},
	}, &raw)
	if err != nil {
		return nil, err
	}
	res := &Results{Tracks: []Item{}, Albums: []Item{}, Playlists: []Item{}}
	for _, t := range raw.Tracks.Items {
		res.Tracks = append(res.Tracks, Item{
			Kind: "track", URI: t.URI, Name: t.Name,
			Sub: artistLine(t.Artists), ArtURL: artOf(t.Album.Images),
		})
	}
	for _, a := range raw.Albums.Items {
		res.Albums = append(res.Albums, Item{
			Kind: "album", URI: a.URI, Name: a.Name,
			Sub: artistLine(a.Artists), ArtURL: artOf(a.Images),
		})
	}
	for _, p := range raw.Playlists.Items {
		if p == nil {
			continue
		}
		res.Playlists = append(res.Playlists, Item{
			Kind: "playlist", URI: p.URI, Name: p.Name,
			Sub: p.Owner.DisplayName, ArtURL: artOf(p.Images),
		})
	}
	return res, nil
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

// randomString returns n bytes of randomness, base64url-encoded (unpadded) —
// valid for both PKCE verifiers (RFC 7636 charset) and state values.
func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
