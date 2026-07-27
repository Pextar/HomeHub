package store

import (
	"strings"
	"testing"
)

// Sonos and KEF share one uniqueness rule (uniqueSpeaker) but identify a
// device differently — RINCON uuid vs MAC. These cover both bridges through
// the rule they share, since the wording of a conflict is what users see
// when they add a speaker twice.

func sonosStore(t *testing.T) *Store {
	t.Helper()
	return New(t.TempDir(), nil)
}

func TestValidateSonosSpeaker(t *testing.T) {
	t.Run("a name is required", func(t *testing.T) {
		s := sonosStore(t)
		err := s.ValidateSonosSpeaker(&SonosSpeaker{IP: "192.168.1.10"})
		if err == nil || err.Error() != "name is required" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("the address is checked", func(t *testing.T) {
		s := sonosStore(t)
		if err := s.ValidateSonosSpeaker(&SonosSpeaker{Name: "Lounge", IP: "127.0.0.1"}); err == nil {
			t.Error("loopback was accepted")
		}
	})

	t.Run("fields are trimmed", func(t *testing.T) {
		s := sonosStore(t)
		sp := &SonosSpeaker{Name: "  Lounge  ", IP: " 192.168.1.10 ", Room: " Hall "}
		if err := s.ValidateSonosSpeaker(sp); err != nil {
			t.Fatalf("err = %v", err)
		}
		if sp.Name != "Lounge" || sp.IP != "192.168.1.10" || sp.Room != "Hall" {
			t.Errorf("not trimmed: %+v", sp)
		}
	})

	t.Run("a uuid must look like a Sonos identifier", func(t *testing.T) {
		s := sonosStore(t)
		err := s.ValidateSonosSpeaker(&SonosSpeaker{Name: "Lounge", IP: "192.168.1.10", UUID: "nope"})
		if err == nil || !strings.Contains(err.Error(), "RINCON") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("a second speaker cannot reuse an address", func(t *testing.T) {
		s := sonosStore(t)
		s.Sonos["a"] = &SonosSpeaker{ID: "a", Name: "Lounge", IP: "192.168.1.10"}
		err := s.ValidateSonosSpeaker(&SonosSpeaker{ID: "b", Name: "Study", IP: "192.168.1.10"})
		if err == nil || !strings.Contains(err.Error(), "already uses address") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("a second speaker cannot reuse a uuid", func(t *testing.T) {
		s := sonosStore(t)
		s.Sonos["a"] = &SonosSpeaker{ID: "a", Name: "Lounge", IP: "192.168.1.10", UUID: "RINCON_1"}
		err := s.ValidateSonosSpeaker(&SonosSpeaker{
			ID: "b", Name: "Study", IP: "192.168.1.11", UUID: "RINCON_1",
		})
		if err == nil || !strings.Contains(err.Error(), "same device id") {
			t.Errorf("err = %v", err)
		}
	})

	// Editing a speaker in place must not collide with itself.
	t.Run("a speaker may keep its own address", func(t *testing.T) {
		s := sonosStore(t)
		s.Sonos["a"] = &SonosSpeaker{ID: "a", Name: "Lounge", IP: "192.168.1.10", UUID: "RINCON_1"}
		err := s.ValidateSonosSpeaker(&SonosSpeaker{
			ID: "a", Name: "Renamed", IP: "192.168.1.10", UUID: "RINCON_1",
		})
		if err != nil {
			t.Errorf("err = %v", err)
		}
	})

	// An empty device id is "unknown", not a value to match on, or every
	// speaker discovered without a uuid would collide with the last one.
	t.Run("blank uuids do not collide", func(t *testing.T) {
		s := sonosStore(t)
		s.Sonos["a"] = &SonosSpeaker{ID: "a", Name: "Lounge", IP: "192.168.1.10"}
		if err := s.ValidateSonosSpeaker(&SonosSpeaker{
			ID: "b", Name: "Study", IP: "192.168.1.11",
		}); err != nil {
			t.Errorf("err = %v", err)
		}
	})
}

func TestValidateKEFSpeaker(t *testing.T) {
	t.Run("the MAC is normalised before comparison", func(t *testing.T) {
		s := sonosStore(t)
		s.KEF["a"] = &KEFSpeaker{ID: "a", Name: "Study", IP: "192.168.1.20", MAC: "a1b2c3d4e5f6"}
		// Same device, colon-separated by a different firmware version.
		err := s.ValidateKEFSpeaker(&KEFSpeaker{
			ID: "b", Name: "Study 2", IP: "192.168.1.21", MAC: "A1:B2:C3:D4:E5:F6",
		})
		if err == nil || !strings.Contains(err.Error(), "same device id") {
			t.Errorf("err = %v, want the normalised MAC to collide", err)
		}
	})

	t.Run("a second speaker cannot reuse an address", func(t *testing.T) {
		s := sonosStore(t)
		s.KEF["a"] = &KEFSpeaker{ID: "a", Name: "Study", IP: "192.168.1.20"}
		err := s.ValidateKEFSpeaker(&KEFSpeaker{ID: "b", Name: "Other", IP: "192.168.1.20"})
		if err == nil || !strings.Contains(err.Error(), "already uses address") {
			t.Errorf("err = %v", err)
		}
	})

	// These come back from Spotify's API rather than being typed, so the
	// only check is a length cap.
	t.Run("oversized spotify ids are rejected", func(t *testing.T) {
		s := sonosStore(t)
		err := s.ValidateKEFSpeaker(&KEFSpeaker{
			Name: "Study", IP: "192.168.1.20",
			SpotifyDeviceID: strings.Repeat("x", 129),
		})
		if err == nil || !strings.Contains(err.Error(), "Spotify") {
			t.Errorf("err = %v", err)
		}
	})

	// The two registries are independent: a KEF may sit at the same address
	// as a Sonos as far as this check is concerned.
	t.Run("the two bridges have separate registries", func(t *testing.T) {
		s := sonosStore(t)
		s.Sonos["a"] = &SonosSpeaker{ID: "a", Name: "Lounge", IP: "192.168.1.10"}
		if err := s.ValidateKEFSpeaker(&KEFSpeaker{
			ID: "k", Name: "Study", IP: "192.168.1.10",
		}); err != nil {
			t.Errorf("err = %v", err)
		}
	})
}
