package store

import (
	"strings"
	"testing"
)

// Music on a scene or a rule is the join between the two halves of the house
// — the sockets and the speakers — so what is tested here is the seam:
// what a step may say, what happens to it when the speaker it names goes
// away, and that the store hands the work to whoever can actually do it.

func ptr(n int) *int { return &n }

func TestValidateSceneAcceptsMusic(t *testing.T) {
	s := housed(t)
	sc := &Scene{
		Name: "Film",
		Steps: []SceneStep{{
			Actions: []SceneAction{},
			Music: []MusicAction{
				{Room: "sonos:snd", Action: MusicPause},
				{Room: "kef:kf", Action: MusicVolume, Volume: ptr(12)},
			},
		}},
	}
	if err := s.ValidateScene(sc); err != nil {
		t.Fatalf("ValidateScene: %v", err)
	}
	// A step that only quiets the house is a real step: dropping it as
	// "empty" would silently delete the whole point of the scene.
	if len(sc.Steps) != 1 || len(sc.Steps[0].Music) != 2 {
		t.Fatalf("steps = %+v", sc.Steps)
	}
	// pause carries no level, whatever was sent with it.
	if sc.Steps[0].Music[0].Volume != nil {
		t.Error("a pause kept a volume it has no use for")
	}
}

func TestValidateSceneRejectsBadMusic(t *testing.T) {
	cases := map[string]MusicAction{
		"unknown verb":     {Room: "sonos:snd", Action: "skip"},
		"unknown room":     {Room: "sonos:gone", Action: MusicPause},
		"volume, no level": {Room: "sonos:snd", Action: MusicVolume},
		"volume too high":  {Room: "sonos:snd", Action: MusicVolume, Volume: ptr(101)},
		"volume negative":  {Room: "sonos:snd", Action: MusicVolume, Volume: ptr(-1)},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			s := housed(t)
			sc := &Scene{Name: "X", Steps: []SceneStep{{Music: []MusicAction{m}}}}
			if err := s.ValidateScene(sc); err == nil {
				t.Fatal("accepted an action the house can't carry out")
			}
		})
	}
}

// Two rows aimed at one room is a scene nobody can read: whichever ran last
// would win, and which that is isn't visible in the editor.
func TestValidateSceneDedupesMusicByRoom(t *testing.T) {
	s := housed(t)
	sc := &Scene{Name: "X", Steps: []SceneStep{{Music: []MusicAction{
		{Room: "sonos:snd", Action: MusicPause},
		{Room: "sonos:snd", Action: MusicVolume, Volume: ptr(30)},
	}}}}
	if err := s.ValidateScene(sc); err != nil {
		t.Fatalf("ValidateScene: %v", err)
	}
	if got := sc.Steps[0].Music; len(got) != 1 || got[0].Action != MusicPause {
		t.Fatalf("music = %+v, want only the first row for that room", got)
	}
}

// A rule that only touches the music satisfies "at least one action" — it
// does something, and refusing it would be the socket half of the house
// deciding what counts.
func TestValidateAutomationAcceptsMusicOnlyRule(t *testing.T) {
	s := housed(t)
	a := &Automation{
		Name: "Bedtime",
		Rules: []AutomationRule{{
			Trigger: AutomationTrigger{Type: "time", TimeMode: "fixed", Time: "23:00"},
			Music:   []MusicAction{{Room: "zone:z", Action: MusicPause}},
		}},
	}
	if err := s.ValidateAutomation(a); err != nil {
		t.Fatalf("ValidateAutomation: %v", err)
	}
	if len(a.Rules[0].Music) != 1 {
		t.Fatalf("music = %+v", a.Rules[0].Music)
	}
}

func TestValidateAutomationStillNeedsSomethingToDo(t *testing.T) {
	s := housed(t)
	a := &Automation{
		Name:  "Nothing",
		Rules: []AutomationRule{{Trigger: AutomationTrigger{Type: "time", TimeMode: "fixed", Time: "23:00"}}},
	}
	err := s.ValidateAutomation(a)
	if err == nil || !strings.Contains(err.Error(), "at least one action") {
		t.Fatalf("err = %v, want the empty-rule refusal", err)
	}
}

