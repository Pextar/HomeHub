package sonos

// Monitor keeps a live picture of every registered speaker, fed by GENA
// subscriptions (gena.go) instead of by polling every few seconds.
//
// The design goal is that events are an *accelerator*, never a single point
// of failure. Three mechanisms stack:
//
//  1. A notification's payload is applied to the cache immediately, so the
//     UI moves the instant a speaker does.
//  2. Any notification also schedules a debounced SOAP re-read of that
//     speaker, which is authoritative and repairs anything the delta got
//     wrong or didn't carry (track position, queue length).
//  3. A speaker is re-read every 30s regardless, so a subscription that
//     dies silently can't leave stale state on screen for longer than that.
//
// And if subscriptions can't be established at all — the speaker is on a
// subnet that can't reach us, a firewall eats the callbacks, the firmware
// disagrees — Snapshot falls back to reading every speaker synchronously,
// which is exactly what the Music view did before any of this existed.
// Callers can tell the two apart from Snapshot.Live.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// eventSettle is how long to wait after the first notification of a
	// burst before the authoritative re-read. A volume drag fires a dozen
	// events; they should cost one SOAP round-trip, not a dozen.
	eventSettle = 700 * time.Millisecond
	// resyncInterval is the safety net: every speaker gets re-read this
	// often even if it never says anything.
	resyncInterval = 30 * time.Second
	// staleAfter is how old a cached speaker may be before Snapshot stops
	// trusting it and reads everything synchronously instead. Comfortably
	// more than resyncInterval so an ordinary slow round-trip isn't
	// mistaken for a dead subscription.
	staleAfter = 95 * time.Second
	// reconcileEvery is how often the watcher set is compared against the
	// registered speakers. Also triggered directly by Nudge.
	reconcileEvery = 20 * time.Second

	minBackoff     = 15 * time.Second
	maxBackoff     = 5 * time.Minute
	perCallTimeout = 3 * time.Second
)

// Speaker is the identity Monitor needs to watch one speaker. It mirrors the
// stored speaker without importing the store, which would be a cycle —
// store already imports this package for ValidateHost.
type Speaker struct {
	ID   string
	IP   string
	UUID string
}

// SpeakerState is one speaker's cached state. GroupState is only ever set on
// a coordinator, matching what a live read would return.
type SpeakerState struct {
	Reachable  bool
	State      *State
	GroupState *GroupState
	// At is when this reading was taken. It matters because the one field
	// in State that keeps moving on its own — the track position — is only
	// ever as current as the read it came from, and events don't carry it:
	// Sonos pushes transport and track changes, never RelTime, so a cached
	// position is up to a resync interval old. A client extrapolating from
	// "now" would run that whole error into the number it draws.
	At time.Time
}

// Snapshot is the household as the monitor currently understands it.
type Snapshot struct {
	Speakers map[string]SpeakerState // by speaker ID
	Groups   []Group
	// Live reports whether event subscriptions are feeding the cache. When
	// false everything here came from a synchronous read and callers should
	// keep polling at the old rate.
	Live bool
}

// MonitorConfig wires a Monitor to its surroundings.
type MonitorConfig struct {
	// Speakers returns the currently registered speakers. Called often, so
	// it should be cheap; the api implementation reads the store under a
	// read lock.
	Speakers func() []Speaker
	// CallbackURL returns the base URL a given speaker should POST its
	// notifications to — the address of ours it can actually reach, which
	// on a multi-homed host is not just any local address. Monitor appends
	// its own per-speaker token.
	CallbackURL func(speakerIP string) (string, error)
	// OnChange fires after the cache changes, for pushing to clients.
	OnChange func()
	Logf     func(format string, args ...any)
}

// Monitor owns the subscriptions and the cache behind them.
type Monitor struct {
	cfg MonitorConfig

	mu      sync.RWMutex
	entries map[string]*entry // by speaker ID
	byToken map[string]*entry // by callback token
	groups  []Group

	nudge chan struct{}

	// running reports whether Run is supervising watchers. Without it a
	// monitor that was never started looks exactly like one whose speakers
	// all refuse subscriptions, and the two need different advice.
	running atomic.Bool

	// refreshMu serialises full synchronous fan-outs so a burst of client
	// polls against a cold cache costs one sweep, not one per request.
	refreshMu sync.Mutex
}

