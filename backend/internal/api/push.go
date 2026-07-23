package api

import (
	"encoding/json"
	"net/http"

	"homehub/internal/push"
	"homehub/internal/store"
)

// notifyBulkState sends a single summary "device state" notification for a
// bulk action (all-off, room, group, scene) so users don't get one push per
// affected socket. A no-op when push is disabled or nothing changed.
func (s *Server) notifyBulkState(title string, changed int) {
	if s.Push == nil || changed == 0 {
		return
	}
	go s.Push.NotifyEvent(push.CategoryStateChanges, "", push.PushPayload{
		Title: title,
		URL:   "/#/dashboard",
		Tag:   "bulk-state",
	})
}

// getPushVAPIDKey returns the server's VAPID public key so the browser can
// subscribe to push notifications. No authentication required — the public
// key is not a secret.
func (s *Server) getPushVAPIDKey(w http.ResponseWriter, _ *http.Request) {
	if s.Push == nil {
		writeError(w, http.StatusServiceUnavailable, "push notifications not configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": s.Push.VAPIDPublicKey})
}

// subscribePush saves a browser push subscription for the authenticated user.
// If this is the user's first subscription, all notification categories are
// enabled by default.
func (s *Server) subscribePush(w http.ResponseWriter, r *http.Request) {
	if s.Push == nil {
		writeError(w, http.StatusServiceUnavailable, "push notifications not configured")
		return
	}
	user := currentUser(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body struct {
		Endpoint string          `json:"endpoint"`
		Keys     push.SubKeys    `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Endpoint == "" || body.Keys.P256dh == "" || body.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "endpoint, keys.p256dh and keys.auth are required")
		return
	}

	sub := push.PushSubscription{
		UserID:   user.ID,
		Endpoint: body.Endpoint,
		Keys:     body.Keys,
	}
	if err := s.Push.Subs.Add(sub); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save subscription: "+err.Error())
		return
	}

	// Enable all notification categories by default for users subscribing for
	// the first time (all-false zero value → upgrade to all-true).
	prefs := user.NotifPrefs
	if !prefs.SensorAlerts && !prefs.StateChanges && !prefs.ScheduleFired && !prefs.DeviceOffline {
		s.Store.Mu.Lock()
		if u := s.Store.Users[user.ID]; u != nil {
			u.NotifPrefs = store.NotifPrefs{
				SensorAlerts:  true,
				StateChanges:  true,
				ScheduleFired: true,
				DeviceOffline: true,
			}
			_ = s.Store.Save()
		}
		s.Store.Mu.Unlock()
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "subscribed"})
}

// unsubscribePush removes the browser push subscription identified by its
// endpoint URL.
func (s *Server) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	if s.Push == nil {
		writeError(w, http.StatusServiceUnavailable, "push notifications not configured")
		return
	}

	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}

	if err := s.Push.Subs.Remove(body.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove subscription: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}

// testPush sends a test notification to the authenticated user's own
// subscriptions so they can confirm push is working end-to-end.
func (s *Server) testPush(w http.ResponseWriter, r *http.Request) {
	if s.Push == nil {
		writeError(w, http.StatusServiceUnavailable, "push notifications not configured")
		return
	}
	user := currentUser(r)
	var userID *string
	if user != nil {
		id := user.ID
		userID = &id
	}
	// Bypass category/quiet-hours/mute filters — a test should always arrive.
	s.Push.Notify(userID, push.PushPayload{
		Title: "🔔 HomeHub test",
		Body:  "Push notifications are working.",
		URL:   "/#/settings",
		Tag:   "test",
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

// updatePushPrefs updates the authenticated user's notification preferences.
func (s *Server) updatePushPrefs(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body store.NotifPrefs
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()
	u := s.Store.Users[user.ID]
	if u == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	u.NotifPrefs = body
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u.NotifPrefs)
}
