// Wiring for the Sonos event monitor (internal/sonos/monitor.go): the
// callback endpoint speakers post their notifications to, the address they
// should post it to, and the monitor's lifecycle.
package api

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"

	"github.com/gorilla/mux"

	"homehub/internal/sonos"
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
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	out := make([]sonos.Speaker, 0, len(s.Store.Sonos))
	for _, sp := range s.Store.Sonos {
		out = append(out, sonos.Speaker{ID: sp.ID, IP: sp.IP, UUID: sp.UUID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil || addr.IP.IsUnspecified() {
		return "", fmt.Errorf("no route to %s", ip)
	}
	return addr.IP.String(), nil
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
