package api

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Matter commissioning runs in the background and is polled by the
// frontend because the operation can take 30–90s — far longer than
// the HTTP server's WriteTimeout, and longer than iOS Safari is
// willing to keep a single fetch alive. The handler returns a job
// id immediately; the frontend polls /commission/jobs/{id} until
// the job reaches "done" or "error".

type commissionStatus string

const (
	jobPending commissionStatus = "pending"
	jobDone    commissionStatus = "done"
	jobError   commissionStatus = "error"
)

type commissionJob struct {
	ID        string           `json:"id"`
	Status    commissionStatus `json:"status"`
	NodeID    string           `json:"node_id,omitempty"`
	Error     string           `json:"error,omitempty"`
	StartedAt time.Time        `json:"started_at"`
	EndedAt   *time.Time       `json:"ended_at,omitempty"`
}

type commissionJobs struct {
	mu   sync.Mutex
	jobs map[string]*commissionJob
}

func newCommissionJobs() *commissionJobs {
	return &commissionJobs{jobs: make(map[string]*commissionJob)}
}

func (c *commissionJobs) create() *commissionJob {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Opportunistic GC on each create; jobs are rare and the registry
	// is tiny, so a full scan is fine.
	c.gcLocked(15 * time.Minute)
	id := newJobID()
	j := &commissionJob{ID: id, Status: jobPending, StartedAt: time.Now()}
	c.jobs[id] = j
	return j
}

func (c *commissionJobs) get(id string) (commissionJob, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	j, ok := c.jobs[id]
	if !ok {
		return commissionJob{}, false
	}
	return *j, true // return a copy so callers can't mutate registry
}

func (c *commissionJobs) complete(id, nodeID string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	j, ok := c.jobs[id]
	if !ok {
		return
	}
	now := time.Now()
	j.EndedAt = &now
	if err != nil {
		j.Status = jobError
		j.Error = err.Error()
		return
	}
	j.Status = jobDone
	j.NodeID = nodeID
}

func (c *commissionJobs) gcLocked(retention time.Duration) {
	cutoff := time.Now().Add(-retention)
	for id, j := range c.jobs {
		if j.EndedAt != nil && j.EndedAt.Before(cutoff) {
			delete(c.jobs, id)
		}
	}
}

func newJobID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
