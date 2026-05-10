package api

import (
	"crypto/subtle"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// basicAuthMiddleware gates the whole app behind HTTP basic auth. It is
// only installed when both user and pass are non-empty. Comparison uses
// constant-time to avoid leaking the credentials via timing. CORS
// preflight (OPTIONS) is allowed through so browsers can negotiate
// cross-origin in dev.
func basicAuthMiddleware(user, pass string) mux.MiddlewareFunc {
	expectUser := []byte(user)
	expectPass := []byte(pass)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			u, p, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(u), expectUser) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), expectPass) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="rf-socket-controller", charset="UTF-8"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
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
