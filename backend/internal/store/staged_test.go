package store

import (
	"errors"
	"sync"
	"testing"
)

// recRF records every Send and can fail selected codes.
type recRF struct {
	mu    sync.Mutex
	sent  []string // "code:state"
	fail  map[string]bool
	calls int
}

func (r *recRF) Send(code, protocol string, state bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	key := code
	if state {
		key += ":on"
	} else {
		key += ":off"
	}
	r.sent = append(r.sent, key)
	if r.fail[code] {
		return errors.New("device unreachable")
	}
	return nil
}

func stagedStore(rf RFSender) *Store {
	return &Store{
		Sockets: map[string]*Socket{
			"a": {ID: "a", Name: "Lamp A", Code: "1:0", Protocol: "nexa", State: false},
			"b": {ID: "b", Name: "Lamp B", Code: "10.0.0.2", Protocol: "tasmota", State: true},
		},
		Groups: map[string]*Group{
			"g": {ID: "g", SocketIDs: []string{"a", "b", "gone"}},
		},
		Rooms:  map[string]*Room{},
		Scenes: map[string]*Scene{},
		RF:     rf,
	}
}

func TestStageActionGroupResolvesToggleAndMissing(t *testing.T) {
	s := stagedStore(&recRF{})
	s.Mu.Lock()
	staged, err := s.StageAction("group", "g", "toggle")
	s.Mu.Unlock()
	if err != nil {
		t.Fatalf("StageAction: %v", err)
	}
	if len(staged) != 3 {
		t.Fatalf("staged %d sends, want 3", len(staged))
	}
	// Toggle resolves against the state at stage time.
	if staged[0].SocketID != "a" || staged[0].State != true {
		t.Errorf("socket a staged %+v, want desired state true", staged[0])
	}
	if staged[1].SocketID != "b" || staged[1].State != false {
		t.Errorf("socket b staged %+v, want desired state false", staged[1])
	}
	// Missing member becomes an errored entry, not a dropped one.
	if staged[2].SocketID != "gone" || staged[2].Err == nil {
		t.Errorf("missing socket staged %+v, want pre-set Err", staged[2])
	}
}

func TestSendAndApplyStaged(t *testing.T) {
	rf := &recRF{fail: map[string]bool{"10.0.0.2": true}}
	s := stagedStore(rf)
	var changes []string
	s.OnStateChange = func(sock Socket, newState bool) {
		changes = append(changes, sock.ID)
	}

	s.Mu.Lock()
	staged, err := s.StageAction("group", "g", "toggle")
	s.Mu.Unlock()
	if err != nil {
		t.Fatalf("StageAction: %v", err)
	}

	s.SendStaged(staged)
	if rf.calls != 2 {
		t.Fatalf("RF.Send called %d times, want 2 (errored entry skipped)", rf.calls)
	}

	s.Mu.Lock()
	firstErr := s.ApplyStaged(staged)
	s.Mu.Unlock()
	if firstErr == nil {
		t.Fatal("ApplyStaged returned nil, want first failure")
	}
	// a succeeded → state applied; b's send failed → state untouched.
	if !s.Sockets["a"].State {
		t.Error("socket a state not applied")
	}
	if !s.Sockets["b"].State {
		t.Error("socket b state changed despite send failure")
	}
	if len(changes) != 1 || changes[0] != "a" {
		t.Errorf("OnStateChange fired for %v, want [a]", changes)
	}
}

func TestApplyStagedSkipsDeletedSocket(t *testing.T) {
	s := stagedStore(&recRF{})
	s.Mu.Lock()
	staged, _ := s.StageAction("socket", "a", "on")
	s.Mu.Unlock()

	s.SendStaged(staged)

	// Socket deleted while the send was in flight.
	s.Mu.Lock()
	delete(s.Sockets, "a")
	if err := s.ApplyStaged(staged); err != nil {
		t.Errorf("ApplyStaged: %v", err)
	}
	s.Mu.Unlock()
}

// blockingRF blocks every Send until released, to prove SendStaged runs
// without holding Mu.
type blockingRF struct {
	release chan struct{}
}

func (b *blockingRF) Send(code, protocol string, state bool) error {
	<-b.release
	return nil
}

func TestSendStagedDoesNotHoldMu(t *testing.T) {
	rf := &blockingRF{release: make(chan struct{})}
	s := stagedStore(rf)

	s.Mu.Lock()
	staged, _ := s.StageAction("socket", "b", "off") // tasmota → network path
	s.Mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.SendStaged(staged)
		close(done)
	}()

	// While the send is blocked, the store lock must remain available.
	locked := make(chan struct{})
	go func() {
		s.Mu.Lock()
		s.Mu.Unlock() //nolint:staticcheck // probing lock availability
		close(locked)
	}()
	select {
	case <-locked:
	case <-done:
		t.Fatal("send finished before release — blockingRF broken")
	}

	close(rf.release)
	<-done
}

func TestStageSceneQueuesLightsAndDelayedSteps(t *testing.T) {
	lvl := 40
	s := stagedStore(&recRF{})
	s.Scenes["sc"] = &Scene{ID: "sc", Steps: []SceneStep{
		{Actions: []SceneAction{{SocketID: "b", Action: "on", Level: &lvl}}},
	}}

	s.Mu.Lock()
	staged, err := s.StageAction("scene", "sc", "activate")
	if err != nil {
		s.Mu.Unlock()
		t.Fatalf("StageAction: %v", err)
	}
	if len(staged) != 1 || staged[0].SocketID != "b" || !staged[0].State {
		t.Fatalf("staged %+v, want one 'on' send for b", staged)
	}
	if len(s.pendingLights) != 1 || s.pendingLights[0].socket.ID != "b" {
		t.Fatalf("pendingLights %+v, want one entry for b", s.pendingLights)
	}
	s.Mu.Unlock()
}
