package media

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder is an Endpoint that logs what was done to it, plus the optional
// interfaces selected by its capability set. It is one type rather than
// several so a test can assert on ordering across a whole zone.
type recorder struct {
	desc Descriptor

	mu   sync.Mutex
	log  []string
	fail map[string]error

	// joined is the coordinator this endpoint was last grouped onto.
	joined string
	// coordinator is what Coordinator() reports, i.e. who leads it now.
	coordinator string
	// playedURI and playedMeta capture the stream route's handover.
	playedURI  string
	playedMeta Metadata
	// nativeURI captures the native route's handover.
	nativeURI string
}

func rec(name string, vendor Vendor, caps Capability, groupKey string) *recorder {
	return &recorder{
		desc: Descriptor{ID: strings.ToLower(name), Name: name, Vendor: vendor,
			Caps: caps, GroupKey: groupKey},
		fail: map[string]error{},
	}
}

func (r *recorder) note(what string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, what)
	return r.fail[what]
}

func (r *recorder) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.log...)
}

func (r *recorder) did(what string) bool {
	for _, c := range r.calls() {
		if c == what {
			return true
		}
	}
	return false
}

func (r *recorder) Descriptor() Descriptor { return r.desc }
func (r *recorder) State(context.Context) (*NowPlaying, error) {
	return &NowPlaying{State: StateStopped}, r.note("state")
}
func (r *recorder) Play(context.Context) error     { return r.note("play") }
func (r *recorder) Pause(context.Context) error    { return r.note("pause") }
func (r *recorder) Next(context.Context) error     { return r.note("next") }
func (r *recorder) Previous(context.Context) error { return r.note("previous") }
func (r *recorder) SetVolume(_ context.Context, v int) error {
	return r.note("volume")
}
func (r *recorder) SetMute(context.Context, bool) error { return r.note("mute") }

func (r *recorder) Wake(context.Context) error { return r.note("wake") }

func (r *recorder) Join(_ context.Context, c Endpoint) error {
	r.mu.Lock()
	r.joined = c.Descriptor().Name
	r.mu.Unlock()
	return r.note("join")
}
func (r *recorder) Leave(context.Context) error { return r.note("leave") }
func (r *recorder) Coordinator(context.Context) (string, error) {
	r.mu.Lock()
	c := r.coordinator
	r.mu.Unlock()
	return c, r.note("coordinator")
}

func (r *recorder) PlayURI(_ context.Context, uri string, m Metadata) error {
	r.mu.Lock()
	r.playedURI, r.playedMeta = uri, m
	r.mu.Unlock()
	return r.note("play_uri")
}

func (r *recorder) PlayNative(_ context.Context, uri, _ string) error {
	r.mu.Lock()
	r.nativeURI = uri
	r.mu.Unlock()
	return r.note("play_native")
}

func (r *recorder) ServiceAccount(context.Context, string) (Account, error) {
	return Account{SID: 9, Serial: "1", Type: 2311}, r.note("service_account")
}

func (r *recorder) ConnectHint() (string, []string) {
	return "", []string{r.desc.Name}
}

func sonosRec(name string) *recorder { return rec(name, VendorSonos, sonosCaps, "household-1") }
func kefRec(name string) *recorder   { return rec(name, VendorKEF, kefCaps, "") }

// fakeHost is a StreamHost that hands out a fixed URL and records teardown.
type fakeHost struct {
	url     string
	delay   time.Duration
	mu      sync.Mutex
	stopped int
	err     error
}

func (h *fakeHost) Publish(context.Context, *Stream) (string, func(), error) {
	if h.err != nil {
		return "", nil, h.err
	}
	return h.url, func() {
		h.mu.Lock()
		h.stopped++
		h.mu.Unlock()
	}, nil
}

func (h *fakeHost) StartDelay(Vendor) time.Duration { return h.delay }

func (h *fakeHost) stops() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopped
}

// streamingProvider is a provider whose stream route hands back a body the
// test controls.
type streamingProvider struct {
	*fakeProvider
	nativeImpl
	connectImpl
	body        io.ReadCloser
	contentType string
}

func (s streamingProvider) StreamAvailable() Availability {
	return Availability{OK: true, Configured: true}
}

