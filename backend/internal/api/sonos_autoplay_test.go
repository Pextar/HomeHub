package api

import (
	"net/http"
	"testing"

	"homehub/internal/store"
)

// The switch's HTTP round trip. What autoplay then does with the setting —
// which live states it acts on, how long a quiet room stays eligible — is the
// engine's own test; this pins that the route reaches it, both ways.
func TestAutoplayRouteFlipsTheSetting(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	srv.Store.Sonos["sp1"] = &store.SonosSpeaker{
		ID: "sp1", Name: "Kitchen", IP: "10.0.0.5", UUID: "RINCON_AAA",
	}

	if !srv.Autoplay.Enabled("sp1") {
		t.Fatal("a speaker nobody has touched has autoplay off, want on")
	}

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodPut,
		"/api/sonos/sp1/autoplay", `{"enabled":false}`), http.StatusNoContent)
	if srv.Autoplay.Enabled("sp1") {
		t.Error("autoplay still on after opting out")
	}

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodPut,
		"/api/sonos/sp1/autoplay", `{"enabled":true}`), http.StatusNoContent)
	if !srv.Autoplay.Enabled("sp1") {
		t.Error("autoplay still off after turning it back on")
	}
}

// The setting is per group coordinator, like every other group-level control
// on this surface, so an unregistered id has to be refused rather than
// silently recorded against nothing.
func TestAutoplayRouteRefusesAnUnknownSpeaker(t *testing.T) {
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	mustStatus(t, doAs(t, srv, admin, pass, http.MethodPut,
		"/api/sonos/nope/autoplay", `{"enabled":false}`), http.StatusNotFound)
}
