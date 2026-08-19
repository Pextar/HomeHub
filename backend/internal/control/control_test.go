package control

import (
	"strings"
	"sync"
	"testing"

	"homehub/internal/store"
)

// recordRF stands in for the radio: it accepts everything and remembers what
// it was asked to send, which is all the action layer's contract needs.
type recordRF struct {
	mu    sync.Mutex
	sends []string
}

func (r *recordRF) Send(code, protocol string, state bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sends = append(r.sends, code)
	return nil
}

func (r *recordRF) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sends)
}

func testActions(t *testing.T) (*Actions, *store.Store, *recordRF) {
	t.Helper()
	rf := &recordRF{}
	st := store.New(t.TempDir(), rf)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	st.Sockets["s1"] = &store.Socket{ID: "s1", Name: "Lamp", Code: "1001", Protocol: "nexa", Room: "Lounge"}
	st.Sockets["s2"] = &store.Socket{ID: "s2", Name: "Fan", Code: "1002", Protocol: "nexa", Room: "Lounge"}
	st.Sockets["s3"] = &store.Socket{ID: "s3", Name: "Desk", Code: "1003", Protocol: "nexa", Room: "Study"}
	st.Groups["g1"] = &store.Group{ID: "g1", Name: "Downstairs", SocketIDs: []string{"s1", "s2"}}
	return New(Config{Store: st}), st, rf
}

// ── One socket ───────────────────────────────────────────────────────────

func TestSocketSwitchesAndReportsTheNewState(t *testing.T) {
	a, _, rf := testActions(t)

	sock, found, err := a.Socket("s1", "on", SourceManual)
	if err != nil || !found {
		t.Fatalf("Socket(on) = found %v, err %v", found, err)
	}
	if !sock.State {
		t.Error("the socket reports itself off after being turned on")
	}
	if rf.count() != 1 {
		t.Errorf("%d transmissions, want 1", rf.count())
	}
}

func TestSocketTogglesFromWhereverItWas(t *testing.T) {
	a, st, _ := testActions(t)
	st.Sockets["s1"].State = true

	sock, _, err := a.Socket("s1", "toggle", SourceManual)
	if err != nil {
		t.Fatalf("Socket(toggle) = %v", err)
	}
	if sock.State {
		t.Error("toggling an on socket left it on")
	}
}

// A missing socket is "not found", not an error: the caller turns that into a
// 404, and an error would be reported as a device failure instead.
func TestSocketReportsAMissingIDAsNotFound(t *testing.T) {
	a, _, _ := testActions(t)
	if _, found, err := a.Socket("nope", "on", SourceManual); found || err != nil {
		t.Errorf("Socket(unknown) = found %v, err %v, want false and no error", found, err)
	}
}

// An unsupported verb is the caller's mistake and names the three that work,
// because the caller may be a language model choosing from a schema.
func TestSocketRejectsAnUnknownAction(t *testing.T) {
	a, _, _ := testActions(t)
	_, _, err := a.Socket("s1", "dim", SourceManual)
	if err == nil {
		t.Fatal("Socket(dim) = nil, want an error")
	}
	for _, want := range []string{"on", "off", "toggle"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name %q", err, want)
		}
	}
}

// ── Whole rooms and the whole house ──────────────────────────────────────

// Switching an empty set is a success. Nobody asked about a particular thing,
// so there is nothing that could be missing.
func TestAllOnAnEmptyHouseIsFound(t *testing.T) {
	rf := &recordRF{}
	st := store.New(t.TempDir(), rf)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	res, err := New(Config{Store: st}).All(nil, true, SourceManual)
	if err != nil || !res.Found {
		t.Errorf("All on an empty house = found %v, err %v, want a success", res.Found, err)
	}
}

// A room nothing is in *is* a 404: the caller named something and the house
// has no such room.
func TestRoomWithNothingInItIsNotFound(t *testing.T) {
	a, _, _ := testActions(t)
	res, err := a.Room("Cellar", nil, true, SourceManual)
	if err != nil {
		t.Fatalf("Room = %v", err)
	}
	if res.Found {
		t.Error("an empty room reported itself found")
	}
}

