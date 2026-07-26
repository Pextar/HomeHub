package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/kef"
	"homehub/internal/store"
)

// The KEF integration is the second speaker bridge, sitting beside the Sonos
// one (sonos.go) rather than on top of it. The two devices don't share a
// protocol and — more importantly — don't share a model: a Sonos household
// is zones that group and share a queue, while a KEF speaker is one
// standalone stereo pair with an input selector. Folding them into one
// abstraction would mean either inventing groups KEF doesn't have or
// dropping the ones Sonos does, so they stay separate and the Music view
// renders both.
//
// All endpoints are admin-gated (registered in server.go), matching the
// other whole-home surfaces.

// kefSpeakerView is a registered speaker plus its live state. State is nil
// when the speaker didn't answer.
type kefSpeakerView struct {
	store.KEFSpeaker
	Reachable bool       `json:"reachable"`
	State     *kef.State `json:"state,omitempty"`
	// ReadAt is when the state was taken, in unix milliseconds, so the
	// client can extrapolate playback position from the right instant
	// rather than from when its own request happened to land.
	ReadAt int64 `json:"read_at,omitempty"`
}

// kefStatus handles GET /api/kef/status — every registered KEF speaker's
// live state, the Music view's single poll for them.
//
// The reading itself belongs to the monitor (internal/kef/monitor.go), which
// polls the speakers once for the whole process and normally answers this
// from a warm cache without touching the network. "warm" tells the frontend
// which of the two it got.
func (s *Server) kefStatus(w http.ResponseWriter, r *http.Request) {
	speakers := s.kefStoredSpeakers()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	snap := s.kefEvents().Snapshot(ctx)

	views := make([]kefSpeakerView, len(speakers))
	for i, sp := range speakers {
		cached := snap.Speakers[sp.ID]
		// Snapshot hands out copies, so rewriting the art URI can't corrupt
		// what the next reader sees.
		if cached.State != nil && cached.State.Track != nil {
			cached.State.Track.ArtURI = s.kefArtURL(sp.ID, cached.State.Track.ArtURI)
		}
		views[i] = kefSpeakerView{
			KEFSpeaker: sp,
			Reachable:  cached.Reachable,
			State:      cached.State,
		}
		if !cached.At.IsZero() {
			views[i].ReadAt = cached.At.UnixMilli()
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"speakers": views,
		"warm":     snap.Warm,
	})
}