// entry is one speaker's slot in the cache. Every field is guarded by
// Monitor.mu; network calls always happen with the lock released.
type entry struct {
	sp    Speaker
	token string

	sids map[string]string // service key → active SID
	seqs map[string]int    // service key → last SEQ applied
	// gen counts subscription generations. A watcher releases only the
	// generation it created, so one winding down can't tear down the
	// subscriptions its own replacement has just established.
	gen int

	state      *State
	groupState *GroupState
	reachable  bool
	at         time.Time // last authoritative read attempt

	// Health reporting. None of it is load-bearing for the cache — it exists
	// so the UI can explain, in the user's own terms, whether push is working
	// and what to do when it isn't (see Health).
	callback  string    // callback URL this speaker was last handed
	renewAt   time.Time // when the current subscriptions come up for renewal
	lastEvent time.Time // last notification accepted from this speaker
	events    int       // notifications accepted since the process started
	lastErr   string    // why subscribing last failed; cleared once subscribed

	// dirty carries "this speaker said something" to its watcher. Buffered
	// and non-blocking: a burst collapses into one pending signal.
	dirty chan struct{}
	// retry wakes a watcher that is sitting in its backoff, so a user who
	// has just fixed their network doesn't wait out the full interval.
	// Buffered and non-blocking, like dirty.
	retry chan struct{}
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
		byToken: make(map[string]*entry),
		nudge:   make(chan struct{}, 1),
	}
}

