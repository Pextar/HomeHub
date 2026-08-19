package announce

// Service is the announcer's runtime: the clip host, and the rule that the
// house is announced to one sentence at a time.
//
// Both are here rather than in the HTTP layer because both outlive a request.
// A clip is published for a couple of minutes so speakers can fetch it, and
// the house stays claimed for as long as the audio is *audible* — several
// seconds after the request that started it has been answered.

import (
	"net/http"
	"sync"
)

// Service owns the published clips and the one-at-a-time claim.
//
// The zero value is not usable: BaseURL has to be supplied. Everything else
// is built on first use, because a house that never announces should not have
// an HTTP host sitting in it.
type Service struct {
	// BaseURL returns the address speakers should fetch clips from, or empty
	// when this server has none they can reach. It is a function rather than
	// a string because the answer depends on a speaker being registered,
	// which may not be true when the service is built.
	BaseURL func() string

	// PathPrefix is where Handler is mounted.
	PathPrefix string

	mu   sync.Mutex
	host *Host
	// busy is held for as long as an announcement is audible, not just for
	// as long as its request is. See Begin.
	busy bool
}

// Host returns the clip host, creating it on first use, or nil when this
// server has no address the speakers can reach.
//
// It shares the audio stream's address discovery — the requirement is
// identical, and solving it twice would mean two ways to be wrong on a
// multi-homed box.
func (s *Service) Host() *Host {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.host != nil {
		return s.host
	}
	if s.BaseURL == nil {
		return nil
	}
	base := s.BaseURL()
	if base == "" {
		return nil
	}
	s.host = &Host{BaseURL: base, PathPrefix: s.PathPrefix}
	return s.host
}

// Handler serves the published clips to speakers.
//
// It resolves the host per request rather than capturing it when routes are
// built, because the host is created on first announcement and the routes are
// built at startup. With nothing published there is nothing to serve, and a
// 404 is the honest answer — the same one an expired clip id gets.
func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		host := s.host
		s.mu.Unlock()
		if host == nil {
			http.NotFound(w, r)
			return
		}
		host.Handler().ServeHTTP(w, r)
	})
}

// Begin claims the house for one announcement, reporting false when another
// is still playing. End releases it.
//
// The claim covers the audio rather than the request. A second announcement
// starting mid-clip would snapshot the *clip* as what each room was doing, and
// then "restore" every room to a dead clip URL at announcement volume, with
// the music gone for good.
func (s *Service) Begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return false
	}
	s.busy = true
	return true
}

// End releases the claim taken by Begin.
func (s *Service) End() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}
