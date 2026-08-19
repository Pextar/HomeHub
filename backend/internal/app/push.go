package app

import (
	"fmt"
	"log"

	"homehub/internal/push"
	"homehub/internal/store"
)

// newPushService builds the Web Push sender and connects it to the household's
// notification preferences.
//
// The adapter below is the whole reason this is a function rather than a
// struct literal in New: the push package must not import the store — it knows
// about subscriptions and categories, not about users and sockets — so the
// translation from one to the other lives here, at the seam, where both are
// already in scope.
func newPushService(dataDir string, st *store.Store) (*push.Service, error) {
	// VAPID keys are generated on first run and reused across restarts.
	keys, err := push.LoadOrGenerateVAPIDKeys(dataDir)
	if err != nil {
		return nil, fmt.Errorf("loading/generating VAPID keys: %w", err)
	}
	subs, err := push.NewSubscriptionStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("loading push subscriptions: %w", err)
	}

	svc := &push.Service{
		VAPIDPublicKey:  keys.PublicKey,
		VAPIDPrivateKey: keys.PrivateKey,
		Subs:            subs,
		GetUserPrefs:    func() []push.UserPrefs { return userPrefs(st) },
	}
	log.Printf("Web Push notifications enabled (VAPID public key: %s...)",
		keys.PublicKey[:min(12, len(keys.PublicKey))])
	return svc, nil
}

// userPrefs snapshots every user's notification preferences.
//
// It reads under the store's read lock, so it is safe to call from the
// goroutines the push callbacks spawn. The two muted-id lists are flattened
// into one set because a notification is muted by subject, and nothing
// downstream cares whether that subject was a socket or a sensor.
func userPrefs(st *store.Store) []push.UserPrefs {
	return store.ViewValue(st, func() []push.UserPrefs {
		out := make([]push.UserPrefs, 0, len(st.Users))
		for _, u := range st.Users {
			muted := make(map[string]bool, len(u.NotifPrefs.MutedSocketIDs)+len(u.NotifPrefs.MutedSensorIDs))
			for _, id := range u.NotifPrefs.MutedSocketIDs {
				muted[id] = true
			}
			for _, id := range u.NotifPrefs.MutedSensorIDs {
				muted[id] = true
			}
			out = append(out, push.UserPrefs{
				ID:            u.ID,
				SensorAlerts:  u.NotifPrefs.SensorAlerts,
				StateChanges:  u.NotifPrefs.StateChanges,
				ScheduleFired: u.NotifPrefs.ScheduleFired,
				DeviceOffline: u.NotifPrefs.DeviceOffline,
				QuietHours:    u.NotifPrefs.QuietHours,
				QuietStart:    u.NotifPrefs.QuietStart,
				QuietEnd:      u.NotifPrefs.QuietEnd,
				MutedIDs:      muted,
			})
		}
		return out
	})
}
