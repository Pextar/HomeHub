package api

// The Spotify Connect picker: everywhere this account can play, where it is
// playing now, and moving it.
//
// This is a *remote control*, and keeping that straight is what stops it
// colliding with the rest of the Music view. Everywhere else in HomeHub, the
// speakers are the subject: a zone is a set of them, a route is how content
// reaches them, and HomeHub decides. Here the subject is the account's single
// playback session — one per Spotify account, wherever in the house or the
// world it currently is — and HomeHub is only asking Spotify to move it.
//
// Two consequences worth stating, because both surface in the wording:
//
//   - Devices here are Spotify's, not HomeHub's. A phone in someone's pocket
//     is on this list; a Sonos that HomeHub drives over SOAP is not, because
//     it plays without a Connect session at all. The overlap is partial and
//     the list says which is which rather than pretending the two inventories
//     are one.
//   - Moving the session takes it away from wherever it was. If HomeHub is
//     the decoder for a room right now, transferring to a phone stops that
//     room. Said before the tap, in the response's own fields, rather than
//     discovered as the music stopping.

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"homehub/internal/spotify"
)

// connectTimeout bounds a Connect call. These are cloud round trips, and a
// transfer waits on a device answering, so it is longer than a search.
const connectTimeout = 15 * time.Second

// connectDevice is one Connect endpoint as the picker shows it.
type connectDevice struct {
	spotify.Device
	// HomeHub marks HomeHub's own decoder, which registers as a Connect
	// device whenever it is feeding a room. Without this it looks like a
	// mysterious extra speaker with the household's name on it — and
	// transferring *to* it does nothing useful, because HomeHub starts that
	// session itself when a zone plays.
	HomeHub bool `json:"homehub"`
	// Speaker is the HomeHub speaker this device is, when one is pinned or
	// the names match. It is what lets the picker say "this is the Study
	// KEF" rather than listing the same box twice under two names.
	Speaker string `json:"speaker,omitempty"`
}

// spotifyConnect handles GET /api/spotify/connect.
func (s *Server) spotifyConnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	acct := s.spotifyAccount(r)
	if st := acct.Status(); !st.Playback {
		// The grant that reads the player is the same one that moves it, so
		// a login without it cannot do anything on this screen. Said as the
		// one sentence that fixes it rather than as an empty list.
		writeError(w, http.StatusConflict, spotify.ErrPlaybackScope.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), connectTimeout)
	defer cancel()

	devices, err := acct.Devices(ctx)
	if err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}
	// Read after the device list rather than concurrently: they are two
	// views of one session, and a transfer landing between them would be
	// less confusing read in this order — the playback is the newer fact.
	playing, err := acct.Playback(ctx)
	if err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"devices": s.describeConnectDevices(devices),
		// Absent rather than empty when the account is playing nothing: an
		// idle household is most of the day, and it is a state, not a gap.
		"playing": playing,
		// What a transfer would interrupt, named before the tap. Empty when
		// HomeHub is not feeding anything.
		"interrupts": s.connectInterrupts(playing),
	})
}

// describeConnectDevices annotates the raw list with what HomeHub knows about
// the boxes on it.
func (s *Server) describeConnectDevices(devices []spotify.Device) []connectDevice {
	decoderName := strings.TrimSpace(s.Audio.DecoderName())

	// Which Connect device is which HomeHub speaker is a question the KEF
	// bridge already answers — a pinned id, or the name the speaker
	// registers under (see kef_spotify.go). That answer is reused rather
	// than re-derived, so the two surfaces cannot disagree about the same
	// box. Keyed by device id and by lower-cased name, because a pin may be
	// either and an unpinned speaker matches on its own name.
	byID := map[string]string{}
	byName := map[string]string{}
	s.Store.View(func() {
		for _, sp := range s.Store.KEF {
			if sp.SpotifyDeviceID != "" {
				byID[sp.SpotifyDeviceID] = sp.Name
			}
			if n := strings.ToLower(strings.TrimSpace(sp.SpotifyDeviceName)); n != "" {
				byName[n] = sp.Name
			}
			byName[strings.ToLower(sp.Name)] = sp.Name
		}
	})

	out := make([]connectDevice, 0, len(devices))
	for _, d := range devices {
		cd := connectDevice{Device: d}
		cd.HomeHub = decoderName != "" && strings.EqualFold(d.Name, decoderName)
		if name, ok := byID[d.ID]; ok {
			cd.Speaker = name
		} else if name, ok := byName[strings.ToLower(d.Name)]; ok {
			cd.Speaker = name
		}
		out = append(out, cd)
	}
	return out
}

