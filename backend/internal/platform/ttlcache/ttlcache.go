// Package ttlcache is a small read-through cache for answers that cost a
// round trip and change slowly.
//
// It exists for lookups where the honest fetch is correct but wasteful:
// which streaming account a speaker plays through, where it publishes a
// picture of itself. Those move when the household changes something, which is
// to say almost never, and asking on every render turns one tap into several
// requests to the speaker.
package ttlcache

import (
	"sync"
	"time"
)

// Cache holds values for a fixed lifetime.
//
// It deliberately has no single-flight: two callers that miss at the same
// moment both fetch. Holding a lock across a network call to save the second
// fetch would make every reader wait on the slowest speaker, which is the
// worse trade for the traffic this exists to avoid.
type Cache[K comparable, V any] struct {
	ttl time.Duration

	mu      sync.Mutex
	entries map[K]entry[V]
}

type entry[V any] struct {
	val V
	at  time.Time
}

// New returns a cache whose entries live for ttl.
func New[K comparable, V any](ttl time.Duration) *Cache[K, V] {
	return &Cache[K, V]{ttl: ttl, entries: make(map[K]entry[V])}
}

// Do returns the value for key, calling fill when there is none or the one
// held has expired.
//
// A failed fill is not cached: the next caller tries again. An empty *result*
// is, because "this speaker publishes no picture" is an answer, and re-asking
// for it on every render is exactly what this is here to stop.
func (c *Cache[K, V]) Do(key K, fill func() (V, error)) (V, error) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok && time.Since(e.at) < c.ttl {
		c.mu.Unlock()
		return e.val, nil
	}
	c.mu.Unlock()

	val, err := fill()
	if err != nil {
		var zero V
		return zero, err
	}

	c.mu.Lock()
	c.entries[key] = entry[V]{val: val, at: time.Now()}
	c.mu.Unlock()
	return val, nil
}
