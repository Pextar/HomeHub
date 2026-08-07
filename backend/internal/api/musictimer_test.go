package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"homehub/internal/store"
)

// The sleep gesture and the arithmetic behind a timer row. The engine's own
// work — playing, fading, restoring — is speaker I/O and is covered where the
// pieces live (media.Fade, store.ValidateMusicTimer); what is worth pinning
// here is the shape of the two things a person actually does.

func timerServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	srv, _ := actionServer(t)
	admin, pass := seedAdmin(t, srv)
	srv.Store.Sonos["sp1"] = &store.SonosSpeaker{
		ID: "sp1", Name: "Bedroom", IP: "10.0.0.5", UUID: "RINCON_AAA",
	}
	return srv, admin, pass
}

// "Forty minutes" is the whole of what someone in bed says. The backend does
// the arithmetic: the fade is the tail of the wait, not extra time on top, so
// the room is quiet at forty minutes rather than at forty-eight.
func TestSleepPutsTheRoomQuietWhenAsked(t *testing.T) {
	srv, admin, pass := timerServer(t)
	before := time.Now()

	rec := doAs(t, srv, admin, pass, http.MethodPost, "/api/media/timers/sleep",
		`{"room":"sonos:sp1","minutes":40}`)
	mustStatus(t, rec, http.StatusCreated)

	var got struct {
		Timer   store.MusicTimer `json:"timer"`
		QuietAt time.Time        `json:"quiet_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Timer.Action != store.MusicStop || !got.Timer.Enabled {
		t.Errorf("timer = %+v, want an enabled stop", got.Timer)
	}
	if got.Timer.FadeMinutes != 8 {
		t.Errorf("fade = %d, want a fifth of the wait", got.Timer.FadeMinutes)
	}
	if got.Timer.Volume == nil || *got.Timer.Volume != 0 {
		t.Errorf("volume = %v, want a fade down to nothing", got.Timer.Volume)
	}
	// The fade starts at 32 minutes so the room is quiet at 40.
	wantQuiet := before.Add(40 * time.Minute)
	if got.QuietAt.Sub(wantQuiet).Abs() > time.Minute {
		t.Errorf("quiet_at = %v, want about %v", got.QuietAt, wantQuiet)
	}
	if fadeStart := got.Timer.FiresAt; fadeStart.After(got.QuietAt) {
		t.Errorf("the fade starts at %v, after the room is meant to be quiet at %v", fadeStart, got.QuietAt)
	}
}

// Two sleep timers on one room is never what someone means; it is what
// happens when they tap the button again because they changed their mind.
func TestSleepReplacesTheRoomsExistingSleepTimer(t *testing.T) {
	srv, admin, pass := timerServer(t)

	for _, mins := range []string{"40", "20"} {
		mustStatus(t, doAs(t, srv, admin, pass, http.MethodPost, "/api/media/timers/sleep",
			`{"room":"sonos:sp1","minutes":`+mins+`}`), http.StatusCreated)
	}

	var timers []*store.MusicTimer
	srv.Store.View(func() {
		for _, mt := range srv.Store.MusicTimers {
			timers = append(timers, mt)
		}
	})
	if len(timers) != 1 {
		t.Fatalf("%d sleep timers on one room, want 1", len(timers))
	}
	if timers[0].FadeMinutes != 4 {
		t.Errorf("fade = %d, want the second request's", timers[0].FadeMinutes)
	}
}

// A fade longer than the wait is a request to turn the music down now, not a
// fade that started before the timer was set.
func TestSleepClampsAFadeLongerThanTheWait(t *testing.T) {
	srv, admin, pass := timerServer(t)
	before := time.Now()

	rec := doAs(t, srv, admin, pass, http.MethodPost, "/api/media/timers/sleep",
		`{"room":"sonos:sp1","minutes":5,"fade_minutes":30}`)
	mustStatus(t, rec, http.StatusCreated)

	var got struct {
		Timer store.MusicTimer `json:"timer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Timer.FadeMinutes != 5 {
		t.Errorf("fade = %d, want it clamped to the wait", got.Timer.FadeMinutes)
	}
	if got.Timer.FiresAt.Sub(before) > time.Minute {
		t.Errorf("fires_at = %v, want it to start about now", got.Timer.FiresAt)
	}
}

