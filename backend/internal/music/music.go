// Package music is HomeHub's music service: where a household means when it
// names a room, what can play there, and what stays running once it does.
//
// It sits between three neighbours and belongs to none of them. internal/media
// is the vendor-neutral protocol — routes, plans, quality — and knows nothing
// about this house. internal/mediabridge adapts one make of speaker to that
// protocol. internal/store holds which speakers exist and how the household
// arranged them. This package is what joins them: it turns "zone:kitchen" into
// live endpoints, decides which provider serves a URI, and remembers the one
// kind of playback that leaves something running.
//
// Everything above it — HTTP handlers, the sleep timer, autoplay, a scene's
// music step — asks the same questions here rather than each assembling the
// answer from the three packages below.
package music

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"homehub/internal/audio"
	"homehub/internal/media"
	"homehub/internal/mediabridge"
	"homehub/internal/qobuz"
	"homehub/internal/speakermon"
	"homehub/internal/spotify"
	"homehub/internal/store"
)

// Timeout caps one operation against a room. Generous because the stream route
// can involve waking a speaker, a cloud round trip and several UPnP calls
// before anything comes out.
const Timeout = 45 * time.Second

// kefStreamSettle is how long after a command a streamed KEF is re-read a
// second time. The audio comes back to it over the network, so it takes a
// moment to actually start, and a single immediate poll would catch it still
// silent and report so.
const kefStreamSettle = 3 * time.Second

// Config is what the service needs from the rest of the application.
type Config struct {
	// Store holds the registered speakers and the household's zones.
	Store *store.Store
	// Speakers is the cached view of what each of them is doing.
	Speakers *speakermon.Monitors
	// Audio holds the stream, the decoders and the AirPlay sender.
	Audio *audio.Engine

	// Spotify and Qobuz are the catalogues. Either may be nil, which makes
	// the matching provider report itself unconfigured rather than fail.
	Spotify *spotify.Client
	Qobuz   *qobuz.Client

	// CancelFade stops a volume ramp in flight on a room, if there is one.
	//
	// A hook rather than a dependency because the thing that owns ramps —
	// internal/musictimer — is built on top of this package and cannot be
	// imported from it. What it is here for: a scene setting a room's volume
	// would otherwise be fought by a sleep fade still walking it, and the
	// fade would win, because it writes every few seconds.
	CancelFade func(room string) bool

	Logf func(format string, args ...any)
}

// Service resolves rooms and providers, and owns the live sessions.
type Service struct {
	cfg Config

	// sessions tracks live zone playbacks, keyed by zone id.
	//
	// It is deliberately small: a map, and the rule that starting something
	// new in a zone ends what was there. It is not a playback state machine.
	// What is playing is read from the speakers, which are the only honest
	// source for it — someone pausing from the Sonos app must not leave
	// HomeHub reporting otherwise.
	sessionMu sync.Mutex
	sessions  map[string]*media.Session
}

// New returns a service. It starts nothing.
func New(cfg Config) *Service {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.CancelFade == nil {
		cfg.CancelFade = func(string) bool { return false }
	}
	return &Service{cfg: cfg, sessions: map[string]*media.Session{}}
}

// Deps is what executing a media plan needs: the stream host, the AirPlay
// sender and somewhere to log. Callers that run a plan themselves — a zone
// play, a wake-up timer — pass this rather than assembling it.
func (s *Service) Deps() media.Deps { return s.cfg.Audio.Deps() }

// Quality is what the house decodes at, for callers describing the audio chain.
func (s *Service) Quality() media.StreamQuality { return s.cfg.Audio.Quality() }

// ── Where ────────────────────────────────────────────────────────────────