// Run supervises one watcher goroutine per registered speaker until ctx is
// cancelled, at which point every subscription is released.
func (m *Monitor) Run(ctx context.Context) {
	m.running.Store(true)
	defer m.running.Store(false)

	ticker := time.NewTicker(reconcileEvery)
	defer ticker.Stop()

	watchers := make(map[string]context.CancelFunc)
	var wg sync.WaitGroup
	defer func() {
		for _, cancel := range watchers {
			cancel()
		}
		wg.Wait()
	}()

	for {
		m.reconcile(ctx, watchers, &wg)
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

// reconcile starts watchers for new speakers and stops them for speakers
// that were removed or re-addressed.
func (m *Monitor) reconcile(ctx context.Context, watchers map[string]context.CancelFunc, wg *sync.WaitGroup) {
	want := make(map[string]Speaker)
	for _, sp := range m.cfg.Speakers() {
		if sp.ID == "" || sp.IP == "" {
			continue
		}
		want[sp.ID] = sp
	}

	for id, cancel := range watchers {
		sp, still := want[id]
		m.mu.RLock()
		e := m.entries[id]
		moved := e != nil && still && e.sp.IP != sp.IP
		m.mu.RUnlock()
		if still && !moved {
			continue
		}
		cancel()
		delete(watchers, id)
		// Forgotten either way. For a removed speaker that's the point; for
		// a re-addressed one it's what releases the subscriptions held
		// against the *old* address, which the entry still carries here —
		// the loop below re-creates it under the new one.
		m.forget(id)
	}

	for id, sp := range want {
		if _, running := watchers[id]; running {
			continue
		}
		m.ensureEntry(sp)
		wctx, cancel := context.WithCancel(ctx)
		watchers[id] = cancel
		wg.Add(1)
		go func(sp Speaker) {
			defer wg.Done()
			m.watch(wctx, sp)
		}(sp)
	}
}

// watch keeps one speaker subscribed, resubscribing with backoff whenever
// the subscription lapses.
func (m *Monitor) watch(ctx context.Context, sp Speaker) {
	backoff := minBackoff
	for ctx.Err() == nil {
		gen, renewIn, err := m.subscribeAll(ctx, sp)
		if err != nil {
			m.noteSubscribeErr(sp.ID, err)
			m.cfg.Logf("sonos: subscribing to %s (%s) failed: %v — retrying in %s", sp.ID, sp.IP, err, backoff)
			// Events are out, but the speaker may still answer SOAP, and
			// the snapshot path needs to know which. Keep reading it.
			m.resync(ctx, sp.ID)
			m.cfg.OnChange()
			asked, alive := m.waitRetry(ctx, sp.ID, backoff)
			if !alive {
				return
			}
			// An interval that was waited out earns a longer next one; one
			// cut short by a user asking to retry starts over, so repeated
			// taps don't inherit a five-minute backoff.
			if asked {
				backoff = minBackoff
			} else {
				backoff = nextBackoff(backoff)
			}
			continue
		}
		backoff = minBackoff
		m.cfg.Logf("sonos: subscribed to %s (%s), renewing every %s", sp.ID, sp.IP, renewIn)

		m.resync(ctx, sp.ID) // authoritative baseline behind the event stream
		m.cfg.OnChange()

		m.serve(ctx, sp.ID, renewIn)
		m.releaseAll(sp.ID, gen)

		if ctx.Err() == nil {
			m.cfg.Logf("sonos: subscription to %s lapsed — resubscribing", sp.ID)
		}
	}
}

// serve runs one speaker's event loop: renew on schedule, re-read on a
// settle timer after notifications, and re-read periodically regardless.
// Returns when the subscription needs rebuilding or ctx ends.
func (m *Monitor) serve(ctx context.Context, id string, renewIn time.Duration) {
	m.mu.RLock()
	e := m.entries[id]
	m.mu.RUnlock()
	if e == nil {
		return
	}

	renew := time.NewTimer(renewIn)
	defer renew.Stop()
	resync := time.NewTicker(resyncInterval)
	defer resync.Stop()
	// Starts fired-and-drained so the first Reset behaves predictably.
	settle := time.NewTimer(0)
	<-settle.C
	defer settle.Stop()
	armed := false

	for {
		select {
		case <-ctx.Done():
			return

		case <-renew.C:
			next, err := m.renewAll(ctx, id)
			if err != nil {
				m.cfg.Logf("sonos: renewing %s failed: %v", id, err)
				return
			}
			renew.Reset(next)
			m.mu.Lock()
			e.renewAt = time.Now().Add(next)
			m.mu.Unlock()

		case <-resync.C:
			m.resync(ctx, id)
			m.cfg.OnChange()

		case <-e.dirty:
			// Only arm on the first event of a burst, never re-arm on the
			// ones behind it: a continuous stream (a volume drag) must not
			// postpone the re-read indefinitely.
			if !armed {
				armed = true
				settle.Reset(eventSettle)
			}

		case <-settle.C:
			armed = false
			m.resync(ctx, id)
			m.cfg.OnChange()
		}
	}
}

// subscribeAll subscribes to every evented service on one speaker and
// returns how long to wait before renewing. A partial failure is rolled
// back, so a speaker is never left notifying a callback we've forgotten.
func (m *Monitor) subscribeAll(ctx context.Context, sp Speaker) (gen int, renewIn time.Duration, err error) {
	base, err := m.cfg.CallbackURL(sp.IP)
	if err != nil {
		return 0, 0, err
	}
	e := m.ensureEntry(sp)
	m.mu.RLock()
	callback := strings.TrimRight(base, "/") + "/" + e.token
	m.mu.RUnlock()

	// A fresh subscription restarts the sequence numbering, so last-seen
	// counts from the previous one must not outlive it.
	m.mu.Lock()
	e.gen++
	gen = e.gen
	e.sids = make(map[string]string, len(EventServices))
	e.seqs = make(map[string]int, len(EventServices))
	e.callback = callback
	m.mu.Unlock()

	shortest := SubscribeTimeout
	for _, svc := range EventServices {
		cctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
		sid, granted, err := Subscribe(cctx, sp.IP, svc, callback, SubscribeTimeout)
		cancel()
		if err != nil {
			m.releaseAll(sp.ID, gen) // roll back the ones that did take
			return 0, 0, err
		}
		// Record it before moving to the next service. A speaker sends its
		// first notification the moment it accepts, and one arriving
		// against a SID we hadn't written down yet would be refused.
		m.mu.Lock()
		e.sids[svc.Key] = sid
		m.mu.Unlock()

		if granted < shortest {
			shortest = granted
		}
	}

	// Renew at half the shortest grant: one missed renewal still leaves a
	// full interval to notice and retry before anything actually expires.
	renewIn = shortest / 2
	m.mu.Lock()
	e.lastErr = ""
	e.renewAt = time.Now().Add(renewIn)
	m.mu.Unlock()
	return gen, renewIn, nil
}

// noteSubscribeErr records why a speaker has no event subscription, so the
// UI can say which speaker is failing and why rather than only that push is
// off somewhere in the house.
func (m *Monitor) noteSubscribeErr(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.entries[id]; e != nil {
		e.lastErr = err.Error()
		e.renewAt = time.Time{}
	}
}

// renewAll extends every subscription on one speaker. Any failure means the
// whole set should be rebuilt — a speaker that forgot one SID has usually
// rebooted and forgotten them all.
func (m *Monitor) renewAll(ctx context.Context, id string) (time.Duration, error) {
	m.mu.RLock()
	e := m.entries[id]
	if e == nil {
		// The speaker was removed while we held its subscriptions; the
		// watcher should wind down rather than renew.
		m.mu.RUnlock()
		return 0, errors.New("sonos: speaker is no longer registered")
	}
	ip := e.sp.IP
	sids := make(map[string]string, len(e.sids))
	for k, v := range e.sids {
		sids[k] = v
	}
	m.mu.RUnlock()

	shortest := SubscribeTimeout
	for _, svc := range EventServices {
		sid, ok := sids[svc.Key]
		if !ok {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
		granted, err := Renew(cctx, ip, svc, sid, SubscribeTimeout)
		cancel()
		if err != nil {
			return 0, err
		}
		if granted < shortest {
			shortest = granted
		}
	}
	return shortest / 2, nil
}

// releaseAll drops the subscriptions of one generation on one speaker. It
// deliberately uses a fresh context: this runs on the way out, when the
// watcher's own context is usually already cancelled, and a speaker left
// holding a subscription would keep POSTing to a callback nobody answers
// until the grant expires.
//
// A generation that no longer matches means another watcher has taken this
// speaker over — its subscriptions are not ours to release.
func (m *Monitor) releaseAll(id string, gen int) {
	m.mu.Lock()
	e := m.entries[id]
	if e == nil || e.gen != gen {
		m.mu.Unlock()
		return
	}
	ip := e.sp.IP
	sids := e.sids
	e.sids = make(map[string]string)
	m.mu.Unlock()

	m.unsubscribeSet(ip, sids)
}

// unsubscribeSet drops a set of subscriptions concurrently. Serially, an
// unreachable speaker would cost one timeout per service — long enough on
// shutdown to be cut off before the last one is released.
func (m *Monitor) unsubscribeSet(ip string, sids map[string]string) {
	var wg sync.WaitGroup
	for key, sid := range sids {
		wg.Add(1)
		go func(key, sid string) {
			defer wg.Done()
			m.unsubscribeOne(ip, key, sid)
		}(key, sid)
	}
	wg.Wait()
}

func (m *Monitor) unsubscribeOne(ip, key, sid string) {
	var svc EventService
	for _, s := range EventServices {
		if s.Key == key {
			svc = s
			break
		}
	}
	if svc.Key == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	if err := Unsubscribe(ctx, ip, svc, sid); err != nil {
		m.cfg.Logf("sonos: unsubscribing %s from %s: %v", key, ip, err)
	}
}

// ── Notifications ────────────────────────────────────────────────────────

// Notify applies one GENA notification. It reports false when the
// notification can't be attributed to a live subscription — an unknown
// token, a SID we don't hold, or a source address that isn't the speaker
// the token belongs to — which the HTTP handler answers with 412 so the
// speaker stops sending.
func (m *Monitor) Notify(token, sid string, seq int, body, srcIP string) bool {
	m.mu.Lock()
	e := m.byToken[token]
	if e == nil {
		m.mu.Unlock()
		return false
	}
	// A notification must come from the speaker its token was issued to.
	if srcIP != "" && srcIP != e.sp.IP {
		m.mu.Unlock()
		return false
	}
	key := ""
	for k, s := range e.sids {
		if s == sid {
			key = k
			break
		}
	}
	if key == "" {
		m.mu.Unlock()
		return false
	}
	// SEQ counts per subscription; 0 is the full state sent at subscribe
	// time and restarts the count. Anything at or below what we already
	// applied is a reordered duplicate — drop the payload, but still treat
	// the notification as ours so the speaker keeps talking to us.
	if seq != 0 {
		if last, seen := e.seqs[key]; seen && seq <= last {
			m.mu.Unlock()
			return true
		}
	}
	e.seqs[key] = seq
	e.lastEvent = time.Now()
	e.events++
	dirty := e.dirty
	m.mu.Unlock()

	props := ParsePropertySet(body)
	changed := false
	switch key {
	case EventTransport.Key:
		changed = m.applyTransport(e, ParseLastChange(props["LastChange"]))
	case EventRendering.Key:
		changed = m.applyRendering(e, ParseLastChange(props["LastChange"]))
	case EventTopology.Key:
		changed = m.applyTopology(props["ZoneGroupState"])
	}

	// Whatever we could or couldn't read out of it, a notification means
	// something moved — so it always earns an authoritative re-read.
	select {
	case dirty <- struct{}{}:
	default:
	}
	if changed {
		m.cfg.OnChange()
	}
	return true
}

// applyTransport folds an AVTransport LastChange into the cache. Values the
// notification doesn't carry are left alone; the re-read behind it fills in
// what isn't evented at all (position, and the queue's actual contents).
func (m *Monitor) applyTransport(e *entry, vals map[string]string) bool {
	if len(vals) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.state == nil {
		e.state = &State{}
	}
	st := e.state
	if v, ok := vals["TransportState"]; ok {
		st.TransportState = v
		st.Playing = v == "PLAYING" || v == "TRANSITIONING"
	}
	if v, ok := vals["CurrentTrackDuration"]; ok {
		st.Duration = normalizeClock(v)
	}
	if v, ok := vals["CurrentTrack"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			st.QueueTrack = n
		}
	}
	if v, ok := vals["CurrentTrackMetaData"]; ok && v != "" && v != "NOT_IMPLEMENTED" {
		if t := ParseTrackMeta(v); t != nil {
			st.Track = t
		}
	}
	// Play modes belong to the group, so they only mean anything on the one
	// speaker carrying a GroupState — the coordinator.
	if gs := e.groupState; gs != nil {
		if v, ok := vals["CurrentPlayMode"]; ok {
			pm := ParsePlayMode(v)
			gs.Shuffle, gs.Repeat = pm.Shuffle, pm.Repeat
		}
		if v, ok := vals["CurrentCrossfadeMode"]; ok {
			gs.Crossfade = v == "1"
		}
		if v, ok := vals["NumberOfTracks"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				gs.QueueLength = n
			}
		}
		if v, ok := vals["AVTransportURI"]; ok {
			gs.FromQueue = strings.HasPrefix(v, "x-rincon-queue:")
		}
	}
	e.reachable = true
	return true
}

// applyRendering folds a RenderingControl LastChange into the cache.
func (m *Monitor) applyRendering(e *entry, vals map[string]string) bool {
	if len(vals) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.state == nil {
		e.state = &State{}
	}
	changed := false
	if v, ok := vals["Volume"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			e.state.Volume = n
			changed = true
		}
	}
	if v, ok := vals["Mute"]; ok {
		e.state.Muted = v == "1"
		changed = true
	}
	if changed {
		e.reachable = true
	}
	return changed
}

