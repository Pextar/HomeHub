package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/sonos"
	"homehub/internal/store"
)

// The Sonos integration replaces day-to-day use of the Sonos app: playback,
// volume, grouping and favorites over the speakers' local UPnP API. All
// endpoints are admin-gated (registered in server.go), matching the posture
// of the other whole-home surfaces.

// sonosSpeakerView is a registered speaker plus its live state. State is
// nil when the speaker didn't answer within the status timeout. GroupState
// is present only on coordinators — shuffle, repeat, crossfade and the queue
// belong to the group, and asking every follower for them would triple the
// poll for no new information.
type sonosSpeakerView struct {
	store.SonosSpeaker
	Reachable  bool              `json:"reachable"`
	State      *sonos.State      `json:"state,omitempty"`
	GroupState *sonos.GroupState `json:"group_state,omitempty"`
}

// sonosGroupView is one live zone group mapped onto registered speaker IDs.
// Members that are grouped on the Sonos side but not registered in HomeHub
// are surfaced by name so the UI can suggest adding them.
type sonosGroupView struct {
	CoordinatorID string   `json:"coordinator_id"`
	MemberIDs     []string `json:"member_ids"`
	Unregistered  []string `json:"unregistered,omitempty"` // zone names
}

// sonosStatus handles GET /api/sonos/status — the Music view's single poll:
// every registered speaker's live state plus the current group topology.
func (s *Server) sonosStatus(w http.ResponseWriter, r *http.Request) {
	s.Store.Mu.RLock()
	speakers := make([]store.SonosSpeaker, 0, len(s.Store.Sonos))
	for _, sp := range s.Store.Sonos {
		speakers = append(speakers, *sp)
	}
	s.Store.Mu.RUnlock()
	sort.Slice(speakers, func(i, j int) bool { return speakers[i].Name < speakers[j].Name })

	views := make([]sonosSpeakerView, len(speakers))
	var mu sync.Mutex
	var topology []sonos.Group

	// Fan the state fetches out concurrently — with several speakers a
	// serial poll would multiply the per-device latency.
	var wg sync.WaitGroup
	for i, sp := range speakers {
		wg.Add(1)
		go func(i int, sp store.SonosSpeaker) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			st, err := sonos.GetState(ctx, sp.IP)
			v := sonosSpeakerView{SonosSpeaker: sp, Reachable: err == nil, State: st}
			if st != nil && st.Track != nil {
				st.Track.ArtURI = s.sonosArtURL(sp.ID, st.Track.ArtURI)
			}
			mu.Lock()
			views[i] = v
			// Topology comes from the first speaker that answers; any
			// speaker can describe the whole household.
			if err == nil && topology == nil {
				tctx, tcancel := context.WithTimeout(r.Context(), 3*time.Second)
				if groups, terr := sonos.GetTopology(tctx, sp.IP); terr == nil {
					topology = groups
				}
				tcancel()
			}
			mu.Unlock()
		}(i, sp)
	}
	wg.Wait()

	groups := s.sonosMapGroups(topology, speakers)

	// Second pass: the group-level settings, for coordinators only. This
	// needs the topology, which only exists once the first pass is done.
	index := make(map[string]int, len(speakers))
	for i, sp := range speakers {
		index[sp.ID] = i
	}
	var wg2 sync.WaitGroup
	for _, g := range groups {
		i, ok := index[g.CoordinatorID]
		if !ok || !views[i].Reachable {
			continue
		}
		wg2.Add(1)
		go func(i int, ip string) {
			defer wg2.Done()
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			gs, err := sonos.GetGroupState(ctx, ip)
			if err != nil {
				return // leave it absent; the UI hides the controls
			}
			mu.Lock()
			views[i].GroupState = gs
			mu.Unlock()
		}(i, speakers[i].IP)
	}
	wg2.Wait()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"speakers": views,
		"groups":   groups,
	})
}

// sonosMapGroups translates live topology (RINCON UUIDs) into registered
// speaker IDs. Groups with no registered member at all are dropped.
func (s *Server) sonosMapGroups(topology []sonos.Group, speakers []store.SonosSpeaker) []sonosGroupView {
	byUUID := make(map[string]string, len(speakers)) // RINCON → speaker id
	for _, sp := range speakers {
		if sp.UUID != "" {
			byUUID[sp.UUID] = sp.ID
		}
	}
	out := make([]sonosGroupView, 0, len(topology))
	for _, g := range topology {
		var v sonosGroupView
		v.CoordinatorID = byUUID[g.CoordinatorUUID]
		for _, m := range g.Members {
			if id, ok := byUUID[m.UUID]; ok {
				v.MemberIDs = append(v.MemberIDs, id)
			} else {
				v.Unregistered = append(v.Unregistered, m.Name)
			}
		}
		if len(v.MemberIDs) > 0 {
			out = append(out, v)
		}
	}
	return out
}