// connectInterrupts names what HomeHub is currently feeding, so the picker can
// warn before a transfer takes the session away from it.
//
// Only the routes HomeHub decodes for are at risk: those hold the account's
// single session. A Sonos streaming from its own account link keeps playing
// whatever a phone does, and warning about it would be a lie in the other
// direction.
func (s *Server) connectInterrupts(playing *spotify.Playback) string {
	decoder := strings.TrimSpace(s.Audio.DecoderName())
	if decoder == "" || playing == nil || !strings.EqualFold(playing.DeviceName, decoder) {
		return ""
	}
	names := s.zonesUsingTheDecoder()
	switch len(names) {
	case 0:
		// The decoder holds the session but no zone is registered against
		// it — a session left over from a previous run, say. Still worth
		// naming honestly rather than claiming a room.
		return "what HomeHub is decoding"
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// zonesUsingTheDecoder names the zones whose live session is one HomeHub is
// decoding for, so a warning can say which rooms a transfer would silence.
func (s *Server) zonesUsingTheDecoder() []string {
	ids := s.Music.DecodedZones()
	var names []string
	s.Store.View(func() {
		for _, id := range ids {
			if z, ok := s.Store.Zones[id]; ok {
				names = append(names, z.Name)
			}
		}
	})
	sort.Strings(names)
	return names
}

// spotifyConnectTransfer handles PUT /api/spotify/connect/transfer with
// {"device_id":"…","play":true}.
func (s *Server) spotifyConnectTransfer(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	var body struct {
		DeviceID string `json:"device_id"`
		// Play defaults to true: a user who picked a device wants the music
		// there, and landing it paused would be a second tap for the common
		// case. Sent explicitly as false to move it quietly.
		Play *bool `json:"play"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.DeviceID) == "" {
		writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	play := true
	if body.Play != nil {
		play = *body.Play
	}

	ctx, cancel := context.WithTimeout(r.Context(), connectTimeout)
	defer cancel()
	if err := s.spotifyAccount(r).Transfer(ctx, body.DeviceID, play); err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}

	// A transfer away from HomeHub's decoder ends the zone sessions it was
	// feeding. Spotify has already taken the audio; leaving HomeHub's own
	// bookkeeping claiming those rooms are playing would have the Music view
	// showing a stream nobody is receiving.
	s.releaseDecodedZones()
	w.WriteHeader(http.StatusNoContent)
}

// spotifyConnectVolume handles PUT /api/spotify/connect/volume with
// {"device_id":"…","level":40}.
func (s *Server) spotifyConnectVolume(w http.ResponseWriter, r *http.Request) {
	if !s.requireSpotify(w) {
		return
	}
	var body struct {
		DeviceID string `json:"device_id"`
		Level    *int   `json:"level"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.DeviceID) == "" {
		writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	if body.Level == nil {
		writeError(w, http.StatusBadRequest, "level is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), connectTimeout)
	defer cancel()
	if err := s.spotifyAccount(r).SetDeviceVolume(ctx, body.DeviceID, *body.Level); err != nil {
		writeError(w, spotifyErrStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// releaseDecodedZones ends the zone sessions HomeHub was decoding for, after
// something else took the account's playback session away.
func (s *Server) releaseDecodedZones() {
	for _, id := range s.Music.DecodedZones() {
		s.Music.EndSession(id)
	}
}
