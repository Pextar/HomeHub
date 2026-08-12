package media

import (
	"fmt"
	"sort"
	"strings"
)

// Route is how content gets from a provider onto a zone.
//
// The ordering of the constants is the preference order, best first, and
// Routes() depends on it. That ordering is a guarantee, not a detail: because
// RouteStream sorts last, a zone that any native route can serve never
// reaches it, which is what makes the cross-vendor work strictly additive.
// A Sonos-only listener gets exactly the SOAP calls they got before this
// package existed.
type Route string

const (
	// RouteNative: the speakers stream the service themselves from their
	// own account link. Perfect sync, full quality, command never leaves
	// the LAN.
	RouteNative Route = "native"
	// RouteConnect: the service's cloud is pointed at a single speaker.
	// Full quality; single endpoint only.
	RouteConnect Route = "connect"
	// RouteGroup: group same-vendor speakers natively, then serve the
	// coordinator by one of the routes above.
	RouteGroup Route = "group"
	// RouteAirPlay: HomeHub decodes once and pushes the samples to every
	// receiver itself, keeping the clock. Ranked above RouteStream because
	// it is the same decode with a real clock on the end of it — receivers
	// are told which sample belongs at which moment instead of each filling
	// a buffer and starting when it feels ready.
	RouteAirPlay Route = "airplay"
	// RouteStream: HomeHub decodes once and fans the audio out over HTTP.
	// The cross-vendor path for speakers that fetch rather than receive,
	// and the one with no clock. Always last.
	RouteStream Route = "stream"
)

// order is the preference ranking. Lower wins.
var order = map[Route]int{
	RouteNative:  0,
	RouteGroup:   1,
	RouteConnect: 2,
	RouteAirPlay: 3,
	RouteStream:  4,
}

// Rank returns the preference of a route, best first. Unknown routes sort
// last so an unrecognised value can never outrank a known-good one.
func (r Route) Rank() int {
	if n, ok := order[r]; ok {
		return n
	}
	return len(order)
}

// Sync is how well a route keeps multiple speakers together, reported to the
// caller so the UI can be honest about what is about to happen. There is no
// "perfect" beyond SyncExact and no pretending Buffered is better than it is.
type Sync string

const (
	// SyncExact is the vendor's own multi-room clock. Sample-accurate.
	SyncExact Sync = "exact"
	// SyncSingle is a single speaker, where sync is not a question.
	SyncSingle Sync = "single"
	// SyncBuffered is the stream route: each speaker fills its own jitter
	// buffer and starts when ready. Stable once running, but offset by a
	// few hundred milliseconds and not correctable to better than that
	// without a real clock protocol.
	SyncBuffered Sync = "buffered"
	// SyncClocked is the AirPlay route: one sender, one clock, and every
	// receiver told which sample belongs at which moment on it. Much
	// tighter than buffered — the offset is corrected rather than merely
	// stable — and still not a vendor's own multi-room bus, because the
	// clock is HomeHub's and it is disciplined over UDP on a home network.
	SyncClocked Sync = "clocked"
)

// Sync reports the sync characteristic of a route for a zone of n endpoints.
func (r Route) Sync(n int) Sync {
	if n <= 1 {
		return SyncSingle
	}
	switch r {
	case RouteStream:
		return SyncBuffered
	case RouteAirPlay:
		return SyncClocked
	}
	return SyncExact
}

// RouteSet is the set of routes a provider can serve.
type RouteSet []Route

// Has reports membership.
func (rs RouteSet) Has(r Route) bool {
	for _, x := range rs {
		if x == r {
			return true
		}
	}
	return false
}

// Plan is a resolved decision: which route, which endpoints, and — when the
// route needs speakers grouped or woken first — what has to happen before the
// content is handed over.
type Plan struct {
	Route Route
	Sync  Sync
	// Coordinator is the endpoint the content is handed to. For RouteGroup
	// it leads the group; for RouteNative and RouteConnect it is the single
	// target; for RouteStream it is empty, because every endpoint is
	// addressed directly and none of them leads.
	Coordinator Endpoint
	// Followers are grouped onto the coordinator before playing
	// (RouteGroup only).
	Followers []Endpoint
	// Targets is every endpoint addressed directly rather than through a
	// coordinator: the ones handed the stream URL (RouteStream) or pushed
	// audio (RouteAirPlay).
	Targets []Endpoint
	// Wake are endpoints that must be woken before anything else. Populated
	// for any route touching a CapWake endpoint, since a sleeping speaker
	// is not on the network and would fail with a confusing "not found".
	Wake []Endpoint
	// Reason explains the choice in a sentence fit to show a user. Set for
	// every plan, not just surprising ones, so the UI never has to
	// reconstruct why.
	Reason string
}

