// Package qobuz talks to the Qobuz streaming API.
//
// Qobuz is here because it is the one streaming service HomeHub can actually
// deliver losslessly. Spotify cannot be: its Connect backend serves only the
// legacy Ogg Vorbis stream to third-party endpoints, and its lossless tier
// lives inside Spotify's own apps on a pipeline nobody else is given. Qobuz
// hands out a signed URL to the FLAC file itself, so the audio HomeHub decodes
// is the master it was sold — 16-bit/44.1 kHz at worst, 24-bit/192 kHz at best.
//
// Two things about this API are worth knowing before reading further.
//
// First, it needs an app_id and app_secret issued by Qobuz to the application,
// not to the listener — they are obtained by writing to api@qobuz.com. HomeHub
// does not ship any, because embedding someone else's would be both a licence
// breach and a credential in a public repository. A household supplies its own,
// and until it does the provider reports itself unconfigured rather than
// half-working.
//
// Second, track/getFileUrl is signed. Everything else is an ordinary GET with
// the app id and the user's auth token attached; that one call carries an MD5
// over its own parameters and the app secret, which is what stops a leaked auth
// token from being turned into a file-download service. See sign.
package qobuz

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// baseURL is the v0.2 JSON API root.
	baseURL = "https://www.qobuz.com/api.json/0.2"
	// stateFile is where credentials live under the data dir, beside
	// Spotify's own.
	stateFile = "qobuz.json"
	// requestTimeout caps a single API call. Generous enough for a slow
	// catalogue search, short enough that a tap doesn't hang a room.
	requestTimeout = 20 * time.Second
)

// Errors callers distinguish. A household that hasn't supplied app credentials
// needs a different sentence from one that hasn't logged in, and both need a
// different sentence from a network failure.
var (
	ErrNoApp         = errors.New("qobuz: no app credentials — HomeHub needs an app_id and app_secret issued by Qobuz")
	ErrNotLoggedIn   = errors.New("qobuz: not signed in to a Qobuz account")
	ErrNotStreamable = errors.New("qobuz: this track isn't streamable on your subscription")
)

// FormatID selects which encoding Qobuz hands over. Only the FLAC ones are
// used by HomeHub — the MP3 tier exists in the API and is named here so that
// a stray 5 in a config file is recognisable rather than mysterious, not
// because anything offers it.
type FormatID int

const (
	FormatMP3320   FormatID = 5  // 320 kbps MP3. Lossy; never requested.
	FormatCD       FormatID = 6  // FLAC 16-bit/44.1 kHz.
	FormatHiRes96  FormatID = 7  // FLAC 24-bit, up to 96 kHz.
	FormatHiRes192 FormatID = 27 // FLAC 24-bit, up to 192 kHz.
)

// Lossless reports whether this format preserves the master exactly.
func (f FormatID) Lossless() bool { return f == FormatCD || f == FormatHiRes96 || f == FormatHiRes192 }

// Label is the format as a listener would read it.
func (f FormatID) Label() string {
	switch f {
	case FormatMP3320:
		return "MP3 320 kbps"
	case FormatCD:
		return "FLAC 16-bit/44.1 kHz"
	case FormatHiRes96:
		return "FLAC 24-bit/96 kHz"
	case FormatHiRes192:
		return "FLAC 24-bit/192 kHz"
	}
	return fmt.Sprintf("format %d", int(f))
}

// persisted is the on-disk shape: the application's credentials and the
// household's session. The password is never stored — only the auth token
// Qobuz returns for it, which is what the API actually wants on each call.
type persisted struct {
	AppID     string `json:"app_id,omitempty"`
	AppSecret string `json:"app_secret,omitempty"`
	AuthToken string `json:"user_auth_token,omitempty"`
	// DisplayName and Plan are for the "connected as" line. Plan matters
	// more than it looks: Qobuz's Studio tiers stream hi-res and the older
	// ones cap at CD, and a listener wondering why a 24-bit album is
	// arriving at 16-bit is usually looking at their subscription.
	DisplayName string `json:"display_name,omitempty"`
	Plan        string `json:"plan,omitempty"`
	// MaxFormat is the best format this account may request, as Qobuz
	// reported it at login. Requesting above it is not an error — Qobuz
	// quietly serves the best it will give — which is exactly why it is
	// recorded, so quality reporting can say what will actually arrive
	// rather than what was asked for.
	MaxFormat FormatID `json:"max_format,omitempty"`
}

