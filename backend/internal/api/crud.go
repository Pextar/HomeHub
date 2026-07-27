package api

import (
	"encoding/json"
	"net/http"
)

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