// Endpoints returns every endpoint the plan touches, coordinator included.
func (p *Plan) Endpoints() []Endpoint {
	// The two routes HomeHub decodes for itself address every speaker
	// directly and have no coordinator to speak for them.
	if p.Route == RouteStream || p.Route == RouteAirPlay {
		return p.Targets
	}
	out := make([]Endpoint, 0, 1+len(p.Followers))
	if p.Coordinator != nil {
		out = append(out, p.Coordinator)
	}
	return append(out, p.Followers...)
}

// RouteError explains why nothing could serve a zone, per endpoint. A flat
// "unsupported" tells a user nothing they can act on; naming the speaker and
// what it lacks tells them whether to change the zone, the service, or to
// install librespot.
type RouteError struct {
	Provider string
	// Blocked maps a route to why it was rejected, in preference order.
	Blocked []RouteBlock
}

// RouteBlock is one rejected route and its reason.
type RouteBlock struct {
	Route  Route
	Reason string
}

func (e *RouteError) Error() string {
	var b strings.Builder
	b.WriteString("media: no route can play ")
	b.WriteString(e.Provider)
	b.WriteString(" to these speakers")
	for _, blk := range e.Blocked {
		b.WriteString("; ")
		b.WriteString(string(blk.Route))
		b.WriteString(": ")
		b.WriteString(blk.Reason)
	}
	return b.String()
}

func (e *RouteError) Unwrap() error { return ErrNoRoute }

// Resolve picks the best route that can serve every endpoint in the zone.
//
// This is a pure function of the endpoints' declared capabilities and the
// provider's declared routes — no device I/O, no store access, no clock. That
// is deliberate: this is the logic that decides whether a tap plays in the
// right room, so it is the logic that gets exhaustively table-tested.
//
// Routes are tried best-first and the first that fits wins. Every rejection
// is recorded, so a total failure can explain itself.
func Resolve(p Provider, endpoints []Endpoint) (*Plan, error) {
	if len(endpoints) == 0 {
		return nil, ErrEmptyZone
	}
	serves := p.Routes()
	rerr := &RouteError{Provider: p.Name()}

	for _, r := range rankedRoutes() {
		if !serves.Has(r) {
			rerr.Blocked = append(rerr.Blocked, RouteBlock{r,
				fmt.Sprintf("%s can't be played this way", p.Name())})
			continue
		}
		plan, why := tryRoute(r, p, endpoints)
		if plan != nil {
			plan.Wake = wakeable(plan.Endpoints())
			plan.Sync = r.Sync(len(plan.Endpoints()))
			return plan, nil
		}
		rerr.Blocked = append(rerr.Blocked, RouteBlock{r, why})
	}
	return nil, rerr
}

// rankedRoutes lists every route in preference order.
func rankedRoutes() []Route {
	all := []Route{RouteNative, RouteConnect, RouteGroup, RouteAirPlay, RouteStream}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Rank() < all[j].Rank() })
	return all
}

