package kef

// Monitor keeps a live picture of every registered KEF speaker.
//
// Where the Sonos bridge subscribes to the speakers' own change
// notifications (GENA), KEF's local API has no callback to subscribe to —
// the KEF Connect app polls it too. So this polls, and the design goal
// shifts: rather than every open tab polling the speakers, *one* poller in
// the process does, caches what it read, and pushes a `music` SSE signal
// when something actually changed. Clients then read a warm cache. Five
// clients cost the speaker the same as one.
//
// Three properties are worth preserving if this is ever extended:
//
//  1. Snapshot never blocks on the network when the cache is warm, so the
//     status endpoint is cheap enough for a browser to poll as a backstop.
//  2. A cold or stale cache falls back to reading every speaker
//     synchronously, so the endpoint is correct even before Run is called
//     (or if it was never called at all — tests, or a process that only
//     serves one request).
//  3. Touch exists because a control action changes the speaker faster than
//     the next tick would notice. It re-reads one speaker promptly instead
//     of making the user watch a stale volume for five seconds.

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// pollInterval is how often each speaker is re-read in the background.
	// The call is three or four small local HTTP requests, so this is cheap
	// — and it is the *only* poller, however many clients are watching.
	pollInterval = 5 * time.Second
	// touchDelay is how long Touch waits before its re-read. Long enough
	// for the speaker to have applied the action, short enough that the UI's
	// optimistic state hands over without a visible flicker.
	touchDelay = 600 * time.Millisecond
	// staleAfter is how old a cached speaker may be before Snapshot stops
	// trusting it and reads everything synchronously instead. Comfortably
	// more than pollInterval so one slow round-trip isn't mistaken for a
	// dead poller.
	staleAfter = 30 * time.Second
	// reconcileEvery is how often the poller set is compared against the
	// registered speakers. Also triggered directly by Nudge.
	reconcileEvery = 20 * time.Second

	perCallTimeout = 4 * time.Second
)

// Speaker is the identity Monitor needs to watch one speaker. It mirrors the
// stored speaker without importing the store, which would be a cycle — the
// store already imports this package for ValidateHost.
type Speaker struct {
	ID string
	IP string
}

// SpeakerState is one speaker's cached state.
type SpeakerState struct {
	Reachable bool
	State     *State
	// At is when the reading was taken, so a client can tell how fresh the
	// position it is extrapolating from actually is.
	At time.Time
}

// Snapshot is every registered speaker as the monitor currently understands
// it.
type Snapshot struct {
	Speakers map[string]SpeakerState // by speaker ID
	// Warm reports whether this came from the background poller's cache
	// rather than from a synchronous read performed to answer this call.
	Warm bool
}

// MonitorConfig wires a Monitor to its surroundings.
type MonitorConfig struct {
	// Speakers returns the currently registered speakers. Called often, so
	// it should be cheap; the api implementation reads the store under a
	// read lock.
	Speakers func() []Speaker
	// OnChange fires after the cache changes, for pushing to clients.
	OnChange func()
	Logf     func(format string, args ...any)
}

// Monitor owns the pollers and the cache behind them.
type Monitor struct {
	cfg MonitorConfig

	mu      sync.RWMutex
	entries map[string]*entry // by speaker ID

	nudge chan struct{}

	// running reports whether Run is supervising pollers. Without it a
	// monitor that was never started looks exactly like one whose speakers
	// are all unreachable, and the two need different handling.
	running atomic.Bool

	// refreshMu serialises full synchronous fan-outs so a burst of client
	// polls against a cold cache costs one sweep, not one per request.
	refreshMu sync.Mutex
}

// entry is one speaker's slot in the cache. Every field is guarded by
// Monitor.mu; network calls always happen with the lock released.
type entry struct {
	sp Speaker

	state     *State
	reachable bool
	at        time.Time // last read attempt

	// wake asks this speaker's poller to read now rather than at its next
	// tick. Buffered and non-blocking, so a burst collapses into one.
	wake chan struct{}
}

