package store

import "fmt"

// Transactions.
//
// Mu used to be acquired directly by every caller, which put the locking
// rules in the hands of ~130 call sites across api, scheduler, mqtt and rx.
// Each had to remember to unlock on every exit path, to pair a mutation with
// a Save, and to keep device I/O outside the critical section. Nothing
// enforced any of it.
//
// View, Update and Mutate take a closure instead. The closure is still the
// unit of composition the old convention was built around — several reads
// and writes stay atomic because they happen inside one call — but the lock
// is acquired and released in one place, and Update pairs the mutation with
// its Save automatically.
//
// Mu remains exported for the paths that genuinely need to split a critical
// section in two, notably the staged device flow, which must release the
// lock to transmit and reacquire it to record the result.
//
// None of these may be nested: Go's RWMutex is not reentrant, so calling
// View from inside Update deadlocks. Compose within a single closure.

// SaveError marks a failure to persist, as opposed to an error returned by
// the transaction body. Callers map the two to different responses — a
// validation error is the client's problem, a save failure is ours.
type SaveError struct{ Err error }

func (e *SaveError) Error() string { return e.Err.Error() }
func (e *SaveError) Unwrap() error { return e.Err }

// View runs fn with the read lock held. Use it for reads that must see a
// consistent snapshot across several fields.
//
// Do no I/O and take no other locks inside fn; anything slow blocks every
// writer. Marshal inside, write the response outside.
func (s *Store) View(fn func()) {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	fn()
}

// Update runs fn with the write lock held and persists if fn returns nil.
//
// An error from fn aborts before Save, leaving the store as fn left it —
// fn is expected to return before mutating anything if it is going to fail.
// A persistence failure comes back wrapped in *SaveError.
func (s *Store) Update(fn func() error) error {
	return s.UpdateOr(nil, fn)
}

// UpdateOr is Update with an undo that runs if persisting fails, for
// callers that need the in-memory state to match what reached disk.
func (s *Store) UpdateOr(undo func(), fn func() error) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if err := fn(); err != nil {
		return err
	}
	if err := s.Save(); err != nil {
		if undo != nil {
			undo()
		}
		return &SaveError{Err: err}
	}
	return nil
}

// Mutate runs fn with the write lock held without persisting, for changes
// that are deliberately runtime-only — cached device state, discovery
// results, anything reconstructed on restart.
func (s *Store) Mutate(fn func()) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	fn()
}

// saveErrorf is a convenience for tests and callers that need to construct
// the wrapper directly.
func saveErrorf(format string, args ...any) *SaveError {
	return &SaveError{Err: fmt.Errorf(format, args...)}
}
