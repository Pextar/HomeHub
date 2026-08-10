package sonos

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"sync"
	"testing"
	"time"
)

// subscribed registers a speaker and fakes a live subscription on it, as if
// subscribeAll had just succeeded. Lets the notification path be tested
// without a speaker on the network.
func subscribed(m *Monitor, sp Speaker, sids map[string]string) *entry {
	e := m.ensureEntry(sp)
	m.mu.Lock()
	defer m.mu.Unlock()
	e.sids = sids
	e.at = time.Now()
	e.reachable = true
	return e
}

// testMonitor builds a monitor holding one subscribed speaker, and returns a
// counter of how many times OnChange fired.
func testMonitor(t *testing.T) (m *Monitor, e *entry, changes *int) {
	t.Helper()
	n := 0
	sp := Speaker{ID: "sonos_1", IP: "192.168.1.42", UUID: "RINCON_AAA"}
	m = NewMonitor(MonitorConfig{
		Speakers: func() []Speaker { return []Speaker{sp} },
		OnChange: func() { n++ },
	})
	e = subscribed(m, sp, map[string]string{
		EventTransport.Key: "uuid:sub-transport",
		EventRendering.Key: "uuid:sub-rendering",
		EventTopology.Key:  "uuid:sub-topology",
	})
	return m, e, &n
}

func transportNotify(t *testing.T, values string) string {
	t.Helper()
	return notifyBody("LastChange", `<Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/">`+
		`<InstanceID val="0">`+values+`</InstanceID></Event>`)
}

func renderingNotify(t *testing.T, values string) string {
	t.Helper()
	return notifyBody("LastChange", `<Event xmlns="urn:schemas-upnp-org:metadata-1-0/RCS/">`+
		`<InstanceID val="0">`+values+`</InstanceID></Event>`)
}

// ── What the callback endpoint will and won't accept ─────────────────────
//
// The callback can't be authenticated — speakers have no credentials — so
// these three checks are the whole of its defence.

func TestNotifyRejectsUnknownToken(t *testing.T) {
	m, _, _ := testMonitor(t)
	body := transportNotify(t, `<TransportState val="PLAYING"/>`)
	if m.Notify("not-a-token", "uuid:sub-transport", 1, body, "192.168.1.42") {
		t.Error("Notify accepted an unknown token")
	}
}

func TestNotifyRejectsForeignSourceAddress(t *testing.T) {
	m, e, _ := testMonitor(t)
	body := transportNotify(t, `<TransportState val="PLAYING"/>`)
	if m.Notify(e.token, "uuid:sub-transport", 1, body, "10.0.0.9") {
		t.Error("Notify accepted a notification from an address that isn't the speaker's")
	}
	if e.state != nil {
		t.Error("a rejected notification still changed the cache")
	}
}

func TestNotifyRejectsUnknownSID(t *testing.T) {
	m, e, _ := testMonitor(t)
	body := transportNotify(t, `<TransportState val="PLAYING"/>`)
	if m.Notify(e.token, "uuid:some-other-subscription", 1, body, "192.168.1.42") {
		t.Error("Notify accepted a SID we never issued")
	}
}

// After a speaker is removed its token must stop working immediately, not at
// whenever its subscription would have expired.
func TestForgetInvalidatesToken(t *testing.T) {
	m, e, _ := testMonitor(t)
	token := e.token
	m.forget("sonos_1")
	body := transportNotify(t, `<TransportState val="PLAYING"/>`)
	if m.Notify(token, "uuid:sub-transport", 1, body, "192.168.1.42") {
		t.Error("Notify accepted the token of a forgotten speaker")
	}
	if _, ok := m.entries["sonos_1"]; ok {
		t.Error("forget left the entry behind")
	}
}

// ── Applying deltas ──────────────────────────────────────────────────────

