// Package api: users.go — login profiles and per-user socket access.
//
// Role model:
//   - Owner  (Admin=true, Owner=true): the one bootstrapped admin. Cannot be
//     deleted or demoted. Signs in with username + password set via AUTH_PASS.
//   - Manager (Admin=true, Owner=false): created by the owner or another admin.
//     Gets a one-time invite link; they set their own password on first use.
//   - Limited (Admin=false): limited profile. Signs in with a short numeric
//     login code. Can only see/control the sockets assigned to them.
//
// Admins have unrestricted access and can manage other profiles; non-admin
// users only see and control the sockets assigned to them. Access is
// enforced server-side via requireAdmin / requireSocketAccess and the
// list-filtering helpers, so a hidden socket can't be reached by guessing
// its API path.
package api

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"homehub/internal/store"
)

// userView is the client-facing shape of a user — everything except the
// password hash, which never leaves the backend. LoginCode is included so
// an admin can read back and re-share a profile's code; it's only ever set
// for limited (non-admin) profiles.
type userView struct {
	ID            string           `json:"id"`
	Username      string           `json:"username"`
	Admin         bool             `json:"admin"`
	Owner         bool             `json:"owner,omitempty"`
	Kid           bool             `json:"kid"`
	LoginCode     string           `json:"login_code,omitempty"`
	PendingInvite bool             `json:"pending_invite,omitempty"`
	SocketIDs     []string         `json:"socket_ids"`
	CreatedAt     time.Time        `json:"created_at"`
	NotifPrefs    store.NotifPrefs `json:"notif_prefs,omitempty"`
}

