package ttlcache

import (
	"errors"
	"testing"
	"time"
)

func TestSecondCallIsServedFromTheCache(t *testing.T) {
	c := New[string, int](time.Minute)
	calls := 0
	fill := func() (int, error) { calls++; return 7, nil }

	for i := 0; i < 3; i++ {
		got, err := c.Do("k", fill)
		if err != nil || got != 7 {
			t.Fatalf("Do() = %d, %v", got, err)
		}
	}
	if calls != 1 {
		t.Errorf("fill ran %d times, want 1", calls)
	}
}

func TestKeysDoNotShareEntries(t *testing.T) {
	c := New[string, string](time.Minute)
	a, _ := c.Do("a", func() (string, error) { return "first", nil })
	b, _ := c.Do("b", func() (string, error) { return "second", nil })
	if a != "first" || b != "second" {
		t.Errorf("got %q and %q, want the two distinct answers", a, b)
	}
}

func TestAnExpiredEntryIsRefetched(t *testing.T) {
	c := New[string, int](time.Nanosecond)
	calls := 0
	fill := func() (int, error) { calls++; return calls, nil }

	_, _ = c.Do("k", fill)
	time.Sleep(time.Millisecond)
	got, _ := c.Do("k", fill)
	if got != 2 {
		t.Errorf("Do() = %d after expiry, want the second fill's answer", got)
	}
}

// A failure has to stay uncached, or one unreachable moment would be repeated
// back to the household for the whole TTL.
func TestAFailedFillIsNotCached(t *testing.T) {
	c := New[string, int](time.Minute)
	boom := errors.New("speaker asleep")
	if _, err := c.Do("k", func() (int, error) { return 0, boom }); !errors.Is(err, boom) {
		t.Fatalf("Do() = %v, want the fill's error", err)
	}
	got, err := c.Do("k", func() (int, error) { return 42, nil })
	if err != nil || got != 42 {
		t.Errorf("Do() = %d, %v after a failure, want the retry's answer", got, err)
	}
}

// An empty answer is still an answer. "This speaker publishes no picture" is
// exactly the result that must not be re-asked on every render.
func TestAnEmptyResultIsCached(t *testing.T) {
	c := New[string, string](time.Minute)
	calls := 0
	fill := func() (string, error) { calls++; return "", nil }

	_, _ = c.Do("k", fill)
	_, _ = c.Do("k", fill)
	if calls != 1 {
		t.Errorf("fill ran %d times for an empty answer, want 1", calls)
	}
}
