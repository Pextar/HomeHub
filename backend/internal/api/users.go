// Package api: users.go — login profiles and per-user socket access.
//
// Admins have unrestricted access and can manage other profiles; non-admin
// users only see and control the sockets assigned to them. Access is
// enforced server-side via requireAdmin / requireSocketAccess and the
// list-filtering helpers, so a hidden socket can't be reached by guessing
// its API path.
package api

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"rf-socket-controller/internal/store"
)

// userView is the client-facing shape of a user — everything except the
// password hash, which never leaves the backend. LoginCode is included so
// an admin can read back and re-share a profile's code; it's only ever set
// for limited (non-admin) profiles.
type userView struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Admin     bool      `json:"admin"`
	Kid       bool      `json:"kid"`
	LoginCode string    `json:"login_code,omitempty"`
	SocketIDs []string  `json:"socket_ids"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserView(u *store.User) userView {
	ids := u.SocketIDs
	if ids == nil {
		ids = []string{}
	}
	return userView{
		ID:        u.ID,
		Username:  u.Username,
		Admin:     u.Admin,
		Kid:       u.Kid,
		LoginCode: u.LoginCode,
		SocketIDs: ids,
		CreatedAt: u.CreatedAt,
	}
}

// generateLoginCode returns a fresh 6-digit code not currently in use by
// any profile. Codes are short because this is a local-network convenience,
// not a hardened secret. Caller must hold Mu.
func generateLoginCode(st *store.Store) string {
	for i := 0; i < 100; i++ {
		code := fmt.Sprintf("%06d", rand.Intn(1_000_000))
		if st.UserByLoginCode(code) == nil {
			return code
		}
	}
	// Astronomically unlikely with a handful of users; fall back to a
	// time-derived value rather than loop forever.
	return fmt.Sprintf("%06d", int(time.Now().UnixNano()%1_000_000))
}

// canAccess reports whether the request's user may touch the given socket.
// A nil user means auth is disabled (no users configured) → full access.
func canAccess(user *store.User, socketID string) bool {
	return user == nil || user.CanAccessSocket(socketID)
}

// isAdmin reports whether the request's user is an admin. A nil user means
// auth is disabled → treated as admin so the app stays fully usable.
func isAdmin(user *store.User) bool {
	return user == nil || user.Admin
}

// requireAdmin wraps a handler so only admins (or anyone, when auth is off)
// may reach it.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(currentUser(r)) {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, r)
	}
}

// requireSocketAccess returns true if the request's user may act on the
// given socket; otherwise it writes 403 and returns false.
func (s *Server) requireSocketAccess(w http.ResponseWriter, r *http.Request, socketID string) bool {
	if canAccess(currentUser(r), socketID) {
		return true
	}
	writeError(w, http.StatusForbidden, "you don't have access to that device")
	return false
}

// getMe returns the authenticated profile. When auth is disabled it
// returns a synthetic admin so the SPA renders the full UI.
func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeJSON(w, http.StatusOK, userView{Admin: true, SocketIDs: []string{}})
		return
	}
	writeJSON(w, http.StatusOK, toUserView(u))
}

func (s *Server) listUsers(w http.ResponseWriter, _ *http.Request) {
	s.Store.Mu.RLock()
	out := make([]userView, 0, len(s.Store.Users))
	for _, u := range s.Store.Users {
		out = append(out, toUserView(u))
	}
	s.Store.Mu.RUnlock()
	sortUserViews(out)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username  string   `json:"username"`
		Password  string   `json:"password"`
		Admin     bool     `json:"admin"`
		Kid       bool     `json:"kid"`
		SocketIDs []string `json:"socket_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	username := strings.TrimSpace(body.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	// Admins sign in with a password; limited profiles get a generated
	// login code instead and have no password.
	var hash string
	if body.Admin {
		if strings.TrimSpace(body.Password) == "" {
			writeError(w, http.StatusBadRequest, "admin profiles need a password")
			return
		}
		h, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		hash = string(h)
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if s.Store.UserByUsername(username) != nil {
		writeError(w, http.StatusConflict, "a user with that name already exists")
		return
	}
	user := &store.User{
		ID:        fmt.Sprintf("user_%d", time.Now().UnixNano()),
		Username:  username,
		Admin:     body.Admin,
		Kid:       body.Kid && !body.Admin, // kid mode is a flavor of a limited profile
		SocketIDs: sanitizeSocketIDs(s.Store, body.SocketIDs),
		CreatedAt: time.Now(),
	}
	user.PasswordHash = hash
	if !body.Admin {
		user.LoginCode = generateLoginCode(s.Store)
	}
	s.Store.Users[user.ID] = user
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Users, user.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toUserView(user))
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Username       *string   `json:"username"`
		Password       *string   `json:"password"`
		Admin          *bool     `json:"admin"`
		Kid            *bool     `json:"kid"`
		SocketIDs      *[]string `json:"socket_ids"`
		RegenerateCode bool      `json:"regenerate_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	user, ok := s.Store.Users[id]
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if body.Username != nil {
		name := strings.TrimSpace(*body.Username)
		if name == "" {
			writeError(w, http.StatusBadRequest, "username cannot be empty")
			return
		}
		if existing := s.Store.UserByUsername(name); existing != nil && existing.ID != user.ID {
			writeError(w, http.StatusConflict, "a user with that name already exists")
			return
		}
		user.Username = name
	}
	if body.SocketIDs != nil {
		user.SocketIDs = sanitizeSocketIDs(s.Store, *body.SocketIDs)
	}

	// Resolve the target role, then reconcile credentials with it: admins
	// have a password and no code; limited profiles have a code and no
	// password.
	targetAdmin := user.Admin
	if body.Admin != nil {
		targetAdmin = *body.Admin
	}
	if user.Admin && !targetAdmin && s.Store.AdminCount() <= 1 {
		writeError(w, http.StatusBadRequest, "can't remove the last admin")
		return
	}

	if targetAdmin {
		if body.Password != nil && strings.TrimSpace(*body.Password) != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), bcrypt.DefaultCost)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to hash password")
				return
			}
			user.PasswordHash = string(hash)
		}
		if user.PasswordHash == "" {
			writeError(w, http.StatusBadRequest, "set a password to make this profile an admin")
			return
		}
		user.Admin = true
		user.Kid = false // admins never use the kid layout
		user.LoginCode = ""
	} else {
		user.Admin = false
		user.PasswordHash = ""
		if body.Kid != nil {
			user.Kid = *body.Kid
		}
		if user.LoginCode == "" || body.RegenerateCode {
			user.LoginCode = generateLoginCode(s.Store)
		}
	}

	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toUserView(user))
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	user, ok := s.Store.Users[id]
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if user.Admin && s.Store.AdminCount() <= 1 {
		writeError(w, http.StatusBadRequest, "can't delete the last admin")
		return
	}
	delete(s.Store.Users, id)
	if err := s.Store.Save(); err != nil {
		s.Store.Users[id] = user
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sanitizeSocketIDs keeps only IDs that refer to real sockets, dropping
// duplicates and unknowns. Caller must hold Mu.
func sanitizeSocketIDs(st *store.Store, ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		if _, ok := st.Sockets[id]; ok {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out
}

func sortUserViews(v []userView) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && strings.ToLower(v[j-1].Username) > strings.ToLower(v[j].Username); j-- {
			v[j-1], v[j] = v[j], v[j-1]
		}
	}
}

// Bootstrap seeds an initial admin from AUTH_USER/AUTH_PASS when no users
// exist yet, so existing single-credential deployments keep working and a
// fresh install has a way in. A no-op once any user is present.
func (s *Server) Bootstrap() error {
	if s.AuthUser == "" || s.AuthPass == "" {
		return nil
	}
	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()
	if len(s.Store.Users) > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.AuthPass), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}
	user := &store.User{
		ID:        fmt.Sprintf("user_%d", time.Now().UnixNano()),
		Username:  s.AuthUser,
		Admin:     true,
		SocketIDs: []string{},
		CreatedAt: time.Now(),
	}
	user.PasswordHash = string(hash)
	s.Store.Users[user.ID] = user
	return s.Store.Save()
}
