package store

import (
	"fmt"
	"reflect"
	"testing"
)

// notPersisted lists the exported Store fields that deliberately have no
// file. Anything else must appear in the collections table.
//
// Keeping the exclusions here rather than the inclusions is the point: a new
// map added to Store fails this test until someone decides, explicitly,
// whether it is persisted. The old hand-written Load/Save could silently
// omit it.
var notPersisted = map[string]string{
	"Mu":                  "the lock itself",
	"Activity":            "an in-memory ring buffer; restarts wipe it by design",
	"Discovery":           "the pair window, meaningless across a restart",
	"DataDir":             "configuration, not state",
	"RF":                  "injected dependency",
	"Light":               "injected dependency",
	"OnChange":            "callback",
	"OnStateChange":       "callback",
	"OnMusic":             "callback",
	"OnSensorAlert":       "callback",
	"SuppressStateChange": "a transient flag held across one bulk operation",
}

// TestEveryPersistedFieldIsInTheTable is the guard against the drift that
// made the first-run room migration dead code: three separate lists (read,
// write, nil-guard) that nothing forced to agree.
func TestEveryPersistedFieldIsInTheTable(t *testing.T) {
	inTable := map[string]bool{}
	for _, c := range collections {
		if c.field == "" {
			t.Errorf("collection %q has no field name", c.label)
			continue
		}
		inTable[c.field] = true
	}

	typ := reflect.TypeOf(Store{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue // unexported bookkeeping (pendingLights, txMu, …)
		}
		if _, ok := notPersisted[f.Name]; ok {
			continue
		}
		if !inTable[f.Name] {
			t.Errorf("Store.%s (%s) is neither in the collections table nor "+
				"listed in notPersisted — decide which and say so", f.Name, f.Type)
		}
	}

	// And the reverse: nothing in the table names a field that no longer
	// exists, which would silently stop being loaded.
	for _, c := range collections {
		if _, ok := typ.FieldByName(c.field); !ok {
			t.Errorf("collection %q names Store.%s, which does not exist", c.label, c.field)
		}
	}
}

func TestCollectionTableIsWellFormed(t *testing.T) {
	seenLabel := map[string]bool{}
	seenFile := map[string]bool{}
	for _, c := range collections {
		if c.load == nil || c.save == nil {
			t.Errorf("collection %q is missing a load or save", c.label)
		}
		if seenLabel[c.label] {
			t.Errorf("duplicate label %q — errors would be ambiguous", c.label)
		}
		if seenFile[c.file] {
			t.Errorf("duplicate file %q — one collection would overwrite the other", c.file)
		}
		seenLabel[c.label] = true
		seenFile[c.file] = true
	}
}

// Save skips exactly the collections that have a saver of their own, because
// each is written on a path where rewriting the whole store would be absurd:
// readings arrive several times a second from a chatty sensor, a play is
// recorded on every tap of a shelf, and the listening log is written whenever
// a song changes in any room — with or without anyone asking HomeHub for it.
// Stated as a test so flipping the flag by accident is caught, and so adding
// a fourth exception is a deliberate edit.
func TestOnlyOwnSaverCollectionsAreExcludedFromFullSave(t *testing.T) {
	ownSaver := map[string]bool{"readings": true, "media history": true, "heard tracks": true}
	for _, c := range collections {
		if want := !ownSaver[c.label]; c.inFullSave != want {
			t.Errorf("collection %q inFullSave = %v, want %v", c.label, c.inFullSave, want)
		}
	}
}

// Every collection substitutes an empty map for a nil one, so callers can
// index them under the lock without checking. Rooms used to be excluded so
// that Load could read its nil-ness as "first run"; that signal never worked
// and has been replaced by reconciliation, so the exception is gone.
func TestEveryCollectionGuardsAgainstANilMap(t *testing.T) {
	for _, c := range collections {
		if c.ensure == nil {
			t.Errorf("collection %q has no nil-guard; a file holding `null` "+
				"would leave the field nil", c.label)
		}
	}
}

// ── Announce presets ─────────────────────────────────────────────────────
// The sentences the panel offers before its text box. Household settings
// rather than a constant in a component: typing is the worst thing a wall
// asks anyone to do, and the voice that reads them speaks one language.

func TestValidateSettingsTidiesAnnouncePresets(t *testing.T) {
	s := New(t.TempDir(), nil)
	set := &Settings{AnnouncePresets: []string{
		"  Middagen är klar  ",
		"",
		"middagen är klar", // the same sentence, differently cased
		"Läggdags",
		"   ",
	}}
	if err := s.ValidateSettings(set); err != nil {
		t.Fatalf("ValidateSettings: %v", err)
	}
	want := []string{"Middagen är klar", "Läggdags"}
	if len(set.AnnouncePresets) != len(want) {
		t.Fatalf("presets = %q, want %q", set.AnnouncePresets, want)
	}
	for i, p := range want {
		if set.AnnouncePresets[i] != p {
			t.Errorf("preset %d = %q, want %q", i, set.AnnouncePresets[i], p)
		}
	}
}

// Blanks and duplicates are tidied because they are rows in an editor.
// Length is refused, because trimming a sentence changes what was typed.
func TestValidateSettingsRefusesOversizedPresets(t *testing.T) {
	s := New(t.TempDir(), nil)

	long := make([]rune, MaxAnnouncePresetLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := s.ValidateSettings(&Settings{AnnouncePresets: []string{string(long)}}); err == nil {
		t.Error("an over-long preset was accepted, want an error")
	}

	many := make([]string, 0, MaxAnnouncePresets+1)
	for i := 0; i <= MaxAnnouncePresets; i++ {
		many = append(many, fmt.Sprintf("preset %d", i))
	}
	if err := s.ValidateSettings(&Settings{AnnouncePresets: many}); err == nil {
		t.Error("an over-long preset list was accepted, want an error")
	}
}

// Nil and empty are different, and the difference is what lets a household
// have none. Validation must not collapse one into the other.
func TestAnnouncePresetsTellNeverSetFromDeliberatelyNone(t *testing.T) {
	s := New(t.TempDir(), nil)

	never := &Settings{}
	if err := s.ValidateSettings(never); err != nil {
		t.Fatalf("ValidateSettings: %v", err)
	}
	if never.AnnouncePresets != nil {
		t.Errorf("nil presets became %q, want nil left alone", never.AnnouncePresets)
	}
	if got := never.Presets(); len(got) != len(DefaultAnnouncePresets) {
		t.Errorf("Presets() on an unset household = %q, want the built-in list", got)
	}

	none := &Settings{AnnouncePresets: []string{}}
	if err := s.ValidateSettings(none); err != nil {
		t.Fatalf("ValidateSettings: %v", err)
	}
	if none.AnnouncePresets == nil {
		t.Error("an explicitly empty list became nil, want it kept — that is a household saying 'none'")
	}
	if got := none.Presets(); len(got) != 0 {
		t.Errorf("Presets() on a household that wants none = %q, want empty", got)
	}
}

// Presets hands out a copy: the panel's list must not be a window onto the
// store's own slice.
func TestPresetsHandsOutACopy(t *testing.T) {
	set := &Settings{AnnouncePresets: []string{"Kom ner"}}
	got := set.Presets()
	got[0] = "something else"
	if set.AnnouncePresets[0] != "Kom ner" {
		t.Errorf("the store's list was rewritten through Presets(): %q", set.AnnouncePresets)
	}
}
