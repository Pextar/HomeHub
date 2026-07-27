package api

// Wiring for the Sonos event monitor (internal/sonos/monitor.go): the
// callback endpoint speakers post their notifications to, the address they
// should post it to, and the monitor's lifecycle.

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/sonos"
	"homehub/internal/store"
)

// sonosEventPath is the callback prefix; the monitor appends a per-speaker
// token. Deliberately outside /api: the speakers posting here can't
// authenticate, so the route must not sit behind the API's auth middleware.
const sonosEventPath = "/sonos/event"

// maxSonosEventBody caps a notification. The largest thing a speaker sends
// is the household topology, which is kilobytes even in a big house.
const maxSonosEventBody = 512 << 10

// sonosEvents returns the speaker monitor, building it on first use.
func (s *Server) sonosEvents() *sonos.Monitor {
	s.sonosMonMu.Lock()
	defer s.sonosMonMu.Unlock()
	if s.sonosMon == nil {
		s.sonosMon = sonos.NewMonitor(sonos.MonitorConfig{
			Speakers:    s.sonosSpeakerList,
			CallbackURL: s.sonosCallbackURL,
			OnChange:    s.broadcastMusic,
			Logf:        log.Printf,
		})
	}
	return s.sonosMon
}

// RunSonosEvents keeps the subscriptions alive until ctx is cancelled, at
// which point every one of them is released. Call it once, from main, after
// Handler().
func (s *Server) RunSonosEvents(ctx context.Context) {
	s.sonosEvents().Run(ctx)
}

// broadcastMusic pushes a music-only change signal to connected clients. It
// is a separate SSE topic from the general "changed" signal so a volume drag
// on a speaker doesn't make every open tab refetch every socket, scene and
// sensor in the house.
func (s *Server) broadcastMusic() {
	if s.events == nil {
		return
	}
	s.events.broadcastTopic(topicMusic)
}

// sonosSpeakerList adapts the store's speakers to what the monitor needs.
// Sorted so the synchronous fallback always asks the same speaker for the
// household topology.
func (s *Server) sonosSpeakerList() []sonos.Speaker {
	return store.ViewValue(s.Store, func() []sonos.Speaker {
		out := make([]sonos.Speaker, 0, len(s.Store.Sonos))
		for _, sp := range s.Store.Sonos {
			out = append(out, sonos.Speaker{ID: sp.ID, IP: sp.IP, UUID: sp.UUID})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	})
}

// sonosCallbackURL builds the address one speaker should post notifications
// to. Two things make this more than string formatting:
//
//   - It must be plain HTTP. Speakers will not post to an HTTPS endpoint,
//     and HomeHub's certificate is self-signed anyway — so the callback
//     always names the HTTP listener, even when HTTPS_PORT is also up.
//   - It must be the address on the network *that speaker* is on. A host
//     with a second interface (a Pi on Wi-Fi and Ethernet, a VPN, Docker's
//     bridge) has several local addresses, and all but one are unreachable
//     from any given speaker.
func (s *Server) sonosCallbackURL(speakerIP string) (string, error) {
	local, err := localAddrFor(speakerIP)
	if err != nil {
		return "", fmt.Errorf("no local address can reach %s: %w", speakerIP, err)
	}
	port := s.HTTPPort
	if port == "" {
		port = "8080"
	}
	return "http://" + net.JoinHostPort(local, port) + sonosEventPath, nil
}

// localAddrFor asks the kernel which of our addresses it would use to reach
// ip. Dialling UDP sends no packets — it only resolves the route — so this
// is free, and unlike "first non-loopback interface" it is correct on a
// multi-homed host.
func localAddrFor(ip string) (string, error) {
	conn, err := net.Dial("udp", net.JoinHostPort(ip, strconv.Itoa(sonos.Port)))
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil || addr.IP.IsUnspecified() {
		return "", fmt.Errorf("no route to %s", ip)
	}
	return addr.IP.String(), nil
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
	health := s.sonosEvents().Health()

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
	url, err := s.sonosCallbackURL(ips[0])
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
	s.sonosEvents().Retry()
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
	if !s.sonosEvents().Notify(mux.Vars(r)["token"], r.Header.Get("SID"), seq, string(body), host) {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	w.WriteHeader(http.StatusOK)
}
