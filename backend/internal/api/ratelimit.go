package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Login brute-force throttling. Login codes are short numeric strings, so
// without a brake an attacker on the network could exhaust the keyspace.
// We count failures per client IP and lock that IP out once it crosses the
// threshold inside a rolling window. Successful logins clear the counter.
//
// The per-IP limit alone is bypassable by rotating source addresses (an
// IPv6 /64 hands an attacker billions), so a second counter under
// globalLoginKey caps total failures across all IPs. Tripping it pauses
// logins for everyone for the lockout window — for a home hub that beats
// letting a distributed guesser keep chipping at 6-digit codes, and
// existing sessions keep working throughout.
const (
	maxLoginFailures = 10
	loginWindow      = 15 * time.Minute
	loginLockout     = 15 * time.Minute

	globalLoginKey         = "\x00global" // NUL prefix can't collide with an IP
	maxGlobalLoginFailures = 50
)

type loginAttempts struct {
	failures    int
	windowEnd   time.Time
	lockedUntil time.Time
}

type loginLimiter struct {
	mu  sync.Mutex
	by  map[string]*loginAttempts
	now func() time.Time // injectable for tests
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{by: make(map[string]*loginAttempts), now: time.Now}
}

// allowed reports whether key may attempt a login now. When locked out it
// returns the remaining cooldown so the caller can set Retry-After.
func (l *loginLimiter) allowed(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.by[key]
	if a == nil {
		return true, 0
	}
	now := l.now()
	if now.Before(a.lockedUntil) {
		return false, a.lockedUntil.Sub(now)
	}
	return true, 0
}

// recordFailure registers a failed attempt for key and locks it out once it
// crosses the threshold within the window. Every failure also counts toward
// the cross-IP global counter (see globalLoginKey).
func (l *loginLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bump(key, maxLoginFailures)
	if key != globalLoginKey {
		l.bump(globalLoginKey, maxGlobalLoginFailures)
	}
}

// bump increments the failure count for key, locking it out at max.
// Caller must hold l.mu.
func (l *loginLimiter) bump(key string, max int) {
	now := l.now()
	a := l.by[key]
	if a == nil || now.After(a.windowEnd) {
		a = &loginAttempts{windowEnd: now.Add(loginWindow)}
		l.by[key] = a
	}
	a.failures++
	if a.failures >= max {
		a.lockedUntil = now.Add(loginLockout)
		a.failures = 0
		a.windowEnd = now.Add(loginLockout)
	}
}

// recordSuccess clears any tracked failures for key. The global counter is
// left untouched — a distributed guesser shouldn't get its budget refilled
// by an unrelated successful login.
func (l *loginLimiter) recordSuccess(key string) {
	l.mu.Lock()
	delete(l.by, key)
	l.mu.Unlock()
}

// clientIP returns the remote IP for rate-limiting purposes. It uses the
// transport-level RemoteAddr rather than X-Forwarded-For, which a client
// can forge to dodge the limiter.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
