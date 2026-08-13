package media

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StreamHost serves decoded audio to speakers over HTTP and hands back the
// URL they should play. Implemented by internal/stream.
//
// It is an interface here for the same reason Decoder is: the media layer
// should be usable, and testable, without a subprocess and a listening
// socket.
type StreamHost interface {
	// Publish begins serving s and returns the URL endpoints should fetch.
	// The returned stop function releases the stream and disconnects any
	// listeners; it must be safe to call more than once.
	Publish(ctx context.Context, s *Stream) (url string, stop func(), err error)
	// StartDelay is how long to wait after pointing an endpoint of the
	// given vendor at the stream before pointing the next one, to line
	// their buffers up. Zero is valid and means "no compensation".
	StartDelay(v Vendor) time.Duration
}

// Deps are what executing a plan needs beyond the plan itself.
type Deps struct {
	// Stream hosts audio for RouteStream. Nil disables that route at
	// execution time; Resolve should already have rejected it via the
	// provider's StreamAvailable, so reaching Execute with a stream plan
	// and no host is a wiring bug and is reported as one.
	Stream StreamHost
	// AirPlay sends audio for RouteAirPlay. Same contract as Stream: nil is
	// a wiring bug by the time a plan names the route.
	AirPlay AirPlayHost
	// Logf is optional.
	Logf func(format string, args ...any)
}

func (d Deps) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}

// Session is a running zone playback. For every route but RouteStream it is
// inert: the speakers hold the content themselves and there is nothing for
// HomeHub to keep alive. For RouteStream it owns the decoder and the HTTP
// stream, and Close is what shuts both down.
type Session struct {
	Route Route
	Sync  Sync
	// URL is the stream endpoints were pointed at, for RouteStream only.
	URL string

	closeOnce sync.Once
	stop      func()
}

// Close releases anything the session owns. Safe to call more than once, and
// safe to call on a session that owns nothing.
func (s *Session) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.stop != nil {
			s.stop()
		}
	})
}

// Play executes a resolved plan: it wakes what needs waking, arranges the
// speakers, and hands over the content.
//
// This performs device I/O across several speakers and must never be called
// while store.Mu is held — the rule from CLAUDE.md that store/staged.go exists
// to enforce. Callers resolve under the lock and play off it.
func Play(ctx context.Context, plan *Plan, p Provider, item Item, deps Deps) (*Session, error) {
	if plan == nil {
		return nil, fmt.Errorf("media: no plan")
	}
	// Wake first, and wake everything at once. A KEF takes a second or two
	// to come up, and doing them in series would add that to every speaker
	// in the zone before any of them heard a note.
	if err := wakeAll(ctx, plan.Wake, deps); err != nil {
		return nil, err
	}

	switch plan.Route {
	case RouteNative:
		return &Session{Route: plan.Route, Sync: plan.Sync},
			playNative(ctx, plan.Coordinator, p, item)

	case RouteConnect:
		return &Session{Route: plan.Route, Sync: plan.Sync},
			playConnect(ctx, plan.Coordinator, p, item)

	case RouteGroup:
		if err := groupOnto(ctx, plan.Coordinator, plan.Followers, deps); err != nil {
			return nil, err
		}
		return &Session{Route: plan.Route, Sync: plan.Sync},
			playNative(ctx, plan.Coordinator, p, item)

	case RouteAirPlay:
		return playAirPlay(ctx, plan, p, item, deps)

	case RouteStream:
		return playStream(ctx, plan, p, item, deps)
	}
	return nil, fmt.Errorf("media: don't know how to play route %q", plan.Route)
}