func TestNotifyAppliesTransportDelta(t *testing.T) {
	m, e, changes := testMonitor(t)
	body := transportNotify(t, `<TransportState val="PLAYING"/><CurrentTrack val="3"/>`+
		`<CurrentTrackDuration val="0:03:21"/>`)
	if !m.Notify(e.token, "uuid:sub-transport", 1, body, "192.168.1.42") {
		t.Fatal("Notify rejected a valid notification")
	}
	if e.state == nil {
		t.Fatal("no state after a transport notification")
	}
	if e.state.TransportState != "PLAYING" || !e.state.Playing {
		t.Errorf("TransportState = %q playing = %v, want PLAYING/true", e.state.TransportState, e.state.Playing)
	}
	if e.state.QueueTrack != 3 {
		t.Errorf("QueueTrack = %d, want 3", e.state.QueueTrack)
	}
	if e.state.Duration != "0:03:21" {
		t.Errorf("Duration = %q, want 0:03:21", e.state.Duration)
	}
	if *changes != 1 {
		t.Errorf("OnChange fired %d times, want 1", *changes)
	}
}

// TRANSITIONING is what a speaker reports mid track-change; the UI must keep
// showing "playing" through it rather than flickering to paused.
func TestNotifyTransitioningCountsAsPlaying(t *testing.T) {
	m, e, _ := testMonitor(t)
	body := transportNotify(t, `<TransportState val="TRANSITIONING"/>`)
	m.Notify(e.token, "uuid:sub-transport", 1, body, "192.168.1.42")
	if !e.state.Playing {
		t.Error("TRANSITIONING did not count as playing")
	}
}

func TestNotifyAppliesVolumeAndMute(t *testing.T) {
	m, e, _ := testMonitor(t)
	body := renderingNotify(t, `<Volume channel="Master" val="35"/><Volume channel="LF" val="100"/>`+
		`<Mute channel="Master" val="1"/>`)
	if !m.Notify(e.token, "uuid:sub-rendering", 1, body, "192.168.1.42") {
		t.Fatal("Notify rejected a valid rendering notification")
	}
	if e.state.Volume != 35 {
		t.Errorf("Volume = %d, want 35", e.state.Volume)
	}
	if !e.state.Muted {
		t.Error("Muted = false, want true")
	}
}

// Track metadata is escaped twice over — once into the val attribute, once
// into the LastChange property — so it is worth proving it survives both.
func TestNotifyAppliesTrackMetadata(t *testing.T) {
	m, e, _ := testMonitor(t)
	didl := `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">` +
		`<item><dc:title>Teardrop</dc:title><dc:creator>Massive Attack</dc:creator>` +
		`<upnp:album>Mezzanine</upnp:album></item></DIDL-Lite>`
	var attr bytes.Buffer
	_ = xml.EscapeText(&attr, []byte(didl))
	body := transportNotify(t, `<CurrentTrackMetaData val="`+attr.String()+`"/>`)

	if !m.Notify(e.token, "uuid:sub-transport", 1, body, "192.168.1.42") {
		t.Fatal("Notify rejected a valid notification")
	}
	if e.state.Track == nil {
		t.Fatal("no track parsed out of the notification")
	}
	if e.state.Track.Title != "Teardrop" || e.state.Track.Artist != "Massive Attack" {
		t.Errorf("track = %+v, want Teardrop / Massive Attack", e.state.Track)
	}
}

// Play modes belong to the group, and only the coordinator carries a
// GroupState. A follower reporting a play mode must not have one invented
// for it — that would light up shuffle/repeat controls on a speaker that
// doesn't own them.
func TestTransportDeltaSkipsGroupStateOnFollower(t *testing.T) {
	m, e, _ := testMonitor(t)
	body := transportNotify(t, `<CurrentPlayMode val="SHUFFLE_NOREPEAT"/><NumberOfTracks val="9"/>`)
	m.Notify(e.token, "uuid:sub-transport", 1, body, "192.168.1.42")
	if e.groupState != nil {
		t.Errorf("a follower grew a GroupState: %+v", e.groupState)
	}
}