// tryRoute returns a plan for one route, or nil plus the reason it doesn't
// fit. Each branch states its own precondition rather than sharing a helper,
// because the preconditions genuinely differ and folding them together is how
// this kind of engine grows bugs that play music in the wrong room.
func tryRoute(r Route, p Provider, eps []Endpoint) (*Plan, string) {
	switch r {
	case RouteNative:
		// A single speaker that streams the service itself. Multi-speaker
		// native is RouteGroup's job — it has to group them first.
		if len(eps) != 1 {
			return nil, "more than one speaker; they'd each need their own copy"
		}
		if !eps[0].Descriptor().Caps.Has(CapNativeService) {
			return nil, lacks(eps[0], "stream this service itself")
		}
		if _, ok := p.(NativeProvider); !ok {
			return nil, fmt.Sprintf("%s has no native form for this speaker", p.Name())
		}
		return &Plan{
			Route:       RouteNative,
			Coordinator: eps[0],
			Reason: fmt.Sprintf("%s streams %s directly",
				eps[0].Descriptor().Name, p.Name()),
		}, ""

	case RouteConnect:
		if len(eps) != 1 {
			// Worth naming precisely: this is the limit people expect not
			// to exist, and the reason the stream route had to be built.
			return nil, "Connect plays to one speaker at a time"
		}
		if !eps[0].Descriptor().Caps.Has(CapConnect) {
			return nil, lacks(eps[0], "be targeted by Connect")
		}
		if _, ok := p.(ConnectProvider); !ok {
			return nil, fmt.Sprintf("%s has no Connect equivalent", p.Name())
		}
		return &Plan{
			Route:       RouteConnect,
			Coordinator: eps[0],
			Reason: fmt.Sprintf("%s sends %s to %s",
				p.Name(), p.Name(), eps[0].Descriptor().Name),
		}, ""

	case RouteGroup:
		if len(eps) < 2 {
			return nil, "only one speaker; nothing to group"
		}
		if _, ok := p.(NativeProvider); !ok {
			return nil, fmt.Sprintf("%s has no native form to hand the group", p.Name())
		}
		key := eps[0].Descriptor().GroupKey
		for _, e := range eps {
			d := e.Descriptor()
			if !d.Caps.Has(CapGroup | CapNativeService) {
				return nil, lacks(e, "group with the others")
			}
			if d.GroupKey == "" || d.GroupKey != key {
				return nil, fmt.Sprintf("%s can't group with %s — different systems",
					d.Name, eps[0].Descriptor().Name)
			}
		}
		return &Plan{
			Route:       RouteGroup,
			Coordinator: eps[0],
			Followers:   eps[1:],
			Reason: fmt.Sprintf("grouped on %s, streaming %s directly",
				eps[0].Descriptor().Name, p.Name()),
		}, ""

	case RouteAirPlay:
		// Every endpoint must be a receiver: this route pushes, and a
		// speaker that expects to fetch has nothing to receive on. A mixed
		// zone of receivers and fetchers is the one arrangement no route
		// serves, and the error says which speaker fell on which side.
		for _, e := range eps {
			if !e.Descriptor().Caps.Has(CapAirPlay) {
				return nil, lacks(e, "be sent an AirPlay stream")
			}
		}
		sp, ok := p.(StreamProvider)
		if !ok {
			return nil, fmt.Sprintf("%s can't be decoded by HomeHub", p.Name())
		}
		if av := sp.StreamAvailable(); !av.OK {
			return nil, av.Reason
		}
		return &Plan{
			Route:   RouteAirPlay,
			Targets: eps,
			Reason:  airPlayReason(p, eps),
		}, ""

	case RouteStream:
		for _, e := range eps {
			if !e.Descriptor().Caps.Has(CapPlayURI) {
				return nil, lacks(e, "play a stream URL")
			}
		}
		sp, ok := p.(StreamProvider)
		if !ok {
			return nil, fmt.Sprintf("%s can't be decoded by HomeHub", p.Name())
		}
		if av := sp.StreamAvailable(); !av.OK {
			return nil, av.Reason
		}
		return &Plan{
			Route:   RouteStream,
			Targets: eps,
			Reason:  streamReason(p, eps),
		}, ""
	}
	return nil, "unknown route"
}

// streamReason says why the lossy path was taken, naming the speaker
// responsible. A user who sees "stream" without knowing which speaker forced
// it has no way to choose differently next time.
func streamReason(p Provider, eps []Endpoint) string {
	var culprit string
	for _, e := range eps {
		if !e.Descriptor().Caps.Has(CapNativeService) {
			culprit = e.Descriptor().Name
			break
		}
	}
	if culprit == "" {
		// Every speaker could stream natively but they can't group with
		// each other — mixed vendors that each have their own account link.
		return fmt.Sprintf("these speakers can't group with each other, so HomeHub is decoding %s for all of them", p.Name())
	}
	return fmt.Sprintf("%s can't stream %s itself, so HomeHub is decoding for all of them",
		culprit, p.Name())
}

// airPlayReason says what the AirPlay route is doing, and is written to be
// read next to the stream route's sentence: both are HomeHub decoding, and the
// difference a listener will notice is the clock.
func airPlayReason(p Provider, eps []Endpoint) string {
	if len(eps) == 1 {
		return fmt.Sprintf("HomeHub decodes %s and sends it to %s over AirPlay",
			p.Name(), eps[0].Descriptor().Name)
	}
	return fmt.Sprintf("HomeHub decodes %s once and sends it to all %d receivers on one clock",
		p.Name(), len(eps))
}

// lacks phrases a missing capability as something the speaker can't do,
// naming it, because these strings reach the user.
func lacks(e Endpoint, what string) string {
	return fmt.Sprintf("%s can't %s", e.Descriptor().Name, what)
}

// wakeable filters to the endpoints that need waking first.
func wakeable(eps []Endpoint) []Endpoint {
	var out []Endpoint
	for _, e := range eps {
		if e.Descriptor().Caps.Has(CapWake) {
			out = append(out, e)
		}
	}
	return out
}
