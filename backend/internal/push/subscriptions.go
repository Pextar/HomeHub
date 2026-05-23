package push

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const subsFile = "push_subscriptions.json"

// PushSubscription mirrors the PushSubscriptionJSON shape from the browser's
// PushManager.subscribe() → sub.toJSON() call.
type PushSubscription struct {
	ID       string    `json:"id"`
	UserID   string    `json:"user_id"`
	Endpoint string    `json:"endpoint"`
	Keys     SubKeys   `json:"keys"`
	Created  time.Time `json:"created_at"`
}

// SubKeys holds the cryptographic keys the browser generated for this
// subscription. These are required to encrypt push message payloads.
type SubKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// SubscriptionStore persists push subscriptions to disk and provides
// in-memory access. It is safe for concurrent use.
type SubscriptionStore struct {
	mu      sync.RWMutex
	dataDir string
	subs    map[string]*PushSubscription // keyed by ID
}

// NewSubscriptionStore loads existing subscriptions from disk (or starts
// empty) and returns a ready-to-use store.
func NewSubscriptionStore(dataDir string) (*SubscriptionStore, error) {
	s := &SubscriptionStore{
		dataDir: dataDir,
		subs:    make(map[string]*PushSubscription),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Add saves a new subscription (or replaces an existing one for the same
// endpoint). Persists immediately.
func (s *SubscriptionStore) Add(sub PushSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deduplicate: if a subscription for this endpoint already exists (e.g.
	// the user re-subscribed from the same browser), replace it in place so
	// we don't accumulate stale entries.
	for _, existing := range s.subs {
		if existing.Endpoint == sub.Endpoint {
			sub.ID = existing.ID
			sub.Created = existing.Created
			s.subs[sub.ID] = &sub
			return s.save()
		}
	}

	if sub.ID == "" {
		sub.ID = fmt.Sprintf("sub_%d", time.Now().UnixNano())
	}
	if sub.Created.IsZero() {
		sub.Created = time.Now()
	}
	s.subs[sub.ID] = &sub
	return s.save()
}

// Remove deletes the subscription with the given endpoint. A no-op if it
// does not exist. Persists immediately.
func (s *SubscriptionStore) Remove(endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sub := range s.subs {
		if sub.Endpoint == endpoint {
			delete(s.subs, id)
			return s.save()
		}
	}
	return nil
}

// RemoveByID deletes the subscription with the given ID. Used when the push
// service receives a 410 Gone response (subscription expired server-side).
func (s *SubscriptionStore) RemoveByID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[id]; !ok {
		return nil
	}
	delete(s.subs, id)
	return s.save()
}

// GetAll returns a snapshot of every subscription.
func (s *SubscriptionStore) GetAll() []PushSubscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PushSubscription, 0, len(s.subs))
	for _, sub := range s.subs {
		out = append(out, *sub)
	}
	return out
}

// GetByUser returns all subscriptions for the given user ID.
func (s *SubscriptionStore) GetByUser(userID string) []PushSubscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []PushSubscription
	for _, sub := range s.subs {
		if sub.UserID == userID {
			out = append(out, *sub)
		}
	}
	return out
}

// — persistence helpers —

func (s *SubscriptionStore) load() error {
	path := filepath.Join(s.dataDir, subsFile)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	var list []*PushSubscription
	if err := json.NewDecoder(f).Decode(&list); err != nil {
		return err
	}
	for _, sub := range list {
		s.subs[sub.ID] = sub
	}
	return nil
}

func (s *SubscriptionStore) save() error {
	list := make([]*PushSubscription, 0, len(s.subs))
	for _, sub := range s.subs {
		list = append(list, sub)
	}
	return writeVAPIDJSON(filepath.Join(s.dataDir, subsFile), list)
}