func TestTransportDeltaUpdatesGroupStateOnCoordinator(t *testing.T) {
	m, e, _ := testMonitor(t)
	m.mu.Lock()
	e.groupState = &GroupState{}
	m.mu.Unlock()

	body := transportNotify(t, `<CurrentPlayMode val="SHUFFLE_NOREPEAT"/><NumberOfTracks val="9"/>`+
		`<CurrentCrossfadeMode val="1"/><AVTransportURI val="x-rincon-queue:RINCON_AAA#0"/>`)
	m.Notify(e.token, "uuid:sub-transport", 1, body, "192.168.1.42")

	gs := e.groupState
	if !gs.Shuffle || gs.Repeat != "off" {
		t.Errorf("shuffle = %v repeat = %q, want true/off", gs.Shuffle, gs.Repeat)
	}
	if gs.QueueLength != 9 {
		t.Errorf("QueueLength = %d, want 9", gs.QueueLength)
	}
	if !gs.Crossfade {
		t.Error("Crossfade = false, want true")
	}
	if !gs.FromQueue {
		t.Error("FromQueue = false, want true for an x-rincon-queue: URI")
	}
}

// A notification carries only what changed. Fields it doesn't mention must
// survive it rather than being zeroed.
func TestTransportDeltaLeavesUnmentionedFieldsAlone(t *testing.T) {
	m, e, _ := testMonitor(t)
	m.mu.Lock()
	e.state = &State{Volume: 40, Muted: true, Position: "0:01:00", Track: &Track{Title: "Held"}}
	m.mu.Unlock()

	m.Notify(e.token, "uuid:sub-transport", 1, transportNotify(t, `<TransportState val="PAUSED_PLAYBACK"/>`), "192.168.1.42")

	if e.state.Volume != 40 || !e.state.Muted {
		t.Errorf("volume/mute clobbered: %d / %v", e.state.Volume, e.state.Muted)
	}
	if e.state.Track == nil || e.state.Track.Title != "Held" {
		t.Errorf("track clobbered: %+v", e.state.Track)
	}
	if e.state.Position != "0:01:00" {
		t.Errorf("position clobbered: %q", e.state.Position)
	}
}

// ── Sequencing ───────────────────────────────────────────────────────────

// GENA numbers notifications per subscription. A reordered delivery would
// otherwise apply stale values over fresh ones.
func TestNotifyIgnoresReorderedSequence(t *testing.T) {
	m, e, _ := testMonitor(t)
	m.Notify(e.token, "uuid:sub-transport", 5, transportNotify(t, `<TransportState val="PLAYING"/>`), "192.168.1.42")
	// An older notification arriving late must be dropped...
	if !m.Notify(e.token, "uuid:sub-transport", 4, transportNotify(t, `<TransportState val="STOPPED"/>`), "192.168.1.42") {
		t.Error("a reordered notification should still be acknowledged, not 412'd")
	}
	if e.state.TransportState != "PLAYING" {
		t.Errorf("TransportState = %q, want PLAYING — a stale notification was applied", e.state.TransportState)
	}
	// ...but a newer one still lands.
	m.Notify(e.token, "uuid:sub-transport", 6, transportNotify(t, `<TransportState val="STOPPED"/>`), "192.168.1.42")
	if e.state.TransportState != "STOPPED" {
		t.Errorf("TransportState = %q, want STOPPED", e.state.TransportState)
	}
}

// SEQ 0 is the full state a speaker sends on subscribe, and it restarts the
// count — so it must never be mistaken for a reordered old notification.
func TestNotifySeqZeroRestartsTheCount(t *testing.T) {
	m, e, _ := testMonitor(t)
	m.Notify(e.token, "uuid:sub-transport", 9, transportNotify(t, `<TransportState val="PLAYING"/>`), "192.168.1.42")
	m.Notify(e.token, "uuid:sub-transport", 0, transportNotify(t, `<TransportState val="STOPPED"/>`), "192.168.1.42")
	if e.state.TransportState != "STOPPED" {
		t.Errorf("TransportState = %q, want STOPPED — SEQ 0 was treated as stale", e.state.TransportState)
	}
}

