package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"homehub/internal/store"
)

// statusError carries the HTTP status a failure should produce, so the body
// of a store transaction can say "this is a 404" without holding a
// ResponseWriter. writeErr turns it back into a response once the lock is
// released.
type statusError struct {
	status int
	msg    string
}

func (e *statusError) Error() string { return e.msg }

// errStatus builds an error that writeErr renders with the given status.
func errStatus(status int, format string, args ...any) error {
	return &statusError{status: status, msg: fmt.Sprintf(format, args...)}
}

// The three failures every resource shares, so the wording can't drift
// between them.
func errNotFound(resource string) error {
	return errStatus(http.StatusNotFound, "%s not found", resource)
}

func errConflict(resource string) error {
	return errStatus(http.StatusConflict, "a %s with that id already exists", resource)
}

// errInvalid reports a validation failure as a 400. Validators return plain
// errors, and an unwrapped one would otherwise be rendered as a 500.
func errInvalid(err error) error {
	return errStatus(http.StatusBadRequest, "%s", err.Error())
}

// writeErr renders an error returned by a store transaction.
//
// A statusError carries its own status. A *store.SaveError means persisting
// failed, which is ours rather than the client's. Anything else is
// unexpected and reported as a 500 rather than guessed at.
func writeErr(w http.ResponseWriter, err error) {
	var se *statusError
	if errors.As(err, &se) {
		writeError(w, se.status, se.msg)
		return
	}
	var saveErr *store.SaveError
	if errors.As(err, &saveErr) {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+saveErr.Err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// update runs fn as a store transaction and writes the error response
// itself, so a handler reads:
//
//	if !s.update(w, func() error { ... }) {
//		return
//	}
//
// fn runs with the write lock held; the store is persisted when it returns
// nil. Do no device I/O inside it.
func (s *Server) update(w http.ResponseWriter, fn func() error) bool {
	if err := s.Store.Update(fn); err != nil {
		writeErr(w, err)
		return false
	}
	return true
}

// updateOr is update with an undo run if persisting fails.
func (s *Server) updateOr(w http.ResponseWriter, undo func(), fn func() error) bool {
	if err := s.Store.UpdateOr(undo, fn); err != nil {
		writeErr(w, err)
		return false
	}
	return true
}

// Boilerplate shared by the write handlers.
//
// Nearly every mutating endpoint opens by decoding a JSON body and closes
// by persisting the store, and both steps have exactly one correct failure
// response. Spelling them out per handler meant sixty copies of the first
// and eighteen of the second, which is sixty-eight chances for the status
// code or the message to drift.
//
// Both helpers write the error response themselves and report whether the
// caller should carry on, so a handler reads:
//
//	if !decodeBody(w, r, &thing) {
//		return
//	}

// decodeBody reads the request body into v. On malformed JSON it writes a
// 400 and returns false.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// saveStore persists every store file. On failure it writes a 500 and
// returns false.
//
// Caller must hold Mu. Note that a failed save leaves the in-memory
// mutation in place: handlers that need the store to match what is on disk
// undo their change before returning (see saveStoreOr).
func (s *Server) saveStore(w http.ResponseWriter) bool {
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return false
	}
	return true
}

// saveStoreOr is saveStore with an undo, run before the error is written
// when persisting fails. Caller must hold Mu.
func (s *Server) saveStoreOr(w http.ResponseWriter, undo func()) bool {
	if err := s.Store.Save(); err != nil {
		undo()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return false
	}
	return true
}
