package media

import (
	"errors"
	"strings"
	"testing"
)

// fakeTarget is a ConnectTarget with settable hints.
type fakeTarget struct {
	id    string
	names []string
}

func (f fakeTarget) ConnectHint() (string, []string) { return f.id, f.names }

func TestMatchConnectDevice(t *testing.T) {
	devices := []ConnectDevice{
		{ID: "dev-1", Name: "Study"},
		{ID: "dev-2", Name: "Living  Room"},
		{ID: "dev-3", Name: "Car", Restricted: true},
	}

	t.Run("pinned id wins", func(t *testing.T) {
		got, err := MatchConnectDevice(fakeTarget{id: "dev-2", names: []string{"Study"}}, "Study", devices)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "dev-2" {
			t.Errorf("got %q, want dev-2 — a pin must beat a name match", got.ID)
		}
	})

	t.Run("name match folds whitespace and case", func(t *testing.T) {
		got, err := MatchConnectDevice(fakeTarget{names: []string{"living room"}}, "living room", devices)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "dev-2" {
			t.Errorf("got %q, want dev-2", got.ID)
		}
	})

	t.Run("falls back to the next name when the first misses", func(t *testing.T) {
		got, err := MatchConnectDevice(fakeTarget{names: []string{"Old Name", "Study"}}, "Study", devices)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "dev-1" {
			t.Errorf("got %q, want dev-1", got.ID)
		}
	})

	t.Run("a stale pin falls through to the name", func(t *testing.T) {
		// Spotify rotates device ids on re-registration, so a pin that no
		// longer resolves must not stop the name from matching.
		got, err := MatchConnectDevice(fakeTarget{id: "gone", names: []string{"Study"}}, "Study", devices)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "dev-1" {
			t.Errorf("got %q, want dev-1", got.ID)
		}
	})

	t.Run("restricted device is rejected with a reason", func(t *testing.T) {
		_, err := MatchConnectDevice(fakeTarget{names: []string{"Car"}}, "Car", devices)
		if err == nil {
			t.Fatal("expected a restricted device to be rejected")
		}
		if !errors.Is(err, ErrNoConnectDevice) {
			t.Errorf("want ErrNoConnectDevice, got %v", err)
		}
		if !strings.Contains(err.Error(), "Car") {
			t.Errorf("error should name the device: %v", err)
		}
	})

	t.Run("no match explains what to do", func(t *testing.T) {
		_, err := MatchConnectDevice(fakeTarget{names: []string{"Bedroom"}}, "Bedroom", devices)
		if err == nil {
			t.Fatal("expected no match")
		}
		if !strings.Contains(err.Error(), "Bedroom") {
			t.Errorf("error should name the speaker: %v", err)
		}
	})

	t.Run("nothing is guessed when there are no usable hints", func(t *testing.T) {
		// An endpoint with no pin and no names must not fall back to
		// "whatever device is first" — that plays in the wrong room.
		_, err := MatchConnectDevice(fakeTarget{names: []string{"", "  "}}, "Unnamed", devices)
		if err == nil {
			t.Fatal("expected no match when there is nothing to match on")
		}
	})
}
