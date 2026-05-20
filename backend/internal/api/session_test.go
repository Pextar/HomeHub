package api

import (
	"testing"
	"time"
)

func TestSession_RoundTrip(t *testing.T) {
	secret := []byte("test-secret-key-0123456789abcdef")
	value := signSession(secret, "user_42", 3, time.Now().Add(time.Hour))

	id, version, ok := verifySession(secret, value)
	if !ok {
		t.Fatal("expected a freshly signed session to verify")
	}
	if id != "user_42" || version != 3 {
		t.Errorf("got (id=%q, version=%d), want (\"user_42\", 3)", id, version)
	}
}

func TestSession_RejectsTamperedSignature(t *testing.T) {
	secret := []byte("test-secret-key-0123456789abcdef")
	value := signSession(secret, "user_42", 1, time.Now().Add(time.Hour))

	// Flip the last character of the signature.
	tampered := value[:len(value)-1] + flip(value[len(value)-1:])
	if _, _, ok := verifySession(secret, tampered); ok {
		t.Error("expected a tampered signature to be rejected")
	}
}

func TestSession_RejectsWrongSecret(t *testing.T) {
	value := signSession([]byte("secret-one-aaaaaaaaaaaaaaaaaaaaaa"), "user_42", 1, time.Now().Add(time.Hour))
	if _, _, ok := verifySession([]byte("secret-two-bbbbbbbbbbbbbbbbbbbbbb"), value); ok {
		t.Error("expected verification under a different secret to fail")
	}
}

func TestSession_RejectsExpired(t *testing.T) {
	secret := []byte("test-secret-key-0123456789abcdef")
	value := signSession(secret, "user_42", 1, time.Now().Add(-time.Minute))
	if _, _, ok := verifySession(secret, value); ok {
		t.Error("expected an expired session to be rejected")
	}
}

func TestSession_RejectsMalformed(t *testing.T) {
	secret := []byte("test-secret-key-0123456789abcdef")
	for _, v := range []string{"", "nope", "a:b:c", "id:notanint:123:sig"} {
		if _, _, ok := verifySession(secret, v); ok {
			t.Errorf("expected malformed value %q to be rejected", v)
		}
	}
}

func TestSession_VersionBumpInvalidatesOldCookie(t *testing.T) {
	secret := []byte("test-secret-key-0123456789abcdef")
	old := signSession(secret, "user_42", 1, time.Now().Add(time.Hour))

	// The cookie still verifies cryptographically and reports its version;
	// authMiddleware compares that against the user's current TokenVersion.
	_, version, ok := verifySession(secret, old)
	if !ok {
		t.Fatal("expected the cookie to verify")
	}
	currentTokenVersion := 2 // user changed their password since
	if version == currentTokenVersion {
		t.Error("expected the old cookie's version to differ from the bumped one")
	}
}

// flip returns a single character guaranteed to differ from s.
func flip(s string) string {
	if s == "A" {
		return "B"
	}
	return "A"
}