func (s streamingProvider) OpenStream(context.Context, Item) (*Stream, error) {
	return &Stream{
		Body:        s.body,
		ContentType: s.contentType,
		Meta:        Metadata{Title: "Test Track", Artist: "Tester"},
	}, nil
}

func newStreamingProvider() streamingProvider {
	return streamingProvider{
		fakeProvider: &fakeProvider{routes: RouteSet{
			RouteNative, RouteGroup, RouteConnect, RouteStream,
		}},
		body:        io.NopCloser(strings.NewReader("audio")),
		contentType: "audio/flac",
	}
}

func testItem() Item {
	return Item{Provider: "fake", Kind: KindTrack, URI: "spotify:track:x", Title: "Test Track"}
}

func TestPlayNativeRoute(t *testing.T) {
	sp := sonosRec("Living Room")
	plan, err := Resolve(newStreamingProvider(), []Endpoint{sp})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Route != RouteNative {
		t.Fatalf("route = %q, want native", plan.Route)
	}
	sess, err := Play(t.Context(), plan, newStreamingProvider(), testItem(), Deps{})
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	defer sess.Close()

	if !sp.did("service_account") {
		t.Error("native play must resolve the speaker's service account")
	}
	if !sp.did("play_native") {
		t.Error("native play must hand content to the speaker")
	}
	// The stream route must not have been touched at all — this is the
	// no-regression guarantee at the execution level rather than the
	// planning level.
	if sp.did("play_uri") {
		t.Error("a native play must never fall back to a stream URL")
	}
}

func TestPlayGroupRoute(t *testing.T) {
	a, b, c := sonosRec("A"), sonosRec("B"), sonosRec("C")
	p := newStreamingProvider()
	plan, err := Resolve(p, []Endpoint{a, b, c})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Route != RouteGroup {
		t.Fatalf("route = %q, want group", plan.Route)
	}
	sess, err := Play(t.Context(), plan, p, testItem(), Deps{})
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	defer sess.Close()

	for _, f := range []*recorder{b, c} {
		if !f.did("join") {
			t.Errorf("%s should have joined the group", f.desc.Name)
		}
		if f.joined != "A" {
			t.Errorf("%s joined %q, want A", f.desc.Name, f.joined)
		}
		// Followers must not each be handed the content — only the
		// coordinator plays, and the group follows it.
		if f.did("play_native") {
			t.Errorf("%s was handed content directly; only the coordinator should be", f.desc.Name)
		}
	}
	if !a.did("play_native") {
		t.Error("the coordinator should have been handed the content")
	}
}

