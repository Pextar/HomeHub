package push

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Notification categories. Used as the `category` argument to NotifyEvent and
// matched against per-user preferences.
const (
	CategorySensorAlerts  = "SensorAlerts"
	CategoryStateChanges  = "StateChanges"
	CategoryScheduleFired = "ScheduleFired"
	CategoryDeviceOffline = "DeviceOffline"
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
	DeviceOffline bool
	QuietHours    bool
	QuietStart    string          // "HH:MM"
	QuietEnd      string          // "HH:MM"
	MutedIDs      map[string]bool // socket + sensor IDs the user has muted
}

// Service sends Web Push notifications to subscribed browsers.
type Service struct {
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	Subs            *SubscriptionStore
	// GetUserPrefs, if set, is called by NotifyEvent to look up every user's
	// current notification preferences. Keeping this as a function field
	// avoids an import cycle between push and store.
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

// NotifyEvent sends payload to every user who (a) has `category` enabled,
// (b) is not currently in quiet hours — sensor alerts bypass quiet hours
// since they can be safety-critical, and (c) has not muted deviceID. Pass an
// empty deviceID for events not tied to a specific device (e.g. bulk
// summaries, schedule fires) so the mute check is skipped.
func (s *Service) NotifyEvent(category, deviceID string, payload PushPayload) {
	if s.GetUserPrefs == nil {
		// No user prefs function wired; broadcast to everyone.
		s.Notify(nil, payload)
		return
	}
	now := time.Now()
	for _, u := range s.GetUserPrefs() {
		if !u.categoryEnabled(category) {
			continue
		}
		// Quiet hours suppress everything except sensor alerts.
		if category != CategorySensorAlerts && u.QuietHours &&
			inQuietHours(now, u.QuietStart, u.QuietEnd) {
			continue
		}
		if deviceID != "" && u.MutedIDs[deviceID] {
			continue
		}
		id := u.ID
		s.Notify(&id, payload)
	}
}

func (u UserPrefs) categoryEnabled(category string) bool {
	switch category {
	case CategorySensorAlerts:
		return u.SensorAlerts
	case CategoryStateChanges:
		return u.StateChanges
	case CategoryScheduleFired:
		return u.ScheduleFired
	case CategoryDeviceOffline:
		return u.DeviceOffline
	}
	return false
}

// inQuietHours reports whether now's local time falls within [start, end),
// where both are "HH:MM". The window may wrap past midnight (start > end),
// e.g. 22:00–07:00. Returns false if either bound is unparseable or equal.
func inQuietHours(now time.Time, start, end string) bool {
	s, okS := parseHHMM(start)
	e, okE := parseHHMM(end)
	if !okS || !okE || s == e {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	if s < e {
		return cur >= s && cur < e
	}
	// Wrap-around window.
	return cur >= s || cur < e
}

// parseHHMM converts "HH:MM" to minutes-since-midnight.
func parseHHMM(v string) (int, bool) {
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
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

