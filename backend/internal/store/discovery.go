package store

import (
	"sort"
	"time"
)

// DiscoveryCandidate is one unknown 433MHz emitter heard while pairing.
// Code is "<model>:<id>" (or just <model>/<id> if the other is empty),
// matching the form a user would later put in Sensor.Code.
type DiscoveryCandidate struct {
	Protocol  string             `json:"protocol"`
	Code      string             `json:"code"`
	Fields    map[string]float64 `json:"fields"`
	Count     int                `json:"count"`
	FirstSeen time.Time          `json:"first_seen"`
	LastSeen  time.Time          `json:"last_seen"`
}

// Discovery is the in-memory state for the "pair a sensor" flow: while
// active, any incoming packet that doesn't match an existing sensor is
// recorded as a candidate the user can adopt.
type Discovery struct {
	Until      time.Time
	Candidates map[string]*DiscoveryCandidate // keyed by Code
}

// StartDiscovery opens (or extends) the pair window and clears any
// candidates from previous sessions. Caller must hold Mu.
func (s *Store) StartDiscovery(d time.Duration) time.Time {
	s.Discovery.Until = time.Now().Add(d)
	s.Discovery.Candidates = make(map[string]*DiscoveryCandidate)
	return s.Discovery.Until
}

// DiscoveryActive reports whether the pair window is currently open.
// Caller must hold Mu (read lock is fine).
func (s *Store) DiscoveryActive() bool {
	return time.Now().Before(s.Discovery.Until)
}

// RecordCandidate is called by the RX listener for every packet seen
// while the pair window is open and no existing sensor claimed it.
// Caller must hold Mu (write lock).
func (s *Store) RecordCandidate(protocol, code string, fields map[string]float64) {
	if !s.DiscoveryActive() || code == "" {
		return
	}
	now := time.Now().UTC()
	c, ok := s.Discovery.Candidates[code]
	if !ok {
		c = &DiscoveryCandidate{
			Protocol:  protocol,
			Code:      code,
			Fields:    make(map[string]float64),
			FirstSeen: now,
		}
		s.Discovery.Candidates[code] = c
	}
	c.LastSeen = now
	c.Count++
	for k, v := range fields {
		c.Fields[k] = v
	}
}

// DiscoverySnapshot returns the current pair-window state as a value
// safe to return from an HTTP handler. Caller must hold Mu (read lock).
func (s *Store) DiscoverySnapshot() (active bool, until time.Time, candidates []*DiscoveryCandidate) {
	active = s.DiscoveryActive()
	until = s.Discovery.Until
	candidates = make([]*DiscoveryCandidate, 0, len(s.Discovery.Candidates))
	for _, c := range s.Discovery.Candidates {
		candidates = append(candidates, c)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Count != candidates[j].Count {
			return candidates[i].Count > candidates[j].Count
		}
		return candidates[i].Code < candidates[j].Code
	})
	return
}
