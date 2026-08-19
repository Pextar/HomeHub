package api

// Wiring for the Sonos event monitor (internal/sonos/monitor.go): the
// callback endpoint speakers post their notifications to, the address they
// should post it to, and the monitor's lifecycle.

import (
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/store"
)

// maxSonosEventBody caps a notification. The largest thing a speaker sends
// is the household topology, which is kilobytes even in a big house.
const maxSonosEventBody = 512 << 10

// SpeakersChanged is what the speaker monitors call when a cached reading
// moves. It is exported because the monitors are wired to it by the
// composition root rather than built here.
//
// It pushes a music-only change signal to connected clients — a separate SSE
// topic from the general "changed" signal, so a volume drag on a speaker
// doesn't make every open tab refetch every socket, scene and sensor in the
// house.
func (s *Server) SpeakersChanged() {
	if s.events != nil {
		s.events.broadcastTopic(topicMusic)
	}
	// A cache change is also the moment the song may have changed, and this
	// is the only hook that fires without anyone watching — a house whose
	// speakers are subscribed keeps its listening log whether or not a phone
	// is open. Reads the caches, never a speaker. See heard.go.
	s.Listening.NoteCached()
}

// ── Health ───────────────────────────────────────────────────────────────

// sonosEventSpeakerView is one speaker's push status, joined with the name
// and address the user knows it by — the monitor only ever sees IDs.
//
// Times are RFC3339 strings rather than a duration computed here: the client
// renders them as "4s ago", and a relative number baked server-side would be
// wrong by however long the response sat in flight or on screen.
type sonosEventSpeakerView struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	IP         string   `json:"ip"`
	Subscribed bool     `json:"subscribed"`
	Reachable  bool     `json:"reachable"`
	Services   []string `json:"services,omitempty"`
	// Callback is the address this speaker posts to, with the per-speaker
	// token stripped — see redactToken.
	Callback  string `json:"callback,omitempty"`
	Events    int    `json:"events"`
	LastEvent string `json:"last_event,omitempty"`
	RenewAt   string `json:"renew_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

// sonosEventHealthView is the whole answer to "is this working, and if not,
// why". Callback carries the address speakers are told to post back to even
// when no subscription has ever succeeded, since that address is the thing
// the user has to make reachable.
type sonosEventHealthView struct {
	Live          bool                    `json:"live"`
	Running       bool                    `json:"running"`
	Subscribed    int                     `json:"subscribed"`
	Total         int                     `json:"total"`
	Callback      string                  `json:"callback,omitempty"`
	CallbackError string                  `json:"callback_error,omitempty"`
	Speakers      []sonosEventSpeakerView `json:"speakers"`
}

// sonosEventHealth handles GET /api/sonos/events — how the push subsystem is
// doing, per speaker. Read-only and cheap: it reports the monitor's own
// bookkeeping and never touches the network.
func (s *Server) sonosEventHealth(w http.ResponseWriter, r *http.Request) {
	health := s.Speakers.Sonos.Health()

	var names map[string]*store.SonosSpeaker
	s.Store.View(func() {
		names = make(map[string]*store.SonosSpeaker, len(s.Store.Sonos))
		for _, sp := range s.Store.Sonos {
			names[sp.ID] = sp
		}
	})

	out := sonosEventHealthView{
		Live:       health.Live,
		Running:    health.Running,
		Subscribed: health.Subscribed,
		Total:      health.Total,
		Speakers:   make([]sonosEventSpeakerView, 0, len(health.Speakers)),
	}
	for _, h := range health.Speakers {
		v := sonosEventSpeakerView{
			ID:         h.ID,
			Subscribed: h.Subscribed,
			Reachable:  h.Reachable,
			Services:   h.Services,
			Callback:   redactToken(h.Callback),
			Events:     h.Events,
			LastEvent:  rfc3339OrEmpty(h.LastEvent),
			RenewAt:    rfc3339OrEmpty(h.RenewAt),
			Error:      h.Error,
		}
		if sp := names[h.ID]; sp != nil {
			v.Name, v.IP = sp.Name, sp.IP
		}
		out.Speakers = append(out.Speakers, v)
	}
	// Sort by the name the user reads, not the ID the monitor sorted by.
	sort.Slice(out.Speakers, func(i, j int) bool { return out.Speakers[i].Name < out.Speakers[j].Name })

	out.Callback, out.CallbackError = s.sonosCallbackProbe(names)
	writeJSON(w, http.StatusOK, out)
}

// sonosCallbackProbe works out the address speakers would be told to post to,
// so the UI can show it even when nothing has subscribed yet. It resolves
// against a real speaker's address because the answer is route-dependent on a
// multi-homed host — there is no single "our address" to report.
func (s *Server) sonosCallbackProbe(speakers map[string]*store.SonosSpeaker) (callback, failure string) {
	ips := make([]string, 0, len(speakers))
	for _, sp := range speakers {
		if sp.IP != "" {
			ips = append(ips, sp.IP)
		}
	}
	if len(ips) == 0 {
		return "", ""
	}
	sort.Strings(ips) // stable answer across requests
	url, err := s.Speakers.CallbackURL(ips[0])
	if err != nil {
		return "", err.Error()
	}
	return url, ""
}

// sonosEventRetry handles POST /api/sonos/events/retry — resubscribe now
// rather than at the watchers' own backoff. The work is asynchronous, so the
// response says only that it was asked for; the client re-reads the health
// endpoint to see what came of it.
func (s *Server) sonosEventRetry(w http.ResponseWriter, r *http.Request) {
	s.Speakers.Sonos.Retry()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// redactToken drops the per-speaker token from a callback URL, leaving the
// address the user actually needs to know about.
//
// The token is one of the three things guarding the unauthenticated NOTIFY
// route (see handleSonosEvent), so it should not travel any further than it
// has to — and a diagnostic screen has no use for it. The base address is
// identical for every speaker on the same interface anyway.
func redactToken(callback string) string {
	if callback == "" {
		return ""
	}
	i := strings.LastIndex(callback, "/")
	if i <= 0 {
		return callback
	}
	return callback[:i]
}

func rfc3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// handleSonosEvent handles NOTIFY /sonos/event/{token} — a speaker reporting
// that something changed.
//
// This is the one route in the app that cannot be authenticated: speakers
// have no credentials and no way to acquire any. Three things guard it
// instead — the unguessable per-speaker token in the path, the SID we issued
// that speaker, and a check that the request came from that speaker's own
// address. A notification failing any of them gets 412, which is GENA's
// "that subscription no longer exists" and makes a stray sender stop.
func (s *Server) handleSonosEvent(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSonosEventBody))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	seq, _ := strconv.Atoi(r.Header.Get("SEQ"))
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !s.Speakers.Sonos.Notify(mux.Vars(r)["token"], r.Header.Get("SID"), seq, string(body), host) {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	w.WriteHeader(http.StatusOK)
}
