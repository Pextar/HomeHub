package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"rf-socket-controller/internal/store"
)

// ctxKey is the private type for request-context keys so values set here
// can't collide with keys from other packages.
type ctxKey int

const userCtxKey ctxKey = iota

// authMiddleware gates protected routes. It accepts either a valid signed
// session cookie (set by /api/login) or HTTP basic auth matching a stored
// user's credentials — the latter so curl / scripted clients still work
// without going through the login flow. The authenticated *store.User is
// stashed in the request context for handlers to read via currentUser.
//
// Browsers no longer get a Basic-Auth WWW-Authenticate challenge (which
// would pop the native login dialog and bypass our cookie flow); they get
// a plain 401 the SPA handles with its custom login form.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(cookieName); err == nil {
			if id, version, expires, ok := verifySession(s.SessionSecret, c.Value); ok {
				if u := s.lookupUser(id); u != nil && u.TokenVersion == version {
					// Rolling renewal: re-issue the cookie once it's past
					// half its life so active devices never expire while
					// a stolen cookie still dies within cookieTTL.
					if time.Until(expires) < cookieTTL/2 {
						setSessionCookie(w, s.SessionSecret, u.ID, u.TokenVersion, isSecureRequest(r))
					}
					next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u)))
					return
				}
			}
		}
		if u, p, ok := r.BasicAuth(); ok {
			if user := s.verifyCredentials(u, p); user != nil {
				next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
				return
			}
		}
		writeError(w, http.StatusUnauthorized, "unauthorized")
	})
}

// lookupUser returns a copy of the stored user with the given ID, or nil.
// A copy (not the live pointer) is returned because the result is stashed in
// the request context and read concurrently with user-mutating handlers.
func (s *Server) lookupUser(id string) *store.User {
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	return s.Store.UserByID(id).Clone()
}

// verifyCredentials returns the user if username/password match a stored
// account (bcrypt), else nil. The compare runs even when no user is found
// so the response timing doesn't leak which usernames exist. A user with
// no password (a code-only profile) can never match this path.
func (s *Server) verifyCredentials(username, password string) *store.User {
	s.Store.Mu.RLock()
	u := s.Store.UserByUsername(username)
	s.Store.Mu.RUnlock()
	hash := []byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvali")
	if u != nil && u.PasswordHash != "" {
		hash = []byte(u.PasswordHash)
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return nil
	}
	return u
}

// verifyLoginCode returns the user whose login code matches, else nil.
func (s *Server) verifyLoginCode(code string) *store.User {
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	return s.Store.UserByLoginCode(strings.TrimSpace(code))
}

func withUser(ctx context.Context, u *store.User) context.Context {
	return context.WithValue(ctx, userCtxKey, u)
}

// currentUser returns the authenticated user attached to the request by
// authMiddleware, or nil if the route is unauthenticated.
func currentUser(r *http.Request) *store.User {
	u, _ := r.Context().Value(userCtxKey).(*store.User)
	return u
}

// maxBodyBytes caps the size of request bodies so the JSON decoders
// (notably /import) can't be made to read an unbounded payload. Oversized
// bodies surface to handlers as a decode error, which they already map to
// a 400.
func maxBodyBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, n)
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

// Flush forwards to the underlying writer so streaming responses (SSE on
// /api/events) keep working through the logging wrapper. Embedding the
// ResponseWriter interface alone wouldn't promote Flush, since it isn't
// part of http.ResponseWriter's method set.
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
