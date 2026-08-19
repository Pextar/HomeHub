package lanaddr

import (
	"strings"
	"testing"
)

// Garbage in has to fail rather than resolve to something plausible. The
// callers use the result to tell a speaker where to fetch from, and a wrong
// address is a silence that looks like a broken speaker.
func TestForRejectsUnroutable(t *testing.T) {
	if _, err := For("not an ip"); err == nil {
		t.Error("For(garbage) = nil, want an error")
	}
}

func TestBaseURLReportsWhyItCouldNotResolve(t *testing.T) {
	_, err := BaseURL("not an ip", "8080")
	if err == nil {
		t.Fatal("BaseURL(garbage) = nil, want an error")
	}
	// The message is read by someone staring at a speaker that won't play,
	// so it has to name the address that couldn't be reached.
	if want := "not an ip"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should name %q", err, want)
	}
}

// Loopback is always routable, so it is the one address a test can assert a
// real answer for on any machine.
func TestBaseURLUsesTheGivenPort(t *testing.T) {
	got, err := BaseURL("127.0.0.1", "9999")
	if err != nil {
		t.Fatalf("BaseURL(loopback) = %v", err)
	}
	if want := "http://127.0.0.1:9999"; got != want {
		t.Errorf("BaseURL = %q, want %q", got, want)
	}
}

func TestBaseURLDefaultsThePort(t *testing.T) {
	got, err := BaseURL("127.0.0.1", "")
	if err != nil {
		t.Fatalf("BaseURL(loopback) = %v", err)
	}
	if want := "http://127.0.0.1:8080"; got != want {
		t.Errorf("BaseURL = %q, want %q", got, want)
	}
}