// applyTopology replaces the cached grouping. Every speaker is marked dirty
// with it: which speaker coordinates which group has just changed, and
// group-level state is only read from coordinators.
func (m *Monitor) applyTopology(state string) bool {
	if state == "" {
		return false
	}
	groups, err := ParseZoneGroupState(state)
	if err != nil || len(groups) == 0 {
		return false
	}
	m.mu.Lock()
	m.groups = groups
	for _, e := range m.entries {
		select {
		case e.dirty <- struct{}{}:
		default:
		}
	}
	m.mu.Unlock()
	return true
}

// ── Reading ──────────────────────────────────────────────────────────────

// Snapshot returns the household's state, reading it synchronously if the
// cache isn't being kept warm by subscriptions.
func (m *Monitor) Snapshot(ctx context.Context) Snapshot {
	speakers := m.cfg.Speakers()
	if !m.freshFor(speakers) {
		m.refreshAll(ctx, speakers)
	}
	return m.read()
}

// Cached returns what the cache holds right now and never touches a speaker.
//
// Snapshot is for answering a question — it will go and read the household if
// the cache can't be trusted. This is for a caller that only wants to know
// what the monitor already learned, and must not turn a cold cache into a
// fan-out of SOAP calls: the listening log reads it on every cache change,
// which is several times a minute on a house that is playing something.
func (m *Monitor) Cached() Snapshot {
	return m.read()
}

