package api

import (
	"crypto/subtle"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// authMiddleware gates protected routes. It accepts either a valid signed
// session cookie (set by /api/login) or HTTP basic auth matching the
// configured AUTH_USER/AUTH_PASS — the latter so curl / scripted clients
// still work without going through the login flow.
//
// Browsers no longer get a Basic-Auth WWW-Authenticate challenge (which
// would pop the native login dialog and bypass our cookie flow); they get
// a plain 401 the SPA handles with its custom login form.
func authMiddleware(user, pass string, secret []byte) mux.MiddlewareFunc {
	expectUser := []byte(user)
	expectPass := []byte(pass)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			if c, err := r.Cookie(cookieName); err == nil {
				if _, ok := verifySession(secret, c.Value); ok {
					next.ServeHTTP(w, r)
					return
				}
			}
			u, p, ok := r.BasicAuth()
			if ok &&
				subtle.ConstantTimeCompare([]byte(u), expectUser) == 1 &&
				subtle.ConstantTimeCompare([]byte(p), expectPass) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusUnauthorized, "unauthorized")
		})
	}
}

// loggingMiddleware logs each request's method, path, status and duration.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