// kefStoredSpeakers returns every registered speaker, sorted by name.
func (s *Server) kefStoredSpeakers() []store.KEFSpeaker {
	s.Store.Mu.RLock()
	out := make([]store.KEFSpeaker, 0, len(s.Store.KEF))
	for _, sp := range s.Store.KEF {
		out = append(out, *sp)
	}
	s.Store.Mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// kefDiscover handles GET /api/kef/discover — an SSDP scan, with every
// responder then asked whether it speaks the KEF API. Slowish by nature
// (~3s); the frontend shows a skeleton.
func (s *Server) kefDiscover(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	devices, err := kef.Discover(ctx, 2*time.Second)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Mark devices that are already registered so the UI can filter them.
	s.Store.Mu.RLock()
	known := make(map[string]bool, len(s.Store.KEF))
	for _, sp := range s.Store.KEF {
		known[sp.MAC] = true
	}
	s.Store.Mu.RUnlock()

	type candidate struct {
		kef.Device
		Registered bool `json:"registered"`
	}
	out := make([]candidate, 0, len(devices))
	for _, d := range devices {
		out = append(out, candidate{Device: d, Registered: known[d.MAC]})
	}
	writeJSON(w, http.StatusOK, out)
}

// kefCreateSpeaker handles POST /api/kef/speakers. The speaker must be
// reachable: its identity (MAC, model, device name) is read from the device
// itself, which both verifies the address points at a KEF and fills in
// fields the user shouldn't have to type.
func (s *Server) kefCreateSpeaker(w http.ResponseWriter, r *http.Request) {
	var sp store.KEFSpeaker
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := kef.ValidateHost(sp.IP); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*kef.DefaultTimeout)
	defer cancel()
	dev, err := kef.Describe(ctx, sp.IP)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	sp.MAC = dev.MAC
	sp.Model = dev.Model
	if strings.TrimSpace(sp.Name) == "" {
		sp.Name = dev.Name
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()
	sp.ID = fmt.Sprintf("kef_%d", time.Now().UnixNano())
	if err := s.Store.ValidateKEFSpeaker(&sp); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Store.KEF[sp.ID] = &sp
	if err := s.Store.Save(); err != nil {
		delete(s.Store.KEF, sp.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.kefEvents().Nudge() // start watching it now, not at the next reconcile
	writeJSON(w, http.StatusCreated, sp)
}

// kefUpdateSpeaker handles PUT /api/kef/speakers/{id}. Only name, room and
// address are user-editable; identity fields stay device-derived.
func (s *Server) kefUpdateSpeaker(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updates store.KEFSpeaker
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()
	existing, ok := s.Store.KEF[id]
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
	if err := s.Store.ValidateKEFSpeaker(&merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	*existing = merged
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	// A re-addressed speaker needs its poller repointed; the old one is
	// reading an address that is no longer it.
	s.kefEvents().Nudge()
	writeJSON(w, http.StatusOK, existing)
}

// kefDeleteSpeaker handles DELETE /api/kef/speakers/{id}.
func (s *Server) kefDeleteSpeaker(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.Store.Mu.Lock()
	if _, ok := s.Store.KEF[id]; !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "speaker not found")
		return
	}
	delete(s.Store.KEF, id)
	// Drop it from any zone that held it — see sonosDeleteSpeaker.
	s.Store.CascadeDeleteSpeaker(store.QualifyKEF(id))
	if err := s.Store.Save(); err != nil {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()
	s.kefEvents().Nudge() // stop polling it
	w.WriteHeader(http.StatusNoContent)
}

// kefSpeaker resolves a {id} route var to the stored speaker (a copy, safe
// to use off-lock). Writes the error response itself on failure.
func (s *Server) kefSpeaker(w http.ResponseWriter, r *http.Request) (store.KEFSpeaker, bool) {
	id := mux.Vars(r)["id"]
	s.Store.Mu.RLock()
	sp, ok := s.Store.KEF[id]
	var cp store.KEFSpeaker
	if ok {
		cp = *sp
	}
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "speaker not found")
		return store.KEFSpeaker{}, false
	}
	return cp, true
}

// kefTransport builds the handler for the parameterless transport actions:
// play, pause, next, previous.
func (s *Server) kefTransport(action func(context.Context, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sp, ok := s.kefSpeaker(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), kef.DefaultTimeout)
		defer cancel()
		if err := action(ctx, sp.IP); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.kefEvents().Touch(sp.ID)
		w.WriteHeader(http.StatusNoContent)
	}
}

