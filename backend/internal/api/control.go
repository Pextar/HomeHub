package api

// The HTTP side of internal/control: who is allowed to act, and how a
// multi-device result becomes a response.

import (
	"net/http"

	"homehub/internal/control"
	"homehub/internal/store"
)

// allowedTo turns a request's user into the predicate the action layer
// enforces. It exists so internal/control stays out of the permissions model:
// that package applies the answer, this one computes it.
func allowedTo(user *store.User) control.Allow {
	return func(socketID string) bool { return canAccess(user, socketID) }
}

// writeStaged renders the two failures every multi-device action shares, and
// reports whether the handler should carry on to write its own body.
//
// notFound is the message for a target nothing matched; an empty one means the
// action cannot miss — switching every socket in an empty house is a success,
// not a 404.
func writeStaged(w http.ResponseWriter, notFound string, res control.Result, err error) bool {
	if notFound != "" && !res.Found {
		writeError(w, http.StatusNotFound, notFound+" not found")
		return false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return false
	}
	return true
}