// NewMonitor builds a Monitor. It is usable — via Snapshot — without ever
// calling Run; it simply never has a warm cache in that case.
func NewMonitor(cfg MonitorConfig) *Monitor {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.OnChange == nil {
		cfg.OnChange = func() {}
	}
	if cfg.Speakers == nil {
		cfg.Speakers = func() []Speaker { return nil }
	}
	return &Monitor{
		cfg:     cfg,
		entries: make(map[string]*entry),
		nudge:   make(chan struct{}, 1),
	}
}

// Run supervises one poller goroutine per registered speaker until ctx is
// cancelled.
func (m *Monitor) Run(ctx context.Context) {
	m.running.Store(true)
	defer m.running.Store(false)

	ticker := time.NewTicker(reconcileEvery)
	defer ticker.Stop()

	pollers := make(map[string]context.CancelFunc)
	var wg sync.WaitGroup
	defer func() {
		for _, cancel := range pollers {
			cancel()
		}
		wg.Wait()
	}()

	for {
		m.reconcile(ctx, pollers, &wg)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-m.nudge:
		}
	}
}

// Nudge asks the supervisor to reconcile now rather than at the next tick.
// Called when speakers are added, edited or removed.
func (m *Monitor) Nudge() {
	select {
	case m.nudge <- struct{}{}:
	default:
	}
}

// Touch schedules a prompt re-read of one speaker, after a short settle.
// Called once a control action has been accepted, so the cache catches up
// with what the user just did without waiting out the poll interval.
func (m *Monitor) Touch(id string) {
	m.mu.RLock()
	e := m.entries[id]
	m.mu.RUnlock()
	if e == nil {
		return
	}
	time.AfterFunc(touchDelay, func() {
		select {
		case e.wake <- struct{}{}:
		default:
		}
	})
}

// reconcile starts pollers for new speakers and stops them for speakers
// that were removed or re-addressed.
func (m *Monitor) reconcile(ctx context.Context, pollers map[string]context.CancelFunc, wg *sync.WaitGroup) {
	want := make(map[string]Speaker)
	for _, sp := range m.cfg.Speakers() {
		if sp.ID == "" || ValidateHost(sp.IP) != nil {
			continue
		}
		want[sp.ID] = sp
	}

	m.mu.Lock()
	// Stop pollers for speakers that went away or moved.
	for id, cancel := range pollers {
		sp, ok := want[id]
		if ok && m.entries[id] != nil && m.entries[id].sp.IP == sp.IP {
			continue
		}
		cancel()
		delete(pollers, id)
		delete(m.entries, id)
	}
	// Start pollers for speakers that don't have one.
	var started []*entry
	for id, sp := range want {
		if _, ok := pollers[id]; ok {
			continue
		}
		e := &entry{sp: sp, wake: make(chan struct{}, 1)}
		m.entries[id] = e
		started = append(started, e)
	}
	m.mu.Unlock()

	for _, e := range started {
		cctx, cancel := context.WithCancel(ctx)
		pollers[e.sp.ID] = cancel
		wg.Add(1)
		go func(e *entry) {
			defer wg.Done()
			m.poll(cctx, e)
		}(e)
	}
}

// poll reads one speaker on a ticker until its context is cancelled, waking
// early when Touch asks it to.
func (m *Monitor) poll(ctx context.Context, e *entry) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if m.read(ctx, e) {
			m.cfg.OnChange()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-e.wake:
		}
	}
}

// read fetches one speaker's state and stores it, reporting whether
// anything a client would notice actually changed.
func (m *Monitor) read(ctx context.Context, e *entry) bool {
	cctx, cancel := context.WithTimeout(ctx, perCallTimeout)
	st, err := GetState(cctx, e.sp.IP)
	cancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	was, wasReachable := e.state, e.reachable
	e.at = time.Now()
	if err != nil {
		e.state, e.reachable = nil, false
		return wasReachable
	}
	e.state, e.reachable = st, true
	return !wasReachable || !sameState(was, st)
}

