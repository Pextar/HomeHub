package store

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// A socket is the most widely referenced thing in the store, and
// CascadeDeleteSocket is a hand-written list of the places its id can
// appear. The project's own notes flag it as something to keep in step by
// hand whenever a new reference is added — a rule that rots the moment
// someone adds a field without reading them.
//
// This walks the type graph reachable from Store and finds every field that
// holds a socket id. Each one has to be accounted for below, so adding a new
// one fails the build until somebody decides what deleting a socket should
// do about it.

// socketIDFields maps "Type.Field" to what the cascade does about it.
// A field found by the walk and missing from here fails the test.
var socketIDFields = map[string]string{
	"Group.SocketIDs":              "cleared — filterStrings drops the id from membership",
	"User.SocketIDs":               "cleared — a limited profile's allow-list",
	"SceneAction.SocketID":         "cleared — the action is dropped from its step",
	"AutomationTrigger.SocketID":   "the rule is dropped: it could never fire again",
	"AutomationCondition.SocketID": "the condition is dropped, the rule survives",

	// Not stale data: scheduleSocketID, the scheduler and three frontend
	// components still read this as the fallback for a schedule saved before
	// target_type existed. Clearing it would break them.
	"Schedule.SocketID": "live compatibility field. Load mirrors it into " +
		"TargetType/TargetID, and the schedule is deleted by target, so the " +
		"record goes as a whole rather than the field being cleared in place",

	"NotifPrefs.MutedSocketIDs": "cleared — the per-user notification mute list " +
		"holds ids of the same devices as the allow-list",
}

// looksLikeSocketID reports whether a field name denotes a socket id.
func looksLikeSocketID(name string) bool {
	return name == "SocketID" || name == "SocketIDs" ||
		(strings.Contains(name, "SocketID") && !strings.Contains(name, "Sensor"))
}

func TestSocketIDFieldsAreAllAccountedFor(t *testing.T) {
	found := map[string]bool{}
	seen := map[reflect.Type]bool{}

	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice ||
			typ.Kind() == reflect.Map || typ.Kind() == reflect.Array {
			if typ.Kind() == reflect.Map {
				walk(typ.Key())
			}
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			if looksLikeSocketID(f.Name) {
				found[typ.Name()+"."+f.Name] = true
			}
			walk(f.Type)
		}
	}
	walk(reflect.TypeOf(Store{}))

	if len(found) == 0 {
		t.Fatal("the walk found no socket id fields at all; it is not working")
	}

	var missing []string
	for name := range found {
		if _, ok := socketIDFields[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("%s holds a socket id but is not accounted for in socketIDFields — "+
			"decide what CascadeDeleteSocket should do about it and record it there", name)
	}

	// And the reverse, so the list doesn't accumulate entries for fields
	// that have since been renamed or removed.
	for name := range socketIDFields {
		if !found[name] {
			t.Errorf("socketIDFields lists %s, which no longer exists in the type graph", name)
		}
	}
}

// Both halves of a user's notification preferences hold device ids, and both
// are cleared. The mute list was missed originally: a deleted device's id
// stayed there indefinitely.
func TestDeletingADeviceClearsItsMuteEntry(t *testing.T) {
	t.Run("socket", func(t *testing.T) {
		s := New(t.TempDir(), noopRF{})
		s.Users["u"] = &User{
			ID: "u", Username: "kid",
			SocketIDs:  []string{"sk", "keep"},
			NotifPrefs: NotifPrefs{MutedSocketIDs: []string{"sk", "keep"}},
		}
		s.CascadeDeleteSocket("sk")

		if got := s.Users["u"].SocketIDs; len(got) != 1 || got[0] != "keep" {
			t.Errorf("allow-list = %v", got)
		}
		if got := s.Users["u"].NotifPrefs.MutedSocketIDs; len(got) != 1 || got[0] != "keep" {
			t.Errorf("muted sockets = %v, want the deleted id dropped", got)
		}
	})

	t.Run("sensor", func(t *testing.T) {
		s := New(t.TempDir(), noopRF{})
		s.Users["u"] = &User{
			ID: "u", Username: "kid",
			NotifPrefs: NotifPrefs{MutedSensorIDs: []string{"sn", "keep"}},
		}
		s.Automations["a"] = &Automation{ID: "a", Name: "A", Rules: []AutomationRule{{
			Trigger: AutomationTrigger{Type: "sensor", SensorID: "sn"},
			Actions: []AutomationAction{{TargetType: "socket", TargetID: "x", Action: "on"}},
		}}}
		s.CascadeDeleteSensor("sn")

		if got := s.Users["u"].NotifPrefs.MutedSensorIDs; len(got) != 1 || got[0] != "keep" {
			t.Errorf("muted sensors = %v, want the deleted id dropped", got)
		}
		if _, ok := s.Automations["a"]; ok {
			t.Error("an automation left with no rules survived")
		}
	})
}

// CascadeDeleteTarget is the shared half of every target cascade, so it is
// worth showing it behaves identically for each type that uses it.
func TestCascadeDeleteTargetIsUniformAcrossTypes(t *testing.T) {
	for _, targetType := range []string{"socket", "group", "room", "scene"} {
		s := New(t.TempDir(), noopRF{})
		s.Schedules["hit"] = &Schedule{ID: "hit", TargetType: targetType, TargetID: "x", Action: "on"}
		s.Schedules["othertype"] = &Schedule{ID: "othertype", TargetType: "sensor", TargetID: "x", Action: "on"}
		s.Schedules["otherid"] = &Schedule{ID: "otherid", TargetType: targetType, TargetID: "y", Action: "on"}
		s.Timers["hit"] = &Timer{ID: "hit", TargetType: targetType, TargetID: "x", Action: "off"}
		s.Timers["otherid"] = &Timer{ID: "otherid", TargetType: targetType, TargetID: "y", Action: "off"}
		s.Automations["a"] = &Automation{ID: "a", Name: "A", Rules: []AutomationRule{{
			Trigger: AutomationTrigger{Type: "time", Time: "07:00"},
			Actions: []AutomationAction{{TargetType: targetType, TargetID: "x", Action: "on"}},
		}}}

		s.CascadeDeleteTarget(targetType, "x")

		if _, ok := s.Schedules["hit"]; ok {
			t.Errorf("%s: matching schedule survived", targetType)
		}
		if _, ok := s.Schedules["othertype"]; !ok {
			t.Errorf("%s: a schedule with the same id but another type was deleted", targetType)
		}
		if _, ok := s.Schedules["otherid"]; !ok {
			t.Errorf("%s: a schedule for another id was deleted", targetType)
		}
		if _, ok := s.Timers["hit"]; ok {
			t.Errorf("%s: matching timer survived", targetType)
		}
		if _, ok := s.Timers["otherid"]; !ok {
			t.Errorf("%s: a timer for another id was deleted", targetType)
		}
		if _, ok := s.Automations["a"]; ok {
			t.Errorf("%s: an automation left with no actions survived", targetType)
		}
	}
}