// freshFor reports whether every registered speaker has a recently-read
// cache entry and at least one speaker is actually subscribed. Both halves
// matter: subscriptions alone don't prove the reads are current, and recent
// reads alone would let a dead event stream masquerade as a live one.
func (m *Monitor) freshFor(speakers []Speaker) bool {
	if len(speakers) == 0 {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now()
	live := false
	for _, sp := range speakers {
		e := m.entries[sp.ID]
		if e == nil || e.at.IsZero() || now.Sub(e.at) > staleAfter {
			return false
		}
		if len(e.sids) > 0 {
			live = true
		}
	}
	return live
}

// read copies the cache out. States are deep-copied because callers rewrite
// fields on them (the API rewrites album-art URIs to proxy through itself).
func (m *Monitor) read() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := Snapshot{
		Speakers: make(map[string]SpeakerState, len(m.entries)),
		Groups:   append([]Group(nil), m.groups...),
	}
	for id, e := range m.entries {
		if len(e.sids) > 0 {
			out.Live = true
		}
		out.Speakers[id] = SpeakerState{
			Reachable:  e.reachable,
			State:      cloneState(e.state),
			GroupState: cloneGroupState(e.groupState),
			At:         e.at,
		}
	}
	return out
}

// ── Health ───────────────────────────────────────────────────────────────

