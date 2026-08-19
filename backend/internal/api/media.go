package api

// The unified media endpoints.
//
// These sit on top of internal/media and replace nothing: /api/sonos/* and
// /api/kef/* stay exactly as they are, because the per-speaker detail views
// need vendor specifics — crossfade, KEF source selection, Sonos queues — that
// do not generalise and should not be flattened into a common shape just to
// have one.
//
// What is new is the zone: "these speakers, playing this, together", including
// the mix of makes that was impossible before. See docs/MEDIA-PROTOCOL.md.
//
// Locking follows the rule in CLAUDE.md and is the thing to be careful about
// here. Resolving a zone to live endpoints reads the store under Mu; playing
// to it is multi-speaker device I/O and must run off-lock. Every handler below
// is written as: read + resolve under the lock, release, then talk to
// speakers.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/kef"
	"homehub/internal/media"
	"homehub/internal/mediabridge"
	"homehub/internal/sonos"
	"homehub/internal/store"
)

// mediaTimeout caps a zone operation. Generous because the stream route can
// involve waking a speaker, a cloud round trip and several UPnP calls before
// anything comes out.
const mediaTimeout = 45 * time.Second

// endpoints builds live endpoints for every registered speaker, keyed by
// qualified id. Caller must hold Mu (read is enough).
//
// The state functions close over the monitors, so the media layer reads from
// the same event-driven and polled caches the vendor views do rather than
// adding a second round of traffic to every speaker.
func (s *Server) endpoints() map[string]media.Endpoint {
	out := make(map[string]media.Endpoint,
		len(s.Store.Sonos)+len(s.Store.KEF)+len(s.Store.AirPlay)+len(s.Store.UPnP))
	for id, sp := range s.Store.Sonos {
		out[store.QualifySonos(id)] = mediabridge.NewSonosEndpoint(*sp, "", s.sonosState)
	}
	for id, sp := range s.Store.KEF {
		out[store.QualifyKEF(id)] = mediabridge.NewKEFEndpoint(*sp, s.kefState)
	}
	// A UPnP renderer holds its own transport state, so unlike an AirPlay
	// receiver it is asked rather than inferred — see mediabridge/upnp.go.
	for id, rn := range s.Store.UPnP {
		out[store.QualifyUPnP(id)] = mediabridge.NewUPnPEndpoint(*rn)
	}
	// AirPlay receivers have no monitor to read from — there is nothing on
	// the device to poll — so their state comes from the live cast instead.
	s.airplayEndpoints(out)
	return out
}

// sonosState reads a speaker's state from the GENA monitor's cache.
func (s *Server) sonosState(ctx context.Context, sp store.SonosSpeaker) (*sonos.State, error) {
	snap := s.sonosEvents().Snapshot(ctx)
	if cached, ok := snap.Speakers[sp.ID]; ok && cached.State != nil {
		return cached.State, nil
	}
	return sonos.GetState(ctx, sp.IP)
}

// kefState reads a speaker's state from the polling monitor's cache.
func (s *Server) kefState(ctx context.Context, sp store.KEFSpeaker) (*kef.State, error) {
	snap := s.kefEvents().Snapshot(ctx)
	if cached, ok := snap.Speakers[sp.ID]; ok && cached.State != nil {
		return cached.State, nil
	}
	return kef.GetState(ctx, sp.IP)
}

// provider returns the media provider for an id. The lookup is by name so
// that adding one is a registration rather than a new branch at every call
// site.
//
// The empty id still means Spotify. It is the default because it is the
// provider every household has wired up, not because it is the better one —
// a caller that wants lossless asks for it.
func (s *Server) provider(id string) (media.Provider, error) {
	switch {
	case id == "" || strings.EqualFold(id, "spotify"):
		return mediabridge.NewSpotifyProvider(s.Spotify, s.Audio.Decoder(), s.Audio.Quality()), nil
	case strings.EqualFold(id, "qobuz"):
		return mediabridge.NewQobuzProvider(s.qobuzAccount(), s.Audio.QobuzDecoder()), nil
	}
	return nil, fmt.Errorf("%w: %q", media.ErrUnknownProvider, id)
}

