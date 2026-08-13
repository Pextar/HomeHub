package api

// Live zone playbacks, and the pieces of the server that own them.
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

import (
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"homehub/internal/airplay"
	"homehub/internal/media"
	"homehub/internal/mediabridge"
	"homehub/internal/store"
	"homehub/internal/stream"
)

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

// CloseMedia releases every live zone playback and stops the decoder. Called
// at shutdown: the stream route holds the account's Spotify session, and
// leaving it behind would keep the user's Spotify pointed at a HomeHub that
// has stopped serving audio.
func (s *Server) CloseMedia() {
	s.zoneMu.Lock()
	sessions := s.zoneSessions
	s.zoneSessions = map[string]*media.Session{}
	s.zoneMu.Unlock()
	for _, sess := range sessions {
		sess.Close()
	}

	s.streamMu.Lock()
	decoder := s.librespot
	s.streamMu.Unlock()
	if decoder != nil {
		if err := decoder.Close(); err != nil {
			log.Printf("media: stopping the decoder: %v", err)
		}
	}

	// A cast is the one session that keeps *sending* after HomeHub stops
	// serving: a receiver holds no state to notice the silence, so it would
	// sit on an open RTSP session that nothing will ever feed.
	s.casterMu.Lock()
	caster := s.caster
	s.casterMu.Unlock()
	if caster != nil {
		caster.Close()
	}
}

// streamHost returns the HTTP stream host, creating it on first use.
//
// BaseURL has to be an address the *speakers* can reach, which on a
// multi-homed host is not just any local address — the same problem the Sonos
// event callback already solves, so it is solved the same way.
func (s *Server) streamHost() media.StreamHost {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.stream != nil {
		return s.stream
	}
	base := s.streamBaseURL()
	if base == "" {
		// No reachable address: return nil rather than a host that would
		// hand speakers a URL they can't fetch. The media layer reports
		// "no stream host configured", which is the truth.
		return nil
	}
	s.stream = stream.NewHost(stream.Config{
		BaseURL:     base,
		PathPrefix:  streamPath,
		StartDelays: streamStartDelays(),
		Logf:        log.Printf,
	})
	return s.stream
}

// streamPath is where the stream handler is mounted. Outside /api on purpose:
// the clients are speakers, and everything under /api is session-gated.
const streamPath = "/stream"

// streamHandler serves the audio streams.
//
// It resolves the host per request rather than capturing it at route-building
// time, because the host is created on first play and the routes are built at
// startup. Before anything has been published there is nothing to serve, and a
// 404 is the honest answer — the same one an expired stream id gets.
func (s *Server) streamHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.streamMu.Lock()
		host := s.stream
		s.streamMu.Unlock()
		if host == nil {
			http.NotFound(w, r)
			return
		}
		host.Handler().ServeHTTP(w, r)
	})
}

// streamBaseURL is the plain-HTTP address speakers should fetch from.
//
// Plain HTTP, not HTTPS, and not negotiable: speakers will not present a
// client certificate and many will not accept a self-signed one, which is the
// same reason Sonos event callbacks use the HTTP listener even when TLS is up.
//
// Finding the right address is the same multi-homed problem sonosCallbackURL
// solves, with one difference that matters: a stream has a single URL serving
// every listener, so it cannot be resolved per speaker. The address is
// resolved toward a registered speaker, which gives the LAN-facing interface
// — correct whenever the speakers share a subnet, which is the normal case.
// A setup where they don't can set HOMEHUB_STREAM_URL explicitly.
func (s *Server) streamBaseURL() string {
	if env := strings.TrimSpace(os.Getenv("HOMEHUB_STREAM_URL")); env != "" {
		return strings.TrimRight(env, "/")
	}
	speaker := s.anySpeakerIP()
	if speaker == "" {
		return "" // nothing registered yet; nothing to stream to either
	}
	local, err := localAddrFor(speaker)
	if err != nil {
		log.Printf("media: no local address can reach %s: %v", speaker, err)
		return ""
	}
	port := s.HTTPPort
	if port == "" {
		port = "8080"
	}
	return "http://" + net.JoinHostPort(local, port)
}

