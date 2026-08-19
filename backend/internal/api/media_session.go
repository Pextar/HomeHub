package api

// Live zone playbacks.
//
// Most routes leave nothing running: the speakers hold the content and HomeHub
// is out of the loop the moment the command lands. The stream route is the
// exception — it owns a decoder holding the account's Spotify session and an
// HTTP stream several speakers are pulling from — so something has to remember
// it and shut it down.
//
// That "something" is deliberately small: a map of zone id to session, and the
// rule that starting a new playback in a zone ends the old one. It is not a
// playback state machine. What is playing is read from the speakers, which are
// the only honest source for it — a user pausing from the Sonos app must not
// leave HomeHub reporting otherwise.
//
// The pieces that hold live sound — the stream host, the decoders, the AirPlay
// caster — live in internal/audio. This file only decides when a session
// starts and ends.

import (
	"homehub/internal/media"
)

// StreamPath is where the stream handler is mounted. Outside /api on purpose:
// the clients are speakers, and everything under /api is session-gated.
const StreamPath = "/stream"

// setZoneSession records a zone's live session, ending whatever it replaces.
// Starting something new in a room means the old thing stopped.
func (s *Server) setZoneSession(zoneID string, sess *media.Session) {
	s.zoneMu.Lock()
	old := s.zoneSessions[zoneID]
	if s.zoneSessions == nil {
		s.zoneSessions = map[string]*media.Session{}
	}
	s.zoneSessions[zoneID] = sess
	s.zoneMu.Unlock()

	// Closed off-lock: releasing a stream disconnects listeners and stops a
	// subprocess, neither of which should happen under a mutex other
	// requests are waiting on.
	old.Close()
}

// endZoneSession releases a zone's session, if it has one.
func (s *Server) endZoneSession(zoneID string) {
	s.zoneMu.Lock()
	sess := s.zoneSessions[zoneID]
	delete(s.zoneSessions, zoneID)
	s.zoneMu.Unlock()
	sess.Close()
}

// zonePlan returns the plan a zone's transport commands should follow.
//
// A live session knows which route it started on, and transport has to match:
// a natively grouped zone is addressed through its coordinator, while a zone
// HomeHub is feeding — streamed or cast over AirPlay — has no coordinator and
// every speaker is addressed. Those two are one case here because they are
// one case for transport: `Plan.Endpoints()` returns the targets for both.
// With no session — after a restart, or for speakers someone started from a
// vendor app — every speaker is addressed, which is correct if noisier than
// necessary.
func (s *Server) zonePlan(zoneID string, members []media.Endpoint) *media.Plan {
	s.zoneMu.Lock()
	sess := s.zoneSessions[zoneID]
	s.zoneMu.Unlock()

	if sess != nil && (sess.Route == media.RouteGroup || sess.Route == media.RouteNative) {
		return &media.Plan{Route: sess.Route, Coordinator: members[0], Followers: members[1:]}
	}
	return &media.Plan{Route: media.RouteStream, Targets: members}
}

// CloseMedia releases every live zone playback and stops the audio engine.
// Called at shutdown: the stream route holds the account's Spotify session,
// and leaving it behind would keep the user's Spotify pointed at a HomeHub
// that has stopped serving audio.
func (s *Server) CloseMedia() {
	s.zoneMu.Lock()
	sessions := s.zoneSessions
	s.zoneSessions = map[string]*media.Session{}
	s.zoneMu.Unlock()
	for _, sess := range sessions {
		sess.Close()
	}
	s.Audio.Close()
}
