package spotify

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestExchangeRedirectParsing exercises the pasted-URL parsing paths that
// fail before any network call — the error messages are user-facing setup
// guidance, so their triggers matter.
func TestExchangeRedirectParsing(t *testing.T) {
	c := &Client{pending: map[string]pendingAuth{}}
	ctx := context.Background()

	if err := c.ExchangeRedirect(ctx, ""); err == nil {
		t.Error("empty paste should error")
	}
	if err := c.ExchangeRedirect(ctx, "http://127.0.0.1:8080/api/spotify/callback"); err == nil ||
		!strings.Contains(err.Error(), "no login code") {
		t.Errorf("code-less URL should say the code is missing, got %v", err)
	}
	if err := c.ExchangeRedirect(ctx, "http://127.0.0.1:8080/cb?error=access_denied"); err == nil ||
		!strings.Contains(err.Error(), "refused") {
		t.Errorf("error param should surface as refusal, got %v", err)
	}
	// A valid-shaped paste with an unknown state fails the pending lookup —
	// proof the query string was parsed and the flow guard works.
	err := c.ExchangeRedirect(ctx, "http://127.0.0.1:8080/api/spotify/callback?code=abc&state=nope")
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("unknown state should report an expired/foreign login, got %v", err)
	}
	// Bare query strings are accepted too.
	err = c.ExchangeRedirect(ctx, "?code=abc&state=nope")
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("bare query paste should parse, got %v", err)
	}
}

func TestSetClientIDClearsTokensOnChange(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.p = persisted{ClientID: "old", RefreshToken: "tok", DisplayName: "petter"}
	if err := c.SetClientID("new-id"); err != nil {
		t.Fatal(err)
	}
	st := c.Status()
	if st.Connected || st.DisplayName != "" {
		t.Errorf("changing client id should drop tokens, got %+v", st)
	}
	// Same ID again is a no-op for tokens.
	c.p.RefreshToken = "tok2"
	if err := c.SetClientID("new-id"); err != nil {
		t.Fatal(err)
	}
	if !c.Status().Connected {
		t.Error("re-saving the same client id must keep tokens")
	}
}

// TestMarketBackfillsFromMe covers the bug where an artist page came back
// with a name and picture but no top tracks or albums: those endpoints
// silently answer with an empty list (not an error) when the request
// carries no market, so a login stored before Country was recorded needs
// ensureCountry to backfill it from /me before those calls go out.
func TestMarketBackfillsFromMe(t *testing.T) {
	c := connected(t, "", roundTripFunc(func(r *http.Request) *http.Response {
		if !strings.HasSuffix(r.URL.Path, "/me") {
			t.Fatalf("unexpected request to %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"country":"SE"}`)
	}))

	if m := c.market(); m != nil {
		t.Errorf("market() before backfill should be nil, got %v", m)
	}

	c.ensureCountry(context.Background())

	if got := c.market().Get("market"); got != "SE" {
		t.Errorf("market() after backfill = %q, want SE", got)
	}
	if c.p.Country != "SE" {
		t.Errorf("Country not persisted, got %q", c.p.Country)
	}

	// Already-known country must not trigger another /me round trip.
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) *http.Response {
		t.Fatal("ensureCountry should be a no-op once Country is known")
		return nil
	})}
	c.ensureCountry(context.Background())
}
