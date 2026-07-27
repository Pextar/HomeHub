package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func txStore(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir(), noopRF{})
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	return s
}

func TestUpdatePersistsOnSuccess(t *testing.T) {
	s := txStore(t)
	err := s.Update(func() error {
		s.Groups["g1"] = &Group{ID: "g1", Name: "Downstairs"}
		return nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Reading it back through a second Store proves it reached disk rather
	// than only the map.
	again := New(s.DataDir, noopRF{})
	if err := again.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.Groups["g1"] == nil {
		t.Error("the group was not persisted")
	}
}

func TestUpdateDoesNotSaveWhenTheBodyFails(t *testing.T) {
	s := txStore(t)
	sentinel := errors.New("nope")

	if err := s.Update(func() error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("Update returned %v, want the body's error", err)
	}
	// Nothing was written, so there is no groups file to read back.
	if _, err := os.Stat(filepath.Join(s.DataDir, "groups.json")); err == nil {
		t.Error("a failed transaction still wrote to disk")
	}
}

// A save failure has to be distinguishable from a validation failure: one is
// the client's fault and one is ours, and they map to different statuses.
func TestSaveFailureIsWrapped(t *testing.T) {
	s := txStore(t)
	s.DataDir = filepath.Join(s.DataDir, "no", "such", "dir")

	err := s.Update(func() error {
		s.Groups["g1"] = &Group{ID: "g1", Name: "Downstairs"}
		return nil
	})
	var saveErr *SaveError
	if !errors.As(err, &saveErr) {
		t.Fatalf("Update returned %T (%v), want a *SaveError", err, err)
	}
	// The body's error type must not be confused with it.
	sentinel := errors.New("validation")
	if errors.As(s.Update(func() error { return sentinel }), &saveErr) {
		t.Error("a body error was reported as a save failure")
	}
}

func TestUpdateOrRunsTheUndoOnlyWhenSavingFails(t *testing.T) {
	t.Run("undo runs when the save fails", func(t *testing.T) {
		s := txStore(t)
		s.DataDir = filepath.Join(s.DataDir, "missing")
		err := s.UpdateOr(func() { delete(s.Groups, "g1") }, func() error {
			s.Groups["g1"] = &Group{ID: "g1", Name: "Downstairs"}
			return nil
		})
		if err == nil {
			t.Fatal("expected a save failure")
		}
		if _, ok := s.Groups["g1"]; ok {
			t.Error("the undo did not run; memory now disagrees with disk")
		}
	})

	t.Run("undo does not run on success", func(t *testing.T) {
		s := txStore(t)
		err := s.UpdateOr(func() { delete(s.Groups, "g1") }, func() error {
			s.Groups["g1"] = &Group{ID: "g1", Name: "Downstairs"}
			return nil
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if _, ok := s.Groups["g1"]; !ok {
			t.Error("the undo ran on a successful save")
		}
	})

	t.Run("undo does not run when the body fails", func(t *testing.T) {
		s := txStore(t)
		ran := false
		_ = s.UpdateOr(func() { ran = true }, func() error { return errors.New("no") })
		if ran {
			t.Error("the undo ran for a body error, which never mutated anything")
		}
	})
}

func TestMutateDoesNotPersist(t *testing.T) {
	s := txStore(t)
	s.Mutate(func() { s.Groups["g1"] = &Group{ID: "g1", Name: "Runtime only"} })

	again := New(s.DataDir, noopRF{})
	if err := again.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.Groups["g1"] != nil {
		t.Error("Mutate persisted, but it exists precisely so callers can avoid that")
	}
}

// The point of the closure API: several reads and writes stay atomic, so a
// concurrent writer can never interleave halfway through one.
func TestTransactionsSerialiseConcurrentWriters(t *testing.T) {
	s := txStore(t)
	s.Groups["g"] = &Group{ID: "g", Name: "counter"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Mutate(func() {
				g := s.Groups["g"]
				g.SocketIDs = append(g.SocketIDs, "x")
			})
		}()
	}
	wg.Wait()

	var n int
	s.View(func() { n = len(s.Groups["g"].SocketIDs) })
	if n != 50 {
		t.Errorf("appended %d times, want 50 — a write was lost", n)
	}
}

func TestViewTakesAReadLock(t *testing.T) {
	s := txStore(t)
	s.Groups["g1"] = &Group{ID: "g1", Name: "Downstairs"}

	// Concurrent readers must not block each other; if View took the write
	// lock this would still pass but serialise, so the assertion here is
	// simply that nested-in-parallel reads complete without deadlock.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.View(func() { _ = s.Groups["g1"].Name })
		}()
	}
	wg.Wait()
}

func TestSaveErrorUnwraps(t *testing.T) {
	inner := errors.New("disk full")
	err := saveErrorf("saving groups: %w", inner)
	if !errors.Is(err, inner) {
		t.Error("SaveError does not unwrap to the cause")
	}
}
