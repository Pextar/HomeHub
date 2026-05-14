package store

import (
	"sync"
	"time"
)

// ActivityEntry is one row of the recent-events log. It is intentionally
// flat (no nested socket/scene/etc. structs) so the frontend can render
// the row without joining other tables.
type ActivityEntry struct {
	ID     int64     `json:"id"`
	Time   time.Time `json:"time"`
	Kind   string    `json:"kind"`            // "socket" | "group" | "scene" | "room" | "bulk"
	Source string    `json:"source"`          // "manual" | "schedule" | "timer"
	Action string    `json:"action"`          // "on" | "off" | "toggle" | "activate"
	Label  string    `json:"label"`           // human-readable target name
	Status string    `json:"status"`          // "ok" | "error"
	Error  string    `json:"error,omitempty"` // populated when status="error"
}

// ActivityLog is a thread-safe ring buffer of recent ActivityEntries.
// Entries are not persisted: restarts wipe the log, which is fine — this
// is a "what just happened" debugging view, not an audit trail.
type ActivityLog struct {
	mu     sync.Mutex
	buf    []ActivityEntry
	nextID int64
	cap    int
}

// NewActivityLog returns a log that retains the most recent `capacity` entries.
func NewActivityLog(capacity int) *ActivityLog {
	if capacity <= 0 {
		capacity = 200
	}
	return &ActivityLog{cap: capacity}
}

// Add appends an entry. ID and Time are filled in if zero. Status
// defaults to "ok".
func (a *ActivityLog) Add(e ActivityEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nextID++
	e.ID = a.nextID
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if e.Status == "" {
		e.Status = "ok"
	}
	a.buf = append(a.buf, e)
	if len(a.buf) > a.cap {
		a.buf = a.buf[len(a.buf)-a.cap:]
	}
}

// Recent returns up to n most-recent entries, newest first. n<=0 returns all.
func (a *ActivityLog) Recent(n int) []ActivityEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	total := len(a.buf)
	if n <= 0 || n > total {
		n = total
	}
	out := make([]ActivityEntry, 0, n)
	for i := total - 1; i >= total-n; i-- {
		out = append(out, a.buf[i])
	}
	return out
}
