package app

import (
	"testing"
)

// testConfig is a complete, self-contained configuration: a fresh data
// directory, no listeners bound, no credentials to seed. Every optional
// integration reads its own environment and reports itself disabled, which is
// what a laptop and a CI runner both are.
func testConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{
		DataDir:  dir,
		SPADir:   t.TempDir(),
		HTTPPort: "8080",
	}
}

// New has to assemble a whole server without starting anything. If this ever
// binds a port, reaches a speaker or spawns a goroutine, a test suite and a
// --reset-admin run both pay for it.
func TestNewAssemblesWithoutStartingAnything(t *testing.T) {
	a, err := New(testConfig(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.store == nil || a.api == nil || a.music == nil || a.control == nil {
		t.Error("New returned an app with a subsystem missing")
	}
	if a.http == nil {
		t.Error("the HTTP listener was never prepared")
	}
	if a.https != nil {
		t.Error("an HTTPS listener was prepared without HTTPS_PORT")
	}
}

// The store reaches back into the rest of the application through five hooks.
// Every one of them is a feature that fails silently when it is not installed:
// an unwired OnMusic means no scene ever touches a speaker, an unwired
// MusicPlaying means no music rule ever fires, and nothing anywhere reports an
// error. This is the test that says they were wired.
func TestEveryStoreHookIsWired(t *testing.T) {
	a, err := New(testConfig(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if a.store.OnChange == nil {
		t.Error("OnChange is unwired: connected clients would never see a change")
	}
	if a.store.OnMusic == nil {
		t.Error("OnMusic is unwired: a scene could never quiet a room")
	}
	if a.store.MusicPlaying == nil {
		t.Error("MusicPlaying is unwired: a music rule could never fire")
	}
	if a.store.OnStateChange == nil {
		t.Error("OnStateChange is unwired: no push when a device switches")
	}
	if a.store.OnSensorAlert == nil {
		t.Error("OnSensorAlert is unwired: no push when a sensor crosses a threshold")
	}
}

// The read half of the music hooks must not claim to know about a room the
// house does not have — a rule that fires because the Wi-Fi hiccuped is the
// failure the third answer exists to prevent.
func TestMusicPlayingHookAnswersHonestly(t *testing.T) {
	a, err := New(testConfig(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, known := a.store.RoomPlaying("sonos:gone"); known {
		t.Error("the hook claimed to know about a room the house lacks")
	}
}

// HTTPS is opt-in, and asking for it has to produce a second listener on the
// same handler rather than a second application.
func TestHTTPSListenerIsPreparedWhenAsked(t *testing.T) {
	cfg := testConfig(t)
	cfg.HTTPSPort = "8443"
	cfg.TLSCert = cfg.DataDir + "/cert.pem"
	cfg.TLSKey = cfg.DataDir + "/key.pem"

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.https == nil {
		t.Fatal("HTTPS_PORT was set but no TLS listener was prepared")
	}
	if a.https.Handler == nil || a.http.Handler == nil {
		t.Fatal("a listener was prepared with no handler")
	}
	if len(a.https.TLSConfig.Certificates) == 0 {
		t.Error("the TLS listener has no certificate")
	}
}