// Sequences are per subscription, so transport and rendering must not share
// a counter.
func TestNotifySequencesAreIndependentPerService(t *testing.T) {
	m, e, _ := testMonitor(t)
	m.Notify(e.token, "uuid:sub-transport", 7, transportNotify(t, `<TransportState val="PLAYING"/>`), "192.168.1.42")
	m.Notify(e.token, "uuid:sub-rendering", 1, renderingNotify(t, `<Volume channel="Master" val="22"/>`), "192.168.1.42")
	if e.state.Volume != 22 {
		t.Errorf("Volume = %d, want 22 — rendering was judged against transport's sequence", e.state.Volume)
	}
}

// Every notification, even one carrying nothing we understand, must leave
// the speaker marked for an authoritative re-read.
func TestNotifyAlwaysMarksDirty(t *testing.T) {
	m, e, _ := testMonitor(t)
	drain(e)
	if !m.Notify(e.token, "uuid:sub-transport", 1, notifyBody("LastChange", `<Event/>`), "192.168.1.42") {
		t.Fatal("Notify rejected a valid but empty notification")
	}
	select {
	case <-e.dirty:
	default:
		t.Error("an unreadable notification did not schedule a re-read")
	}
}

func drain(e *entry) {
	select {
	case <-e.dirty:
	default:
	}
}

// ── Topology ─────────────────────────────────────────────────────────────

const twoGroupTopology = `<ZoneGroupState><ZoneGroups>
<ZoneGroup Coordinator="RINCON_AAA" ID="RINCON_AAA:1">
  <ZoneGroupMember UUID="RINCON_AAA" Location="http://192.168.1.42:1400/xml/device_description.xml" ZoneName="Kitchen"/>
</ZoneGroup>
<ZoneGroup Coordinator="RINCON_BBB" ID="RINCON_BBB:2">
  <ZoneGroupMember UUID="RINCON_BBB" Location="http://192.168.1.43:1400/xml/device_description.xml" ZoneName="Study"/>
</ZoneGroup>
</ZoneGroups></ZoneGroupState>`

// Grouping changes which speaker coordinates which zone, and group-level
// state is only ever read off a coordinator — so every speaker's cached
// state is suspect, not just the one that reported the change.
func TestTopologyNotifyMarksEverySpeakerDirty(t *testing.T) {
	m, e, _ := testMonitor(t)
	other := subscribed(m, Speaker{ID: "sonos_2", IP: "192.168.1.43", UUID: "RINCON_BBB"}, map[string]string{
		EventTopology.Key: "uuid:sub-topology-2",
	})
	drain(e)
	drain(other)

	if !m.Notify(e.token, "uuid:sub-topology", 1, notifyBody("ZoneGroupState", twoGroupTopology), "192.168.1.42") {
		t.Fatal("Notify rejected a valid topology notification")
	}
	if len(m.groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(m.groups))
	}
	for name, ent := range map[string]*entry{"reporter": e, "other": other} {
		select {
		case <-ent.dirty:
		default:
			t.Errorf("%s speaker was not marked for a re-read after a topology change", name)
		}
	}
}

func TestTopologyNotifyIgnoresUnparseablePayload(t *testing.T) {
	m, e, _ := testMonitor(t)
	m.mu.Lock()
	m.groups = []Group{{CoordinatorUUID: "RINCON_AAA"}}
	m.mu.Unlock()

	m.Notify(e.token, "uuid:sub-topology", 1, notifyBody("ZoneGroupState", "<nonsense/>"), "192.168.1.42")
	if len(m.groups) != 1 || m.groups[0].CoordinatorUUID != "RINCON_AAA" {
		t.Errorf("a garbage topology replaced a good one: %+v", m.groups)
	}
}