// Client is the Qobuz API client. Safe for concurrent use.
type Client struct {
	mu      sync.Mutex
	dataDir string
	p       persisted

	// HTTP is swappable for tests; defaults to a client with a timeout.
	HTTP *http.Client
	// Now is swappable so signature tests are not clock-dependent.
	Now func() time.Time
}

// New loads any persisted credentials and returns a ready client.
func New(dataDir string) (*Client, error) {
	c := &Client{dataDir: dataDir}
	raw, err := os.ReadFile(filepath.Join(dataDir, stateFile))
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, fmt.Errorf("qobuz: load state: %w", err)
	}
	if err := json.Unmarshal(raw, &c.p); err != nil {
		return nil, fmt.Errorf("qobuz: parse state: %w", err)
	}
	return c, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: requestTimeout}
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// save writes the state file. The caller must hold mu.
func (c *Client) save() error {
	if c.dataDir == "" {
		return nil
	}
	raw, err := json.MarshalIndent(c.p, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(c.dataDir, stateFile)
	tmp := path + ".tmp"
	// The file holds an app secret and a session token, so it is written
	// 0600 from the start rather than tightened afterwards.
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Status is what the UI shows about the connection.
type Status struct {
	// Configured is whether app credentials are present. Separate from
	// Connected because they fail for different reasons and are fixed by
	// different people — one is the installation, the other is the listener.
	Configured  bool     `json:"configured"`
	Connected   bool     `json:"connected"`
	DisplayName string   `json:"display_name,omitempty"`
	Plan        string   `json:"plan,omitempty"`
	MaxFormat   FormatID `json:"max_format,omitempty"`
	// MaxFormatLabel saves every caller formatting the same number.
	MaxFormatLabel string `json:"max_format_label,omitempty"`
}

// Status reports the connection state.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := Status{
		Configured:  c.p.AppID != "" && c.p.AppSecret != "",
		Connected:   c.p.AuthToken != "",
		DisplayName: c.p.DisplayName,
		Plan:        c.p.Plan,
		MaxFormat:   c.p.MaxFormat,
	}
	if st.MaxFormat != 0 {
		st.MaxFormatLabel = st.MaxFormat.Label()
	}
	return st
}