// The promise CascadeDeleteSocket makes for sockets, kept for speakers: a
// deleted room leaves nothing behind that fires into it.
func TestPruneMusicRooms(t *testing.T) {
	s := housed(t)
	s.Scenes["sc"] = &Scene{Name: "Film", Steps: []SceneStep{
		{
			Actions: []SceneAction{{SocketID: "lamp", Action: "off"}},
			Music:   []MusicAction{{Room: "sonos:snd", Action: MusicPause}},
		},
		{Music: []MusicAction{{Room: "kef:kf", Action: MusicPause}}},
	}}
	s.Automations["au"] = &Automation{Name: "Bedtime", Rules: []AutomationRule{
		{Music: []MusicAction{{Room: "sonos:snd", Action: MusicPause}}},
	}}

	delete(s.Sonos, "snd")
	delete(s.Zones, "z") // the zone referenced the speaker too

	if !s.PruneMusicRooms() {
		t.Fatal("PruneMusicRooms reported no change")
	}

	// The step that still switches a socket keeps its purpose and loses only
	// the part that no longer exists.
	sc := s.Scenes["sc"]
	if len(sc.Steps) != 2 {
		t.Fatalf("steps = %+v, want the socket step and the surviving KEF step", sc.Steps)
	}
	if len(sc.Steps[0].Music) != 0 {
		t.Errorf("step 0 kept music for a deleted speaker: %+v", sc.Steps[0].Music)
	}
	if len(sc.Steps[0].Actions) != 1 {
		t.Errorf("step 0 lost its sockets: %+v", sc.Steps[0].Actions)
	}
	if len(sc.Steps[1].Music) != 1 {
		t.Errorf("the KEF step was dropped although its speaker is still here")
	}

	// A rule with nothing left can never fire, and an automation with no
	// rules left is not an automation.
	if _, still := s.Automations["au"]; still {
		t.Error("an automation left with no rules survived")
	}
}

func TestPruneMusicRoomsLeavesAHealthyHouseAlone(t *testing.T) {
	s := housed(t)
	s.Scenes["sc"] = &Scene{Name: "Film", Steps: []SceneStep{
		{Music: []MusicAction{{Room: "sonos:snd", Action: MusicPause}}},
	}}
	if s.PruneMusicRooms() {
		t.Fatal("reported a change with nothing to prune")
	}
	if len(s.Scenes["sc"].Steps) != 1 {
		t.Fatal("a live scene was pruned")
	}
}

// The store never reaches a speaker itself — it hands the actions to whoever
// installed OnMusic. Nothing installed means nothing happens, which is what
// every test that builds a bare Store relies on.
func TestRunMusicUsesTheHook(t *testing.T) {
	s := housed(t)
	var got []MusicAction
	s.OnMusic = func(a []MusicAction) { got = a }

	want := []MusicAction{{Room: "sonos:snd", Action: MusicPause}}
	s.RunMusic(want)
	if len(got) != 1 || got[0].Room != "sonos:snd" {
		t.Fatalf("hook got %+v", got)
	}

	// Nothing to do, and nothing to do it with: neither may panic.
	s.RunMusic(nil)
	s.OnMusic = nil
	s.RunMusic(want)
}

// The reliability argument for buffering rather than calling the hook
// directly: a scene can be activated from six places, and every one of them
// resolves it through StageAction and drains afterwards. Staging must be
// what queues the music, or five of the six would quietly not do it.
func TestStagingAScenePicksUpItsMusic(t *testing.T) {
	s := housed(t)
	s.Scenes["sc"] = &Scene{Name: "Goodnight", Steps: []SceneStep{
		{Music: []MusicAction{{Room: "sonos:snd", Action: MusicPause}}},
		{DelayMinutes: 30, Music: []MusicAction{{Room: "kef:kf", Action: MusicPause}}},
	}}

	var ran []MusicAction
	s.OnMusic = func(a []MusicAction) { ran = append(ran, a...) }

	s.Mu.Lock()
	_, err := s.StageAction("scene", "sc", "activate")
	s.Mu.Unlock()
	if err != nil {
		t.Fatalf("StageAction: %v", err)
	}

	// Nothing may have reached a speaker yet: staging happens under the lock
	// and device I/O never does.
	if len(ran) != 0 {
		t.Fatalf("music ran during staging: %+v", ran)
	}

	s.FlushMusic()
	if len(ran) != 1 || ran[0].Room != "sonos:snd" {
		t.Fatalf("flushed %+v, want the immediate step's action", ran)
	}

	// And the buffer is empty afterwards — a second drain must not repeat
	// what the first already did.
	ran = nil
	s.FlushMusic()
	if len(ran) != 0 {
		t.Fatalf("a second flush replayed %+v", ran)
	}
}
