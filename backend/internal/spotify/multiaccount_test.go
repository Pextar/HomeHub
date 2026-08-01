package spotify

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Multi-account: one household account (the "" key — Music view, KEF
// Connect, autoplay) plus one account per kid profile, all sharing the
// developer app's client ID. These tests pin the split: storage, the
// legacy-file migration, isolation between accounts, and which account a
// callback connects.

// TestLegacyFileFoldsIntoHousehold: a spotify.json written before
// multi-account (flat tokens next to the client ID) still connects the
// grown-ups — the tokens become the household account, and the legacy
// fields are dropped so they can't be read twice.
func TestLegacyFileFoldsIntoHousehold(t *testing.T) {
	dir := t.TempDir()
	legacy := map[string]interface{}{
		"client_id":     "cid",
		"refresh_token": "rt-legacy",
		"access_token":  "at-legacy",
		"display_name":  "petter",
		"scope":         fullScope,
		"country":       "SE",
	}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, stateFile), raw, 0600); err != nil {
		t.Fatal(err)
	}

	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	st := c.Status()
	if !st.Connected || st.DisplayName != "petter" || !st.Playback {
		t.Errorf("household after migration = %+v, want connected with playback", st)
	}
	if got := c.For("").market().Get("market"); got != "SE" {
		t.Errorf("market after migration = %q, want SE", got)
	}
	if c.p.RefreshToken != "" || c.p.AccessToken != "" || c.p.DisplayName != "" {
		t.Error("legacy flat fields should be cleared after folding")
	}
	if err := c.save(); err != nil {
		t.Fatal(err)
	}
	reread, _ := os.ReadFile(filepath.Join(dir, stateFile))
	if !strings.Contains(string(reread), `"household"`) {
		t.Errorf("saved file kept the flat shape: %s", reread)
	}
}

// TestAccountsAreIsolated: a kid's account and the household's live side by
// side — statuses differ, and disconnecting one leaves the other standing.
func TestAccountsAreIsolated(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.p.ClientID = "cid"
	c.account("").RefreshToken = "household-rt"
	c.account("user_kid").RefreshToken = "kid-rt"

	if !c.For("").Status().Connected || !c.For("user_kid").Status().Connected {
		t.Fatal("both accounts should read connected")
	}
	if c.For("user_other").Status().Connected {
		t.Error("a never-connected kid account should read not connected")
	}
	// Peeking must not attach ghost entries.
	if c.accountPeek("user_other") != nil {
		t.Error("status reads must not create account entries")
	}

	if err := c.For("user_kid").Disconnect(); err != nil {
		t.Fatal(err)
	}
	if c.For("user_kid").Status().Connected {
		t.Error("kid account should be disconnected")
	}
	if !c.For("").Status().Connected {
		t.Error("disconnecting the kid must leave the household connected")
	}
}

// TestCallbackConnectsTheAccountItStartedFor: the household and a kid can
// have logins in flight at once; each callback lands on its own account,
// and the returned key tells the API whose login just finished.
func TestCallbackConnectsTheAccountItStartedFor(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.p.ClientID = "cid"
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) *http.Response {
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/token"):
			return jsonResponse(200, `{"access_token":"at","refresh_token":"rt","expires_in":3600,"scope":"`+fullScope+`"}`)
		case strings.HasSuffix(r.URL.Path, "/me"):
			return jsonResponse(200, `{"display_name":"Ebbe","country":"SE"}`)
		default:
			t.Fatalf("unexpected request to %s", r.URL.Path)
			return nil
		}
	})}

	householdURL, err := c.AuthURL("", "http://127.0.0.1:8080/api/spotify/callback")
	if err != nil {
		t.Fatal(err)
	}
	kidURL, err := c.AuthURL("user_kid", "http://127.0.0.1:8080/api/spotify/callback")
	if err != nil {
		t.Fatal(err)
	}
	stateOf := func(rawurl string) string {
		q := strings.SplitN(rawurl, "?", 2)[1]
		for _, pair := range strings.Split(q, "&") {
			if strings.HasPrefix(pair, "state=") {
				return strings.TrimPrefix(pair, "state=")
			}
		}
		return ""
	}

	key, err := c.HandleCallback(context.Background(), "code-1", stateOf(kidURL))
	if err != nil {
		t.Fatal(err)
	}
	if key != "user_kid" {
		t.Errorf("callback returned account %q, want user_kid", key)
	}
	if st := c.For("user_kid").Status(); !st.Connected || st.DisplayName != "Ebbe" {
		t.Errorf("kid account = %+v, want connected as Ebbe", st)
	}
	if c.For("").Status().Connected {
		t.Error("the kid's callback must not touch the household account")
	}

	key, err = c.HandleCallback(context.Background(), "code-2", stateOf(householdURL))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		t.Errorf("household callback returned account %q, want \"\"", key)
	}
	if !c.For("").Status().Connected {
		t.Error("household account should be connected after its callback")
	}
}

// TestSetClientIDWipesEveryAccount: changing the developer app invalidates
// all tokens — the household's and every kid's — since they all belong to
// the old app.
func TestSetClientIDWipesEveryAccount(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.p = persisted{
		ClientID:  "old",
		Household: &accountState{RefreshToken: "rt"},
		Accounts:  map[string]*accountState{"user_kid": {RefreshToken: "kid-rt"}},
	}
	if err := c.SetClientID("new"); err != nil {
		t.Fatal(err)
	}
	if c.For("").Status().Connected || c.For("user_kid").Status().Connected {
		t.Error("changing the client ID must drop every account")
	}
}

// TestRefreshKeepsAccountsApart: a token refresh writes back to the account
// it refreshed, never onto a neighbour.
func TestRefreshKeepsAccountsApart(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.p = persisted{
		ClientID:  "cid",
		Household: &accountState{RefreshToken: "hh-rt", Expiry: time.Now().Add(-time.Minute)},
		Accounts:  map[string]*accountState{"user_kid": {RefreshToken: "kid-rt", Expiry: time.Now().Add(-time.Minute)}},
	}
	var sawRefresh string
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) *http.Response {
		_ = r.ParseForm()
		sawRefresh = r.Form.Get("refresh_token")
		return jsonResponse(200, `{"access_token":"fresh","expires_in":3600}`)
	})}

	if _, err := c.For("user_kid").accessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sawRefresh != "kid-rt" {
		t.Errorf("refresh used token %q, want the kid's", sawRefresh)
	}
	if c.p.Household.AccessToken == "fresh" {
		t.Error("the kid's refresh landed on the household account")
	}
}
