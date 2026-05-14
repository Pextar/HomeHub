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

func signSession(secret []byte, username string, expires time.Time) string {
	payload := fmt.Sprintf("%s:%d", username, expires.Unix())
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + ":" + sig
}

func verifySession(secret []byte, value string) (string, bool) {
	parts := strings.SplitN(value, ":", 3)
	if len(parts) != 3 {
		return "", false
	}
	username, expStr, gotSig := parts[0], parts[1], parts[2]
	payload := username + ":" + expStr
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(gotSig), []byte(wantSig)) != 1 {
		return "", false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return username, true
}

func setSessionCookie(w http.ResponseWriter, secret []byte, username string) {
	expires := time.Now().Add(cookieTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    signSession(secret, username, expires),
		Path:     "/",
		MaxAge:   int(cookieTTL.Seconds()),
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