func TestSleepRefusesNonsense(t *testing.T) {
	srv, admin, pass := timerServer(t)
	for _, body := range []string{
		`{"room":"sonos:sp1","minutes":0}`,
		`{"room":"sonos:sp1","minutes":-5}`,
		`{"room":"sonos:sp1","minutes":1000}`,
	} {
		mustStatus(t, doAs(t, srv, admin, pass, http.MethodPost,
			"/api/media/timers/sleep", body), http.StatusBadRequest)
	}
	// A room that isn't in the house is caught by the validator, which is
	// what stops a timer firing into nothing at 06:45.
	mustStatus(t, doAs(t, srv, admin, pass, http.MethodPost, "/api/media/timers/sleep",
		`{"room":"sonos:gone","minutes":30}`), http.StatusBadRequest)
}

func TestMusicTimerCRUDRoundTrip(t *testing.T) {
	srv, admin, pass := timerServer(t)

	rec := doAs(t, srv, admin, pass, http.MethodPost, "/api/media/timers", `{
		"name": "Weekday wake-up",
		"room": "sonos:sp1",
		"action": "start",
		"enabled": true,
		"time": "06:45",
		"days": [1,2,3,4,5],
		"item": {"uri": "spotify:playlist:p", "title": "Morning", "kind": "playlist"},
		"volume": 20,
		"fade_minutes": 10
	}`)
	mustStatus(t, rec, http.StatusCreated)

	var made store.MusicTimer
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil {
		t.Fatal(err)
	}

	list := doAs(t, srv, admin, pass, http.MethodGet, "/api/media/timers", "")
	mustStatus(t, list, http.StatusOK)
	var rows []musicTimerView
	if err := json.Unmarshal(list.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows, want 1", len(rows))
	}
	if rows[0].RoomName != "Bedroom" {
		t.Errorf("room_name = %q, want the speaker's current name", rows[0].RoomName)
	}
	if rows[0].NextAt.IsZero() {
		t.Error("next_at is zero on an enabled recurring timer")
	}

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodPut, "/api/media/timers/"+made.ID, `{
		"room": "sonos:sp1", "action": "stop", "enabled": false, "time": "23:30"
	}`), http.StatusOK)

	srv.Store.View(func() {
		got := srv.Store.MusicTimers[made.ID]
		if got.Action != store.MusicStop || got.Enabled || got.Time != "23:30" {
			t.Errorf("after update = %+v, want the replacement", got)
		}
		if got.Item.URI != "" {
			t.Errorf("item = %+v, want it dropped by a stop", got.Item)
		}
	})

	mustStatus(t, doAs(t, srv, admin, pass, http.MethodDelete,
		"/api/media/timers/"+made.ID, ""), http.StatusNoContent)
	mustStatus(t, doAs(t, srv, admin, pass, http.MethodDelete,
		"/api/media/timers/"+made.ID, ""), http.StatusNotFound)
}

// "sleep" and "fade" must never be read as timer ids.
func TestMusicTimerFixedPathsBeatTheIDRoute(t *testing.T) {
	srv, admin, pass := timerServer(t)
	mustStatus(t, doAs(t, srv, admin, pass, http.MethodPost,
		"/api/media/timers/fade/cancel", `{"room":"sonos:sp1"}`), http.StatusOK)
}

