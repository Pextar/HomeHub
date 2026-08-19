// Package speakermon keeps HomeHub's picture of what the speakers are doing.
//
// Two mechanisms, one answer. Sonos speakers push: they are subscribed to over
// GENA and report changes as they happen. KEF speakers cannot — their local
// API has no callback — so they are polled. Everything above this package asks
// the same question of both and does not care which it was.
//
// The point of a monitor is that a speaker is asked once for the whole
// process, no matter how many phones are watching. Reading state here is a map
// lookup; the fallback to a live request exists for the speaker that has not
// reported yet, not as the normal path. Anything running on the scheduler's
// tick must use Cached and never Snapshot, or the house pays a round trip
// every five seconds whether or not anyone is listening.
//
// Beside the monitors sit the two lookups that are neither state nor
// discovery: which streaming account a speaker plays through, and where it
// publishes a picture of itself. Both cost a round trip, both change when the
// household changes something, and both used to be re-asked on every render.
package speakermon

import (
	"context"
	"sort"
	"strings"
	"time"

	"homehub/internal/kef"
	"homehub/internal/platform/lanaddr"
	"homehub/internal/platform/ttlcache"
	"homehub/internal/sonos"
	"homehub/internal/store"
)

// accountTTL is how long a speaker's service account is trusted. The sid/sn
// pair only changes when the household's service links change, so an hour of
// caching keeps a play tap at four SOAP calls instead of six.
const accountTTL = time.Hour

// iconTTL is how long a speaker's published picture path is trusted. A day:
// it changes when the speaker's firmware does.
const iconTTL = 24 * time.Hour

// Config is what the monitors need from outside.
type Config struct {
	// Store is where the registered speakers live.
	Store *store.Store

	// OnChange fires whenever a speaker's cached state moves. It must be
	// cheap: it runs on the monitor's own goroutine.
	OnChange func()

	// HTTPPort is the plain-HTTP listener Sonos speakers post their change
	// notifications to. They will not post to an HTTPS endpoint, so this is
	// needed even when TLS is also up.
	HTTPPort string
	// EventPath is where those notifications are received.
	EventPath string

	Logf func(format string, args ...any)
}

// Monitors is the house's cached view of every speaker.
type Monitors struct {
	// Sonos watches speakers over GENA. Its subscriptions must be released
	// while the speakers can still reach us — see RunSonos.
	Sonos *sonos.Monitor
	// KEF polls. It has nothing to release, so it can ride any context.
	KEF *kef.Monitor

	cfg      Config
	accounts *ttlcache.Cache[string, *sonos.ServiceAccount]
	icons    *ttlcache.Cache[string, string]
}

// New builds both monitors. Neither starts until Run is called, so this is
// safe to do at wiring time.
func New(cfg Config) *Monitors {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	m := &Monitors{
		cfg:      cfg,
		accounts: ttlcache.New[string, *sonos.ServiceAccount](accountTTL),
		icons:    ttlcache.New[string, string](iconTTL),
	}
	m.Sonos = sonos.NewMonitor(sonos.MonitorConfig{
		Speakers:    m.sonosSpeakers,
		CallbackURL: m.CallbackURL,
		OnChange:    cfg.OnChange,
		Logf:        cfg.Logf,
	})
	m.KEF = kef.NewMonitor(kef.MonitorConfig{
		Speakers: m.kefSpeakers,
		OnChange: cfg.OnChange,
		Logf:     cfg.Logf,
	})
	return m
}

// RunSonos keeps the GENA subscriptions alive until ctx is cancelled, at which
// point every one of them is released.
//
// Cancel it *before* the HTTP server stops. The subscriptions have to be
// released while the speakers can still reach our callback, or they keep
// posting to a dead address until their grants expire.
func (m *Monitors) RunSonos(ctx context.Context) { m.Sonos.Run(ctx) }

// RunKEF keeps the KEF speakers polled until ctx is cancelled. Unlike the
// Sonos side there is nothing to hand back, so it can share any context.
func (m *Monitors) RunKEF(ctx context.Context) { m.KEF.Run(ctx) }

// CallbackURL builds the address one Sonos speaker should post notifications
// to. Two things make this more than string formatting:
//
//   - It must be plain HTTP. Speakers will not post to an HTTPS endpoint, and
//     HomeHub's certificate is self-signed anyway, so the callback always
//     names the HTTP listener even when HTTPS_PORT is also up.
//   - It must be the address on the network *that speaker* is on. A host with
//     a second interface (a Pi on Wi-Fi and Ethernet, a VPN, Docker's bridge)
//     has several local addresses, and all but one are unreachable from any
//     given speaker.
func (m *Monitors) CallbackURL(speakerIP string) (string, error) {
	base, err := lanaddr.BaseURL(speakerIP, m.cfg.HTTPPort)
	if err != nil {
		return "", err
	}
	return base + m.cfg.EventPath, nil
}

// SonosState reads a speaker's state, preferring the monitor's cache.
func (m *Monitors) SonosState(ctx context.Context, sp store.SonosSpeaker) (*sonos.State, error) {
	snap := m.Sonos.Snapshot(ctx)
	if cached, ok := snap.Speakers[sp.ID]; ok && cached.State != nil {
		return cached.State, nil
	}
	return sonos.GetState(ctx, sp.IP)
}

// KEFState reads a speaker's state, preferring the polling monitor's cache.
func (m *Monitors) KEFState(ctx context.Context, sp store.KEFSpeaker) (*kef.State, error) {
	snap := m.KEF.Snapshot(ctx)
	if cached, ok := snap.Speakers[sp.ID]; ok && cached.State != nil {
		return cached.State, nil
	}
	return kef.GetState(ctx, sp.IP)
}

// ServiceAccount resolves a speaker's account for a streaming service, cached
// for an hour.
func (m *Monitors) ServiceAccount(ctx context.Context, ip, service string) (*sonos.ServiceAccount, error) {
	return m.accounts.Do(ip+"|"+strings.ToLower(service), func() (*sonos.ServiceAccount, error) {
		return sonos.GetServiceAccount(ctx, ip, service)
	})
}

// IconPath resolves where a speaker publishes its own picture, cached for a
// day. An empty path means it publishes none, and is cached too — a model
// without one should not be re-asked on every render of the speaker list.
func (m *Monitors) IconPath(ctx context.Context, ip string) (string, error) {
	return m.icons.Do(ip, func() (string, error) {
		info, err := sonos.DescribeFull(ctx, ip)
		if err != nil {
			return "", err
		}
		return info.IconPath, nil
	})
}

// sonosSpeakers adapts the store's speakers to what the monitor needs. Sorted
// so the synchronous fallback always asks the same speaker for the household
// topology.
func (m *Monitors) sonosSpeakers() []sonos.Speaker {
	return store.ViewValue(m.cfg.Store, func() []sonos.Speaker {
		out := make([]sonos.Speaker, 0, len(m.cfg.Store.Sonos))
		for _, sp := range m.cfg.Store.Sonos {
			out = append(out, sonos.Speaker{ID: sp.ID, IP: sp.IP, UUID: sp.UUID})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	})
}

// kefSpeakers adapts the store's speakers to what the poller needs.
func (m *Monitors) kefSpeakers() []kef.Speaker {
	return store.ViewValue(m.cfg.Store, func() []kef.Speaker {
		out := make([]kef.Speaker, 0, len(m.cfg.Store.KEF))
		for _, sp := range m.cfg.Store.KEF {
			out = append(out, kef.Speaker{ID: sp.ID, IP: sp.IP})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	})
}
