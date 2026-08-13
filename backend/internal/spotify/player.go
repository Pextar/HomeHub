package spotify

// Reading and moving the account's playback session.
//
// Spotify's player model is the thing to keep in mind here: an account has
// **one** active playback session, wherever it is. A phone, a laptop, a
// speaker, HomeHub's own decoder — they are all candidates, and exactly one of
// them holds the session at a time. Everything in this file is about that one
// session: where it is, what it is playing, and moving it somewhere else.
//
// That is also why moving it is a heavier action than it looks. Transferring
// playback to a phone stops whatever the session was driving, which may be a
// room full of speakers HomeHub is feeding. Callers are expected to say so
// before doing it, not after.

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Playback is the account's current session, as much of it as Spotify says.
//
// A nil *Playback from Playback() is not an error: it is the ordinary answer
// when the account is playing nothing anywhere, which Spotify signals with a
// 204. Callers must render that as "nothing is playing" rather than as a
// failure, because it is the state an idle household is in most of the day.
type Playback struct {
	// DeviceID and DeviceName are where the session currently is. Empty
	// when Spotify reports a session with no device attached, which happens
	// briefly during a transfer.
	DeviceID   string `json:"device_id,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	Playing    bool   `json:"playing"`
	// Item is the track, when there is one. Absent for an ad or a podcast
	// episode Spotify declines to describe.
	Item *Item `json:"item,omitempty"`
	// ProgressMS and DurationMS are zero when unknown. Milliseconds,
	// matching what the rest of the media surfaces already use.
	ProgressMS int64 `json:"progress_ms,omitempty"`
	DurationMS int64 `json:"duration_ms,omitempty"`
	// At is when this reading was taken, so a client extrapolating the
	// progress bar knows how stale its starting point is — the same
	// contract media.NowPlaying keeps.
	At time.Time `json:"at"`
	// Volume is the active device's level, 0-100, and -1 when the device
	// has no volume of its own (some car and TV integrations). Not a
	// pointer: a client that renders -1 as "no slider" cannot accidentally
	// render it as silence, which a nil-turned-zero would.
	Volume int `json:"volume"`
}

// Playback reads where the account's session is and what it is doing.
//
// Returns (nil, nil) when nothing is playing anywhere.
func (a *Account) Playback(ctx context.Context) (*Playback, error) {
	if err := a.requirePlayback(); err != nil {
		return nil, err
	}

	var raw struct {
		Device struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			VolumePercent *int   `json:"volume_percent"`
		} `json:"device"`
		IsPlaying  bool  `json:"is_playing"`
		ProgressMS int64 `json:"progress_ms"`
		Item       *struct {
			Name       string `json:"name"`
			URI        string `json:"uri"`
			DurationMS int64  `json:"duration_ms"`
			Artists    []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Name   string `json:"name"`
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"album"`
		} `json:"item"`
	}

	// A 204 is Spotify saying "nothing is playing", which apiGet leaves as an
	// untouched target rather than reporting as a parse error — so an empty
	// device with no item is the signal, and it needs no special case here.
	if err := a.apiGet(ctx, "/me/player", a.market(), &raw); err != nil {
		return nil, err
	}
	if raw.Device.ID == "" && raw.Item == nil {
		return nil, nil
	}

	pb := &Playback{
		DeviceID:   raw.Device.ID,
		DeviceName: strings.TrimSpace(raw.Device.Name),
		Playing:    raw.IsPlaying,
		ProgressMS: raw.ProgressMS,
		At:         time.Now(),
		Volume:     -1,
	}
	if raw.Device.VolumePercent != nil {
		pb.Volume = *raw.Device.VolumePercent
	}
	if raw.Item != nil {
		names := make([]string, 0, len(raw.Item.Artists))
		for _, ar := range raw.Item.Artists {
			if n := strings.TrimSpace(ar.Name); n != "" {
				names = append(names, n)
			}
		}
		item := Item{
			Kind: "track",
			URI:  raw.Item.URI,
			Name: strings.TrimSpace(raw.Item.Name),
			Sub:  strings.Join(names, ", "),
		}
		if len(raw.Item.Album.Images) > 0 {
			item.ArtURL = raw.Item.Album.Images[0].URL
		}
		pb.Item = &item
		pb.DurationMS = raw.Item.DurationMS
	}
	return pb, nil
}

// Transfer moves the account's playback session to one device.
//
// `play` is what to do on arrival: true resumes, false lands the session there
// paused. Spotify treats it as a hint rather than a promise — a device that
// was handed a paused session sometimes starts anyway — so a caller must read
// the state back rather than assume.
//
// The device must be one Spotify currently lists. A speaker that has gone to
// sleep since the list was read produces the same "not ready" answer PlayOn
// handles, and is retried once for the same reason: the difference between
// "didn't work" and "took a second".
func (a *Account) Transfer(ctx context.Context, deviceID string, play bool) error {
	if strings.TrimSpace(deviceID) == "" {
		return errors.New("spotify: no Connect device to move playback to")
	}
	if err := a.requirePlayback(); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{
		"device_ids": []string{deviceID},
		"play":       play,
	})
	if err != nil {
		return err
	}

	err = a.apiPut(ctx, "/me/player", nil, body)
	if errors.Is(err, errDeviceNotReady) {
		select {
		case <-time.After(700 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		err = a.apiPut(ctx, "/me/player", nil, body)
		if errors.Is(err, errDeviceNotReady) {
			return errors.New("spotify: the device didn't pick it up — wake it and try again")
		}
	}
	return err
}

// SetDeviceVolume sets one Connect device's volume, 0-100.
//
// Not every device has one: phones report their own, speakers report theirs,
// and some integrations have none at all and answer 403. That refusal is
// passed through rather than swallowed — a slider that moves and changes
// nothing is worse than one that says it cannot.
func (a *Account) SetDeviceVolume(ctx context.Context, deviceID string, level int) error {
	if strings.TrimSpace(deviceID) == "" {
		return errors.New("spotify: no Connect device to set the volume on")
	}
	if err := a.requirePlayback(); err != nil {
		return err
	}
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	q := url.Values{
		"volume_percent": {strconv.Itoa(level)},
		"device_id":      {deviceID},
	}
	return a.apiPut(ctx, "/me/player/volume", q, nil)
}

// Playback reads the default (household) account's session. One of the two
// Client-level wrappers here, which mirror the shape Devices and PlayOn
// already have: a household with one login never names it.
func (c *Client) Playback(ctx context.Context) (*Playback, error) { return c.For("").Playback(ctx) }

// Transfer moves the default (household) account's session to one device.
func (c *Client) Transfer(ctx context.Context, deviceID string, play bool) error {
	return c.For("").Transfer(ctx, deviceID, play)
}
