package api

// The AirPlay receiver endpoints.
//
// Thinner than the Sonos and KEF bridges, and the thinness is the protocol
// showing through. Those two expose a speaker's own settings — crossfade,
// source selection, EQ — because the speaker holds them. An AirPlay receiver
// holds nothing: it has an address, a volume, and whatever HomeHub is sending
// it. So this file is registration, discovery and volume, and everything about
// playing is the zone's business, through the media layer.

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/airplay"
	"homehub/internal/media"
	"homehub/internal/mediabridge"
	"homehub/internal/store"
)

// airplayScanWindow is how long a scan listens. mDNS answers arrive in the
// first few hundred milliseconds when they arrive at all; the rest of the
// window is for the responder that missed the first query.
const airplayScanWindow = 2500 * time.Millisecond

// airplayStatus handles GET /api/airplay/status — the registered receivers.
//
// No reachability probe, unlike the other two bridges. Asking a receiver
// whether it is there means opening an RTSP session, and an RTSP session is
// exactly the thing that takes it away from whatever else is playing to it:
// checking would interrupt the Mac that is using it. So the list says what is
// registered, and a receiver that has gone away is discovered at the moment
// something tries to play to it, with the connection error naming it.
func (s *Server) airplayStatus(w http.ResponseWriter, r *http.Request) {
	type view struct {
		store.AirPlaySpeaker
		// Member is the qualified id a zone stores, so the Music view can
		// build a zone from this list without knowing the prefix.
		Member string `json:"member"`
		// Casting is whether HomeHub is sending to it right now.
		Casting bool `json:"casting"`
	}
	caster := s.airplayCaster()

	var out []view
	s.Store.View(func() {
		out = make([]view, 0, len(s.Store.AirPlay))
		for _, sp := range s.Store.AirPlay {
			_, live := caster.Live(sp.ID)
			out = append(out, view{
				AirPlaySpeaker: *sp,
				Member:         store.QualifyAirPlay(sp.ID),
				Casting:        live,
			})
		}
	})
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSON(w, http.StatusOK, out)
}

// airplayDiscover handles GET /api/airplay/discover.
//
// Each result carries the receiver's own answer to "will you take the session
// HomeHub opens", asked during the scan rather than guessed from its
// advertisement. That distinction is the difference between finding an
// AirPlay 2 RoPieee and refusing one.
func (s *Server) airplayDiscover(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	devices, err := airplay.Discover(ctx, airplayScanWindow)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Registered receivers are matched by identity first and address
	// second: a box that moved to a new DHCP lease is the same box, and one
	// that took over an old lease is not.
	var knownID, knownIP map[string]bool
	s.Store.View(func() {
		knownID = make(map[string]bool, len(s.Store.AirPlay))
		knownIP = make(map[string]bool, len(s.Store.AirPlay))
		for _, sp := range s.Store.AirPlay {
			if sp.DeviceID != "" {
				knownID[sp.DeviceID] = true
			}
			knownIP[sp.IP] = true
		}
	})

	type candidate struct {
		airplay.Device
		// Supported and Problem say whether HomeHub could actually drive
		// this receiver. A scan that lists a FairPlay-only Apple TV as
		// though it were addable is a scan that sets up a failure two taps
		// later.
		Supported bool   `json:"supported"`
		Problem   string `json:"problem,omitempty"`
	}
	out := make([]candidate, 0, len(devices))
	for _, d := range devices {
		d.Registered = (d.ID != "" && knownID[d.ID]) || knownIP[d.IP]
		ok, why := d.Supported()
		out = append(out, candidate{Device: d, Supported: ok, Problem: why})
	}
	writeJSON(w, http.StatusOK, out)
}