// wakeAll wakes every endpoint that needs it, concurrently. A speaker that
// fails to wake fails the whole play: the alternative is music coming out of
// some of the room, which is more confusing than an error.
func wakeAll(ctx context.Context, eps []Endpoint, deps Deps) error {
	if len(eps) == 0 {
		return nil
	}
	errs := make([]error, len(eps))
	var wg sync.WaitGroup
	for i, e := range eps {
		w, ok := e.(Waker)
		if !ok {
			// Declared CapWake without implementing Waker. The adapter test
			// catches this, but reaching it at runtime should still name
			// the speaker rather than panic.
			errs[i] = fmt.Errorf("media: %s claims it can be woken but has no way to do it",
				e.Descriptor().Name)
			continue
		}
		wg.Add(1)
		go func(i int, e Endpoint, w Waker) {
			defer wg.Done()
			if err := w.Wake(ctx); err != nil {
				errs[i] = fmt.Errorf("waking %s: %w", e.Descriptor().Name, err)
			}
		}(i, e, w)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	deps.logf("media: woke %d speaker(s)", len(eps))
	return nil
}

// playNative hands service content to a speaker that streams it itself.
func playNative(ctx context.Context, e Endpoint, p Provider, item Item) error {
	if e == nil {
		return fmt.Errorf("media: native route has no speaker to play on")
	}
	np, ok := p.(NativeProvider)
	if !ok {
		return fmt.Errorf("media: %s has no native form", p.Name())
	}
	player, ok := e.(NativeServicePlayer)
	if !ok {
		return fmt.Errorf("media: %s can't stream %s itself", e.Descriptor().Name, p.Name())
	}
	vendor := e.Descriptor().Vendor
	acct, err := player.ServiceAccount(ctx, np.ServiceName(vendor))
	if err != nil {
		// The household has no link for this service. That is a setup
		// problem the user fixes in the speaker's own app, so say which.
		return fmt.Errorf("%s isn't linked to %s: %w", p.Name(), e.Descriptor().Name, err)
	}
	uri, metadata, err := np.NativeItem(vendor, item, acct)
	if err != nil {
		return err
	}
	return player.PlayNative(ctx, uri, metadata)
}

// playConnect asks the service's cloud to play on the speaker.
func playConnect(ctx context.Context, e Endpoint, p Provider, item Item) error {
	if e == nil {
		return fmt.Errorf("media: connect route has no speaker to play on")
	}
	cp, ok := p.(ConnectProvider)
	if !ok {
		return fmt.Errorf("media: %s has no Connect equivalent", p.Name())
	}
	target, ok := e.(ConnectTarget)
	if !ok {
		return fmt.Errorf("media: %s can't be targeted by %s", e.Descriptor().Name, p.Name())
	}
	devices, err := cp.ConnectDevices(ctx)
	if err != nil {
		return err
	}
	dev, err := matchWaking(ctx, cp, target, e.Descriptor().Name, devices)
	if err != nil {
		return err
	}
	return cp.PlayOn(ctx, dev.ID, item)
}

// connectRetryDelay is how long a just-woken speaker gets to register itself
// with the service before the device list is read again.
const connectRetryDelay = 1500 * time.Millisecond

// matchWaking resolves the endpoint's Connect device, retrying once. A
// speaker woken a moment ago takes a second or two to appear, so the first
// listing can legitimately miss it; one retry turns "tapped a sleeping
// speaker" from an error into a short pause.
func matchWaking(ctx context.Context, cp ConnectProvider, t ConnectTarget, name string, devices []ConnectDevice) (ConnectDevice, error) {
	dev, err := MatchConnectDevice(t, name, devices)
	if err == nil {
		return dev, nil
	}
	select {
	case <-time.After(connectRetryDelay):
	case <-ctx.Done():
		return ConnectDevice{}, ctx.Err()
	}
	fresh, ferr := cp.ConnectDevices(ctx)
	if ferr != nil {
		return ConnectDevice{}, err // the original reason is the useful one
	}
	return MatchConnectDevice(t, name, fresh)
}

// groupOnto arranges followers behind a coordinator, skipping any that are
// already there. Re-joining a speaker that is already in the group would
// interrupt whatever it is playing for no reason.
func groupOnto(ctx context.Context, coord Endpoint, followers []Endpoint, deps Deps) error {
	if coord == nil {
		return fmt.Errorf("media: group route has no coordinator")
	}
	cg, ok := coord.(Grouper)
	if !ok {
		return fmt.Errorf("media: %s can't lead a group", coord.Descriptor().Name)
	}
	// The coordinator must lead, not follow. If it is currently somebody
	// else's follower, break it out first — otherwise the join below would
	// arrange the zone behind a speaker that isn't in it.
	if leader, err := cg.Coordinator(ctx); err == nil && leader != "" {
		if err := cg.Leave(ctx); err != nil {
			return fmt.Errorf("media: freeing %s to lead: %w", coord.Descriptor().Name, err)
		}
	}
	for _, f := range followers {
		g, ok := f.(Grouper)
		if !ok {
			return fmt.Errorf("media: %s can't be grouped", f.Descriptor().Name)
		}
		if err := g.Join(ctx, coord); err != nil {
			return fmt.Errorf("media: grouping %s onto %s: %w",
				f.Descriptor().Name, coord.Descriptor().Name, err)
		}
	}
	deps.logf("media: grouped %d speaker(s) onto %s", len(followers), coord.Descriptor().Name)
	return nil
}

// playStream decodes the content once and points every endpoint at the
// result. This is the cross-vendor path; see docs/MEDIA-PROTOCOL.md for why
// nothing cheaper works.
func playStream(ctx context.Context, plan *Plan, p Provider, item Item, deps Deps) (*Session, error) {
	sp, ok := p.(StreamProvider)
	if !ok {
		return nil, fmt.Errorf("media: %s can't be decoded by HomeHub", p.Name())
	}
	if deps.Stream == nil {
		return nil, fmt.Errorf("media: no stream host is configured on this server")
	}

	stream, err := sp.OpenStream(ctx, item)
	if err != nil {
		return nil, err
	}
	url, stop, err := deps.Stream.Publish(ctx, stream)
	if err != nil {
		_ = stream.Body.Close()
		return nil, err
	}
	sess := &Session{Route: RouteStream, Sync: plan.Sync, URL: url, stop: stop}

	// Speakers are started in vendor order with a per-vendor delay, so
	// their buffers line up as closely as this route allows. Sequential on
	// purpose: the whole point is the spacing between starts, which
	// concurrency would destroy.
	meta := stream.Meta
	meta.ContentType = stream.ContentType
	meta.Live = true
	for i, e := range plan.Targets {
		player, ok := e.(URIPlayer)
		if !ok {
			sess.Close()
			return nil, fmt.Errorf("media: %s can't play a stream URL", e.Descriptor().Name)
		}
		if d := deps.Stream.StartDelay(e.Descriptor().Vendor); d > 0 && i > 0 {
			select {
			case <-time.After(d):
			case <-ctx.Done():
				sess.Close()
				return nil, ctx.Err()
			}
		}
		if err := player.PlayURI(ctx, url, meta); err != nil {
			// One speaker failing means the zone is half-playing, which is
			// worse than not playing: tear the whole thing down so the user
			// gets one clear error instead of music in some of the room.
			sess.Close()
			return nil, fmt.Errorf("media: starting %s: %w", e.Descriptor().Name, err)
		}
	}
	deps.logf("media: streaming to %d speaker(s) at %s", len(plan.Targets), url)
	return sess, nil
}

// playAirPlay decodes the content once and pushes it to every receiver.
//
// The shape mirrors playStream — one decode, many sinks — and differs in who
// initiates: there, speakers are told a URL and fetch it; here, HomeHub holds
// the samples and sends them, which is what lets one clock cover all of them.
func playAirPlay(ctx context.Context, plan *Plan, p Provider, item Item, deps Deps) (*Session, error) {
	sp, ok := p.(StreamProvider)
	if !ok {
		return nil, fmt.Errorf("media: %s can't be decoded by HomeHub", p.Name())
	}
	if deps.AirPlay == nil {
		return nil, fmt.Errorf("media: no AirPlay sender is configured on this server")
	}

	dests := make([]AirPlayDest, 0, len(plan.Targets))
	for _, e := range plan.Targets {
		t, ok := e.(AirPlayTarget)
		if !ok {
			// Declared CapAirPlay without implementing AirPlayTarget. The
			// adapter test catches this; reaching it at runtime should name
			// the speaker rather than send audio nowhere.
			return nil, fmt.Errorf("media: %s claims AirPlay but has no address for it",
				e.Descriptor().Name)
		}
		dests = append(dests, t.AirPlayDest())
	}

	stream, err := sp.OpenStream(ctx, item)
	if err != nil {
		return nil, err
	}
	stop, err := deps.AirPlay.Cast(ctx, stream, dests)
	if err != nil {
		_ = stream.Body.Close()
		return nil, err
	}
	deps.logf("media: casting to %d AirPlay receiver(s)", len(dests))
	return &Session{Route: RouteAirPlay, Sync: plan.Sync, stop: stop}, nil
}

// Transport is a play/pause/next/previous verb applied to a whole zone.
type Transport string

const (
	TransportPlay     Transport = "play"
	TransportPause    Transport = "pause"
	TransportNext     Transport = "next"
	TransportPrevious Transport = "previous"
)

// Control applies a transport verb across a zone.
//
// Which endpoints actually receive it depends on the route: a native group has
// a coordinator that speaks for all of them, and sending pause to each member
// individually would be both redundant and racy. A streamed zone has no
// coordinator, so every speaker is addressed.
func Control(ctx context.Context, plan *Plan, verb Transport) error {
	targets := plan.Endpoints()
	if plan.Route == RouteGroup || plan.Route == RouteNative || plan.Route == RouteConnect {
		if plan.Coordinator != nil {
			targets = []Endpoint{plan.Coordinator}
		}
	}
	return fanOut(ctx, targets, func(ctx context.Context, e Endpoint) error {
		switch verb {
		case TransportPlay:
			return e.Play(ctx)
		case TransportPause:
			return e.Pause(ctx)
		case TransportNext:
			return e.Next(ctx)
		case TransportPrevious:
			return e.Previous(ctx)
		}
		return fmt.Errorf("media: unknown transport %q", verb)
	})
}

// SetVolume sets an absolute level across every endpoint in a zone.
//
// Deliberately not relative-to-current: reading each speaker, scaling, and
// writing back turns one tap into three round trips per speaker and drifts as
// speakers round differently. A caller wanting relative volume computes it
// from the state it already has.
func SetVolume(ctx context.Context, eps []Endpoint, level int) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	return fanOut(ctx, eps, func(ctx context.Context, e Endpoint) error {
		return e.SetVolume(ctx, level)
	})
}

