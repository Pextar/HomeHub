package speakermon

import (
	"strings"
	"testing"

	"homehub/internal/store"
)

func testMonitors(t *testing.T, port string) *Monitors {
	t.Helper()
	return New(Config{
		Store:     store.New(t.TempDir(), nil),
		HTTPPort:  port,
		EventPath: "/sonos/event",
	})
}

// A speaker can only reach us on the address that faces its own subnet, and
// only over plain HTTP — it will not post to a self-signed TLS listener.
func TestCallbackURL(t *testing.T) {
	got, err := testMonitors(t, "8080").CallbackURL("192.168.1.42")
	if err != nil {
		t.Skipf("no route to a LAN address in this environment: %v", err)
	}
	if !strings.HasPrefix(got, "http://") {
		t.Errorf("callback = %q, want plain http — speakers will not post to TLS", got)
	}
	if !strings.HasSuffix(got, ":8080/sonos/event") {
		t.Errorf("callback = %q, want it to end in :8080/sonos/event", got)
	}
	// An address we listen on but no speaker can route to is the failure
	// this whole lookup exists to avoid.
	if strings.Contains(got, "0.0.0.0") || strings.Contains(got, "127.0.0.1") {
		t.Errorf("callback = %q, want a routable address", got)
	}
}

func TestCallbackURLDefaultsThePort(t *testing.T) {
	got, err := testMonitors(t, "").CallbackURL("192.168.1.42")
	if err != nil {
		t.Skipf("no route to a LAN address in this environment: %v", err)
	}
	if !strings.HasSuffix(got, ":8080/sonos/event") {
		t.Errorf("callback = %q, want the default :8080", got)
	}
}

// Building the monitors must not reach a speaker or start a goroutine: the
// composition root builds them before anything is listening, and a house with
// no speakers registered should stay silent.
func TestNewStartsNothing(t *testing.T) {
	m := testMonitors(t, "8080")
	if m.Sonos == nil || m.KEF == nil {
		t.Fatal("New should build both monitors")
	}
	if got := m.Sonos.Cached(); len(got.Speakers) != 0 {
		t.Errorf("cached %d speakers before Run, want none", len(got.Speakers))
	}
	if got := m.KEF.Cached(); len(got.Speakers) != 0 {
		t.Errorf("cached %d speakers before Run, want none", len(got.Speakers))
	}
}