// sonosDiscover handles GET /api/sonos/discover — SSDP scan plus topology
// expansion. Slowish by nature (~3s); the frontend shows a skeleton.
func (s *Server) sonosDiscover(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	devices, err := sonos.Discover(ctx, 2*time.Second)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Mark devices that are already registered so the UI can filter them.
	s.Store.Mu.RLock()
	known := make(map[string]bool, len(s.Store.Sonos))
	for _, sp := range s.Store.Sonos {
		known[sp.UUID] = true
	}
	s.Store.Mu.RUnlock()

	type candidate struct {
		sonos.Device
		Registered bool `json:"registered"`
	}
	out := make([]candidate, 0, len(devices))
	for _, d := range devices {
		out = append(out, candidate{Device: d, Registered: known[d.UUID]})
	}
	writeJSON(w, http.StatusOK, out)
}

// sonosCreateSpeaker handles POST /api/sonos/speakers. The speaker must be
// reachable: its identity (UUID, model, zone name) is read from the device
// itself, which both verifies the IP points at a Sonos and fills in fields
// the user shouldn't have to type.
func (s *Server) sonosCreateSpeaker(w http.ResponseWriter, r *http.Request) {
	var sp store.SonosSpeaker
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := sonos.ValidateHost(sp.IP); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	dev, err := sonos.Describe(ctx, sp.IP)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	sp.UUID = dev.UUID
	sp.Model = dev.Model
	if strings.TrimSpace(sp.Name) == "" {
		sp.Name = dev.Room
	}
	if strings.TrimSpace(sp.Room) == "" {
		sp.Room = dev.Room
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()
	sp.ID = fmt.Sprintf("sonos_%d", time.Now().UnixNano())
	if err := s.Store.ValidateSonosSpeaker(&sp); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Store.Sonos[sp.ID] = &sp
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Sonos, sp.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sp)
}

// sonosUpdateSpeaker handles PUT /api/sonos/speakers/{id}. Only name, room
// and IP are user-editable; identity fields stay device-derived.
func (s *Server) sonosUpdateSpeaker(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updates store.SonosSpeaker
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()
	existing, ok := s.Store.Sonos[id]
	if !ok {
		writeError(w, http.StatusNotFound, "speaker not found")
		return
	}
	merged := *existing
	if v := strings.TrimSpace(updates.Name); v != "" {
		merged.Name = v
	}
	if v := strings.TrimSpace(updates.IP); v != "" {
		merged.IP = v
	}
	merged.Room = strings.TrimSpace(updates.Room)
	if err := s.Store.ValidateSonosSpeaker(&merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	*existing = merged
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// sonosDeleteSpeaker handles DELETE /api/sonos/speakers/{id}.
func (s *Server) sonosDeleteSpeaker(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.Store.Mu.Lock()
	if _, ok := s.Store.Sonos[id]; !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "speaker not found")
		return
	}
	delete(s.Store.Sonos, id)
	if err := s.Store.Save(); err != nil {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// sonosSpeaker resolves a {id} route var to the stored speaker (a copy,
// safe to use off-lock). Writes the error response itself on failure.
func (s *Server) sonosSpeaker(w http.ResponseWriter, r *http.Request) (store.SonosSpeaker, bool) {
	id := mux.Vars(r)["id"]
	s.Store.Mu.RLock()
	sp, ok := s.Store.Sonos[id]
	var cp store.SonosSpeaker
	if ok {
		cp = *sp
	}
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "speaker not found")
		return store.SonosSpeaker{}, false
	}
	return cp, true
}