func TestIsCoordinator(t *testing.T) {
	groups := []Group{{CoordinatorUUID: "RINCON_AAA"}, {CoordinatorUUID: "RINCON_BBB"}}
	if !isCoordinator(groups, "RINCON_BBB") {
		t.Error("RINCON_BBB should be a coordinator")
	}
	if isCoordinator(groups, "RINCON_CCC") {
		t.Error("RINCON_CCC should not be a coordinator")
	}
	// A speaker whose UUID we never learned must not match the zero value
	// of some group's coordinator.
	if isCoordinator([]Group{{CoordinatorUUID: ""}}, "") {
		t.Error("an empty UUID should never be a coordinator")
	}
}

// ── Freshness and snapshots ──────────────────────────────────────────────

// The cache is only trustworthy when subscriptions are actually feeding it.
// Recent reads alone would let a dead event stream pass as a live one.
func TestFreshForRequiresALiveSubscription(t *testing.T) {
	sp := Speaker{ID: "sonos_1", IP: "192.168.1.42", UUID: "RINCON_AAA"}
	m := NewMonitor(MonitorConfig{Speakers: func() []Speaker { return []Speaker{sp} }})
	e := m.ensureEntry(sp)
	m.mu.Lock()
	e.at = time.Now() // read just now, but nothing is subscribed
	m.mu.Unlock()

	if m.freshFor([]Speaker{sp}) {
		t.Error("freshFor = true with no subscriptions; the cache must not be trusted")
	}
}

func TestFreshForRejectsStaleAndUnknownSpeakers(t *testing.T) {
	sp := Speaker{ID: "sonos_1", IP: "192.168.1.42"}
	m := NewMonitor(MonitorConfig{Speakers: func() []Speaker { return []Speaker{sp} }})
	e := subscribed(m, sp, map[string]string{EventTransport.Key: "uuid:s"})
	if !m.freshFor([]Speaker{sp}) {
		t.Fatal("freshFor = false for a just-read, subscribed speaker")
	}

	m.mu.Lock()
	e.at = time.Now().Add(-staleAfter - time.Second)
	m.mu.Unlock()
	if m.freshFor([]Speaker{sp}) {
		t.Error("freshFor = true for a speaker last read beyond the stale window")
	}

	// A speaker registered a moment ago has no entry yet, so the whole
	// snapshot has to be read live rather than answering without it.
	m.mu.Lock()
	e.at = time.Now()
	m.mu.Unlock()
	newcomer := Speaker{ID: "sonos_2", IP: "192.168.1.43"}
	if m.freshFor([]Speaker{sp, newcomer}) {
		t.Error("freshFor = true although one speaker has never been read")
	}
}

func TestFreshForEmptyHousehold(t *testing.T) {
	m := NewMonitor(MonitorConfig{})
	if m.freshFor(nil) {
		t.Error("freshFor = true with no speakers registered")
	}
}

// Snapshot hands out copies: the API rewrites album-art URIs on what it gets
// back, and that must not corrupt what the next reader sees.
func TestSnapshotIsACopy(t *testing.T) {
	sp := Speaker{ID: "sonos_1", IP: "192.168.1.42"}
	m := NewMonitor(MonitorConfig{Speakers: func() []Speaker { return []Speaker{sp} }})
	e := subscribed(m, sp, map[string]string{EventTransport.Key: "uuid:s"})
	m.mu.Lock()
	e.state = &State{Volume: 30, Track: &Track{Title: "Original", ArtURI: "/getaa?x=1"}}
	e.groupState = &GroupState{QueueLength: 4}
	m.mu.Unlock()

	snap := m.read()
	got := snap.Speakers["sonos_1"]
	got.State.Volume = 99
	got.State.Track.ArtURI = "/api/sonos/rewritten"
	got.GroupState.QueueLength = 0

	if e.state.Volume != 30 {
		t.Errorf("cached volume = %d, want 30 — the snapshot shared its State", e.state.Volume)
	}
	if e.state.Track.ArtURI != "/getaa?x=1" {
		t.Errorf("cached art = %q — the snapshot shared its Track", e.state.Track.ArtURI)
	}
	if e.groupState.QueueLength != 4 {
		t.Errorf("cached queue length = %d, want 4 — the snapshot shared its GroupState", e.groupState.QueueLength)
	}
	if !snap.Live {
		t.Error("Live = false although a subscription is active")
	}
}