// qobuzAccount adapts the optional Qobuz client to the provider's interface.
//
// The nil check is load-bearing rather than defensive: assigning a nil
// *qobuz.Client straight into the interface would produce a non-nil interface
// holding a nil pointer, and the provider's "is it configured" check would pass
// on its way to a panic.
func (s *Server) qobuzAccount() mediabridge.QobuzAccount {
	if s.Qobuz == nil {
		return nil
	}
	return s.Qobuz
}

// itemFormat asks a provider what one item will decode to, for the router.
//
// Nil on any doubt, and that is the safe direction here rather than the
// cautious-looking one: nil blocks no route, so a lookup that fails leaves
// routing exactly as it was before formats were considered at all. Refusing to
// play because a catalogue call timed out would be a worse failure than
// choosing a route that later turns out not to fit — the cast itself still
// refuses to reduce, so nothing is downsampled either way.
func (s *Server) itemFormat(ctx context.Context, p media.Provider, item media.Item) *media.PCMFormat {
	fr, ok := p.(media.ItemFormatReporter)
	if !ok {
		return nil
	}
	f, err := fr.ItemFormat(ctx, item)
	if err != nil {
		log.Printf("media: reading %s format for %q: %v", p.ID(), item.URI, err)
		return nil
	}
	if !f.Valid() {
		return nil
	}
	return &f
}

// providers is every provider the server knows about.
func (s *Server) providers() []media.Provider {
	return []media.Provider{
		mediabridge.NewSpotifyProvider(s.Spotify, s.Audio.Decoder(), s.Audio.Quality()),
		mediabridge.NewQobuzProvider(s.qobuzAccount(), s.Audio.QobuzDecoder()),
	}
}