// airplayCreateSpeaker handles POST /api/airplay/speakers.
//
// The body may carry everything a scan learned, or nothing but an address. A
// bare address is probed — which proves something is answering RAOP there and
// nothing more, since the codec and encryption lists live in the mDNS
// advertisement a direct connection never sees. See airplay.Probe.
func (s *Server) airplayCreateSpeaker(w http.ResponseWriter, r *http.Request) {
	var sp store.AirPlaySpeaker
	if !decodeBody(w, r, &sp) {
		return
	}
	if err := airplay.ValidateHost(sp.IP); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	dev, err := airplay.Probe(ctx, sp.IP, sp.Port)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	sp.Port = dev.Port
	if strings.TrimSpace(sp.Name) == "" {
		sp.Name = dev.Name
	}
	// A probe cannot see the mDNS advertisement, so it never learns that a
	// box is AirPlay 2 — only a scan does, and it puts it in the body.
	if dev.AirPlay2 {
		sp.AirPlay2 = true
	}
	// A registration that carried no codec flags — the typed-in path — takes
	// the conservative pair every RAOP receiver supports. The session falls
	// back through the alternatives anyway, so this is a starting point
	// rather than a commitment.
	if !sp.PCM && !sp.ALAC {
		sp.PCM, sp.ALAC, sp.Metadata = true, true, true
	}

	if !s.update(w, func() error {
		sp.ID = fmt.Sprintf("airplay_%d", time.Now().UnixNano())
		if err := s.Store.ValidateAirPlaySpeaker(&sp); err != nil {
			return errInvalid(err)
		}
		s.Store.AirPlay[sp.ID] = &sp
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusCreated, sp)
}

// airplayUpdateSpeaker handles PUT /api/airplay/speakers/{id}. Name, room and
// address are user-editable; what the receiver said it accepts is not, because
// it is the receiver's answer and not the household's.
func (s *Server) airplayUpdateSpeaker(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updates store.AirPlaySpeaker
	if !decodeBody(w, r, &updates) {
		return
	}

	var existing *store.AirPlaySpeaker
	if !s.update(w, func() error {
		var ok bool
		existing, ok = s.Store.AirPlay[id]
		if !ok {
			return errStatus(http.StatusNotFound, "receiver not found")
		}
		merged := *existing
		if v := strings.TrimSpace(updates.Name); v != "" {
			merged.Name = v
		}
		if v := strings.TrimSpace(updates.IP); v != "" {
			merged.IP = v
		}
		if updates.Port > 0 {
			merged.Port = updates.Port
		}
		merged.Room = strings.TrimSpace(updates.Room)
		if err := s.Store.ValidateAirPlaySpeaker(&merged); err != nil {
			return errInvalid(err)
		}
		*existing = merged
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// airplayDeleteSpeaker handles DELETE /api/airplay/speakers/{id}.
func (s *Server) airplayDeleteSpeaker(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !s.update(w, func() error {
		if _, ok := s.Store.AirPlay[id]; !ok {
			return errNotFound("receiver")
		}
		delete(s.Store.AirPlay, id)
		// Drop it from any zone that held it — see sonosDeleteSpeaker.
		s.Store.CascadeDeleteSpeaker(store.QualifyAirPlay(id))
		return nil
	}) {
		return
	}
	s.pruneDeadRooms()
	w.WriteHeader(http.StatusNoContent)
}

// airplaySetVolume handles PUT /api/airplay/{id}/volume.
//
// Two things happen, and both matter. The level is stored, because a receiver
// only accepts a volume inside a session and the stored one is what the next
// cast opens with. And when a cast *is* running, it is sent — otherwise the
// slider would move without the room getting quieter.
func (s *Server) airplaySetVolume(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Level int `json:"level"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	var name string
	if !s.update(w, func() error {
		sp, ok := s.Store.AirPlay[id]
		if !ok {
			return errNotFound("receiver")
		}
		merged := *sp
		merged.Volume = body.Level
		if err := s.Store.ValidateAirPlaySpeaker(&merged); err != nil {
			return errInvalid(err)
		}
		*sp = merged
		name = sp.Name
		return nil
	}) {
		return
	}

	// Off-lock, as every device call must be.
	if ctrl, live := s.airplayCaster().Live(id); live {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := ctrl.SetVolume(ctx, body.Level); err != nil {
			writeError(w, http.StatusBadGateway,
				fmt.Sprintf("%s took the level but didn't apply it: %v", name, err))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// airplayEndpoints builds live endpoints for every registered receiver.
// Caller must hold Mu (read is enough).
func (s *Server) airplayEndpoints(out map[string]media.Endpoint) {
	caster := s.airplayCaster()
	for id, sp := range s.Store.AirPlay {
		out[store.QualifyAirPlay(id)] = mediabridge.NewAirPlayEndpoint(*sp, caster.Live)
	}
}