// sonosTransport builds the handler for the parameterless transport
// actions: play, pause, next, previous, leave.
func (s *Server) sonosTransport(action func(context.Context, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sp, ok := s.sonosSpeaker(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
		defer cancel()
		if err := action(ctx, sp.IP); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// sonosSetVolume handles PUT /api/sonos/{id}/volume with {"level": 0-100}.
// With "group": true the level is applied to the speaker's whole group
// (send to the coordinator), preserving members' relative levels.
func (s *Server) sonosSetVolume(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Level int  `json:"level"`
		Group bool `json:"group"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Level < 0 || body.Level > 100 {
		writeError(w, http.StatusBadRequest, "level must be between 0 and 100")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	var err error
	if body.Group {
		err = sonos.SetGroupVolume(ctx, sp.IP, body.Level)
	} else {
		err = sonos.SetVolume(ctx, sp.IP, body.Level)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosSetMute handles PUT /api/sonos/{id}/mute with {"muted": bool}.
func (s *Server) sonosSetMute(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Muted bool `json:"muted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	if err := sonos.SetMute(ctx, sp.IP, body.Muted); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosSeek handles PUT /api/sonos/{id}/seek. Either {"position":"0:01:23"}
// to move within the current track, or {"track": 4} to jump to a queue
// position. A track jump goes through PlayFromQueue so it also works when
// the group is parked on another source (radio, line-in), where a bare seek
// would be refused.
func (s *Server) sonosSeek(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Position string `json:"position"`
		Track    int    `json:"track"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Position == "" && body.Track == 0 {
		writeError(w, http.StatusBadRequest, "either position or track is required")
		return
	}

	// A track jump is three SOAP calls; give it the wider budget.
	ctx, cancel := context.WithTimeout(r.Context(), 2*sonos.DefaultTimeout)
	defer cancel()
	var err error
	if body.Track > 0 {
		err = sonos.PlayFromQueue(ctx, sp.IP, sp.UUID, body.Track)
	} else {
		err = sonos.Seek(ctx, sp.IP, body.Position)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosSetPlayMode handles PUT /api/sonos/{id}/playmode with
// {"shuffle": bool, "repeat": "off"|"all"|"one"}. Both axes are always sent
// together because Sonos stores them as one composite value.
func (s *Server) sonosSetPlayMode(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var mode sonos.PlayMode
	if err := json.NewDecoder(r.Body).Decode(&mode); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if mode.Repeat == "" {
		mode.Repeat = sonos.RepeatOff
	}
	if !mode.Valid() {
		writeError(w, http.StatusBadRequest, `repeat must be "off", "all" or "one"`)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	if err := sonos.SetPlayMode(ctx, sp.IP, mode); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosSetCrossfade handles PUT /api/sonos/{id}/crossfade with
// {"enabled": bool}.
func (s *Server) sonosSetCrossfade(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	if err := sonos.SetCrossfade(ctx, sp.IP, body.Enabled); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosQueue handles GET /api/sonos/{id}/queue — the group queue of the
// coordinator {id}, capped at sonos.MaxQueueFetch entries.
func (s *Server) sonosQueue(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	items, err := sonos.ListQueue(ctx, sp.IP)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	for i := range items {
		items[i].ArtURI = s.sonosArtURL(sp.ID, items[i].ArtURI)
	}
	writeJSON(w, http.StatusOK, items)
}

// sonosQueueAdd handles POST /api/sonos/{id}/queue — enqueue without
// disturbing what is playing. The body is either a streaming-service item
// ({"service":"Spotify","uri":"spotify:track:…","title":"…"}) or a favorite's
// raw uri/metadata pair. With {"next": true} it lands after the current
// track instead of at the end.
func (s *Server) sonosQueueAdd(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Service  string `json:"service"`
		URI      string `json:"uri"`
		Title    string `json:"title"`
		Metadata string `json:"metadata"`
		Next     bool   `json:"next"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(body.URI) == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*sonos.DefaultTimeout)
	defer cancel()

	uri, meta := body.URI, body.Metadata
	// A service item arrives as a canonical service URI and has to be
	// resolved against the household's linked account first; a favorite
	// already carries a playable uri/metadata pair, so it passes straight
	// through.
	if body.Service != "" {
		if !strings.EqualFold(body.Service, "Spotify") {
			writeError(w, http.StatusBadRequest, "only Spotify items are supported so far")
			return
		}
		acct, err := s.sonosServiceAccount(ctx, sp.IP, body.Service)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		uri, meta, err = sonos.SpotifyItem(body.URI, body.Title, acct)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	added, err := sonos.AddToQueue(ctx, sp.IP, uri, meta, body.Next)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, added)
}

// sonosQueueRemove handles DELETE /api/sonos/{id}/queue/{track}.
func (s *Server) sonosQueueRemove(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	track, err := strconv.Atoi(mux.Vars(r)["track"])
	if err != nil || track < 1 {
		writeError(w, http.StatusBadRequest, "track must be a positive number")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	if err := sonos.RemoveFromQueue(ctx, sp.IP, track); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosQueueClear handles DELETE /api/sonos/{id}/queue — empties the group
// queue, which also stops playback.
func (s *Server) sonosQueueClear(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	if err := sonos.ClearQueue(ctx, sp.IP); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosJoin handles POST /api/sonos/{id}/join with {"target_id": "..."} —
// the speaker joins the group whose coordinator is target_id.
func (s *Server) sonosJoin(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		TargetID string `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	s.Store.Mu.RLock()
	target, ok := s.Store.Sonos[body.TargetID]
	var targetUUID string
	if ok {
		targetUUID = target.UUID
	}
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "target speaker not found")
		return
	}
	if targetUUID == "" {
		writeError(w, http.StatusBadRequest, "target speaker has no device id — re-add it")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	if err := sonos.Join(ctx, sp.IP, targetUUID); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosFavorites handles GET /api/sonos/{id}/favorites. Favorites are
// household-wide; any registered speaker can list them.
func (s *Server) sonosFavorites(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	favs, err := sonos.ListFavorites(ctx, sp.IP)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	for i := range favs {
		favs[i].ArtURI = s.sonosArtURL(sp.ID, favs[i].ArtURI)
	}
	writeJSON(w, http.StatusOK, favs)
}

// sonosPlayFavorite handles POST /api/sonos/{id}/favorites/play. The body
// round-trips the uri/metadata pair from the favorites listing. {id} must
// be the coordinator of the group that should start playing.
func (s *Server) sonosPlayFavorite(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var fav sonos.Favorite
	if err := json.NewDecoder(r.Body).Decode(&fav); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(fav.URI) == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	// Favorite playback is up to four SOAP calls; give it a bit more room.
	ctx, cancel := context.WithTimeout(r.Context(), 2*sonos.DefaultTimeout)
	defer cancel()
	if err := sonos.PlayFavorite(ctx, sp.IP, sp.UUID, fav); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosPlayItem handles POST /api/sonos/{id}/play-item — plays a streaming-
// service item (currently Spotify) on the group led by speaker {id}. The
// body carries the canonical service URI from search/browse results:
// {"service": "Spotify", "uri": "spotify:track:…", "title": "…"}.
// The speaker streams with the household's linked account; sid/sn are
// resolved from the speaker and cached per address.
func (s *Server) sonosPlayItem(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Service string `json:"service"`
		URI     string `json:"uri"`
		Title   string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Service == "" {
		body.Service = "Spotify"
	}
	if !strings.EqualFold(body.Service, "Spotify") {
		writeError(w, http.StatusBadRequest, "only Spotify items are supported so far")
		return
	}

	// Item playback is several SOAP round-trips (+ account resolution on
	// the first play); give it more room than a single call.
	ctx, cancel := context.WithTimeout(r.Context(), 3*sonos.DefaultTimeout)
	defer cancel()

	acct, err := s.sonosServiceAccount(ctx, sp.IP, body.Service)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	uri, meta, err := sonos.SpotifyItem(body.URI, body.Title, acct)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := sonos.PlayServiceItem(ctx, sp.IP, sp.UUID, uri, meta); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sonosServiceAccount resolves (and caches) a speaker's account for a
// streaming service. The sid/sn pair only changes when the household's
// service links change, so an hour of caching keeps play taps at four SOAP
// calls instead of six.
func (s *Server) sonosServiceAccount(ctx context.Context, ip, service string) (*sonos.ServiceAccount, error) {
	key := ip + "|" + strings.ToLower(service)
	s.sonosAcctMu.Lock()
	if s.sonosAccts == nil {
		s.sonosAccts = make(map[string]sonosAcctEntry)
	}
	if e, ok := s.sonosAccts[key]; ok && time.Since(e.at) < time.Hour {
		s.sonosAcctMu.Unlock()
		return e.acct, nil
	}
	s.sonosAcctMu.Unlock()

	acct, err := sonos.GetServiceAccount(ctx, ip, service)
	if err != nil {
		return nil, err
	}
	s.sonosAcctMu.Lock()
	s.sonosAccts[key] = sonosAcctEntry{acct: acct, at: time.Now()}
	s.sonosAcctMu.Unlock()
	return acct, nil
}

// sonosArtURL rewrites a speaker-relative album-art path into our proxy
// endpoint (the app may be served over HTTPS, where a plain-http image from
// the speaker would be blocked as mixed content). Absolute URLs — typically
// CDN art from streaming services — pass through untouched.
func (s *Server) sonosArtURL(speakerID, artURI string) string {
	if artURI == "" || !strings.HasPrefix(artURI, "/") {
		return artURI
	}
	return "/api/sonos/" + url.PathEscape(speakerID) + "/art?u=" + url.QueryEscape(artURI)
}

// sonosArt handles GET /api/sonos/{id}/art?u=<path> — proxies album art
// from the speaker. Only speaker-relative paths are accepted, so this
// cannot be used to fetch arbitrary URLs.
func (s *Server) sonosArt(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	p := r.URL.Query().Get("u")
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.Contains(p, "..") {
		writeError(w, http.StatusBadRequest, "u must be a speaker-relative path")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()
	u := fmt.Sprintf("http://%s:%d%s", sp.IP, sonos.Port, p)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("speaker returned HTTP %d", resp.StatusCode))
		return
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	// Art is immutable per URL; let the browser cache it.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 5<<20))
}