// SpeakerEvents is one speaker's subscription health. Everything here is
// reporting only — nothing the cache depends on — and it exists so a screen
// can name the speaker that isn't pushing and say why.
type SpeakerEvents struct {
	ID string
	// Subscribed is whether this speaker currently holds subscriptions.
	Subscribed bool
	// Services are the subscribed service keys, in EventServices order.
	Services []string
	// Callback is the URL this speaker was last told to POST to. The most
	// useful single field when push isn't working: it names the address the
	// speaker has to be able to reach.
	Callback  string
	RenewAt   time.Time
	LastEvent time.Time
	Events    int
	// Error is why the last subscription attempt failed, empty once one
	// succeeds.
	Error     string
	Reachable bool
	LastRead  time.Time
}

// EventHealth is the whole subsystem's report: whether push is feeding the
// cache at all, and the per-speaker detail behind that answer.
type EventHealth struct {
	// Live matches Snapshot.Live — at least one speaker is subscribed.
	Live bool
	// Running is whether the supervisor is up. False means nobody has
	// called Run, which is a different problem from speakers refusing.
	Running    bool
	Subscribed int
	Total      int
	Speakers   []SpeakerEvents
}

// Health reports the state of the event subsystem, ordered by speaker ID so
// a screen rendering it doesn't reshuffle between polls.
func (m *Monitor) Health() EventHealth {
	speakers := m.cfg.Speakers()

	m.mu.RLock()
	defer m.mu.RUnlock()
	out := EventHealth{
		Running:  m.running.Load(),
		Total:    len(speakers),
		Speakers: make([]SpeakerEvents, 0, len(speakers)),
	}
	for _, sp := range speakers {
		se := SpeakerEvents{ID: sp.ID}
		if e := m.entries[sp.ID]; e != nil {
			se.Subscribed = len(e.sids) > 0
			se.Callback = e.callback
			se.RenewAt = e.renewAt
			se.LastEvent = e.lastEvent
			se.Events = e.events
			se.Error = e.lastErr
			se.Reachable = e.reachable
			se.LastRead = e.at
			for _, svc := range EventServices {
				if _, ok := e.sids[svc.Key]; ok {
					se.Services = append(se.Services, svc.Key)
				}
			}
		}
		if se.Subscribed {
			out.Subscribed++
			out.Live = true
		}
		out.Speakers = append(out.Speakers, se)
	}
	sort.Slice(out.Speakers, func(i, j int) bool { return out.Speakers[i].ID < out.Speakers[j].ID })
	return out
}

