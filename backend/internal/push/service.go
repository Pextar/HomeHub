package push

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// PushPayload is the JSON body delivered to the browser's push event handler.
// Tag is optional — when set, the browser replaces any earlier notification
// with the same tag rather than stacking a new one.
type PushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// UserPrefs bundles the fields the push service needs per user.
type UserPrefs struct {
	ID            string
	SensorAlerts  bool
	StateChanges  bool
	ScheduleFired bool
}

// Service sends Web Push notifications to subscribed browsers.
type Service struct {
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	Subs            *SubscriptionStore
	// GetUserPrefs, if set, is called by NotifyUsersWithPref to look up every
	// user's current notification preferences. Keeping this as a function
	// field avoids an import cycle between push and store.
	GetUserPrefs func() []UserPrefs
}

// Notify sends payload to every subscription belonging to userID. If
// userID is nil it sends to all subscribers. Stale subscriptions (HTTP
// 410) are removed automatically. Delivery errors are logged but do not
// propagate to the caller.
func (s *Service) Notify(userID *string, payload PushPayload) {
	var targets []PushSubscription
	if userID != nil {
		targets = s.Subs.GetByUser(*userID)
	} else {
		targets = s.Subs.GetAll()
	}
	if len(targets) == 0 {
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("push: marshal payload: %v", err)
		return
	}

	var wg sync.WaitGroup
	for _, sub := range targets {
		sub := sub
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.send(sub, body)
		}()
	}
	wg.Wait()
}

// NotifyUsersWithPref sends payload to all users whose named preference is
// true. pref must be one of "SensorAlerts", "StateChanges", "ScheduleFired".
func (s *Service) NotifyUsersWithPref(pref string, payload PushPayload) {
	if s.GetUserPrefs == nil {
		// No user prefs function wired; broadcast to everyone.
		s.Notify(nil, payload)
		return
	}
	for _, u := range s.GetUserPrefs() {
		var enabled bool
		switch pref {
		case "SensorAlerts":
			enabled = u.SensorAlerts
		case "StateChanges":
			enabled = u.StateChanges
		case "ScheduleFired":
			enabled = u.ScheduleFired
		}
		if !enabled {
			continue
		}
		id := u.ID
		s.Notify(&id, payload)
	}
}

func (s *Service) send(sub PushSubscription, body []byte) {
	resp, err := webpush.SendNotification(body, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.Keys.P256dh,
			Auth:   sub.Keys.Auth,
		},
	}, &webpush.Options{
		VAPIDPublicKey:  s.VAPIDPublicKey,
		VAPIDPrivateKey: s.VAPIDPrivateKey,
		TTL:             60 * 60, // 1 hour
	})
	if err != nil {
		log.Printf("push: send to %s: %v", sub.Endpoint[:min(40, len(sub.Endpoint))], err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusGone {
		log.Printf("push: subscription expired, removing %s", sub.ID)
		if rmErr := s.Subs.RemoveByID(sub.ID); rmErr != nil {
			log.Printf("push: remove stale sub: %v", rmErr)
		}
	}
}

