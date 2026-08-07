package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homehub/internal/store"
)

// The clip is fetched by a speaker, which has no session, so it has to sit
// outside the API's auth the way the audio stream and the Sonos callback do
// — and answer 404 for anything it did not mint.
func TestAnnounceClipRouteIsUnauthenticatedAndGuarded(t *testing.T) {
	srv := testServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, announcePath+"/deadbeef.wav", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown clip id = %d, want 404 (and never a login redirect)", rec.Code)
	}
}

// Interrupting every room is the whole feature, so refusing to do it has to
// say which fixable thing is missing rather than failing opaquely.
func TestAnnounceRefusesWhenThereIsNowhereToSendIt(t *testing.T) {
	srv := testServer(t)
	rec := httptest.NewRecorder()
	srv.announceSend(rec, httptest.NewRequest(http.MethodPost, "/api/announce",
		strings.NewReader(`{"text":"Dinner's ready"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("announce with no speakers = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "nowhere to announce") {
		t.Errorf("body = %q, want the reason someone can act on", rec.Body.String())
	}
	// And the household must not be left claimed by a request that never
	// reached a speaker — the next attempt has to be allowed to try.
	if !srv.announceBegin() {
		t.Error("a refused announcement left the household claimed")
	}
	srv.announceEnd()
}

// A second announcement mid-clip would snapshot the *clip* as what the rooms
// were playing, then restore them to a dead URL at announcement volume with
// the music gone. The claim is held for as long as the audio is, not for as
// long as the request.
func TestAnnounceIsOneAtATime(t *testing.T) {
	srv := testServer(t)
	if !srv.announceBegin() {
		t.Fatal("first claim was refused")
	}
	rec := httptest.NewRecorder()
	srv.announceSend(rec, httptest.NewRequest(http.MethodPost, "/api/announce",
		strings.NewReader(`{"text":"Again"}`)))
	if rec.Code != http.StatusConflict {
		t.Errorf("second announcement = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "already being announced") {
		t.Errorf("body = %q, want the reason", rec.Body.String())
	}
	srv.announceEnd()
	if !srv.announceBegin() {
		t.Error("the claim was never released")
	}
	srv.announceEnd()
}

func TestAnnounceRejectsAParagraph(t *testing.T) {
	srv := testServer(t)
	body, _ := json.Marshal(map[string]string{"text": strings.Repeat("å", 500)})
	rec := httptest.NewRecorder()
	srv.announceSend(rec, httptest.NewRequest(http.MethodPost, "/api/announce", strings.NewReader(string(body))))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a paragraph = %d, want 400 — measured in runes, not bytes", rec.Code)
	}
}

// The history endpoint answers per room, and falls back to the household's
// list saying so — the label is what keeps a shelf from implying a room
// played something it didn't.
func TestMediaHistoryFallsBackToTheHouseholdAndSaysSo(t *testing.T) {
	srv := testServer(t)
	srv.Store.Mutate(func() {
		srv.Store.RecordPlay("sonos:kitchen", store.MediaPlay{
			Provider: "spotify", URI: "spotify:album:x", Title: "X", RoomName: "Kitchen",
		})
	})

	var own struct {
		Plays     []store.MediaPlay `json:"plays"`
		Household bool              `json:"household"`
	}
	rec := httptest.NewRecorder()
	srv.mediaHistory(rec, httptest.NewRequest(http.MethodGet, "/api/media/history?room=sonos:kitchen", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &own); err != nil {
		t.Fatal(err)
	}
	if len(own.Plays) != 1 || own.Household {
		t.Errorf("own history = %+v, household=%v; want the room's own play", own.Plays, own.Household)
	}

	var other struct {
		Plays     []store.MediaPlay `json:"plays"`
		Household bool              `json:"household"`
	}
	rec = httptest.NewRecorder()
	srv.mediaHistory(rec, httptest.NewRequest(http.MethodGet, "/api/media/history?room=kef:study", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &other); err != nil {
		t.Fatal(err)
	}
	if len(other.Plays) != 1 || !other.Household {
		t.Errorf("a room with no history = %+v, household=%v; want the household's, flagged",
			other.Plays, other.Household)
	}
}

// A deleted speaker must not leave a shelf that plays to nothing.
func TestPruneHistoryFollowsDeletedRooms(t *testing.T) {
	srv := testServer(t)
	srv.Store.Mutate(func() {
		srv.Store.Sonos["kitchen"] = &store.SonosSpeaker{ID: "kitchen", Name: "Kitchen"}
		srv.Store.RecordPlay("sonos:kitchen", store.MediaPlay{URI: "spotify:track:1", Title: "One"})
		srv.Store.RecordPlay("kef:gone", store.MediaPlay{URI: "spotify:track:2", Title: "Two"})
	})

	srv.pruneDeadRooms()

	srv.Store.View(func() {
		if len(srv.Store.History("kef:gone")) != 0 {
			t.Error("history outlived the speaker it belonged to")
		}
		if len(srv.Store.History("sonos:kitchen")) != 1 {
			t.Error("a live room lost its history")
		}
	})
}
