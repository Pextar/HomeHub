package api

import (
	"testing"
	"time"
)

func TestLoginLimiter_LocksOutAfterThreshold(t *testing.T) {
	now := time.Now()
	l := newLoginLimiter()
	l.now = func() time.Time { return now }

	for i := 0; i < maxLoginFailures-1; i++ {
		l.recordFailure("1.2.3.4")
		if ok, _ := l.allowed("1.2.3.4"); !ok {
			t.Fatalf("locked out early after %d failures", i+1)
		}
	}
	l.recordFailure("1.2.3.4") // crosses the threshold
	ok, retryAfter := l.allowed("1.2.3.4")
	if ok {
		t.Fatal("expected lockout after crossing threshold")
	}
	if retryAfter <= 0 || retryAfter > loginLockout {
		t.Errorf("retryAfter = %v, want within (0, %v]", retryAfter, loginLockout)
	}
}

func TestLoginLimiter_SuccessClearsFailures(t *testing.T) {
	now := time.Now()
	l := newLoginLimiter()
	l.now = func() time.Time { return now }

	for i := 0; i < maxLoginFailures-1; i++ {
		l.recordFailure("1.2.3.4")
	}
	l.recordSuccess("1.2.3.4")
	for i := 0; i < maxLoginFailures-1; i++ {
		l.recordFailure("1.2.3.4")
		if ok, _ := l.allowed("1.2.3.4"); !ok {
			t.Fatalf("lockout triggered too early; counter wasn't cleared by success")
		}
	}
}

func TestLoginLimiter_LockoutExpires(t *testing.T) {
	now := time.Now()
	l := newLoginLimiter()
	l.now = func() time.Time { return now }

	for i := 0; i < maxLoginFailures; i++ {
		l.recordFailure("1.2.3.4")
	}
	if ok, _ := l.allowed("1.2.3.4"); ok {
		t.Fatal("expected lockout")
	}
	now = now.Add(loginLockout + time.Second)
	if ok, _ := l.allowed("1.2.3.4"); !ok {
		t.Fatal("expected lockout to expire")
	}
}

func TestLoginLimiter_WindowResetsBeforeLockout(t *testing.T) {
	now := time.Now()
	l := newLoginLimiter()
	l.now = func() time.Time { return now }

	// A handful of failures, then a long gap: the window expires so the
	// next failure starts a fresh count and never trips the lockout.
	for i := 0; i < maxLoginFailures-1; i++ {
		l.recordFailure("1.2.3.4")
	}
	now = now.Add(loginWindow + time.Minute)
	l.recordFailure("1.2.3.4")
	if ok, _ := l.allowed("1.2.3.4"); !ok {
		t.Fatal("stale failures should not contribute to a lockout")
	}
}

func TestLoginLimiter_PerKeyIsolation(t *testing.T) {
	now := time.Now()
	l := newLoginLimiter()
	l.now = func() time.Time { return now }

	for i := 0; i < maxLoginFailures; i++ {
		l.recordFailure("1.2.3.4")
	}
	if ok, _ := l.allowed("5.6.7.8"); !ok {
		t.Fatal("one IP's lockout must not affect another")
	}
}