// Retry asks every watcher to act now instead of at its own pace: one sitting
// out a backoff resubscribes immediately, and one that is already subscribed
// re-reads its speaker. It is what a "try again" control in the UI calls
// after someone changes their network, so the answer arrives while they are
// still looking at the screen rather than up to five minutes later.
//
// Fire-and-forget by design — the work happens on the watchers, and the
// caller sees the result in the next Health or Snapshot.
func (m *Monitor) Retry() {
	m.mu.RLock()
	entries := make([]*entry, 0, len(m.entries))
	for _, e := range m.entries {
		entries = append(entries, e)
	}
	m.mu.RUnlock()

	for _, e := range entries {
		// Both signals are buffered and non-blocking: whichever state the
		// watcher is in, exactly one of these is what it is waiting on, and
		// the other simply sits pending until it is drained or replaced.
		select {
		case e.retry <- struct{}{}:
		default:
		}
		select {
		case e.dirty <- struct{}{}:
		default:
		}
	}
	// Covers the case with no watchers to signal — a speaker added while the
	// supervisor was between ticks.
	m.Nudge()
}

// waitRetry waits out a watcher's backoff, returning early when Retry asks
// for one. asked distinguishes the two; alive is false only when the watcher
// should stop, either because its context ended or its speaker is gone.
func (m *Monitor) waitRetry(ctx context.Context, id string, d time.Duration) (asked, alive bool) {
	m.mu.RLock()
	e := m.entries[id]
	m.mu.RUnlock()
	if e == nil {
		return false, false
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false, false
	case <-e.retry:
		return true, true
	case <-t.C:
		return false, true
	}
}

// refreshAll reads every speaker synchronously — the cold path, and the
// permanent path in a house where subscriptions can't be established.
func (m *Monitor) refreshAll(ctx context.Context, speakers []Speaker) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	// Another caller may have swept while we waited for the lock.
	if m.freshFor(speakers) {
		return
	}

	states := make([]*State, len(speakers))
	errs := make([]error, len(speakers))
	var wg sync.WaitGroup
	for i, sp := range speakers {
		m.ensureEntry(sp)
		wg.Add(1)
		go func(i int, sp Speaker) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, perCallTimeout)
			defer cancel()
			states[i], errs[i] = GetState(cctx, sp.IP)
		}(i, sp)
	}
	wg.Wait()

	// Topology from the first speaker that answered — any speaker can
	// describe the whole household.
	for i, sp := range speakers {
		if errs[i] == nil {
			m.refreshTopology(ctx, sp.IP)
			break
		}
	}

	m.mu.RLock()
	groups := m.groups
	m.mu.RUnlock()

	// Second pass: group-level settings, for coordinators only. Needs the
	// topology, so it can't be folded into the first.
	groupStates := make([]*GroupState, len(speakers))
	var wg2 sync.WaitGroup
	for i, sp := range speakers {
		if errs[i] != nil || !isCoordinator(groups, sp.UUID) {
			continue
		}
		wg2.Add(1)
		go func(i int, sp Speaker) {
			defer wg2.Done()
			cctx, cancel := context.WithTimeout(ctx, perCallTimeout)
			defer cancel()
			if gs, err := GetGroupState(cctx, sp.IP); err == nil {
				groupStates[i] = gs
			}
		}(i, sp)
	}
	wg2.Wait()

	now := time.Now()
	m.mu.Lock()
	for i, sp := range speakers {
		e := m.entries[sp.ID]
		if e == nil {
			continue
		}
		e.reachable = errs[i] == nil
		e.state = states[i]
		e.groupState = groupStates[i]
		e.at = now
	}
	m.mu.Unlock()
}