func TestReadReportsNotLiveWithoutSubscriptions(t *testing.T) {
	sp := Speaker{ID: "sonos_1", IP: "192.168.1.42"}
	m := NewMonitor(MonitorConfig{Speakers: func() []Speaker { return []Speaker{sp} }})
	m.ensureEntry(sp)
	if m.read().Live {
		t.Error("Live = true with no subscriptions")
	}
}

// ── Entry bookkeeping ────────────────────────────────────────────────────

// Re-registering a speaker must update it in place: a new token would strand
// the subscriptions already issued against the old one.
func TestEnsureEntryUpdatesInPlace(t *testing.T) {
	m := NewMonitor(MonitorConfig{})
	first := m.ensureEntry(Speaker{ID: "sonos_1", IP: "192.168.1.42"})
	token := first.token

	second := m.ensureEntry(Speaker{ID: "sonos_1", IP: "192.168.1.99", UUID: "RINCON_AAA"})
	if second != first {
		t.Error("ensureEntry replaced the entry instead of updating it")
	}
	if second.token != token {
		t.Error("ensureEntry minted a new token for an existing speaker")
	}
	if second.sp.IP != "192.168.1.99" || second.sp.UUID != "RINCON_AAA" {
		t.Errorf("entry not updated: %+v", second.sp)
	}
	if m.byToken[token] != first {
		t.Error("token index no longer points at the entry")
	}
}

func TestNewTokenIsUnguessable(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		tok := newToken()
		if len(tok) < 24 {
			t.Fatalf("token %q is too short to be unguessable", tok)
		}
		if seen[tok] {
			t.Fatalf("token %q repeated", tok)
		}
		seen[tok] = true
	}
}

func TestNextBackoffClimbsAndCaps(t *testing.T) {
	d := minBackoff
	for i := 0; i < 20; i++ {
		next := nextBackoff(d)
		if next <= d && next != maxBackoff {
			t.Fatalf("backoff did not grow: %v → %v", d, next)
		}
		d = next
	}
	if d != maxBackoff {
		t.Errorf("backoff settled at %v, want the %v cap", d, maxBackoff)
	}
}

// ── Concurrency ──────────────────────────────────────────────────────────