// anySpeakerIP returns the address of some registered speaker, for working
// out which of our interfaces faces the LAN.
func (s *Server) anySpeakerIP() string {
	return store.ViewValue(s.Store, func() string {
		for _, sp := range s.Store.Sonos {
			if sp.IP != "" {
				return sp.IP
			}
		}
		for _, sp := range s.Store.KEF {
			if sp.IP != "" {
				return sp.IP
			}
		}
		for _, sp := range s.Store.AirPlay {
			if sp.IP != "" {
				return sp.IP
			}
		}
		return ""
	})
}

// streamStartDelays reads per-vendor start compensation from the environment.
//
// Empty by default. The right values depend on the speakers, the network and
// the firmware; guessing them would be worse than leaving a zone a few hundred
// milliseconds apart for someone who can actually hear it to tune. See
// docs/MEDIA-PROTOCOL.md.
func streamStartDelays() map[media.Vendor]time.Duration {
	out := map[media.Vendor]time.Duration{}
	for _, v := range []struct {
		vendor media.Vendor
		env    string
	}{
		{media.VendorSonos, "HOMEHUB_STREAM_DELAY_SONOS"},
		{media.VendorKEF, "HOMEHUB_STREAM_DELAY_KEF"},
	} {
		raw := strings.TrimSpace(os.Getenv(v.env))
		if raw == "" {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 {
			log.Printf("media: ignoring %s=%q: not a valid duration", v.env, raw)
			continue
		}
		out[v.vendor] = d
	}
	return out
}

// decoder returns the Spotify decoder, creating it on first use. Nil-safe
// everywhere downstream: with librespot absent it reports why, and only the
// routes HomeHub decodes for are affected.
//
// The bitrate is baked into the process's command line, so a household that
// changes its stream quality needs a new one. Rebuilding here rather than at
// the moment the setting changes keeps that decision in one place, and means
// the running decode is not cut off mid-song by a settings save: the change
// lands on the next thing started.
func (s *Server) decoder() mediabridge.Decoder {
	bitrate := s.streamQuality().Bitrate()

	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.librespot != nil && s.librespotBitrate == bitrate {
		return s.librespot
	}
	if s.librespot != nil {
		if err := s.librespot.Close(); err != nil {
			log.Printf("media: stopping the old decoder: %v", err)
		}
	}
	s.librespot = stream.NewLibrespot(stream.LibrespotConfig{
		Binary:     strings.TrimSpace(os.Getenv("HOMEHUB_LIBRESPOT_BIN")),
		DeviceName: strings.TrimSpace(os.Getenv("HOMEHUB_LIBRESPOT_NAME")),
		CacheDir:   librespotCache(s.Store.DataDir),
		Bitrate:    bitrate,
		Logf:       log.Printf,
	})
	s.librespotBitrate = bitrate
	return s.librespot
}

// streamQuality is the household's chosen decode quality, defaulted.
func (s *Server) streamQuality() media.StreamQuality {
	var q media.StreamQuality
	s.Store.View(func() {
		if s.Store.Settings != nil {
			q = media.StreamQuality(s.Store.Settings.StreamQuality)
		}
	})
	return q.Normalize()
}

// airplayCaster returns the AirPlay sender, creating it on first use.
//
// Its own mutex rather than streamMu: this is reached from inside
// s.endpoints(), which runs under the store lock, and sharing a mutex with the
// decoder — which reads settings under that same lock — would put two locks in
// two orders. The caster's construction touches nothing but itself.
func (s *Server) airplayCaster() *airplay.Caster {
	s.casterMu.Lock()
	defer s.casterMu.Unlock()
	if s.caster == nil {
		s.caster = airplay.NewCaster(log.Printf)
	}
	return s.caster
}

// mediaDeps is everything executing a plan needs from the server. One place,
// so a route added to the media layer cannot be half-wired: a call site that
// forgets the AirPlay host would fail at the moment someone plays to a
// receiver, which is the worst time to find out.
func (s *Server) mediaDeps() media.Deps {
	return media.Deps{
		Stream:  s.streamHost(),
		AirPlay: s.airplayCaster(),
		Logf:    s.mediaLogf,
	}
}

// librespotCache is where librespot keeps its credentials and audio cache,
// which is what makes the second start much faster than the first.
func librespotCache(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	return dataDir + "/librespot"
}