// sameState reports whether two readings are equivalent for the UI's
// purposes. Playback position is deliberately excluded: it advances every
// tick by definition, and pushing an SSE frame five times a minute per
// speaker to say "the song is still playing" is exactly the chatter the
// separate music topic exists to avoid. The client extrapolates position
// between reads anyway.
func sameState(a, b *State) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Status != b.Status || a.Source != b.Source || a.PoweredOn != b.PoweredOn ||
		a.Volume != b.Volume || a.Muted != b.Muted || a.DurationMS != b.DurationMS {
		return false
	}
	if (a.Track == nil) != (b.Track == nil) {
		return false
	}
	return a.Track == nil || *a.Track == *b.Track
}

// Snapshot returns every registered speaker's state. It answers from the
// cache when the background poller is keeping it warm, and otherwise reads
// every speaker synchronously — which is what a bridge with no monitor at
// all would do, and is correct, just slower.
func (m *Monitor) Snapshot(ctx context.Context) Snapshot {
	want := m.cfg.Speakers()
	if len(want) == 0 {
		return Snapshot{Speakers: map[string]SpeakerState{}, Warm: m.running.Load()}
	}
	if snap, ok := m.cached(want); ok {
		return snap
	}
	m.refreshAll(ctx, want)
	snap, _ := m.cached(want)
	snap.Warm = false
	return snap
}

// cached builds a snapshot from the cache, reporting false when any
// registered speaker is missing from it or has gone stale.
func (m *Monitor) cached(want []Speaker) (Snapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := Snapshot{Speakers: make(map[string]SpeakerState, len(want)), Warm: true}
	fresh := time.Now().Add(-staleAfter)
	ok := true
	for _, sp := range want {
		if ValidateHost(sp.IP) != nil {
			// Nothing will ever read this one, so it is reported as
			// unreachable — the UI needs the row to offer a way to fix the
			// address — without ever counting as a cold cache. Otherwise one
			// mistyped speaker would force a synchronous sweep of all the
			// others on every single request.
			out.Speakers[sp.ID] = SpeakerState{At: time.Now()}
			continue
		}
		e := m.entries[sp.ID]
		if e == nil || e.at.Before(fresh) {
			ok = false
			continue
		}
		out.Speakers[sp.ID] = SpeakerState{
			Reachable: e.reachable,
			State:     copyState(e.state),
			At:        e.at,
		}
	}
	return out, ok
}

// refreshAll reads every speaker synchronously into the cache. Serialised
// by refreshMu so concurrent client polls against a cold cache share one
// sweep; the second caller waits for the first and then finds the cache
// warm.
func (m *Monitor) refreshAll(ctx context.Context, want []Speaker) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	// The first caller may already have filled it while this one waited.
	if _, ok := m.cached(want); ok {
		return
	}

	// Ensure a cache slot exists for every speaker, so a monitor that was
	// never Run still has somewhere to put its readings.
	m.mu.Lock()
	entries := make([]*entry, 0, len(want))
	for _, sp := range want {
		if ValidateHost(sp.IP) != nil {
			continue
		}
		e := m.entries[sp.ID]
		if e == nil || e.sp.IP != sp.IP {
			e = &entry{sp: sp, wake: make(chan struct{}, 1)}
			m.entries[sp.ID] = e
		}
		entries = append(entries, e)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, e := range entries {
		wg.Add(1)
		go func(e *entry) {
			defer wg.Done()
			m.read(ctx, e)
		}(e)
	}
	wg.Wait()
}

// copyState deep-copies a cached reading so a caller rewriting a field —
// the API rewrites album-art URLs — can't corrupt what the next reader sees.
func copyState(st *State) *State {
	if st == nil {
		return nil
	}
	cp := *st
	if st.Track != nil {
		t := *st.Track
		cp.Track = &t
	}
	return &cp
}

// Speakers returns the monitor's view of who it is watching, sorted by ID.
// Only used for diagnostics.
func (m *Monitor) Speakers() []Speaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Speaker, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, e.sp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