func TestRoomSwitchesOnlyItsOwnSockets(t *testing.T) {
	a, st, _ := testActions(t)
	res, err := a.Room("Lounge", nil, true, SourceManual)
	if err != nil || !res.Found {
		t.Fatalf("Room(Lounge) = found %v, err %v", res.Found, err)
	}
	if res.OK != 2 {
		t.Errorf("switched %d sockets, want the two in the Lounge", res.OK)
	}
	if st.Sockets["s3"].State {
		t.Error("the Study socket was switched by a Lounge command")
	}
}

// The permission predicate is enforced here, not computed here: a caller that
// may reach one socket switches one socket.
func TestAllowNarrowsWhatIsSwitched(t *testing.T) {
	a, st, _ := testActions(t)
	only := func(id string) bool { return id == "s1" }

	res, err := a.All(only, true, SourceManual)
	if err != nil {
		t.Fatalf("All = %v", err)
	}
	if res.OK != 1 {
		t.Errorf("switched %d sockets, want the one allowed", res.OK)
	}
	if st.Sockets["s2"].State || st.Sockets["s3"].State {
		t.Error("a socket the caller may not reach was switched")
	}
}

// ── Groups ───────────────────────────────────────────────────────────────

func TestGroupSwitchesEveryMember(t *testing.T) {
	a, st, _ := testActions(t)
	res, err := a.Group("g1", "on", SourceManual)
	if err != nil || !res.Found {
		t.Fatalf("Group = found %v, err %v", res.Found, err)
	}
	if res.Label != "Downstairs" {
		t.Errorf("label = %q, want the group's name", res.Label)
	}
	if !st.Sockets["s1"].State || !st.Sockets["s2"].State {
		t.Error("not every member was switched")
	}
	if st.Sockets["s3"].State {
		t.Error("a non-member was switched")
	}
}

func TestGroupReportsAMissingIDAsNotFound(t *testing.T) {
	a, _, _ := testActions(t)
	res, err := a.Group("nope", "on", SourceManual)
	if res.Found || err != nil {
		t.Errorf("Group(unknown) = found %v, err %v", res.Found, err)
	}
}

// ── The activity log ─────────────────────────────────────────────────────

// A household reading back "everything went off at 23:40" wants to know
// whether a person did that or something in the house decided to, so who asked
// has to survive into the log.
func TestTheSourceReachesTheActivityLog(t *testing.T) {
	a, st, _ := testActions(t)

	if _, err := a.Group("g1", "on", SourceAssistant); err != nil {
		t.Fatalf("Group = %v", err)
	}
	entries := st.Activity.Recent(1)
	if len(entries) != 1 {
		t.Fatal("nothing was written to the activity log")
	}
	if entries[0].Source != string(SourceAssistant) {
		t.Errorf("source = %q, want %q", entries[0].Source, SourceAssistant)
	}
	if entries[0].Kind != "group" || entries[0].Action != "on" {
		t.Errorf("entry = %+v, want a group/on row", entries[0])
	}
}

// One summary per multi-device action, never one per socket: a household that
// turns the house off should get one notification, not nine.
func TestOneNotificationPerMultiDeviceAction(t *testing.T) {
	rf := &recordRF{}
	st := store.New(t.TempDir(), rf)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	st.Sockets["s1"] = &store.Socket{ID: "s1", Name: "Lamp", Code: "1001", Protocol: "nexa"}
	st.Sockets["s2"] = &store.Socket{ID: "s2", Name: "Fan", Code: "1002", Protocol: "nexa"}

	var titles []string
	a := New(Config{
		Store:  st,
		Notify: func(title string, _ int) { titles = append(titles, title) },
	})
	if _, err := a.All(nil, true, SourceManual); err != nil {
		t.Fatalf("All = %v", err)
	}
	if len(titles) != 1 {
		t.Fatalf("%d notifications for one bulk action, want 1: %v", len(titles), titles)
	}
	if titles[0] != "All devices turned on" {
		t.Errorf("title = %q", titles[0])
	}
}

// Nothing switched means nothing to announce.
func TestNoNotificationWhenNothingChanged(t *testing.T) {
	a, _, _ := testActions(t)
	var titles []string
	a.cfg.Notify = func(title string, _ int) { titles = append(titles, title) }

	if _, err := a.Room("Cellar", nil, true, SourceManual); err != nil {
		t.Fatalf("Room = %v", err)
	}
	if len(titles) != 0 {
		t.Errorf("notified %v for a room that does not exist", titles)
	}
}