// Endpoints builds live endpoints for every registered speaker, keyed by the
// qualified id a zone stores. Caller must hold the store's read lock.
//
// The state functions close over the monitors, so this reads from the same
// event-driven and polled caches the vendor views do rather than adding a
// second round of traffic to every speaker.
func (s *Service) Endpoints() map[string]media.Endpoint {
	st := s.cfg.Store
	out := make(map[string]media.Endpoint,
		len(st.Sonos)+len(st.KEF)+len(st.AirPlay)+len(st.UPnP))
	for id, sp := range st.Sonos {
		out[store.QualifySonos(id)] = mediabridge.NewSonosEndpoint(*sp, "", s.cfg.Speakers.SonosState)
	}
	for id, sp := range st.KEF {
		out[store.QualifyKEF(id)] = mediabridge.NewKEFEndpoint(*sp, s.cfg.Speakers.KEFState)
	}
	// A UPnP renderer holds its own transport state, so unlike an AirPlay
	// receiver it is asked rather than inferred — see mediabridge/upnp.go.
	for id, rn := range st.UPnP {
		out[store.QualifyUPnP(id)] = mediabridge.NewUPnPEndpoint(*rn)
	}
	// AirPlay receivers have no monitor to read from — there is nothing on
	// the device to poll — so their state comes from the live cast instead.
	caster := s.cfg.Audio.Caster()
	for id, sp := range st.AirPlay {
		out[store.QualifyAirPlay(id)] = mediabridge.NewAirPlayEndpoint(*sp, caster.Live)
	}
	return out
}