func toUserView(u *store.User) userView {
	ids := u.SocketIDs
	if ids == nil {
		ids = []string{}
	}
	return userView{
		ID:            u.ID,
		Username:      u.Username,
		Admin:         u.Admin,
		Owner:         u.Owner,
		Kid:           u.Kid,
		LoginCode:     u.LoginCode,
		PendingInvite: u.Admin && u.InviteToken != "" && u.InviteExpiry.After(time.Now()),
		SocketIDs:     ids,
		CreatedAt:     u.CreatedAt,
		NotifPrefs:    u.NotifPrefs,
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

// generateInviteToken returns a cryptographically random 32-byte hex token.
func generateInviteToken() (string, error) {
	b := make([]byte, 32)
	if _, err := cryptorand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
	var out []userView
	s.Store.View(func() {
		out = make([]userView, 0, len(s.Store.Users))
		for _, u := range s.Store.Users {
			out = append(out, toUserView(u))
		}
	})
	sortUserViews(out)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username  string   `json:"username"`
		Admin     bool     `json:"admin"`
		Kid       bool     `json:"kid"`
		SocketIDs []string `json:"socket_ids"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	username := strings.TrimSpace(body.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	// Both are built inside the transaction and rendered after it.
	var user *store.User
	var inviteURL string
	if !s.updateOr(w, func() { delete(s.Store.Users, user.ID) }, func() error {
		if s.Store.UserByUsername(username) != nil {
			return errStatus(http.StatusConflict, "a user with that name already exists")
		}

		user = &store.User{
			ID:        fmt.Sprintf("user_%d", time.Now().UnixNano()),
			Username:  username,
			Admin:     body.Admin,
			Kid:       body.Kid && !body.Admin, // kid mode is a flavor of a limited profile
			SocketIDs: sanitizeSocketIDs(s.Store, body.SocketIDs),
			CreatedAt: time.Now(),
		}

		if body.Admin {
			// Admin (manager) users get a one-time invite link so they can set
			// their own password — the creating admin never picks it for them.
			token, err := generateInviteToken()
			if err != nil {
				return errStatus(http.StatusInternalServerError, "failed to generate invite token")
			}
			user.InviteToken = token
			user.InviteExpiry = time.Now().Add(7 * 24 * time.Hour)

			// Build the invite URL from the request's host so it points at the
			// actual running instance (works on LAN, custom domains, etc.).
			scheme := "http"
			if isSecureRequest(r) {
				scheme = "https"
			}
			inviteURL = fmt.Sprintf("%s://%s/?invite=%s", scheme, r.Host, token)
		} else {
			// Limited profiles get a generated login code.
			user.LoginCode = generateLoginCode(s.Store)
		}

		s.Store.Users[user.ID] = user
		return nil
	}) {
		return
	}

	// Include the invite URL only in the creation response, never again.
	type createResponse struct {
		userView
		InviteURL string `json:"invite_url,omitempty"`
	}
	writeJSON(w, http.StatusCreated, createResponse{
		userView:  toUserView(user),
		InviteURL: inviteURL,
	})
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
	if !decodeBody(w, r, &body) {
		return
	}

	var user *store.User
	if !s.update(w, func() error {
		var ok bool
		user, ok = s.Store.Users[id]
		if !ok {
			return errStatus(http.StatusNotFound, "user not found")
		}

		// The owner's admin status is immutable — block any attempt to demote.
		if user.Owner && body.Admin != nil && !*body.Admin {
			return errStatus(http.StatusBadRequest, "the owner account cannot be demoted")
		}

		// Snapshot the credentials so we can detect a change below and bump
		// TokenVersion, which invalidates this user's existing sessions.
		prevHash, prevCode := user.PasswordHash, user.LoginCode

		if body.Username != nil {
			name := strings.TrimSpace(*body.Username)
			if name == "" {
				return errStatus(http.StatusBadRequest, "username cannot be empty")
			}
			if existing := s.Store.UserByUsername(name); existing != nil && existing.ID != user.ID {
				return errStatus(http.StatusConflict, "a user with that name already exists")
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
			return errStatus(http.StatusBadRequest, "can't remove the last admin")
		}

		if targetAdmin {
			if body.Password != nil && strings.TrimSpace(*body.Password) != "" {
				hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), bcrypt.DefaultCost)
				if err != nil {
					return errStatus(http.StatusInternalServerError, "failed to hash password")
				}
				user.PasswordHash = string(hash)
				// Setting a password clears any pending invite.
				user.InviteToken = ""
				user.InviteExpiry = time.Time{}
			}
			if user.PasswordHash == "" && user.InviteToken == "" {
				return errStatus(http.StatusBadRequest, "set a password to make this profile an admin")
			}
			user.Admin = true
			user.Kid = false // admins never use the kid layout
			user.LoginCode = ""
		} else {
			user.Admin = false
			user.Owner = false // can't be owner without being admin
			user.PasswordHash = ""
			user.InviteToken = ""
			user.InviteExpiry = time.Time{}
			if body.Kid != nil {
				user.Kid = *body.Kid
			}
			if user.LoginCode == "" || body.RegenerateCode {
				user.LoginCode = generateLoginCode(s.Store)
			}
		}

		if user.PasswordHash != prevHash || user.LoginCode != prevCode {
			user.TokenVersion++
		}
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, toUserView(user))
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	// Captured so the undo can put the user back if the write fails.
	var user *store.User
	if !s.updateOr(w, func() { s.Store.Users[id] = user }, func() error {
		var ok bool
		user, ok = s.Store.Users[id]
		if !ok {
			return errStatus(http.StatusNotFound, "user not found")
		}
		if user.Owner {
			return errStatus(http.StatusBadRequest, "the owner account cannot be deleted")
		}
		if user.Admin && s.Store.AdminCount() <= 1 {
			return errStatus(http.StatusBadRequest, "can't delete the last admin")
		}
		delete(s.Store.Users, id)
		return nil
	}) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// lookupInvite returns basic info about a pending invite so the frontend can
// greet the invitee by name. It's a public endpoint — it only reveals the
// username for an invite that already exists and hasn't expired.
func (s *Server) lookupInvite(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	var user *store.User
	s.Store.View(func() { user = s.Store.UserByInviteToken(token) })

	if user == nil || user.InviteExpiry.Before(time.Now()) {
		writeError(w, http.StatusNotFound, "invite link is invalid or has expired")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": user.Username})
}

// acceptInvite lets a newly-invited admin user set their own password via the
// one-time token they received. On success it logs them in immediately by
// setting a session cookie — no separate login step needed.
func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	token := strings.TrimSpace(body.Token)
	password := strings.TrimSpace(body.Password)
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	if len(password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// Needed after the transaction to log the user straight in.
	var user *store.User
	if !s.update(w, func() error {
		user = s.Store.UserByInviteToken(token)
		if user == nil || user.InviteExpiry.Before(time.Now()) {
			return errStatus(http.StatusNotFound, "invite link is invalid or has expired")
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return errStatus(http.StatusInternalServerError, "failed to hash password")
		}

		user.PasswordHash = string(hash)
		user.InviteToken = ""
		user.InviteExpiry = time.Time{}
		user.TokenVersion++ // invalidate any stale tokens (shouldn't be any, but defensive)
		return nil
	}) {
		return
	}

	// Log the user in immediately — they shouldn't have to re-enter their
	// brand-new password on a separate login screen.
	setSessionCookie(w, s.SessionSecret, user.ID, user.TokenVersion, isSecureRequest(r))
	writeJSON(w, http.StatusOK, map[string]string{"username": user.Username})
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
// fresh install has a way in. The bootstrapped user is marked Owner=true —
// they are the one permanent admin and cannot be deleted or demoted.
// A no-op once any user is present.
func (s *Server) Bootstrap() error {
	if s.AuthUser == "" || s.AuthPass == "" {
		return nil
	}
	return s.Store.Update(func() error {
		if len(s.Store.Users) > 0 {
			// Ensure the first admin is marked as owner (migration for existing installs).
			for _, u := range s.Store.Users {
				if u.Admin && !u.Owner {
					u.Owner = true
					return nil
				}
			}
			return store.ErrNoChange
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(s.AuthPass), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash bootstrap password: %w", err)
		}
		user := &store.User{
			ID:        fmt.Sprintf("user_%d", time.Now().UnixNano()),
			Username:  s.AuthUser,
			Admin:     true,
			Owner:     true, // the one permanent admin
			SocketIDs: []string{},
			CreatedAt: time.Now(),
		}
		user.PasswordHash = string(hash)
		s.Store.Users[user.ID] = user
		return nil
	})
}
