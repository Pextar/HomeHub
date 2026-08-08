package api

import (
	"net/http"
	"testing"

	"homehub/internal/store"
)

// The three routes the wall panel's fan-outs moved onto: a queue that takes a
// run in one request, a grouping call that keeps join-before-leave ordering
// server-side, and a way for a room to stop remembering something.
//
// None of these tests reach a speaker — the reference hardware in CI is a
// tempdir. What they pin is everything decided before the first SOAP call:
// the shape of the request, the bounds on it, and who is allowed to send it.

// seedSonos registers speakers so the {id} lookup passes and the handler gets
// as far as validating its body.
func seedSonos(t *testing.T, srv *Server, ids ...string) {
	t.Helper()
	for i, id := range ids {
		srv.Store.Sonos[id] = &store.SonosSpeaker{
			ID:   id,
			Name: "Room " + id,
			IP:   "192.0.2." + string(rune('1'+i)),
			UUID: "RINCON_" + id,
		}
	}
}

// ── The queue batch ──────────────────────────────────────────────────────

func TestQueueAddRejectsARunWithNoURI(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	seedSonos(t, srv, "sp1")

	for _, body := range []string{
		`{"items":[]}`,                                     // empty falls back to the single shape, which has no uri
		`{"items":[{"uri":"x:1"},{"uri":"  "}]}`,           // one blank in the run
		`{"items":[{"uri":"x:1"},{"uri":"x:2"}],"next":1}`, // malformed
	} {
		rec := doAs(t, srv, admin, pass, http.MethodPost, "/api/sonos/sp1/queue", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST queue %s = %d, want 400", body, rec.Code)
		}
	}
}

// The bound exists so one request cannot hold a speaker for minutes. It is
// checked before anything is resolved or sent, so an over-long run costs the
// household nothing.
func TestQueueAddBoundsTheBatch(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	seedSonos(t, srv, "sp1")

	body := `{"items":[`
	for i := 0; i <= maxQueueBatch; i++ {
		if i > 0 {
			body += ","
		}
		body += `{"uri":"x:track"}`
	}
	body += `]}`

	rec := doAs(t, srv, admin, pass, http.MethodPost, "/api/sonos/sp1/queue", body)
	mustStatus(t, rec, http.StatusBadRequest)
	if msg := errorMessage(t, rec); msg == "" {
		t.Error("an over-long batch answered 400 with no reason")
	}
}

// Only Spotify resolves against a household account, and the refusal has to
// come from the batch path too — not just from the single-item one it used
// to be the only shape of.
func TestQueueAddRefusesUnknownServiceInABatch(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	seedSonos(t, srv, "sp1")

	rec := doAs(t, srv, admin, pass, http.MethodPost, "/api/sonos/sp1/queue",
		`{"items":[{"service":"Tidal","uri":"tidal:track:1"}]}`)
	mustStatus(t, rec, http.StatusBadRequest)
}

// ── Grouping in one call ─────────────────────────────────────────────────

func TestSonosGroupRequiresSomethingToDo(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	seedSonos(t, srv, "sp1")

	rec := doAs(t, srv, admin, pass, http.MethodPost, "/api/sonos/sp1/group", `{}`)
	mustStatus(t, rec, http.StatusBadRequest)
}

// A speaker that isn't registered is a bad request, not a device failure half
// way through the run — the whole point of resolving everything up front.
func TestSonosGroupRefusesUnknownSpeakersBeforeTouchingAnything(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	seedSonos(t, srv, "sp1", "sp2")

	for _, body := range []string{
		`{"join":["sp2","nope"]}`,
		`{"leave":["nope"]}`,
	} {
		rec := doAs(t, srv, admin, pass, http.MethodPost, "/api/sonos/sp1/group", body)
		if rec.Code != http.StatusNotFound {
			t.Errorf("POST group %s = %d, want 404", body, rec.Code)
		}
	}
}

func TestSonosGroupBoundsTheBatch(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	seedSonos(t, srv, "sp1")

	body := `{"join":[`
	for i := 0; i <= maxGroupBatch; i++ {
		if i > 0 {
			body += ","
		}
		body += `"sp1"`
	}
	body += `]}`

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodPost, "/api/sonos/sp1/group", body),
		http.StatusBadRequest)
}

// ── Forgetting a play ────────────────────────────────────────────────────

func TestForgetPlayRouteDropsOneEntry(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	srv.Store.RecordPlay("sonos:sp1", store.MediaPlay{
		Provider: "spotify", Kind: "track", URI: "spotify:track:keep", Title: "Keep",
	})
	srv.Store.RecordPlay("sonos:sp1", store.MediaPlay{
		Provider: "spotify", Kind: "track", URI: "spotify:track:oops", Title: "Oops",
	})

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodDelete,
		"/api/media/history?room=sonos:sp1&uri=spotify:track:oops", ""), http.StatusNoContent)

	left := srv.Store.History("sonos:sp1")
	if len(left) != 1 || left[0].URI != "spotify:track:keep" {
		t.Errorf("history = %+v, want only the kept track", left)
	}
}

func TestForgetPlayRouteWithoutAURIClearsTheRoom(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	srv.Store.RecordPlay("zone:z1", store.MediaPlay{URI: "spotify:track:a", Title: "A"})
	srv.Store.RecordPlay("zone:z1", store.MediaPlay{URI: "spotify:track:b", Title: "B"})
	srv.Store.RecordPlay("kef:k1", store.MediaPlay{URI: "spotify:track:c", Title: "C"})

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodDelete,
		"/api/media/history?room=zone:z1", ""), http.StatusNoContent)

	if n := len(srv.Store.History("zone:z1")); n != 0 {
		t.Errorf("cleared room = %d entries, want 0", n)
	}
	if n := len(srv.Store.History("kef:k1")); n != 1 {
		t.Errorf("other room = %d entries, want 1 — forgetting is per room", n)
	}
}

// "This room no longer offers that" is the caller's goal, so reaching a state
// that was already true is a success. A wall panel that armed a second tap
// only to be told the entry had aged out would be reporting a failure to do
// something that is done.
func TestForgetPlayRouteIsIdempotent(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodDelete,
		"/api/media/history?room=sonos:never&uri=spotify:track:x", ""), http.StatusNoContent)
}

func TestForgetPlayRouteNeedsARoom(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodDelete,
		"/api/media/history?uri=spotify:track:x", ""), http.StatusBadRequest)
}
