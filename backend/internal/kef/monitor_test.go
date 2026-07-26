package kef

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeNet serves several fake speakers behind one listener, routed by the
// Host header — which is the IP the client dialled, since every dial is
// redirected here.
type fakeNet struct {
	t   *testing.T
	mu  sync.Mutex
	ips map[string]*fakeSpeaker
}

func newFakeNet(t *testing.T, ips ...string) *fakeNet {
	t.Helper()
	n := &fakeNet{t: t, ips: map[string]*fakeSpeaker{}}
	for _, ip := range ips {
		n.ips[ip] = &fakeSpeaker{
			t:      t,
			values: map[string]string{},
			writes: map[string]string{},
			fail:   map[string]bool{},
			hits:   map[string]int{},
		}
	}
	srv := httptest.NewServer(n)
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().String()
	prev := client
	client = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}}
	t.Cleanup(func() { client = prev })
	return n
}

func (n *fakeNet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	n.mu.Lock()
	f := n.ips[host]
	n.mu.Unlock()
	if f == nil {
		http.Error(w, "no speaker here", http.StatusServiceUnavailable)
		return
	}
	f.ServeHTTP(w, r)
}

func (n *fakeNet) speaker(ip string) *fakeSpeaker {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.ips[ip]
}

// playable loads the paths a speaker answers while it is awake and playing.
func (f *fakeSpeaker) playable(volume int) {
	f.set(pathSpeakerStatus, `{"type":"kefSpeakerStatus","kefSpeakerStatus":"powerOn"}`)
	f.set(pathSource, `{"type":"kefPhysicalSource","kefPhysicalSource":"wifi"}`)
	f.set(pathMute, `{"type":"bool_","bool_":false}`)
	f.set(pathPlayerData, playerDataJSON)
	f.set(pathPlayTime, `{"type":"i64_","i64_":1000}`)
	f.setVolume(volume)
}

func (f *fakeSpeaker) setVolume(v int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[pathVolume] = `{"type":"i32_","i32_":` + itoa(v) + `}`
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

const ipA = "192.168.1.60"
const ipB = "192.168.1.61"

func testMonitor(speakers ...Speaker) (*Monitor, *atomic.Int32) {
	var changes atomic.Int32
	m := NewMonitor(MonitorConfig{
		Speakers: func() []Speaker { return speakers },
		OnChange: func() { changes.Add(1) },
	})
	return m, &changes
}

// waitFor polls cond until it holds or the deadline passes. Used instead of
// a fixed sleep so the tests aren't timing-fragile on a loaded machine.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSnapshotColdReadsSynchronously(t *testing.T) {
	// A monitor that was never Run must still answer correctly — just
	// slower. This is the path the endpoint takes before Run starts, and
	// the one it takes forever in tests.
	n := newFakeNet(t, ipA)
	n.speaker(ipA).playable(40)

	m, _ := testMonitor(Speaker{ID: "kef_1", IP: ipA})
	snap := m.Snapshot(ctx(t))
	if snap.Warm {
		t.Error("a cold snapshot reported itself warm")
	}
	got := snap.Speakers["kef_1"]
	if !got.Reachable || got.State == nil {
		t.Fatalf("speaker = %+v, want a reading", got)
	}
	if got.State.Volume != 40 || !got.State.Playing {
		t.Errorf("state = %+v", got.State)
	}
}

func TestSnapshotWithNoSpeakers(t *testing.T) {
	m, _ := testMonitor()
	snap := m.Snapshot(ctx(t))
	if len(snap.Speakers) != 0 {
		t.Errorf("speakers = %v, want none", snap.Speakers)
	}
}

func TestSnapshotMarksUnreachable(t *testing.T) {
	// An address with nothing behind it is reported as unreachable, not as
	// missing: the UI needs the row so the user can fix the address.
	newFakeNet(t, ipA) // ipB is not served
	m, _ := testMonitor(Speaker{ID: "kef_1", IP: ipB})

	snap := m.Snapshot(ctx(t))
	got, ok := snap.Speakers["kef_1"]
	if !ok {
		t.Fatal("an unreachable speaker fell out of the snapshot")
	}
	if got.Reachable || got.State != nil {
		t.Errorf("speaker = %+v, want unreachable with no state", got)
	}
}