// Notifications arrive on the HTTP server's goroutines while clients read
// snapshots and the supervisor adds and removes speakers — all at once. Run
// under -race, this is what proves the lock discipline holds.
func TestMonitorConcurrentAccess(t *testing.T) {
	speakers := []Speaker{
		{ID: "sonos_1", IP: "192.168.1.42", UUID: "RINCON_AAA"},
		{ID: "sonos_2", IP: "192.168.1.43", UUID: "RINCON_BBB"},
	}
	m := NewMonitor(MonitorConfig{Speakers: func() []Speaker { return speakers }})
	entries := make([]*entry, len(speakers))
	for i, sp := range speakers {
		entries[i] = subscribed(m, sp, map[string]string{
			EventTransport.Key: "uuid:t" + sp.ID,
			EventRendering.Key: "uuid:r" + sp.ID,
			EventTopology.Key:  "uuid:z" + sp.ID,
		})
	}

	const rounds = 200
	var wg sync.WaitGroup
	for i, sp := range speakers {
		wg.Add(3)
		// Transport notifications.
		go func(e *entry, ip string) {
			defer wg.Done()
			for n := 1; n <= rounds; n++ {
				m.Notify(e.token, "uuid:t"+e.sp.ID, n,
					transportNotify(t, `<TransportState val="PLAYING"/><CurrentTrack val="2"/>`), ip)
			}
		}(entries[i], sp.IP)
		// Volume notifications, as a drag would produce.
		go func(e *entry, ip string) {
			defer wg.Done()
			for n := 1; n <= rounds; n++ {
				m.Notify(e.token, "uuid:r"+e.sp.ID, n,
					renderingNotify(t, `<Volume channel="Master" val="30"/>`), ip)
			}
		}(entries[i], sp.IP)
		// Topology notifications, which touch every entry at once.
		go func(e *entry, ip string) {
			defer wg.Done()
			for n := 1; n <= rounds; n++ {
				m.Notify(e.token, "uuid:z"+e.sp.ID, n, notifyBody("ZoneGroupState", twoGroupTopology), ip)
			}
		}(entries[i], sp.IP)
	}
	// Readers, plus the drain a watcher's settle loop would be doing.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < rounds; n++ {
				snap := m.read()
				if len(snap.Speakers) == 0 {
					t.Error("snapshot came back empty")
					return
				}
				m.freshFor(speakers)
				for _, e := range entries {
					drain(e)
				}
			}
		}()
	}
	wg.Wait()

	for _, e := range entries {
		if e.state == nil || e.state.TransportState != "PLAYING" || e.state.Volume != 30 {
			t.Errorf("%s ended at %+v, want PLAYING at volume 30", e.sp.ID, e.state)
		}
	}
}

// A watcher winding down must release only the subscriptions it created.
// When a speaker's address is edited the old watcher and its replacement
// overlap briefly, and the departing one tearing down its successor's
// subscriptions would leave the speaker silently unwatched.
func TestReleaseAllIgnoresSupersededGeneration(t *testing.T) {
	m, e, _ := testMonitor(t)
	m.mu.Lock()
	e.gen = 2 // a newer watcher has taken over
	m.mu.Unlock()

	m.releaseAll("sonos_1", 1) // the old watcher, winding down

	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(e.sids) != 3 {
		t.Errorf("got %d subscriptions after a superseded release, want the 3 still live", len(e.sids))
	}
}

func TestReleaseAllOnAMissingSpeakerIsANoop(t *testing.T) {
	m, _, _ := testMonitor(t)
	m.releaseAll("sonos_gone", 1) // must not panic
}

// ── Health ───────────────────────────────────────────────────────────────
//
// Health is what the UI reads to explain push to the user, so the states it
// has to tell apart are: subscribed, failing with a reason, and never tried.

func TestHealthReportsASubscribedSpeaker(t *testing.T) {
	m, e, _ := testMonitor(t)
	m.mu.Lock()
	e.callback = "http://192.168.1.5:8080/sonos/event/abc"
	e.events = 7
	e.lastEvent = time.Now()
	m.mu.Unlock()

	h := m.Health()
	if !h.Live || h.Subscribed != 1 || h.Total != 1 {
		t.Fatalf("got live=%v subscribed=%d total=%d, want a single live speaker", h.Live, h.Subscribed, h.Total)
	}
	if len(h.Speakers) != 1 {
		t.Fatalf("got %d speakers in the report, want 1", len(h.Speakers))
	}
	got := h.Speakers[0]
	if !got.Subscribed || got.Events != 7 || got.Callback == "" {
		t.Errorf("got %+v, want it subscribed with its event count and callback", got)
	}
	// Services are reported in EventServices order so the UI doesn't reshuffle.
	if len(got.Services) != 3 || got.Services[0] != EventTransport.Key {
		t.Errorf("got services %v, want all three in EventServices order", got.Services)
	}
}

func TestHealthCarriesTheSubscribeFailure(t *testing.T) {
	m, e, _ := testMonitor(t)
	m.mu.Lock()
	e.sids = map[string]string{} // nothing subscribed
	m.mu.Unlock()
	m.noteSubscribeErr("sonos_1", errors.New("no local address can reach 192.168.1.42"))

	h := m.Health()
	if h.Live || h.Subscribed != 0 {
		t.Errorf("got live=%v subscribed=%d, want push reported as off", h.Live, h.Subscribed)
	}
	if h.Total != 1 {
		t.Errorf("got total=%d, want the speaker still counted", h.Total)
	}
	if h.Speakers[0].Error == "" {
		t.Error("Health dropped the reason the subscription failed")
	}
}

