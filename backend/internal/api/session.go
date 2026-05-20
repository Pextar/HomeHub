// Package api: session.go — HMAC-signed cookie sessions for browser/PWA auth.
//
// The cookie value is "username:expires_unix:base64_hmac". Verification
// re-computes the HMAC and rejects expired or tampered tokens. The HMAC
// secret lives in data/session.secret (auto-generated on first run);
// deleting it invalidates every active session.
package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	cookieName = "rfauth"
	cookieTTL  = 90 * 24 * time.Hour
)

// LoadOrCreateSessionSecret reads (or generates) a 32-byte HMAC secret
// persisted to <dataDir>/session.secret.
func LoadOrCreateSessionSecret(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, "session.secret")
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return b, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate session secret: %w", err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		return nil, fmt.Errorf("write session secret: %w", err)
	}
	return b, nil
}

func signSession(secret []byte, id string, version int, expires time.Time) string {
	payload := fmt.Sprintf("%s:%d:%d", id, version, expires.Unix())
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + ":" + sig
}

// verifySession validates a cookie value and returns the user id and the
// token version it was minted with. The caller must still confirm the
// version matches the user's current TokenVersion (see authMiddleware) so
// a credential change invalidates older cookies.
func verifySession(secret []byte, value string) (id string, version int, ok bool) {
	parts := strings.SplitN(value, ":", 4)
	if len(parts) != 4 {
		return "", 0, false
	}
	uid, verStr, expStr, gotSig := parts[0], parts[1], parts[2], parts[3]
	payload := uid + ":" + verStr + ":" + expStr
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(gotSig), []byte(wantSig)) != 1 {
		return "", 0, false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", 0, false
	}
	ver, err := strconv.Atoi(verStr)
	if err != nil {
		return "", 0, false
	}
	return uid, ver, true
}

func setSessionCookie(w http.ResponseWriter, secret []byte, id string, version int, secure bool) {
	expires := time.Now().Add(cookieTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    signSession(secret, id, version, expires),
		Path:     "/",
		MaxAge:   int(cookieTTL.Seconds()),
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// isSecureRequest reports whether the request reached us over TLS, either
// directly (our own HTTPS listener) or via a reverse proxy that terminated
// TLS and set X-Forwarded-Proto. When true we mark the session cookie
// Secure so it never travels over plaintext HTTP.
func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