func TestRunKeepsTheCacheWarm(t *testing.T) {
	n := newFakeNet(t, ipA)
	n.speaker(ipA).playable(40)
	m, _ := testMonitor(Speaker{ID: "kef_1", IP: ipA})

	rctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(rctx)

	waitFor(t, "the poller's first read", func() bool {
		return m.Snapshot(context.Background()).Warm
	})
	snap := m.Snapshot(context.Background())
	if snap.Speakers["kef_1"].State == nil {
		t.Fatal("warm snapshot carried no state")
	}
	// The whole point: a client poll costs the speaker nothing.
	before := n.speaker(ipA).hitCount(pathVolume)
	for i := 0; i < 5; i++ {
		m.Snapshot(context.Background())
	}
	if after := n.speaker(ipA).hitCount(pathVolume); after != before {
		t.Errorf("five warm snapshots cost %d extra reads, want 0", after-before)
	}
}

func TestPollerReportsRealChangesOnly(t *testing.T) {
	// The position advances on every read by definition; pushing an SSE
	// frame for it would make this topic as chatty as the thing it was
	// split out to avoid.
	n := newFakeNet(t, ipA)
	f := n.speaker(ipA)
	f.playable(40)
	m, changes := testMonitor(Speaker{ID: "kef_1", IP: ipA})

	rctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(rctx)

	waitFor(t, "the first read", func() bool { return changes.Load() >= 1 })
	settled := changes.Load()

	// Position moves, nothing else does.
	f.set(pathPlayTime, `{"type":"i64_","i64_":9000}`)
	m.Touch("kef_1")
	waitFor(t, "the touched re-read", func() bool {
		return m.Snapshot(context.Background()).Speakers["kef_1"].State.PositionMS == 9000
	})
	if got := changes.Load(); got != settled {
		t.Errorf("a position-only change fired %d notifications, want 0", got-settled)
	}

	// Volume moves — that is a change worth pushing.
	f.setVolume(55)
	m.Touch("kef_1")
	waitFor(t, "the volume change", func() bool { return changes.Load() > settled })
	if got := m.Snapshot(context.Background()).Speakers["kef_1"].State.Volume; got != 55 {
		t.Errorf("cached volume = %d, want 55", got)
	}
}

func TestTouchRefreshesPromptly(t *testing.T) {
	// A control action changes the speaker faster than the next tick would
	// notice; without Touch the user watches a stale volume for 5s.
	n := newFakeNet(t, ipA)
	f := n.speaker(ipA)
	f.playable(40)
	m, _ := testMonitor(Speaker{ID: "kef_1", IP: ipA})

	rctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(rctx)
	waitFor(t, "the first read", func() bool { return m.Snapshot(context.Background()).Warm })

	f.setVolume(12)
	m.Touch("kef_1")
	waitFor(t, "the touched re-read", func() bool {
		st := m.Snapshot(context.Background()).Speakers["kef_1"].State
		return st != nil && st.Volume == 12
	})
}

func TestReconcileDropsRemovedSpeakers(t *testing.T) {
	n := newFakeNet(t, ipA, ipB)
	n.speaker(ipA).playable(40)
	n.speaker(ipB).playable(50)

	var mu sync.Mutex
	list := []Speaker{{ID: "kef_1", IP: ipA}, {ID: "kef_2", IP: ipB}}
	m := NewMonitor(MonitorConfig{Speakers: func() []Speaker {
		mu.Lock()
		defer mu.Unlock()
		return append([]Speaker(nil), list...)
	}})

	rctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(rctx)
	waitFor(t, "both speakers to be watched", func() bool { return len(m.Speakers()) == 2 })

	mu.Lock()
	list = list[:1]
	mu.Unlock()
	m.Nudge()

	waitFor(t, "the removed speaker to be dropped", func() bool {
		sp := m.Speakers()
		return len(sp) == 1 && sp[0].ID == "kef_1"
	})
	if _, ok := m.Snapshot(context.Background()).Speakers["kef_2"]; ok {
		t.Error("a removed speaker is still in the snapshot")
	}
}