// A speaker registered but never reached has no entry at all; it must still
// appear, or the UI would silently list fewer speakers than the user has.
func TestHealthIncludesSpeakersWithNoEntry(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		Speakers: func() []Speaker {
			return []Speaker{{ID: "sonos_2", IP: "192.168.1.9"}}
		},
	})
	h := m.Health()
	if h.Total != 1 || len(h.Speakers) != 1 {
		t.Fatalf("got total=%d speakers=%d, want the unreached speaker listed", h.Total, len(h.Speakers))
	}
	if h.Speakers[0].Subscribed || h.Live {
		t.Error("a speaker that was never reached is reported as subscribed")
	}
	if h.Running {
		t.Error("Running is true for a monitor that was never started")
	}
}

// Retry is the "try again" button. Its whole job is to release a watcher
// early from a backoff that can be five minutes long.
func TestRetryWakesABackingOffWatcher(t *testing.T) {
	m, _, _ := testMonitor(t)
	done := make(chan bool, 1)
	go func() {
		asked, alive := m.waitRetry(context.Background(), "sonos_1", time.Hour)
		done <- asked && alive
	}()

	// Give the waiter a moment to park on the channel before signalling.
	time.Sleep(20 * time.Millisecond)
	m.Retry()

	select {
	case ok := <-done:
		if !ok {
			t.Error("waitRetry returned, but not as a retry that should continue")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Retry did not wake the watcher out of its backoff")
	}
}

func TestWaitRetryStopsForARemovedSpeaker(t *testing.T) {
	m, _, _ := testMonitor(t)
	if asked, alive := m.waitRetry(context.Background(), "sonos_gone", time.Millisecond); asked || alive {
		t.Errorf("got asked=%v alive=%v, want the watcher told to stop", asked, alive)
	}
}

func TestRetryWithNoSpeakersIsANoop(t *testing.T) {
	m := NewMonitor(MonitorConfig{})
	m.Retry() // must not panic or block
}

// The snapshot carries when each reading was taken. Clients extrapolate the
// track position from it, and the cache is normally what answers a status
// request: events bring a track change but never a position, so a cached
// position is as old as the last authoritative read, not as old as the
// request that fetched it.
func TestSnapshotCarriesReadTime(t *testing.T) {
	sp := Speaker{ID: "sonos_1", IP: "192.168.1.42"}
	m := NewMonitor(MonitorConfig{Speakers: func() []Speaker { return []Speaker{sp} }})
	e := subscribed(m, sp, map[string]string{EventTransport.Key: "uuid:s"})
	read := time.Now().Add(-25 * time.Second)
	m.mu.Lock()
	e.state = &State{Volume: 30, Position: "0:01:00"}
	e.at = read
	m.mu.Unlock()

	got := m.read().Speakers["sonos_1"]
	if !got.At.Equal(read) {
		t.Errorf("At = %v, want %v — the reading's own timestamp", got.At, read)
	}
}

// A speaker that has never been read has no timestamp to offer, and must not
// invent one: the API drops the field, and the client falls back to counting
// from its own poll rather than from an age that was made up here.
func TestSnapshotHasNoReadTimeBeforeTheFirstRead(t *testing.T) {
	sp := Speaker{ID: "sonos_1", IP: "192.168.1.42"}
	m := NewMonitor(MonitorConfig{Speakers: func() []Speaker { return []Speaker{sp} }})
	m.ensureEntry(sp)
	if got := m.read().Speakers["sonos_1"]; !got.At.IsZero() {
		t.Errorf("At = %v, want zero for a speaker never read", got.At)
	}
}