// SetApp stores the application credentials issued by Qobuz.
func (c *Client) SetApp(id, secret string) error {
	id, secret = strings.TrimSpace(id), strings.TrimSpace(secret)
	if id == "" || secret == "" {
		return errors.New("qobuz: both an app id and an app secret are required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Changing the application invalidates the session token issued under
	// the old one, so it is cleared rather than left to fail confusingly on
	// the next call.
	if c.p.AppID != id {
		c.p.AuthToken, c.p.DisplayName, c.p.Plan, c.p.MaxFormat = "", "", "", 0
	}
	c.p.AppID, c.p.AppSecret = id, secret
	return c.save()
}

// Disconnect forgets the session but keeps the application credentials, which
// belong to the installation rather than to whoever was signed in.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.p.AuthToken, c.p.DisplayName, c.p.Plan, c.p.MaxFormat = "", "", "", 0
	return c.save()
}

// creds returns the app id, secret and auth token under one lock.
func (c *Client) creds() (appID, secret, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.p.AppID, c.p.AppSecret, c.p.AuthToken
}

// MaxFormat is the best format this account is entitled to, or FormatCD when
// nothing better is known. It is a floor rather than a guess: every Qobuz
// subscription that can stream at all can stream CD quality, so assuming it
// cannot would report a household as worse off than any real account is.
func (c *Client) MaxFormat() FormatID {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.p.MaxFormat == 0 {
		return FormatCD
	}
	return c.p.MaxFormat
}

// Login exchanges a Qobuz email and password for a session token.
//
// The password is sent as its MD5 digest, which is what this API expects; it
// is not a security measure and is not treated as one — the call is over TLS,
// and the digest is simply the field format. The plaintext is never stored and
// never leaves this function.
func (c *Client) Login(ctx context.Context, email, password string) error {
	appID, _, _ := c.creds()
	if appID == "" {
		return ErrNoApp
	}
	if strings.TrimSpace(email) == "" || password == "" {
		return errors.New("qobuz: an email and password are required")
	}
	sum := md5.Sum([]byte(password))

	var out struct {
		UserAuthToken string `json:"user_auth_token"`
		User          struct {
			DisplayName string `json:"display_name"`
			Credential  struct {
				Description string `json:"description"`
				Parameters  struct {
					Label             string `json:"label"`
					LossyStreaming    bool   `json:"lossy_streaming"`
					LosslessStreaming bool   `json:"lossless_streaming"`
					HiResStreaming    bool   `json:"hires_streaming"`
					HiResPurchases    bool   `json:"hires_purchase"`
				} `json:"parameters"`
			} `json:"credential"`
		} `json:"user"`
	}
	q := url.Values{
		"email":    {email},
		"password": {hex.EncodeToString(sum[:])},
	}
	if err := c.get(ctx, "/user/login", q, false, &out); err != nil {
		return err
	}
	if out.UserAuthToken == "" {
		return errors.New("qobuz: sign-in didn't return a session token")
	}

	p := out.User.Credential.Parameters
	format := FormatCD
	switch {
	case p.HiResStreaming:
		// Qobuz does not report a rate ceiling here, only that hi-res is
		// allowed. 192 is the top of the catalogue and requesting above
		// what a track holds is harmless — the file's own rate is what
		// arrives — so this asks for the best and lets the file decide.
		format = FormatHiRes192
	case p.LosslessStreaming:
		format = FormatCD
	case p.LossyStreaming:
		format = FormatMP3320
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.p.AuthToken = out.UserAuthToken
	c.p.DisplayName = out.User.DisplayName
	c.p.Plan = p.Label
	if c.p.Plan == "" {
		c.p.Plan = out.User.Credential.Description
	}
	c.p.MaxFormat = format
	return c.save()
}

// get performs an API call and decodes the JSON response into out.
//
// signed is for track/getFileUrl alone. Everything else carries only the app
// id and, when there is one, the user's auth token.
func (c *Client) get(ctx context.Context, path string, q url.Values, signed bool, out any) error {
	appID, secret, token := c.creds()
	if appID == "" {
		return ErrNoApp
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("app_id", appID)
	if signed {
		ts := fmt.Sprint(c.now().Unix())
		q.Set("request_ts", ts)
		q.Set("request_sig", sign(path, q, ts, secret))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("qobuz: %w", err)
	}
	if token != "" {
		req.Header.Set("X-User-Auth-Token", token)
	}
	req.Header.Set("X-App-Id", appID)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("qobuz: %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// The body carries Qobuz's own message, which is more useful than
		// the status alone ("invalid app_id" versus "no such track"), so
		// it is passed through rather than replaced with a generic line.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
		msg := strings.TrimSpace(string(body))
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%w (%s)", ErrNotLoggedIn, msg)
		case http.StatusBadRequest:
			return fmt.Errorf("qobuz: %s rejected: %s", path, msg)
		}
		return fmt.Errorf("qobuz: %s: %s: %s", path, resp.Status, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("qobuz: decoding %s: %w", path, err)
	}
	return nil
}

// sign builds the request_sig for a signed call.
//
// The scheme is Qobuz's: take the endpoint path with its slashes removed
// ("/track/getFileUrl" becomes "trackgetFileUrl"), append every request
// parameter as {name}{value} in alphabetical order by name, then the
// timestamp, then the app secret, and MD5 the lot.
//
// The parameters that must not be included are the ones added by the signing
// itself — app_id, request_ts and request_sig — and they are excluded here
// rather than at the call site so no future caller has to remember.
func sign(path string, q url.Values, ts, secret string) string {
	var b strings.Builder
	b.WriteString(strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", ""))

	names := make([]string, 0, len(q))
	for name := range q {
		switch name {
		case "app_id", "request_ts", "request_sig":
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(q.Get(name))
	}
	b.WriteString(ts)
	b.WriteString(secret)

	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
