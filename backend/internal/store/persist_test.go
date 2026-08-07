package store

import (
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
// readings arrive several times a second from a chatty sensor, and a play is
// recorded on every tap of a shelf. Stated as a test so flipping the flag by
// accident is caught, and so adding a third exception is a deliberate edit.
func TestOnlyOwnSaverCollectionsAreExcludedFromFullSave(t *testing.T) {
	ownSaver := map[string]bool{"readings": true, "media history": true}
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