// resync re-reads one speaker authoritatively.
func (m *Monitor) resync(ctx context.Context, id string) {
	m.mu.RLock()
	e := m.entries[id]
	if e == nil {
		m.mu.RUnlock()
		return
	}
	sp := e.sp
	haveTopology := len(m.groups) > 0
	m.mu.RUnlock()

	// Without a topology there is no telling a coordinator from a follower,
	// and the group-level read below would be skipped for up to a full
	// resync interval. Any speaker can answer for the household.
	if !haveTopology {
		m.refreshTopology(ctx, sp.IP)
	}
	m.mu.RLock()
	coordinator := isCoordinator(m.groups, sp.UUID)
	m.mu.RUnlock()

	cctx, cancel := context.WithTimeout(ctx, perCallTimeout)
	st, err := GetState(cctx, sp.IP)
	cancel()

	var gs *GroupState
	if err == nil && coordinator {
		gctx, gcancel := context.WithTimeout(ctx, perCallTimeout)
		if got, gerr := GetGroupState(gctx, sp.IP); gerr == nil {
			gs = got
		}
		gcancel()
	}

	m.mu.Lock()
	e.reachable = err == nil
	e.state = st
	e.groupState = gs
	e.at = time.Now()
	m.mu.Unlock()
}

// refreshTopology re-reads the household grouping from one speaker.
func (m *Monitor) refreshTopology(ctx context.Context, ip string) {
	tctx, cancel := context.WithTimeout(ctx, perCallTimeout)
	defer cancel()
	groups, err := GetTopology(tctx, ip)
	if err != nil || len(groups) == 0 {
		return
	}
	m.mu.Lock()
	m.groups = groups
	m.mu.Unlock()
}

// ── Bookkeeping ──────────────────────────────────────────────────────────

func (m *Monitor) ensureEntry(sp Speaker) *entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.entries[sp.ID]; e != nil {
		e.sp = sp // address or identity may have been edited
		return e
	}
	e := &entry{
		sp:    sp,
		token: newToken(),
		sids:  make(map[string]string),
		seqs:  make(map[string]int),
		dirty: make(chan struct{}, 1),
		retry: make(chan struct{}, 1),
	}
	m.entries[sp.ID] = e
	m.byToken[e.token] = e
	return e
}

func (m *Monitor) forget(id string) {
	m.mu.Lock()
	e := m.entries[id]
	if e == nil {
		m.mu.Unlock()
		return
	}
	ip, sids := e.sp.IP, e.sids
	// Drop the token before anything that can block: a removed speaker's
	// notifications must stop being accepted immediately, not once we have
	// finished talking to it.
	delete(m.byToken, e.token)
	delete(m.entries, id)
	e.sids = make(map[string]string)
	m.mu.Unlock()

	// Releasing means reaching the speaker, which may be exactly why it was
	// removed. Off the supervisor goroutine, so one unreachable speaker
	// can't hold up the watchers for every speaker that is fine.
	go m.unsubscribeSet(ip, sids)
}

// newToken mints the unguessable path segment a speaker posts to. The
// callback can't be authenticated — speakers have no credentials — so the
// token, plus the source-address check in Notify, is what stands in.
func newToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice; if it ever does, a
		// timestamp-derived token is still better than a predictable
		// constant, and Notify's address check still applies.
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

func isCoordinator(groups []Group, uuid string) bool {
	if uuid == "" {
		return false
	}
	for _, g := range groups {
		if g.CoordinatorUUID == uuid {
			return true
		}
	}
	return false
}

func cloneState(s *State) *State {
	if s == nil {
		return nil
	}
	out := *s
	if s.Track != nil {
		t := *s.Track
		out.Track = &t
	}
	return &out
}

func cloneGroupState(g *GroupState) *GroupState {
	if g == nil {
		return nil
	}
	out := *g
	return &out
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}