// Room resolves a destination key — "sonos:<id>", "kef:<id>", "airplay:<id>"
// or "zone:<id>" — to live endpoints and the name the house calls it.
//
// This is the vocabulary the play history already uses, and having one
// resolver for it is what lets anything that isn't an HTTP handler address a
// room: a music timer names a room the same way a shelf does, and a single
// speaker is simply a zone of one as far as the route engine is concerned.
//
// Takes and releases the read lock itself, so callers are off-lock by the time
// they touch a speaker.
func (s *Service) Room(key string) ([]media.Endpoint, string, error) {
	key = strings.TrimSpace(key)
	st := s.cfg.Store

	var members []string
	var name string
	st.View(func() {
		if id, ok := strings.CutPrefix(key, "zone:"); ok {
			z, exists := st.Zones[id]
			if !exists {
				return
			}
			name = z.Name
			members = append([]string(nil), z.Members...)
			return
		}
		bridge, id, ok := store.SplitMember(key)
		if !ok {
			return
		}
		switch bridge {
		case "kef":
			if sp, exists := st.KEF[id]; exists {
				name, members = sp.Name, []string{key}
			}
		case "airplay":
			if sp, exists := st.AirPlay[id]; exists {
				name, members = sp.Name, []string{key}
			}
		default:
			if sp, exists := st.Sonos[id]; exists {
				name, members = sp.Name, []string{key}
			}
		}
	})

	if name == "" {
		return nil, "", fmt.Errorf("%w: %q", media.ErrUnknownEndpoint, key)
	}

	var eps map[string]media.Endpoint
	st.View(func() { eps = s.Endpoints() })
	out := make([]media.Endpoint, 0, len(members))
	for _, m := range members {
		if e, exists := eps[m]; exists {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil, name, fmt.Errorf("%w: %s", media.ErrEmptyZone, name)
	}
	return out, name, nil
}

// RoomName is what the house calls a destination key, falling back to the key
// itself so a log row or an activity entry is never blank.
//
// Room returns the same name alongside the endpoints; this is for the callers
// that want to *say* where something happened without resolving whether it can
// still be played to.
func (s *Service) RoomName(key string) string {
	st := s.cfg.Store
	name := store.ViewValue(st, func() string {
		if id, ok := strings.CutPrefix(key, "zone:"); ok {
			if z, exists := st.Zones[id]; exists {
				return z.Name
			}
			return ""
		}
		bridge, id, ok := store.SplitMember(key)
		if !ok {
			return ""
		}
		switch bridge {
		case "kef":
			if sp, exists := st.KEF[id]; exists {
				return sp.Name
			}
		case "airplay":
			if sp, exists := st.AirPlay[id]; exists {
				return sp.Name
			}
		default:
			if sp, exists := st.Sonos[id]; exists {
				return sp.Name
			}
		}
		return ""
	})
	if name == "" {
		return key
	}
	return name
}

// RecordPlay files one play under a destination key.
//
// It belongs to this package because this is the layer that knows a play
// succeeded: every surface that starts music comes through here, and a play the
// speaker refused is not something to offer back from a shelf.
//
// Takes the write lock briefly, then persists off-lock — history is never
// worth failing a play that already happened, so a write error is logged and
// swallowed. Mutate rather than Update: Update pairs a mutation with a full
// Save, and the history has its own file precisely so that starting a song
// does not rewrite every socket in the house.
func (s *Service) RecordPlay(roomKey, roomName string, p store.MediaPlay) {
	if strings.TrimSpace(roomKey) == "" || strings.TrimSpace(p.URI) == "" {
		return
	}
	p.RoomName = roomName
	s.cfg.Store.Mutate(func() { s.cfg.Store.RecordPlay(roomKey, p) })
	if err := s.cfg.Store.SaveHistory(); err != nil {
		s.cfg.Logf("history: %v", err)
	}
}

// Touch asks both monitors to re-read the speakers a command just reached, so
// now-playing updates promptly instead of at the next scheduled poll.
func (s *Service) Touch(eps []media.Endpoint) {
	sonosTouched := false
	for _, e := range eps {
		d := e.Descriptor()
		if d.Vendor == media.VendorKEF {
			s.cfg.Speakers.KEF.Touch(d.ID)
			s.cfg.Speakers.KEF.TouchAfter(d.ID, kefStreamSettle)
			continue
		}
		sonosTouched = true
	}
	if sonosTouched {
		// Sonos is event-driven, so there is nothing per-speaker to poke;
		// a nudge makes the monitor reconcile now.
		s.cfg.Speakers.Sonos.Nudge()
	}
}

// ── What ─────────────────────────────────────────────────────────────────

// Provider returns the media provider for an id. The lookup is by name so that
// adding one is a registration rather than a new branch at every call site.
//
// The empty id still means Spotify. It is the default because it is the
// provider every household has wired up, not because it is the better one — a
// caller that wants lossless asks for it.
func (s *Service) Provider(id string) (media.Provider, error) {
	switch {
	case id == "" || strings.EqualFold(id, "spotify"):
		return s.spotifyProvider(), nil
	case strings.EqualFold(id, "qobuz"):
		return s.qobuzProvider(), nil
	}
	return nil, fmt.Errorf("%w: %q", media.ErrUnknownProvider, id)
}

// Providers is every provider the house knows about, configured or not. The
// unconfigured ones are included on purpose: a UI that lists what could play
// here is more useful than one that silently omits it.
func (s *Service) Providers() []media.Provider {
	return []media.Provider{s.spotifyProvider(), s.qobuzProvider()}
}

func (s *Service) spotifyProvider() media.Provider {
	return mediabridge.NewSpotifyProvider(s.cfg.Spotify, s.cfg.Audio.Decoder(), s.cfg.Audio.Quality())
}

// qobuzProvider wires the lossless provider. The nil check is load-bearing
// rather than defensive: assigning a nil *qobuz.Client straight into the
// interface would produce a non-nil interface holding a nil pointer, and the
// provider's "is it configured" check would pass on its way to a panic.
func (s *Service) qobuzProvider() media.Provider {
	var account mediabridge.QobuzAccount
	if s.cfg.Qobuz != nil {
		account = s.cfg.Qobuz
	}
	return mediabridge.NewQobuzProvider(account, s.cfg.Audio.QobuzDecoder())
}

// ItemFormat asks a provider what one item will decode to, for the router.
//
// Nil on any doubt, and that is the safe direction here rather than the
// cautious-looking one: nil blocks no route, so a lookup that fails leaves
// routing exactly as it was before formats were considered at all. Refusing to
// play because a catalogue call timed out would be a worse failure than
// choosing a route that later turns out not to fit — the cast itself still
// refuses to reduce, so nothing is downsampled either way.
func (s *Service) ItemFormat(ctx context.Context, p media.Provider, item media.Item) *media.PCMFormat {
	fr, ok := p.(media.ItemFormatReporter)
	if !ok {
		return nil
	}
	f, err := fr.ItemFormat(ctx, item)
	if err != nil {
		s.cfg.Logf("media: reading %s format for %q: %v", p.ID(), item.URI, err)
		return nil
	}
	if !f.Valid() {
		return nil
	}
	return &f
}

// Pause stops a room and releases anything it was holding.
//
// It is here rather than at each call site because the release is the part
// that is easy to forget: a streamed zone leaves a decoder holding the
// account's Spotify session, and pausing the speakers alone would keep it held
// all night. A scene that quiets the house and a sleep timer that ends it must
// both do this, or they are the same bug twice.
func (s *Service) Pause(ctx context.Context, room string, eps []media.Endpoint) error {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	zoneID, isZone := strings.CutPrefix(room, "zone:")
	err := media.Control(ctx, s.Plan(zoneID, eps), media.TransportPause)
	if isZone {
		s.EndSession(zoneID)
	}
	s.Touch(eps)
	return err
}

// ── What is still running ────────────────────────────────────────────────

// SetSession records a zone's live session, ending whatever it replaces.
// Starting something new in a room means the old thing stopped.
func (s *Service) SetSession(zoneID string, sess *media.Session) {
	s.sessionMu.Lock()
	old := s.sessions[zoneID]
	s.sessions[zoneID] = sess
	s.sessionMu.Unlock()

	// Closed off-lock: releasing a stream disconnects listeners and stops a
	// subprocess, neither of which should happen under a mutex other callers
	// are waiting on.
	old.Close()
}

// EndSession releases a zone's session, if it has one.
func (s *Service) EndSession(zoneID string) {
	s.sessionMu.Lock()
	sess := s.sessions[zoneID]
	delete(s.sessions, zoneID)
	s.sessionMu.Unlock()
	sess.Close()
}

// Plan returns the plan a zone's transport commands should follow.
//
// A live session knows which route it started on, and transport has to match:
// a natively grouped zone is addressed through its coordinator, while a zone
// HomeHub is feeding — streamed or cast over AirPlay — has no coordinator and
// every speaker is addressed. Those two are one case here because they are one
// case for transport: Plan.Endpoints() returns the targets for both. With no
// session — after a restart, or for speakers someone started from a vendor app
// — every speaker is addressed, which is correct if noisier than necessary.
func (s *Service) Plan(zoneID string, members []media.Endpoint) *media.Plan {
	s.sessionMu.Lock()
	sess := s.sessions[zoneID]
	s.sessionMu.Unlock()

	if sess != nil && (sess.Route == media.RouteGroup || sess.Route == media.RouteNative) {
		return &media.Plan{Route: sess.Route, Coordinator: members[0], Followers: members[1:]}
	}
	return &media.Plan{Route: media.RouteStream, Targets: members}
}

// DecodedZones lists the zones whose live session is one HomeHub is decoding
// for — streamed to the speakers, or cast over AirPlay.
//
// It is the answer to "what would stop if the account's playback moved
// elsewhere", which is a different question from "what is playing": a zone on
// a native route holds no decoder and is unaffected.
func (s *Service) DecodedZones() []string {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	var out []string
	for id, sess := range s.sessions {
		if sess != nil && (sess.Route == media.RouteStream || sess.Route == media.RouteAirPlay) {
			out = append(out, id)
		}
	}
	return out
}

// Close releases every live playback and stops the audio engine. Called at
// shutdown: the stream route holds the account's Spotify session, and leaving
// it behind would keep the user's Spotify pointed at a HomeHub that has
// stopped serving audio.
func (s *Service) Close() {
	s.sessionMu.Lock()
	sessions := s.sessions
	s.sessions = map[string]*media.Session{}
	s.sessionMu.Unlock()
	for _, sess := range sessions {
		sess.Close()
	}
	s.cfg.Audio.Close()
}
