package store

import (
	"strings"
	"testing"
)

// The read half of a rule's relationship with the music: a rule can now be
// triggered and gated by what a room is doing, not only drive it. What is
// tested here is what the store is responsible for — which rooms and words it
// will accept, and that a deleted speaker leaves nothing behind that watches
// it. The edge detection itself lives in internal/scheduler.

func dimBedroom() []AutomationAction {
	lvl := 2
	return []AutomationAction{{TargetType: "socket", TargetID: "lamp", Action: "on", Level: &lvl}}
}

// The whole point of the feature, end to end through validation: "when the
// music or the TV stops in the living room, set the bedroom light to 2%".
func TestValidateAutomationAcceptsAMusicTrigger(t *testing.T) {
	s := housed(t)
	s.Sockets["lamp"] = &Socket{ID: "lamp", Name: "Bedroom lamp", Protocol: "matter"}

	a := &Automation{
		Name: "Film over",
		Rules: []AutomationRule{{
			Trigger: AutomationTrigger{Type: "  Music ", Room: " kef:kf ", ToState: "STOPPED"},
			Actions: dimBedroom(),
		}},
	}
	if err := s.ValidateAutomation(a); err != nil {
		t.Fatalf("ValidateAutomation: %v", err)
	}
	tr := a.Rules[0].Trigger
	if tr.Type != "music" || tr.Room != "kef:kf" || tr.ToState != MusicStopped {
		t.Fatalf("trigger = %+v, want it normalised", tr)
	}
	if lvl := a.Rules[0].Actions[0].Level; lvl == nil || *lvl != 2 {
		t.Fatalf("level = %v, want 2%% to survive validation", lvl)
	}
}

func TestValidateAutomationAcceptsEveryKindOfMusicRoom(t *testing.T) {
	s := housed(t)
	s.Sockets["lamp"] = &Socket{ID: "lamp", Name: "Bedroom lamp"}
	for _, room := range []string{"sonos:snd", "kef:kf", "zone:z"} {
		a := &Automation{Name: "X", Rules: []AutomationRule{{
			Trigger: AutomationTrigger{Type: "music", Room: room, ToState: MusicPlaying},
			Actions: dimBedroom(),
		}}}
		if err := s.ValidateAutomation(a); err != nil {
			t.Errorf("room %q rejected: %v", room, err)
		}
	}
}

func TestValidateAutomationRefusesBadMusicTriggers(t *testing.T) {
	cases := map[string]AutomationTrigger{
		"room not in the house": {Type: "music", Room: "sonos:gone", ToState: MusicStopped},
		"unqualified room":      {Type: "music", Room: "living room", ToState: MusicStopped},
		"no room":               {Type: "music", ToState: MusicStopped},
		"socket vocabulary":     {Type: "music", Room: "kef:kf", ToState: "off"},
		"no state":              {Type: "music", Room: "kef:kf"},
	}
	for name, tr := range cases {
		t.Run(name, func(t *testing.T) {
			s := housed(t)
			s.Sockets["lamp"] = &Socket{ID: "lamp", Name: "Bedroom lamp"}
			a := &Automation{Name: "X", Rules: []AutomationRule{{Trigger: tr, Actions: dimBedroom()}}}
			if err := s.ValidateAutomation(a); err == nil {
				t.Fatal("accepted a trigger that can never fire")
			}
		})
	}
}

func TestValidateAutomationAcceptsAMusicCondition(t *testing.T) {
	s := housed(t)
	s.Sockets["lamp"] = &Socket{ID: "lamp", Name: "Bedroom lamp"}
	a := &Automation{Name: "X", Rules: []AutomationRule{{
		Trigger:    AutomationTrigger{Type: "time", TimeMode: "fixed", Time: "23:00"},
		Conditions: []AutomationCondition{{Type: "music", Room: "zone:z", State: "Playing"}},
		Actions:    dimBedroom(),
	}}}
	if err := s.ValidateAutomation(a); err != nil {
		t.Fatalf("ValidateAutomation: %v", err)
	}
	if got := a.Rules[0].Conditions[0].State; got != MusicPlaying {
		t.Fatalf("state = %q, want it normalised", got)
	}

	a.Rules[0].Conditions[0].Room = "zone:gone"
	err := s.ValidateAutomation(a)
	if err == nil || !strings.Contains(err.Error(), "not a speaker or zone") {
		t.Fatalf("err = %v, want the missing-room refusal", err)
	}
}

// The same promise PruneMusicRooms already makes for music actions, extended
// to the rules that watch a room: a deleted speaker leaves nothing behind.
func TestPruneMusicRoomsDropsRulesThatWatchADeletedRoom(t *testing.T) {
	s := housed(t)
	s.Automations["watch"] = &Automation{Name: "Film over", Rules: []AutomationRule{
		// Triggered by the departing speaker: can never fire again.
		{
			Trigger: AutomationTrigger{Type: "music", Room: "sonos:snd", ToState: MusicStopped},
			Actions: dimBedroom(),
		},
		// Only gated by it: keeps running, loses the gate.
		{
			Trigger: AutomationTrigger{Type: "time", TimeMode: "fixed", Time: "23:00"},
			Conditions: []AutomationCondition{
				{Type: "music", Room: "sonos:snd", State: MusicStopped},
				{Type: "device", SocketID: "lamp", State: "on"},
			},
			Actions: dimBedroom(),
		},
		// Watches a speaker that is still here.
		{
			Trigger: AutomationTrigger{Type: "music", Room: "kef:kf", ToState: MusicStopped},
			Actions: dimBedroom(),
		},
	}}

	delete(s.Sonos, "snd")
	delete(s.Zones, "z") // the zone held that speaker

	if !s.PruneMusicRooms() {
		t.Fatal("PruneMusicRooms reported no change")
	}
	rules := s.Automations["watch"].Rules
	if len(rules) != 2 {
		t.Fatalf("rules = %+v, want the triggered-by-a-ghost rule gone", rules)
	}
	if rules[0].Trigger.Type != "time" || len(rules[0].Conditions) != 1 {
		t.Errorf("rule 0 = %+v, want the music condition dropped and the device one kept", rules[0])
	}
	if rules[0].Conditions[0].Type != "device" {
		t.Errorf("the wrong condition was dropped: %+v", rules[0].Conditions)
	}
	if rules[1].Trigger.Room != "kef:kf" {
		t.Errorf("a rule watching a live speaker was pruned: %+v", rules[1])
	}
}

// A rule that is triggered by the music and only drives the music is the
// shape this feature makes ordinary — "when the living room stops, pause the
// kitchen" — and it must survive an unrelated socket being deleted.
func TestDeletingASocketKeepsAMusicOnlyRule(t *testing.T) {
	s := housed(t)
	s.Sockets["lamp"] = &Socket{ID: "lamp", Name: "Bedroom lamp"}
	s.Automations["au"] = &Automation{Name: "Quiet the kitchen", Rules: []AutomationRule{{
		Trigger: AutomationTrigger{Type: "music", Room: "sonos:snd", ToState: MusicStopped},
		Music:   []MusicAction{{Room: "kef:kf", Action: MusicPause}},
	}}}

	s.CascadeDeleteSocket("lamp")

	a, still := s.Automations["au"]
	if !still {
		t.Fatal("an automation that never mentioned the socket was deleted with it")
	}
	if len(a.Rules) != 1 || len(a.Rules[0].Music) != 1 {
		t.Fatalf("rules = %+v", a.Rules)
	}
}