// A room is "fading" only while a ramp is actually walking it. Every path
// that doesn't hand the room to a ramp — no fade asked for, or a failure
// before one could start — has to release it, or the room reads as fading for
// the life of the process and its cancel func is never called.
func TestFiringATimerWithNoRampLeavesNoRoomMarkedFading(t *testing.T) {
	srv, _, _ := timerServer(t)
	vol := 0
	// No speaker answers in a test, so this fails at the point it reaches
	// the network — which is exactly the "failed before a ramp started"
	// path that must still tidy up. The deadline is the test's own: without
	// one this waits out mediaTimeout against an address nobody is at.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	srv.fireMusicTimer(ctx, store.MusicTimer{
		ID: "mt_1", Room: "sonos:sp1", Action: store.MusicStop,
		Enabled: true, Volume: &vol,
	})

	if fading := srv.fadingRooms(); len(fading) != 0 {
		t.Errorf("rooms fading after a timer with no ramp: %v", fading)
	}
	if srv.CancelFade("sonos:sp1") {
		t.Error("a fade was still registered for the room")
	}
}

// A timer aimed at a room that no longer exists is reported, not silently
// dropped — and it must not leave a fade behind either.
func TestFiringATimerForAMissingRoomIsRecordedAsAnError(t *testing.T) {
	srv, _, _ := timerServer(t)
	srv.fireMusicTimer(t.Context(), store.MusicTimer{
		ID: "mt_1", Room: "sonos:gone", Action: store.MusicStop, Enabled: true,
	})

	entries := srv.Store.Activity.Recent(1)
	if len(entries) != 1 {
		t.Fatal("nothing was written to the activity log")
	}
	if entries[0].Status != "error" || entries[0].Source != "music-timer" {
		t.Errorf("entry = %+v, want a music-timer error row", entries[0])
	}
	if fading := srv.fadingRooms(); len(fading) != 0 {
		t.Errorf("rooms fading after a timer that could not resolve: %v", fading)
	}
}

func TestNextMusicFire(t *testing.T) {
	// 2026-03-16 is a Monday.
	monday := func(h, m int) time.Time {
		return time.Date(2026, 3, 16, h, m, 0, 0, time.Local)
	}
	weekday := func() *store.MusicTimer {
		return &store.MusicTimer{Enabled: true, Time: "06:45", Days: []int{1, 2, 3, 4, 5}}
	}

	t.Run("later today wins over the same weekday next week", func(t *testing.T) {
		got := nextMusicFire(weekday(), monday(5, 0))
		if !got.Equal(monday(6, 45)) {
			t.Errorf("next = %v, want this morning's 06:45", got)
		}
	})

	t.Run("past today's, the next matching day", func(t *testing.T) {
		got := nextMusicFire(weekday(), monday(7, 0))
		if !got.Equal(monday(6, 45).AddDate(0, 0, 1)) {
			t.Errorf("next = %v, want Tuesday's", got)
		}
	})

	t.Run("a weekday timer skips the weekend", func(t *testing.T) {
		friday := monday(7, 0).AddDate(0, 0, 4)
		got := nextMusicFire(weekday(), friday)
		if got.Weekday() != time.Monday {
			t.Errorf("next after Friday morning = %v, want a Monday", got)
		}
	})

	t.Run("every day means every day", func(t *testing.T) {
		daily := weekday()
		daily.Days = nil
		saturday := monday(7, 0).AddDate(0, 0, 5)
		if got := nextMusicFire(daily, saturday); got.Weekday() != time.Sunday {
			t.Errorf("next = %v, want Sunday", got)
		}
	})

	t.Run("a disabled timer is not going to fire", func(t *testing.T) {
		off := weekday()
		off.Enabled = false
		if got := nextMusicFire(off, monday(5, 0)); !got.IsZero() {
			t.Errorf("next = %v, want zero", got)
		}
	})

	t.Run("a one-shot fires when it says", func(t *testing.T) {
		at := monday(23, 30)
		once := &store.MusicTimer{Enabled: true, FiresAt: at}
		if got := nextMusicFire(once, monday(22, 0)); !got.Equal(at) {
			t.Errorf("next = %v, want %v", got, at)
		}
	})
}