// SetMute mutes or unmutes every endpoint in a zone.
func SetMute(ctx context.Context, eps []Endpoint, muted bool) error {
	return fanOut(ctx, eps, func(ctx context.Context, e Endpoint) error {
		return e.SetMute(ctx, muted)
	})
}

// States reads every endpoint's live state concurrently. Unreachable speakers
// yield a nil entry rather than failing the batch: a zone with one speaker
// off should still report the others.
func States(ctx context.Context, eps []Endpoint) map[string]*NowPlaying {
	out := make(map[string]*NowPlaying, len(eps))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, e := range eps {
		wg.Add(1)
		go func(e Endpoint) {
			defer wg.Done()
			st, err := e.State(ctx)
			if err != nil {
				st = nil
			}
			mu.Lock()
			out[e.Descriptor().ID] = st
			mu.Unlock()
		}(e)
	}
	wg.Wait()
	return out
}

// fanOut runs fn against every endpoint concurrently and returns the first
// error, having waited for all of them. Waiting matters: returning early
// would leave commands in flight against speakers the caller believes it has
// finished with.
func fanOut(ctx context.Context, eps []Endpoint, fn func(context.Context, Endpoint) error) error {
	if len(eps) == 0 {
		return ErrEmptyZone
	}
	errs := make([]error, len(eps))
	var wg sync.WaitGroup
	for i, e := range eps {
		wg.Add(1)
		go func(i int, e Endpoint) {
			defer wg.Done()
			if err := fn(ctx, e); err != nil {
				errs[i] = fmt.Errorf("%s: %w", e.Descriptor().Name, err)
			}
		}(i, e)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