func TestReconcileRewatchesAReaddressedSpeaker(t *testing.T) {
	// Editing a speaker's IP must repoint the poller; leaving it on the old
	// address would report the speaker as unreachable forever.
	n := newFakeNet(t, ipA, ipB)
	n.speaker(ipB).playable(70) // only the new address answers

	var mu sync.Mutex
	list := []Speaker{{ID: "kef_1", IP: ipA}}
	m := NewMonitor(MonitorConfig{Speakers: func() []Speaker {
		mu.Lock()
		defer mu.Unlock()
		return append([]Speaker(nil), list...)
	}})

	rctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(rctx)
	waitFor(t, "the first read", func() bool { return m.Snapshot(context.Background()).Warm })

	mu.Lock()
	list = []Speaker{{ID: "kef_1", IP: ipB}}
	mu.Unlock()
	m.Nudge()

	waitFor(t, "the speaker to answer on its new address", func() bool {
		st := m.Snapshot(context.Background()).Speakers["kef_1"].State
		return st != nil && st.Volume == 70
	})
}

func TestSnapshotCopiesState(t *testing.T) {
	// The API rewrites album-art URLs on what Snapshot hands back; that must
	// not reach into the cache and corrupt the next reader's copy.
	n := newFakeNet(t, ipA)
	n.speaker(ipA).playable(40)
	m, _ := testMonitor(Speaker{ID: "kef_1", IP: ipA})

	first := m.Snapshot(ctx(t)).Speakers["kef_1"]
	if first.State == nil || first.State.Track == nil {
		t.Fatal("no track in the first snapshot")
	}
	first.State.Track.ArtURI = "/rewritten"
	first.State.Volume = 999

	second := m.Snapshot(ctx(t)).Speakers["kef_1"]
	if second.State.Track.ArtURI == "/rewritten" || second.State.Volume == 999 {
		t.Error("mutating a snapshot changed what the cache hands out next")
	}
}

func TestSnapshotSkipsInvalidAddresses(t *testing.T) {
	// A speaker whose stored address is junk can't be read; it must not
	// wedge the sweep for everybody else.
	n := newFakeNet(t, ipA)
	n.speaker(ipA).playable(40)
	m, _ := testMonitor(
		Speaker{ID: "kef_1", IP: ipA},
		Speaker{ID: "kef_bad", IP: "127.0.0.1"},
	)
	snap := m.Snapshot(ctx(t))
	if snap.Speakers["kef_1"].State == nil {
		t.Error("a bad address stopped a good speaker from being read")
	}
	bad, ok := snap.Speakers["kef_bad"]
	if !ok {
		t.Fatal("a speaker with an invalid address fell out of the snapshot")
	}
	if bad.Reachable || bad.State != nil {
		t.Errorf("kef_bad = %+v, want unreachable with no state", bad)
	}
}

func TestUnreadableAddressDoesNotKeepTheCacheCold(t *testing.T) {
	// One mistyped address must not force a synchronous sweep of every
	// other speaker on every request.
	n := newFakeNet(t, ipA)
	n.speaker(ipA).playable(40)
	m, _ := testMonitor(
		Speaker{ID: "kef_1", IP: ipA},
		Speaker{ID: "kef_bad", IP: "127.0.0.1"},
	)

	rctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(rctx)

	waitFor(t, "the cache to go warm despite the bad address", func() bool {
		return m.Snapshot(context.Background()).Warm
	})
	before := n.speaker(ipA).hitCount(pathVolume)
	for i := 0; i < 5; i++ {
		m.Snapshot(context.Background())
	}
	if after := n.speaker(ipA).hitCount(pathVolume); after != before {
		t.Errorf("warm snapshots still cost %d reads", after-before)
	}
}
