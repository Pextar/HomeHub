package api

// Starting music on a KEF speaker.
//
// This is the one KEF capability that doesn't come off the speaker's own API.
// That API has transport control — play, pause, skip — but nothing that takes
// content: no queue to load, no URI to set, no favorites. Pressing "play" on a
// speaker with nothing loaded does nothing, which is why the Music view could
// control a KEF speaker but never start one.
//
// Spotify Connect is what fills the gap. The speaker registers itself with the
// Spotify account it was signed in to, and the Web API's player endpoints can
// point playback at it — so a search result becomes music on the speaker with
// the same one tap as on Sonos, taking a different road to get there:
//
//	Sonos: HomeHub → speaker (SOAP) → speaker streams with the household's
//	       linked account. The command never leaves the LAN.
//	KEF:   HomeHub → Spotify (Web API) → speaker streams the same way. The
//	       command goes out to Spotify's cloud and back.
//
// Consequences that shape the endpoints below, all of them the "stay honest
// about the backend" rule from DESIGN.md §15:
//
//   - It needs the *user's* Spotify account (Premium, and the two player
//     scopes), not the speaker's. A login made before those scopes existed
//     searches fine and cannot play — hence Status.Playback.
//   - The speaker must be awake and on Wi-Fi to be a Connect device at all,
//     so playing wakes it first over the local API.
//   - Which Connect device *is* this speaker has to be resolved. Its name
//     normally matches, and when it doesn't the user pins one.
//   - Only Spotify works here. Anything else the Search screen might grow
//     later has no Connect equivalent, and this file must not pretend it does.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/kef"
	"homehub/internal/spotify"
	"homehub/internal/store"
)