// mediaEndpoints handles GET /api/media/endpoints — every speaker in one
// uniform shape, with the capabilities the UI needs to know what to offer.
func (s *Server) mediaEndpoints(w http.ResponseWriter, r *http.Request) {
	var eps map[string]media.Endpoint
	s.Store.View(func() {
		eps = s.endpoints()
	})

	type view struct {
		media.Descriptor
		Member string `json:"member"` // qualified id, what a zone stores
	}
	out := make([]view, 0, len(eps))
	for member, e := range eps {
		out = append(out, view{Descriptor: e.Descriptor(), Member: member})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

// mediaProviders handles GET /api/media/providers.
func (s *Server) mediaProviders(w http.ResponseWriter, r *http.Request) {
	type view struct {
		ID     string             `json:"id"`
		Name   string             `json:"name"`
		Avail  media.Availability `json:"availability"`
		Routes media.RouteSet     `json:"routes"`
		// Streaming reports whether the cross-vendor route is usable, which
		// is a different question from whether search works and needs its
		// own sentence when it isn't.
		Streaming media.Availability `json:"streaming"`
	}
	provs := s.providers()
	out := make([]view, 0, len(provs))
	for _, p := range provs {
		v := view{ID: p.ID(), Name: p.Name(), Avail: p.Available(), Routes: p.Routes()}
		if sp, ok := p.(media.StreamProvider); ok {
			v.Streaming = sp.StreamAvailable()
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// zoneView is a zone plus whatever live state the speakers report.
type zoneView struct {
	*store.Zone
	// Speakers is each member's descriptor and state, in zone order.
	Speakers []zoneSpeaker `json:"speakers"`
	// Route is what a play would use right now, and Reason explains it.
	// Both empty when nothing can serve the zone, in which case Problem
	// says why in words the user can act on.
	Route   media.Route `json:"route,omitempty"`
	Sync    media.Sync  `json:"sync,omitempty"`
	Reason  string      `json:"reason,omitempty"`
	Problem string      `json:"problem,omitempty"`
	// Quality is what a play here would actually sound like, source to
	// speaker. Absent when no route can serve the zone, because there is
	// then no path to describe.
	Quality *media.Chain `json:"quality,omitempty"`
}

type zoneSpeaker struct {
	Member string `json:"member"`
	media.Descriptor
	State *media.NowPlaying `json:"state,omitempty"`
	// Missing marks a member whose speaker has since been deleted. Should
	// not happen — CascadeDeleteSpeaker exists to prevent it — but a zone
	// that renders is better than one that 500s if it ever does.
	Missing bool `json:"missing,omitempty"`
}

// mediaZones handles GET /api/media/zones.
func (s *Server) mediaZones(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), mediaTimeout)
	defer cancel()

	var eps map[string]media.Endpoint
	var zones []*store.Zone
	s.Store.View(func() {
		eps = s.endpoints()
		// Deep-copied so the zone views can be built off-lock.
		zones = make([]*store.Zone, 0, len(s.Store.Zones))
		for _, z := range s.Store.Zones {
			cp := *z
			cp.Members = append([]string(nil), z.Members...)
			zones = append(zones, &cp)
		}
	})

	sort.Slice(zones, func(i, j int) bool { return zones[i].Name < zones[j].Name })

	p, _ := s.provider("")
	out := make([]zoneView, 0, len(zones))
	for _, z := range zones {
		out = append(out, s.buildZoneView(ctx, z, eps, p))
	}
	writeJSON(w, http.StatusOK, out)
}

// buildZoneView assembles one zone's live picture. Runs off-lock: it reads
// speaker state, which is device I/O even when it lands in a monitor cache.
func (s *Server) buildZoneView(ctx context.Context, z *store.Zone, eps map[string]media.Endpoint, p media.Provider) zoneView {
	v := zoneView{Zone: z, Speakers: []zoneSpeaker{}}

	members := make([]media.Endpoint, 0, len(z.Members))
	for _, m := range z.Members {
		e, ok := eps[m]
		if !ok {
			v.Speakers = append(v.Speakers, zoneSpeaker{Member: m, Missing: true})
			continue
		}
		members = append(members, e)
	}

	states := media.States(ctx, members)
	for _, e := range members {
		d := e.Descriptor()
		v.Speakers = append(v.Speakers, zoneSpeaker{
			Member:     memberOf(d),
			Descriptor: d,
			State:      states[d.ID],
		})
	}

	// Which route this zone would take, reported before anything is played
	// so the UI can be honest about what a tap will do.
	if len(members) > 0 && p != nil {
		if plan, err := media.Resolve(p, members); err == nil {
			v.Route, v.Sync, v.Reason = plan.Route, plan.Sync, plan.Reason
			chain := media.DescribeQuality(p, plan.Route, s.Audio.Quality())
			v.Quality = &chain
		} else {
			v.Problem = err.Error()
		}
	}
	return v
}

// memberOf rebuilds a qualified id from a descriptor.
func memberOf(d media.Descriptor) string {
	switch d.Vendor {
	case media.VendorKEF:
		return store.QualifyKEF(d.ID)
	case media.VendorAirPlay:
		return store.QualifyAirPlay(d.ID)
	}
	return store.QualifySonos(d.ID)
}

// mediaCreateZone handles POST /api/media/zones.
func (s *Server) mediaCreateZone(w http.ResponseWriter, r *http.Request) {
	var z store.Zone
	if !decodeBody(w, r, &z) {
		return
	}
	z.ID = fmt.Sprintf("zone_%d", time.Now().UnixNano())

	if !s.update(w, func() error {
		if err := s.Store.ValidateZone(&z); err != nil {
			return errInvalid(err)
		}
		s.Store.Zones[z.ID] = &z
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusCreated, z)
}

// mediaUpdateZone handles PUT /api/media/zones/{id}.
func (s *Server) mediaUpdateZone(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updates store.Zone
	if !decodeBody(w, r, &updates) {
		return
	}

	var existing *store.Zone
	if !s.update(w, func() error {
		var ok bool
		existing, ok = s.Store.Zones[id]
		if !ok {
			return errStatus(http.StatusNotFound, "zone not found")
		}
		merged := *existing
		if updates.Name != "" {
			merged.Name = updates.Name
		}
		// Members are replaced wholesale rather than merged: the UI sends the
		// full arrangement, and there is no way to express a removal otherwise.
		if updates.Members != nil {
			merged.Members = updates.Members
		}
		merged.Room = updates.Room
		if err := s.Store.ValidateZone(&merged); err != nil {
			return errInvalid(err)
		}
		*existing = merged
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// mediaDeleteZone handles DELETE /api/media/zones/{id}.
func (s *Server) mediaDeleteZone(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !s.update(w, func() error {
		if _, ok := s.Store.Zones[id]; !ok {
			return errStatus(http.StatusNotFound, "zone not found")
		}
		delete(s.Store.Zones, id)
		return nil
	}) {
		return
	}
	s.pruneDeadRooms() // a room that no longer exists keeps no shelf
	w.WriteHeader(http.StatusNoContent)
}

// resolveZone loads a zone and its live endpoints. Takes and releases the
// read lock itself, so callers are off-lock by the time they touch a speaker.
func (s *Server) resolveZone(w http.ResponseWriter, r *http.Request) ([]media.Endpoint, *store.Zone, bool) {
	id := mux.Vars(r)["id"]

	var zone store.Zone
	var eps map[string]media.Endpoint
	var ok bool
	s.Store.View(func() {
		var z *store.Zone
		if z, ok = s.Store.Zones[id]; ok {
			zone = *z
			zone.Members = append([]string(nil), z.Members...)
			eps = s.endpoints()
		}
	})

	if !ok {
		writeError(w, http.StatusNotFound, "zone not found")
		return nil, nil, false
	}
	members := make([]media.Endpoint, 0, len(zone.Members))
	for _, m := range zone.Members {
		if e, exists := eps[m]; exists {
			members = append(members, e)
		}
	}
	if len(members) == 0 {
		writeError(w, http.StatusBadRequest, "this zone has no speakers in it")
		return nil, nil, false
	}
	return members, &zone, true
}

// mediaRoom resolves a destination key — "sonos:<id>", "kef:<id>" or
// "zone:<id>" — to live endpoints and the name the house calls it.
//
// This is the vocabulary the play history already uses, and having one
// resolver for it is what lets anything that isn't an HTTP handler address a
// room: a music timer names a room the same way a shelf does, and a single
// speaker is simply a zone of one as far as the route engine is concerned.
//
// Takes and releases the read lock itself, so callers are off-lock by the
// time they touch a speaker.
func (s *Server) mediaRoom(key string) ([]media.Endpoint, string, error) {
	key = strings.TrimSpace(key)
	var members []string
	var name string

	s.Store.View(func() {
		if id, ok := strings.CutPrefix(key, "zone:"); ok {
			z, exists := s.Store.Zones[id]
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
			if sp, exists := s.Store.KEF[id]; exists {
				name, members = sp.Name, []string{key}
			}
		case "airplay":
			if sp, exists := s.Store.AirPlay[id]; exists {
				name, members = sp.Name, []string{key}
			}
		default:
			if sp, exists := s.Store.Sonos[id]; exists {
				name, members = sp.Name, []string{key}
			}
		}
	})

	if name == "" {
		return nil, "", fmt.Errorf("%w: %q", media.ErrUnknownEndpoint, key)
	}

	var eps map[string]media.Endpoint
	s.Store.View(func() { eps = s.endpoints() })
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

// mediaZonePlay handles POST /api/media/zones/{id}/play with
// {"provider":"spotify","uri":"spotify:track:…","title":"…"}.
//
// The response says which route was chosen and why, so the UI can tell the
// user what is about to happen rather than presenting every zone as if they
// were equivalent — a streamed zone genuinely is a different thing from a
// natively grouped one, and hiding that would be the kind of dishonesty
// DESIGN.md §15 rules out.
func (s *Server) mediaZonePlay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		URI      string `json:"uri"`
		Title    string `json:"title"`
		Kind     string `json:"kind"`
		Sub      string `json:"sub"`
		ArtURI   string `json:"art_uri"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.URI) == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	p, err := s.provider(body.Provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if av := p.Available(); !av.OK {
		writeError(w, http.StatusConflict, av.Reason)
		return
	}

	members, zone, ok := s.resolveZone(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), mediaTimeout)
	defer cancel()

	item := media.Item{
		Provider: p.ID(),
		Kind:     media.ItemKind(body.Kind),
		URI:      body.URI,
		Title:    body.Title,
	}

	// Resolve knowing what this particular thing is. The route that can carry
	// a CD-quality album is not the route that can carry a 24-bit/192 kHz one,
	// and deciding from the subscription instead would refuse the first
	// wherever it cannot carry the second.
	plan, err := media.ResolveFor(p, members, s.itemFormat(ctx, p, item))
	if err != nil {
		// Nothing can serve this zone. The error names which speaker
		// blocked which route, which is the actionable part.
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	sess, err := media.Play(ctx, plan, p, item, s.Audio.Deps())
	if err != nil {
		writeError(w, mediaErrStatus(err), err.Error())
		return
	}
	s.setZoneSession(zone.ID, sess)

	// Nudge both monitors so now-playing moves off "nothing playing" without
	// waiting for the next poll.
	s.touchZone(members)

	s.recordPlay("zone:"+zone.ID, zone.Name, store.MediaPlay{
		Provider: p.ID(),
		Kind:     body.Kind,
		URI:      body.URI,
		Title:    body.Title,
		Sub:      body.Sub,
		ArtURI:   body.ArtURI,
	})

	chain := media.DescribeQuality(p, plan.Route, s.Audio.Quality())
	writeJSON(w, http.StatusOK, map[string]any{
		"route":      plan.Route,
		"sync":       plan.Sync,
		"reason":     plan.Reason,
		"stream_url": sess.URL,
		"speakers":   names(plan.Endpoints()),
		"quality":    chain,
	})
}

// mediaZoneRoutes handles GET /api/media/zones/{id}/routes — what this zone
// could do and, for anything it can't, why not. This is what lets the UI
// explain a limitation before the user runs into it.
func (s *Server) mediaZoneRoutes(w http.ResponseWriter, r *http.Request) {
	members, _, ok := s.resolveZone(w, r)
	if !ok {
		return
	}
	p, err := s.provider(r.URL.Query().Get("provider"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := map[string]any{"speakers": names(members)}
	plan, err := media.Resolve(p, members)
	if err != nil {
		resp["problem"] = err.Error()
		var rerr *media.RouteError
		if errors.As(err, &rerr) {
			blocked := make([]map[string]string, 0, len(rerr.Blocked))
			for _, b := range rerr.Blocked {
				blocked = append(blocked, map[string]string{
					"route": string(b.Route), "reason": b.Reason,
				})
			}
			resp["blocked"] = blocked
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["route"] = plan.Route
	resp["sync"] = plan.Sync
	resp["reason"] = plan.Reason
	writeJSON(w, http.StatusOK, resp)
}

// mediaZoneTransport builds a handler for one transport verb.
func (s *Server) mediaZoneTransport(verb media.Transport) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		members, zone, ok := s.resolveZone(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), mediaTimeout)
		defer cancel()

		// Transport follows the route the zone is actually on, so a native
		// group is addressed through its coordinator rather than member by
		// member. Without a live session — after a restart, say — fall back
		// to addressing every speaker, which is correct if noisier.
		plan := s.zonePlan(zone.ID, members)
		if err := media.Control(ctx, plan, verb); err != nil {
			writeError(w, mediaErrStatus(err), err.Error())
			return
		}
		s.touchZone(members)
		w.WriteHeader(http.StatusNoContent)
	}
}

// mediaZoneVolume handles PUT /api/media/zones/{id}/volume with {"level":N}.
func (s *Server) mediaZoneVolume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Level *int `json:"level"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Level == nil {
		writeError(w, http.StatusBadRequest, "level is required")
		return
	}
	members, _, ok := s.resolveZone(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaTimeout)
	defer cancel()
	if err := media.SetVolume(ctx, members, *body.Level); err != nil {
		writeError(w, mediaErrStatus(err), err.Error())
		return
	}
	s.rememberAirPlayVolume(members, *body.Level)
	s.touchZone(members)
	w.WriteHeader(http.StatusNoContent)
}

// rememberAirPlayVolume writes a zone's new level onto any AirPlay receiver in
// it.
//
// A receiver only accepts a volume inside a session, so with nothing being
// cast the change had nowhere to travel. Storing it is what makes the slider
// mean something anyway: it is the level the next cast opens with, instead of
// whatever the last sender — possibly somebody else's laptop — left behind.
//
// Runs after the fan-out, off the device path entirely, and a failure to save
// is logged rather than returned: the speakers that could take the change have
// already taken it, and answering with an error would be reporting a
// successful volume change as a failed one.
func (s *Server) rememberAirPlayVolume(members []media.Endpoint, level int) {
	var ids []string
	for _, e := range members {
		if d := e.Descriptor(); d.Vendor == media.VendorAirPlay {
			ids = append(ids, d.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	if err := s.Store.Update(func() error {
		for _, id := range ids {
			if sp, ok := s.Store.AirPlay[id]; ok {
				sp.Volume = level
			}
		}
		return nil
	}); err != nil {
		log.Printf("media: remembering the AirPlay volume: %v", err)
	}
}

// mediaZoneMute handles PUT /api/media/zones/{id}/mute with {"muted":bool}.
func (s *Server) mediaZoneMute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Muted bool `json:"muted"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	members, _, ok := s.resolveZone(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaTimeout)
	defer cancel()
	if err := media.SetMute(ctx, members, body.Muted); err != nil {
		writeError(w, mediaErrStatus(err), err.Error())
		return
	}
	s.touchZone(members)
	w.WriteHeader(http.StatusNoContent)
}

// mediaZoneStop handles POST /api/media/zones/{id}/stop. Distinct from pause:
// it also releases a stream session, so librespot stops holding the account's
// Spotify device.
func (s *Server) mediaZoneStop(w http.ResponseWriter, r *http.Request) {
	members, zone, ok := s.resolveZone(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaTimeout)
	defer cancel()

	plan := s.zonePlan(zone.ID, members)
	err := media.Control(ctx, plan, media.TransportPause)
	s.endZoneSession(zone.ID)
	if err != nil {
		writeError(w, mediaErrStatus(err), err.Error())
		return
	}
	s.touchZone(members)
	w.WriteHeader(http.StatusNoContent)
}

// mediaSearch handles GET /api/media/search?q=&provider=&limit=
func (s *Server) mediaSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	p, err := s.provider(r.URL.Query().Get("provider"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if av := p.Available(); !av.OK {
		writeError(w, http.StatusConflict, av.Reason)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()

	res, err := p.Search(ctx, q, limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// touchZone asks both monitors to re-read the speakers a command just
// touched, so the UI's now-playing updates promptly instead of at the next
// scheduled poll.
func (s *Server) touchZone(eps []media.Endpoint) {
	sonosTouched := false
	for _, e := range eps {
		d := e.Descriptor()
		if d.Vendor == media.VendorKEF {
			s.kefEvents().Touch(d.ID)
			// A streamed KEF takes a moment to actually start, since the
			// audio comes back to it over the network.
			s.kefEvents().TouchAfter(d.ID, 3*time.Second)
			continue
		}
		sonosTouched = true
	}
	if sonosTouched {
		// Sonos is event-driven, so there is nothing per-speaker to poke;
		// a nudge makes the monitor reconcile now.
		s.sonosEvents().Nudge()
	}
}

// names lists endpoint names for a response.
func names(eps []media.Endpoint) []string {
	out := make([]string, 0, len(eps))
	for _, e := range eps {
		out = append(out, e.Descriptor().Name)
	}
	return out
}

// mediaErrStatus maps a media-layer failure to a status code. Everything the
// user can act on — connect an account, wake a speaker, install librespot,
// pick different speakers — is a 409, which is what the frontend keys its
// prompts off. A refusal from a speaker or a service stays a bad gateway.
func mediaErrStatus(err error) int {
	switch {
	case errors.Is(err, media.ErrNoRoute),
		errors.Is(err, media.ErrNoConnectDevice),
		errors.Is(err, media.ErrEmptyZone):
		return http.StatusConflict
	case errors.Is(err, media.ErrUnknownProvider),
		errors.Is(err, media.ErrUnknownEndpoint):
		return http.StatusNotFound
	}
	return http.StatusBadGateway
}

func (s *Server) mediaLogf(format string, args ...any) {
	log.Printf(format, args...)
}