// TestPlayGroupFreesCoordinator covers the case where the speaker chosen to
// lead is currently following somebody else. Grouping onto it without
// breaking it out first would arrange the zone behind a speaker not in it.
func TestPlayGroupFreesCoordinator(t *testing.T) {
	a, b := sonosRec("A"), sonosRec("B")
	a.coordinator = "RINCON_SOMEONE_ELSE"

	p := newStreamingProvider()
	plan, err := Resolve(p, []Endpoint{a, b})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := Play(t.Context(), plan, p, testItem(), Deps{}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	if !a.did("leave") {
		t.Error("a coordinator that was following someone else must leave that group first")
	}
}

// TestPlayGroupKeepsStandaloneCoordinator is the other half: a speaker that
// already leads must not be made to leave, which would interrupt playback for
// no reason.
func TestPlayGroupKeepsStandaloneCoordinator(t *testing.T) {
	a, b := sonosRec("A"), sonosRec("B")
	a.coordinator = "" // stands alone

	p := newStreamingProvider()
	plan, _ := Resolve(p, []Endpoint{a, b})
	if _, err := Play(t.Context(), plan, p, testItem(), Deps{}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	if a.did("leave") {
		t.Error("a coordinator that already stands alone should not be told to leave")
	}
}

func TestPlayStreamRoute(t *testing.T) {
	sp, kf := sonosRec("Living Room"), kefRec("Study")
	p := newStreamingProvider()
	host := &fakeHost{url: "http://homehub.local:8080/stream/abc"}

	plan, err := Resolve(p, []Endpoint{sp, kf})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Route != RouteStream {
		t.Fatalf("route = %q, want stream", plan.Route)
	}

	sess, err := Play(t.Context(), plan, p, testItem(), Deps{Stream: host})
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	defer sess.Close()

	if sess.URL != host.url {
		t.Errorf("session URL = %q, want %q", sess.URL, host.url)
	}
	for _, e := range []*recorder{sp, kf} {
		if !e.did("play_uri") {
			t.Errorf("%s was never pointed at the stream", e.desc.Name)
		}
		if e.playedURI != host.url {
			t.Errorf("%s got URI %q, want %q", e.desc.Name, e.playedURI, host.url)
		}
		// The content type has to reach the speaker: Sonos picks a decoder
		// from the metadata before fetching a byte.
		if e.playedMeta.ContentType != "audio/flac" {
			t.Errorf("%s got content type %q, want audio/flac",
				e.desc.Name, e.playedMeta.ContentType)
		}
		if !e.playedMeta.Live {
			t.Errorf("%s was not told the stream is live", e.desc.Name)
		}
		if e.playedMeta.Title != "Test Track" {
			t.Errorf("%s got title %q, want the stream's", e.desc.Name, e.playedMeta.Title)
		}
	}
	// The KEF has to be woken: it isn't on the network otherwise.
	if !kf.did("wake") {
		t.Error("the KEF should have been woken before being handed the stream")
	}
	if sp.did("wake") {
		t.Error("a Sonos has no standby to wake from and should not be asked")
	}
}

// TestPlayStreamTearsDownOnPartialFailure is the rule that a half-playing
// zone is worse than a silent one: if any speaker refuses, the whole session
// is released so the user gets one clear error rather than music in part of
// the room.
func TestPlayStreamTearsDownOnPartialFailure(t *testing.T) {
	sp, kf := sonosRec("Living Room"), kefRec("Study")
	kf.fail["play_uri"] = errors.New("renderer refused")

	p := newStreamingProvider()
	host := &fakeHost{url: "http://homehub.local:8080/stream/abc"}
	plan, _ := Resolve(p, []Endpoint{sp, kf})

	sess, err := Play(t.Context(), plan, p, testItem(), Deps{Stream: host})
	if err == nil {
		t.Fatal("expected the play to fail when a speaker refuses")
	}
	if sess != nil {
		t.Error("no session should be returned when the play failed")
	}
	if host.stops() != 1 {
		t.Errorf("stream was stopped %d times, want exactly 1 — a failed play must release it",
			host.stops())
	}
	if !strings.Contains(err.Error(), "Study") {
		t.Errorf("error should name the speaker that refused: %v", err)
	}
}

// TestPlayStreamRequiresHost guards the wiring bug: a stream plan with no
// host configured must report that plainly rather than panic.
func TestPlayStreamRequiresHost(t *testing.T) {
	p := newStreamingProvider()
	plan, _ := Resolve(p, []Endpoint{sonosRec("A"), kefRec("B")})
	if _, err := Play(t.Context(), plan, p, testItem(), Deps{}); err == nil {
		t.Fatal("expected an error when no stream host is configured")
	}
}

// TestPlayWakeFailureAborts checks that a speaker that can't be woken stops
// the whole play. Music in some of the room is more confusing than an error.
func TestPlayWakeFailureAborts(t *testing.T) {
	kf := kefRec("Study")
	kf.fail["wake"] = errors.New("speaker unreachable")
	sp := sonosRec("Living Room")

	p := newStreamingProvider()
	host := &fakeHost{url: "http://h/s"}
	plan, _ := Resolve(p, []Endpoint{sp, kf})

	if _, err := Play(t.Context(), plan, p, testItem(), Deps{Stream: host}); err == nil {
		t.Fatal("expected the play to fail when a speaker can't be woken")
	}
	if sp.did("play_uri") {
		t.Error("no speaker should have been started after a wake failure")
	}
	if host.stops() != 0 {
		t.Error("the stream should not have been opened at all")
	}
}

func TestSessionCloseIsIdempotent(t *testing.T) {
	host := &fakeHost{url: "http://h/s"}
	p := newStreamingProvider()
	plan, _ := Resolve(p, []Endpoint{sonosRec("A"), kefRec("B")})

	sess, err := Play(t.Context(), plan, p, testItem(), Deps{Stream: host})
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	sess.Close()
	sess.Close()
	sess.Close()
	if host.stops() != 1 {
		t.Errorf("stream stopped %d times across three Closes, want 1", host.stops())
	}
	// And a session that owns nothing must survive Close too.
	(&Session{}).Close()
	var nilSess *Session
	nilSess.Close()
}

// TestControlTargetsCoordinator covers the routing of transport commands: a
// native group has one speaker that speaks for all of them, and addressing
// each member would be redundant and racy. A streamed zone has no coordinator,
// so every speaker gets the command.
func TestControlTargetsCoordinator(t *testing.T) {
	t.Run("group addresses only the coordinator", func(t *testing.T) {
		a, b := sonosRec("A"), sonosRec("B")
		p := newStreamingProvider()
		plan, _ := Resolve(p, []Endpoint{a, b})
		if plan.Route != RouteGroup {
			t.Fatalf("route = %q, want group", plan.Route)
		}
		if err := Control(t.Context(), plan, TransportPause); err != nil {
			t.Fatalf("Control: %v", err)
		}
		if !a.did("pause") {
			t.Error("the coordinator should have been paused")
		}
		if b.did("pause") {
			t.Error("a follower should not be paused individually")
		}
	})

	t.Run("stream addresses every speaker", func(t *testing.T) {
		sp, kf := sonosRec("Living Room"), kefRec("Study")
		p := newStreamingProvider()
		plan, _ := Resolve(p, []Endpoint{sp, kf})
		if plan.Route != RouteStream {
			t.Fatalf("route = %q, want stream", plan.Route)
		}
		if err := Control(t.Context(), plan, TransportPause); err != nil {
			t.Fatalf("Control: %v", err)
		}
		for _, e := range []*recorder{sp, kf} {
			if !e.did("pause") {
				t.Errorf("%s should have been paused: a streamed zone has no coordinator",
					e.desc.Name)
			}
		}
	})
}

func TestSetVolumeClamps(t *testing.T) {
	a := sonosRec("A")
	for _, level := range []int{-10, 0, 50, 100, 500} {
		if err := SetVolume(t.Context(), []Endpoint{a}, level); err != nil {
			t.Fatalf("SetVolume(%d): %v", level, err)
		}
	}
	// Every call must have reached the speaker; clamping happens before the
	// send, not by skipping it.
	got := 0
	for _, c := range a.calls() {
		if c == "volume" {
			got++
		}
	}
	if got != 5 {
		t.Errorf("volume sent %d times, want 5", got)
	}
}

func TestSetVolumeEmptyZone(t *testing.T) {
	if err := SetVolume(t.Context(), nil, 50); !errors.Is(err, ErrEmptyZone) {
		t.Errorf("want ErrEmptyZone, got %v", err)
	}
}

// TestStatesToleratesUnreachable checks that one dead speaker doesn't take
// the whole zone's state reading with it.
func TestStatesToleratesUnreachable(t *testing.T) {
	a, b := sonosRec("A"), sonosRec("B")
	b.fail["state"] = errors.New("unreachable")

	states := States(t.Context(), []Endpoint{a, b})
	if len(states) != 2 {
		t.Fatalf("got %d states, want an entry per speaker", len(states))
	}
	if states["a"] == nil {
		t.Error("the reachable speaker should have reported its state")
	}
	if states["b"] != nil {
		t.Error("the unreachable speaker should report nil, not a fabricated state")
	}
}

// TestFanOutWaitsForAll checks that a failing endpoint doesn't cause an early
// return while commands are still in flight against the others.
func TestFanOutWaitsForAll(t *testing.T) {
	a, b, c := sonosRec("A"), sonosRec("B"), sonosRec("C")
	b.fail["pause"] = errors.New("nope")

	err := fanOut(t.Context(), []Endpoint{a, b, c}, func(ctx context.Context, e Endpoint) error {
		return e.Pause(ctx)
	})
	if err == nil {
		t.Fatal("expected the failure to surface")
	}
	for _, e := range []*recorder{a, b, c} {
		if !e.did("pause") {
			t.Errorf("%s never received the command; fanOut returned before it finished",
				e.desc.Name)
		}
	}
	if !strings.Contains(err.Error(), "B") {
		t.Errorf("error should name the failing speaker: %v", err)
	}
}