// kefPlayItem handles POST /api/kef/{id}/play-item with
// {"service":"Spotify","uri":"spotify:track:…","title":"…"} — the same body
// the Sonos endpoint takes, so the Search screen sends one shape to either
// bridge.
func (s *Server) kefPlayItem(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.kefSpeaker(w, r)
	if !ok {
		return
	}
	var body struct {
		Service string `json:"service"`
		URI     string `json:"uri"`
		Title   string `json:"title"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Service == "" {
		body.Service = "Spotify"
	}
	if !strings.EqualFold(body.Service, "Spotify") {
		writeError(w, http.StatusBadRequest, "KEF speakers can only be started from Spotify")
		return
	}
	if strings.TrimSpace(body.URI) == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	if !s.requireSpotify(w) {
		return
	}
	// Check the credentials before touching the speaker. Both of these are
	// local reads, and waking a speaker only to report that there is no
	// Spotify account behind it is a worse answer than saying so up front.
	st := s.Spotify.Status()
	if !st.Connected {
		writeError(w, http.StatusConflict,
			"spotify: not connected — link your Spotify account on the Search screen")
		return
	}
	if !st.Playback {
		writeError(w, http.StatusConflict, spotify.ErrPlaybackScope.Error())
		return
	}

	// Wake the speaker onto Wi-Fi first. A speaker in standby — or sitting on
	// its optical input — is not a Connect device, so without this the play
	// would fail with "device not found" for a speaker that is right there.
	// Selecting the source it is already on is a no-op on the speaker.
	kefCtx, cancelKEF := context.WithTimeout(r.Context(), kef.DefaultTimeout)
	err := kef.SetSource(kefCtx, sp.IP, kef.SourceWiFi)
	cancelKEF()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Two cloud round-trips: list the devices to find this speaker, then
	// start it. The device list is a snapshot of what is awake right now, so
	// it is read per play rather than cached. The budget covers both, plus
	// the one retry a just-woken speaker can need.
	ctx, cancel := context.WithTimeout(r.Context(), 3*spotifyTimeout)
	defer cancel()
	dev, err := s.kefConnectDeviceWaking(ctx, sp)
	if err != nil {
		writeError(w, kefSpotifyStatus(err), err.Error())
		return
	}
	if err := s.Spotify.PlayOn(ctx, dev.ID, body.URI); err != nil {
		writeError(w, kefSpotifyStatus(err), err.Error())
		return
	}
	// Spotify has accepted the command; the speaker has not necessarily
	// started yet, since the audio comes back to it from the cloud. So two
	// re-reads: the usual prompt one, and a later one for the handoff. Each
	// pushes the `music` signal if it found a change, which is what moves the
	// caller's now-playing off "nothing playing".
	s.kefEvents().Touch(sp.ID)
	s.kefEvents().TouchAfter(sp.ID, 3*time.Second)
	w.WriteHeader(http.StatusNoContent)
}

// kefSpotifyView is what the settings pane needs to explain and change the
// pairing: which device would be used, whether that came from a pin or from
// the name, and everything else on offer.
type kefSpotifyView struct {
	// Pinned is the stored choice, empty when the speaker is matched by name.
	PinnedID   string `json:"pinned_id,omitempty"`
	PinnedName string `json:"pinned_name,omitempty"`
	// Device is what a play would use right now, nil when nothing matches.
	Device *spotify.Device `json:"device,omitempty"`
	// Reason explains a nil Device in the words the user needs to act on.
	Reason  string           `json:"reason,omitempty"`
	Devices []spotify.Device `json:"devices"`
}

// kefSpotifyDevices handles GET /api/kef/{id}/spotify — the Connect pairing
// for one speaker plus the account's visible devices, for the picker.
func (s *Server) kefSpotifyDevices(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.kefSpeaker(w, r)
	if !ok {
		return
	}
	if !s.requireSpotify(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancel()
	devices, err := s.Spotify.Devices(ctx)
	if err != nil {
		writeError(w, kefSpotifyStatus(err), err.Error())
		return
	}
	view := kefSpotifyView{
		PinnedID:   sp.SpotifyDeviceID,
		PinnedName: sp.SpotifyDeviceName,
		Devices:    devices,
	}
	if dev, err := matchConnectDevice(sp, devices); err == nil {
		view.Device = &dev
	} else {
		view.Reason = err.Error()
	}
	writeJSON(w, http.StatusOK, view)
}

// kefSetSpotifyDevice handles PUT /api/kef/{id}/spotify with
// {"device_id":"…","device_name":"…"} — pin which Connect device this speaker
// is. An empty device_id clears the pin and goes back to matching by name.
func (s *Server) kefSetSpotifyDevice(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		DeviceID   string `json:"device_id"`
		DeviceName string `json:"device_name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	var existing *store.KEFSpeaker
	if !s.update(w, func() error {
		var ok bool
		existing, ok = s.Store.KEF[id]
		if !ok {
			return errStatus(http.StatusNotFound, "speaker not found")
		}
		merged := *existing
		merged.SpotifyDeviceID = body.DeviceID
		merged.SpotifyDeviceName = body.DeviceName
		if err := s.Store.ValidateKEFSpeaker(&merged); err != nil {
			return errInvalid(err)
		}
		*existing = merged
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// kefConnectDeviceWaking resolves the speaker's Connect device the way a play
// needs to: a speaker woken a moment ago takes a second or two to register
// itself with Spotify, so the first listing can legitimately miss it. One
// retry turns "tapped a sleeping speaker" from an error into a short pause.
// The settings read uses the plain resolve — it is reporting, not starting.
func (s *Server) kefConnectDeviceWaking(ctx context.Context, sp store.KEFSpeaker) (spotify.Device, error) {
	dev, err := s.kefConnectDevice(ctx, sp)
	if !errors.Is(err, errNoConnectDevice) {
		return dev, err
	}
	select {
	case <-time.After(1500 * time.Millisecond):
	case <-ctx.Done():
		return spotify.Device{}, ctx.Err()
	}
	return s.kefConnectDevice(ctx, sp)
}

// kefConnectDevice resolves which Connect device a speaker is, listing the
// account's devices to do it.
func (s *Server) kefConnectDevice(ctx context.Context, sp store.KEFSpeaker) (spotify.Device, error) {
	devices, err := s.Spotify.Devices(ctx)
	if err != nil {
		return spotify.Device{}, err
	}
	return matchConnectDevice(sp, devices)
}

// matchConnectDevice picks the device for a speaker out of a device list. Kept
// separate from the fetch so the rules are testable, since they are the part
// that decides whether a tap plays in the right room.
//
// A pinned id wins outright. Otherwise the speaker is matched by name — the
// pinned name first (a pin whose id rotated, which Spotify does when a device
// re-registers), then the speaker's own name. Nothing is guessed beyond that:
// starting music in the wrong room is worse than saying which speaker to pick.
func matchConnectDevice(sp store.KEFSpeaker, devices []spotify.Device) (spotify.Device, error) {
	byID := func(id string) (spotify.Device, bool) {
		for _, d := range devices {
			if d.ID == id {
				return d, true
			}
		}
		return spotify.Device{}, false
	}
	if sp.SpotifyDeviceID != "" {
		if d, ok := byID(sp.SpotifyDeviceID); ok {
			return usable(d)
		}
	}
	for _, want := range []string{sp.SpotifyDeviceName, sp.Name} {
		if normalizeDeviceName(want) == "" {
			continue
		}
		for _, d := range devices {
			if normalizeDeviceName(d.Name) == normalizeDeviceName(want) {
				return usable(d)
			}
		}
	}
	// Nothing matched. Which of the two failures it is changes what the user
	// should do about it, so they get different sentences.
	if sp.SpotifyDeviceID != "" {
		name := sp.SpotifyDeviceName
		if name == "" {
			name = sp.Name
		}
		return spotify.Device{}, fmt.Errorf(
			"%w: %q isn't visible to Spotify right now — wake the speaker, or pick it again under its settings",
			errNoConnectDevice, name)
	}
	return spotify.Device{}, fmt.Errorf(
		"%w: no Spotify Connect speaker is called %q — play to it once from the Spotify app, then pick it under the speaker's settings",
		errNoConnectDevice, sp.Name)
}

// usable rejects a matched device that would refuse the command anyway, so
// the failure names the reason instead of arriving as a silent no-op.
func usable(d spotify.Device) (spotify.Device, error) {
	if d.Restricted {
		return spotify.Device{}, fmt.Errorf("%w: Spotify won't let other apps control %q",
			errNoConnectDevice, d.Name)
	}
	if d.ID == "" {
		return spotify.Device{}, fmt.Errorf("%w: %q has no Spotify device id", errNoConnectDevice, d.Name)
	}
	return d, nil
}

// errNoConnectDevice marks "this speaker isn't a playable Connect device
// right now" — a state the user can fix, so it answers 409 rather than 502.
var errNoConnectDevice = errors.New("spotify")

// normalizeDeviceName folds the differences between what a speaker calls
// itself and what it registered with Spotify: case, surrounding space, and
// runs of whitespace ("Living  Room" vs "Living Room").
func normalizeDeviceName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// kefSpotifyStatus maps a Spotify-side failure to a status code. Everything
// the user can act on — connect, reconnect for the player scopes, pick a
// device — is a 409, which is what the frontend keys its prompts off. A
// refusal from Spotify itself stays a bad gateway.
func kefSpotifyStatus(err error) int {
	switch {
	case errors.Is(err, spotify.ErrNotConnected),
		errors.Is(err, spotify.ErrPlaybackScope),
		errors.Is(err, errNoConnectDevice):
		return http.StatusConflict
	}
	return http.StatusBadGateway
}