// kefSetVolume handles PUT /api/kef/{id}/volume with {"level": 0-100}.
// There is no group volume: a KEF speaker is a stereo pair addressed as one
// device, so its own volume is the only one there is.
func (s *Server) kefSetVolume(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.kefSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Level int `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Level < 0 || body.Level > 100 {
		writeError(w, http.StatusBadRequest, "level must be between 0 and 100")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), kef.DefaultTimeout)
	defer cancel()
	if err := kef.SetVolume(ctx, sp.IP, body.Level); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.kefEvents().Touch(sp.ID)
	w.WriteHeader(http.StatusNoContent)
}

// kefSetMute handles PUT /api/kef/{id}/mute with {"muted": bool}.
func (s *Server) kefSetMute(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.kefSpeaker(w, r)
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
	ctx, cancel := context.WithTimeout(r.Context(), kef.DefaultTimeout)
	defer cancel()
	if err := kef.SetMute(ctx, sp.IP, body.Muted); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.kefEvents().Touch(sp.ID)
	w.WriteHeader(http.StatusNoContent)
}

// kefSetSource handles PUT /api/kef/{id}/source with {"source": "optic"}.
// The speaker's input selector is the KEF equivalent of picking what plays:
// unlike Sonos there is no queue to point somewhere, so switching to the
// optical input *is* the "play the TV" action.
func (s *Server) kefSetSource(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.kefSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if !kef.ValidSource(body.Source) {
		writeError(w, http.StatusBadRequest,
			"source must be one of: "+strings.Join(kef.AllSources, ", "))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), kef.DefaultTimeout)
	defer cancel()
	if err := kef.SetSource(ctx, sp.IP, body.Source); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.kefEvents().Touch(sp.ID)
	w.WriteHeader(http.StatusNoContent)
}

// kefSetPower handles PUT /api/kef/{id}/power with {"on": bool} — wake the
// speaker or send it to standby.
func (s *Server) kefSetPower(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.kefSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), kef.DefaultTimeout)
	defer cancel()
	if err := kef.SetStandby(ctx, sp.IP, !body.On); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.kefEvents().Touch(sp.ID)
	w.WriteHeader(http.StatusNoContent)
}

// kefSettings handles GET /api/kef/{id}/settings — the configuration
// snapshot behind the settings pane. Read on demand, never polled: none of
// it changes on its own.
func (s *Server) kefSettings(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.kefSpeaker(w, r)
	if !ok {
		return
	}
	// Sixteen-odd paths, run in parallel; the budget covers one slow speaker
	// rather than the sum of them all.
	ctx, cancel := context.WithTimeout(r.Context(), 3*kef.DefaultTimeout)
	defer cancel()
	settings, err := kef.GetSettings(ctx, sp.IP)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// kefUpdateSettings handles PUT /api/kef/{id}/settings with a partial
// patch. Only the fields present are written.
func (s *Server) kefUpdateSettings(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.kefSpeaker(w, r)
	if !ok {
		return
	}
	var patch kef.SettingsPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if patch.Empty() {
		writeError(w, http.StatusBadRequest, "no settings to change")
		return
	}
	if err := patch.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*kef.DefaultTimeout)
	defer cancel()
	if err := kef.ApplySettings(ctx, sp.IP, patch); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	// Volume-limit changes can move the live volume, so the status cache
	// should catch up rather than wait out the poll.
	s.kefEvents().Touch(sp.ID)
	w.WriteHeader(http.StatusNoContent)
}

// kefArtURL leaves absolute artwork URLs alone and routes speaker-relative
// ones through our proxy. KEF gets its artwork from the streaming service,
// so in practice these are already absolute — but a relative path served
// over plain HTTP would be blocked as mixed content on an HTTPS install,
// which is the same reason the Sonos bridge proxies its album art.
func (s *Server) kefArtURL(speakerID, artURI string) string {
	if artURI == "" || !strings.HasPrefix(artURI, "/") {
		return artURI
	}
	return "/api/kef/" + url.PathEscape(speakerID) + "/art?u=" + url.QueryEscape(artURI)
}

// kefArt handles GET /api/kef/{id}/art?u=<path> — proxies artwork from the
// speaker. Only speaker-relative paths are accepted, so this cannot be used
// to fetch arbitrary URLs.
func (s *Server) kefArt(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.kefSpeaker(w, r)
	if !ok {
		return
	}
	p := r.URL.Query().Get("u")
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.Contains(p, "..") {
		writeError(w, http.StatusBadRequest, "u must be a speaker-relative path")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), kef.DefaultTimeout)
	defer cancel()
	u := fmt.Sprintf("http://%s%s", sp.IP, p)
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

// ── Monitor lifecycle ────────────────────────────────────────────────────

// kefEvents returns the speaker monitor, building it on first use.
func (s *Server) kefEvents() *kef.Monitor {
	s.kefMonMu.Lock()
	defer s.kefMonMu.Unlock()
	if s.kefMon == nil {
		s.kefMon = kef.NewMonitor(kef.MonitorConfig{
			Speakers: s.kefSpeakerList,
			OnChange: s.broadcastMusic,
			Logf:     log.Printf,
		})
	}
	return s.kefMon
}

// RunKEFEvents keeps the KEF speakers polled until ctx is cancelled. Call it
// once, from main, after Handler().
func (s *Server) RunKEFEvents(ctx context.Context) {
	s.kefEvents().Run(ctx)
}

// kefSpeakerList adapts the store's speakers to what the monitor needs.
func (s *Server) kefSpeakerList() []kef.Speaker {
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	out := make([]kef.Speaker, 0, len(s.Store.KEF))
	for _, sp := range s.Store.KEF {
		out = append(out, kef.Speaker{ID: sp.ID, IP: sp.IP})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
