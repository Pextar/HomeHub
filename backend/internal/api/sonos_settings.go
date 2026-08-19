package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"homehub/internal/sonos"
)

// Per-speaker device settings: the half of the Sonos app that isn't playback.
// See internal/sonos/settings.go for which UPnP service owns what and why the
// EQ block is capability-probed rather than assumed.
//
// These are read on demand, not folded into the status poll — a speaker's bass
// doesn't change on its own, so paying eleven SOAP calls every five seconds to
// watch it not change would be absurd.

// sonosSettingsTimeout covers a full settings read. Eleven SOAP calls run in
// four parallel branches, so this is a wide margin rather than a tight one; a
// speaker that is merely slow shouldn't blank the sheet.
const sonosSettingsTimeout = 12 * time.Second

// sonosSettings handles GET /api/sonos/{id}/settings — one speaker's whole
// settings snapshot, including which of the model-dependent controls it
// actually supports.
func (s *Server) sonosSettings(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonosSettingsTimeout)
	defer cancel()
	settings, err := sonos.LoadSettings(ctx, sp.IP)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// sonosSettingsPatch is the writable half of a settings snapshot. Every field
// is a pointer so "not mentioned" and "set to zero/false" stay distinguishable
// — a bass of 0 and an absent bass mean very different things.
type sonosSettingsPatch struct {
	Bass        *int  `json:"bass"`
	Treble      *int  `json:"treble"`
	Loudness    *bool `json:"loudness"`
	NightMode   *bool `json:"night_mode"`
	DialogLevel *bool `json:"dialog_level"`
	SubEnabled  *bool `json:"sub_enabled"`
	SubGain     *int  `json:"sub_gain"`
	Surround    *bool `json:"surround"`
	LED         *bool `json:"led"`
	ButtonLock  *bool `json:"button_lock"`
	// SleepMinutes is group-scoped — send it to a coordinator. Zero cancels
	// a running timer.
	SleepMinutes *int `json:"sleep_minutes"`
}

// sonosUpdateSettings handles PUT /api/sonos/{id}/settings. Any subset of the
// writable fields may be sent; each one that is present becomes one SOAP call,
// applied in a fixed order.
//
// Fields are applied one at a time and the first refusal stops the run,
// reporting which setting failed. That is honest rather than tidy — a speaker
// can accept bass and refuse night mode, and the UI has to be told which. In
// practice the frontend sends a single field per interaction (a slider
// release, a switch tap), the same contract volume and mute already use, so a
// half-applied patch isn't a state the UI has to reason about.
func (s *Server) sonosUpdateSettings(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	var patch sonosSettingsPatch
	if !decodeBody(w, r, &patch) {
		return
	}

	// Range checks first, so a bad number is a 400 from us rather than a
	// clamped value the speaker silently accepts.
	if patch.Bass != nil && outOfRange(*patch.Bass, sonos.ToneMin, sonos.ToneMax) {
		writeError(w, http.StatusBadRequest, rangeMsg("bass", sonos.ToneMin, sonos.ToneMax))
		return
	}
	if patch.Treble != nil && outOfRange(*patch.Treble, sonos.ToneMin, sonos.ToneMax) {
		writeError(w, http.StatusBadRequest, rangeMsg("treble", sonos.ToneMin, sonos.ToneMax))
		return
	}
	if patch.SubGain != nil && outOfRange(*patch.SubGain, sonos.SubGainMin, sonos.SubGainMax) {
		writeError(w, http.StatusBadRequest, rangeMsg("sub gain", sonos.SubGainMin, sonos.SubGainMax))
		return
	}
	if patch.SleepMinutes != nil && outOfRange(*patch.SleepMinutes, 0, sonos.MaxSleepMinutes) {
		writeError(w, http.StatusBadRequest, rangeMsg("sleep timer", 0, sonos.MaxSleepMinutes))
		return
	}

	// One step per field present, in a fixed order. Each is named so a refusal
	// can say what the speaker refused rather than just "bad gateway". A step
	// is only built when its field is set, so no closure ever dereferences a
	// nil pointer.
	type step struct {
		name  string
		apply func(context.Context) error
	}
	var steps []step
	add := func(name string, present bool, fn func(context.Context) error) {
		if present {
			steps = append(steps, step{name, fn})
		}
	}
	add("bass", patch.Bass != nil,
		func(ctx context.Context) error { return sonos.SetBass(ctx, sp.IP, *patch.Bass) })
	add("treble", patch.Treble != nil,
		func(ctx context.Context) error { return sonos.SetTreble(ctx, sp.IP, *patch.Treble) })
	add("loudness", patch.Loudness != nil,
		func(ctx context.Context) error { return sonos.SetLoudness(ctx, sp.IP, *patch.Loudness) })
	add("night mode", patch.NightMode != nil,
		func(ctx context.Context) error { return sonos.SetNightMode(ctx, sp.IP, *patch.NightMode) })
	add("speech enhancement", patch.DialogLevel != nil,
		func(ctx context.Context) error { return sonos.SetDialogLevel(ctx, sp.IP, *patch.DialogLevel) })
	add("sub", patch.SubEnabled != nil,
		func(ctx context.Context) error { return sonos.SetSubEnabled(ctx, sp.IP, *patch.SubEnabled) })
	add("sub gain", patch.SubGain != nil,
		func(ctx context.Context) error { return sonos.SetSubGain(ctx, sp.IP, *patch.SubGain) })
	add("surround", patch.Surround != nil,
		func(ctx context.Context) error { return sonos.SetSurround(ctx, sp.IP, *patch.Surround) })
	add("status light", patch.LED != nil,
		func(ctx context.Context) error { return sonos.SetLED(ctx, sp.IP, *patch.LED) })
	add("touch controls", patch.ButtonLock != nil,
		func(ctx context.Context) error { return sonos.SetButtonLock(ctx, sp.IP, *patch.ButtonLock) })
	add("sleep timer", patch.SleepMinutes != nil,
		func(ctx context.Context) error { return sonos.SetSleepTimer(ctx, sp.IP, *patch.SleepMinutes) })

	if len(steps) == 0 {
		writeError(w, http.StatusBadRequest, "no settings in request")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sonosSettingsTimeout)
	defer cancel()
	for _, st := range steps {
		if err := st.apply(ctx); err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("%s: %s", st.name, err.Error()))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func outOfRange(v, lo, hi int) bool { return v < lo || v > hi }

func rangeMsg(what string, lo, hi int) string {
	return fmt.Sprintf("%s must be between %d and %d", what, lo, hi)
}

// sonosImage handles GET /api/sonos/{id}/image — a picture of this speaker
// model, proxied from the speaker itself.
//
// The image is whatever the device publishes in its own description's
// iconList; nothing here reaches out to Sonos' website or ships bundled
// artwork, so a speaker that offers no picture gets a 404 and the UI falls
// back to the striped placeholder rather than a stand-in that might be the
// wrong model. Proxied for the same reason album art is: the app may be
// served over HTTPS, where a plain-http image from the speaker is blocked.
func (s *Server) sonosImage(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.sonosSpeaker(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sonos.DefaultTimeout)
	defer cancel()

	path, err := s.Speakers.IconPath(ctx, sp.IP)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if path == "" {
		writeError(w, http.StatusNotFound, "this speaker publishes no picture of itself")
		return
	}

	u := fmt.Sprintf("http://%s:%d%s", sp.IP, sonos.Port, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("speaker returned HTTP %d", resp.StatusCode))
		return
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	// A model's picture never changes; let the browser keep it.
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 2<<20))
}
