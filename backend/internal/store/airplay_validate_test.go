package store

import (
	"strings"
	"testing"
)

func TestValidateAirPlaySpeaker(t *testing.T) {
	t.Run("normalises and defaults", func(t *testing.T) {
		s := New(t.TempDir(), nil)
		sp := &AirPlaySpeaker{
			Name: "  Study Pi  ", IP: " 192.168.1.42 ", Room: " Study ",
			DeviceID: "B8:27:EB:12:34:AB",
		}
		if err := s.ValidateAirPlaySpeaker(sp); err != nil {
			t.Fatalf("validate: %v", err)
		}
		if sp.Name != "Study Pi" || sp.IP != "192.168.1.42" || sp.Room != "Study" {
			t.Errorf("not trimmed: %+v", sp)
		}
		// Stored normalised so the same box read through two firmware
		// versions is still one device.
		if sp.DeviceID != "b827eb1234ab" {
			t.Errorf("device id = %q", sp.DeviceID)
		}
		if sp.Port != 7000 {
			t.Errorf("port = %d, want the RAOP default", sp.Port)
		}
		// A registration with no codec flags is a client that didn't fill
		// them in, not a receiver that accepts nothing.
		if !sp.ALAC {
			t.Error("every RAOP receiver takes ALAC; that should be the fallback")
		}
	})

	t.Run("requires a name and a safe address", func(t *testing.T) {
		s := New(t.TempDir(), nil)
		if err := s.ValidateAirPlaySpeaker(&AirPlaySpeaker{IP: "192.168.1.42"}); err == nil {
			t.Error("a nameless receiver should be refused")
		}
		// The same host policy the other bridges use: nothing that could
		// redirect a server-side request somewhere else.
		if err := s.ValidateAirPlaySpeaker(&AirPlaySpeaker{
			Name: "Evil", IP: "127.0.0.1",
		}); err == nil {
			t.Error("loopback should be refused")
		}
		if err := s.ValidateAirPlaySpeaker(&AirPlaySpeaker{
			Name: "Evil", IP: "192.168.1.42/../x",
		}); err == nil {
			t.Error("an address with a path should be refused")
		}
	})

	t.Run("bounds the port and the volume", func(t *testing.T) {
		s := New(t.TempDir(), nil)
		if err := s.ValidateAirPlaySpeaker(&AirPlaySpeaker{
			Name: "Pi", IP: "192.168.1.42", Port: 70000,
		}); err == nil {
			t.Error("an impossible port should be refused")
		}
		if err := s.ValidateAirPlaySpeaker(&AirPlaySpeaker{
			Name: "Pi", IP: "192.168.1.42", Volume: 150,
		}); err == nil {
			t.Error("a volume above 100 should be refused")
		}
	})

	t.Run("rejects a duplicate device", func(t *testing.T) {
		s := New(t.TempDir(), nil)
		s.AirPlay["a"] = &AirPlaySpeaker{
			ID: "a", Name: "Study", IP: "192.168.1.42", DeviceID: "b827eb1234ab",
		}
		err := s.ValidateAirPlaySpeaker(&AirPlaySpeaker{
			ID: "b", Name: "Kitchen", IP: "192.168.1.43", DeviceID: "B827EB1234AB",
		})
		if err == nil || !strings.Contains(err.Error(), "already registered") {
			t.Errorf("the same box under two names should be refused, got %v", err)
		}
	})
}

// A zone may hold receivers alongside anything else — validation's job is only
// that the members exist. Whether they can actually play together is the route
// engine's answer, given per playback.
func TestZonesAcceptAirPlayMembers(t *testing.T) {
	s := New(t.TempDir(), nil)
	s.AirPlay["a1"] = &AirPlaySpeaker{ID: "a1", Name: "Study Pi", IP: "192.168.1.42"}
	s.Sonos["s1"] = &SonosSpeaker{ID: "s1", Name: "Living", IP: "192.168.1.10"}

	z := &Zone{Name: "Everywhere", Members: []string{
		QualifyAirPlay("a1"), QualifySonos("s1"),
	}}
	if err := s.ValidateZone(z); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(z.Members) != 2 {
		t.Errorf("members = %v", z.Members)
	}

	if err := s.ValidateZone(&Zone{Name: "Ghost", Members: []string{QualifyAirPlay("nope")}}); err == nil {
		t.Error("a member that doesn't exist should be refused")
	}

	// And a deleted receiver leaves no dangling member behind, or the next
	// unrelated edit of the zone would fail with "no such receiver".
	s.CascadeDeleteSpeaker(QualifyAirPlay("a1"))
	if got := s.Zones; len(got) == 0 {
		s.Zones["z"] = z
	}
	s.CascadeDeleteSpeaker(QualifyAirPlay("a1"))
	for _, m := range s.Zones["z"].Members {
		if m == QualifyAirPlay("a1") {
			t.Error("a deleted receiver should be dropped from its zones")
		}
	}
}

func TestSplitMemberKnowsAirPlay(t *testing.T) {
	bridge, id, ok := SplitMember("airplay:a1")
	if !ok || bridge != "airplay" || id != "a1" {
		t.Errorf("got %q/%q/%v", bridge, id, ok)
	}
	if _, _, ok := SplitMember("a1"); ok {
		t.Error("an unqualified member must not be silently misread")
	}
}

func TestSettingsBoundStreamQuality(t *testing.T) {
	s := New(t.TempDir(), nil)

	set := &Settings{StreamQuality: " BEST "}
	if err := s.ValidateSettings(set); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if set.StreamQuality != "best" {
		t.Errorf("stream quality = %q, want normalised", set.StreamQuality)
	}

	// Blank is "never chosen", which is a real state and must survive.
	set = &Settings{}
	if err := s.ValidateSettings(set); err != nil || set.StreamQuality != "" {
		t.Errorf("blank should stay blank: %q, %v", set.StreamQuality, err)
	}

	// Anything else is a client inventing a value, and is refused rather
	// than silently defaulted — the UI offers three choices.
	if err := s.ValidateSettings(&Settings{StreamQuality: "lossless"}); err == nil {
		t.Error("an unknown quality should be refused")
	}
}
