// Package kef provides a client for the local HTTP API on KEF's wireless
// speakers — LS50 Wireless II, LSX II / LSX II LT, and LS60 Wireless.
//
// Like the Sonos bridge next door this is entirely local: the speaker serves
// a small JSON API on port 80 and the KEF Connect app talks to it the same
// way, so playback, volume, source and the DSP settings all work on the LAN
// with no cloud account.
//
// The API is two calls wide. Everything is a *path* in a settings tree that
// is read with GET /api/getData and written with POST /api/setData, and
// every value is wrapped in a one-key envelope naming its own type:
//
//	GET  /api/getData?path=player:volume&roles=value
//	  → [{"type":"i32_","i32_":45}]
//	POST /api/setData
//	  {"path":"player:volume","role":"value","value":{"type":"i32_","i32_":45}}
//
// So this file is mostly two generic helpers (getValue/setValue) plus one
// small typed accessor per path we care about. Where Sonos needs SOAP
// envelopes and DIDL parsing, KEF needs neither.
//
// Deliberately *not* covered, because the speaker's API doesn't back it:
// grouping (KEF speakers have no zone concept — a pair is one speaker),
// queue management, and favorites. See DESIGN.md §15: don't build UI for
// capabilities the bridge can't back.
package kef

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout caps how long we wait for a speaker to answer one call.
const DefaultTimeout = 5 * time.Second

// Port is the port KEF speakers serve their JSON API on.
const Port = 80

// maxBody caps a response read. The largest thing the speaker returns is a
// now-playing record with a long stream URL; kilobytes, not megabytes.
const maxBody = 1 << 20

// ValidateHost checks that host is a bare hostname or IP that is safe to
// interpolate into http://<host>/… . Mirrors sonos.ValidateHost: it rejects
// anything that could redirect the server-side request away from the
// intended device, and IP literals pointing at sensitive targets. KEF
// speakers live on the LAN, so private ranges are intentionally allowed.
func ValidateHost(host string) error {
	h := strings.TrimSpace(host)
	if h == "" {
		return errors.New("speaker address is empty")
	}
	if strings.ContainsAny(h, "/?#@\\ \t\r\n:") {
		// No port is accepted — the API is always on :80.
		return fmt.Errorf("invalid speaker address %q", host)
	}
	if parsed := net.ParseIP(h); parsed != nil {
		if parsed.IsLoopback() || parsed.IsLinkLocalUnicast() ||
			parsed.IsLinkLocalMulticast() || parsed.IsUnspecified() || parsed.IsMulticast() {
			return fmt.Errorf("speaker address %q is not an allowed address", host)
		}
		return nil
	}
	for _, c := range h {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '.'
		if !ok {
			return fmt.Errorf("invalid speaker address %q", host)
		}
	}
	return nil
}

// ── Value envelope ───────────────────────────────────────────────────────

// value is the speaker's self-describing wrapper. "type" names the key the
// payload itself sits under, so {"type":"i32_","i32_":45} and
// {"type":"bool_","bool_":true} are both one of these. Decoding keeps the
// raw payload and unwraps it on demand: a caller that asked for an int
// shouldn't fail because some other field of the same response was a string.
//
// Composite values break that shape: player:player/data answers
// {"type":"playerData","state":"playing","trackRoles":{…}} — the fields sit
// beside the type name rather than under it. So the payload is "whatever
// `type` names, or the whole object when it names nothing".
type value struct {
	Type string
	Raw  json.RawMessage
}

// Envelope type names. The trailing underscores are the speaker's, not ours.
const (
	typeI32           = "i32_"
	typeI64           = "i64_"
	typeBool          = "bool_"
	typeString        = "string_"
	typeSource        = "kefPhysicalSource"
	typeSpeakerStatus = "kefSpeakerStatus"
	typeStandbyMode   = "kefStandbyMode"
)

// UnmarshalJSON reads the envelope and keeps the payload under whichever key
// "type" names.
func (v *value) UnmarshalJSON(b []byte) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	raw, ok := obj["type"]
	if !ok {
		return errors.New("kef: value has no type")
	}
	if err := json.Unmarshal(raw, &v.Type); err != nil {
		return fmt.Errorf("kef: value type: %w", err)
	}
	if payload, ok := obj[v.Type]; ok {
		v.Raw = payload
		return nil
	}
	// A composite value: the object itself is the payload.
	v.Raw = append(json.RawMessage(nil), b...)
	return nil
}

// MarshalJSON rebuilds the envelope for a setData body.
func (v value) MarshalJSON() ([]byte, error) {
	if v.Type == "" {
		return nil, errors.New("kef: value has no type")
	}
	raw := v.Raw
	if raw == nil {
		raw = json.RawMessage("null")
	}
	var b bytes.Buffer
	b.WriteByte('{')
	b.WriteString(`"type":`)
	name, err := json.Marshal(v.Type)
	if err != nil {
		return nil, err
	}
	b.Write(name)
	b.WriteByte(',')
	b.Write(name)
	b.WriteByte(':')
	b.Write(raw)
	b.WriteByte('}')
	return b.Bytes(), nil
}

// newValue builds an envelope of the given type around v.
func newValue(typ string, v any) (value, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return value{}, fmt.Errorf("kef: encode value: %w", err)
	}
	return value{Type: typ, Raw: raw}, nil
}

// decode unwraps the payload into dst. A mismatched envelope type is an
// error rather than a zero value: reading a bool where the speaker sent a
// string means we asked for the wrong path, and silently returning false
// would put a lie on screen.
func (v value) decode(want string, dst any) error {
	if v.Type != want {
		return fmt.Errorf("kef: expected %s, got %s", want, v.Type)
	}
	if len(v.Raw) == 0 {
		return fmt.Errorf("kef: %s value is empty", want)
	}
	if err := json.Unmarshal(v.Raw, dst); err != nil {
		return fmt.Errorf("kef: decode %s: %w", want, err)
	}
	return nil
}

// ── HTTP plumbing ────────────────────────────────────────────────────────

// client is the shared HTTP client. Its own transport rather than
// http.DefaultClient's so the event long-poll (monitor.go) can hold
// connections open per speaker without starving the rest of the process.
var client = &http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

// getValue reads one path's "value" role off the speaker.
func getValue(ctx context.Context, ip, path string) (value, error) {
	if err := ValidateHost(ip); err != nil {
		return value{}, fmt.Errorf("kef: %w", err)
	}
	u := fmt.Sprintf("http://%s/api/getData?path=%s&roles=value", ip, url.QueryEscape(path))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return value{}, fmt.Errorf("kef: build request: %w", err)
	}
	raw, err := do(req, ip, path)
	if err != nil {
		return value{}, err
	}
	// getData answers with a one-element array. Older firmware answers with
	// the bare object, so both shapes are accepted.
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var vals []value
		if err := json.Unmarshal(trimmed, &vals); err != nil {
			return value{}, fmt.Errorf("kef: %s: parse %s: %w", ip, path, err)
		}
		if len(vals) == 0 {
			return value{}, fmt.Errorf("kef: %s returned nothing for %s", ip, path)
		}
		return vals[0], nil
	}
	var v value
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return value{}, fmt.Errorf("kef: %s: parse %s: %w", ip, path, err)
	}
	return v, nil
}

// setValue writes one path's stored value: a type envelope under the
// "value" role, which is how every setting in the tree is written.
func setValue(ctx context.Context, ip, path string, v value) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("kef: encode value: %w", err)
	}
	return setRaw(ctx, ip, path, "value", raw)
}

// setRaw performs a setData with an already-encoded value. Settings go
// through setValue above; the transport control path comes here directly
// because it is an *action* rather than stored state — its role is
// "activate" and its payload is a bare {"control":"…"} with no type
// envelope, which is the one place this API breaks its own pattern.
//
// The field naming the role is "roles" — plural, as on the read side's query
// string, because a request may name more than one. The singular reads
// better and is not what the speaker looks for: sent that way, the transport
// control never reached the player and came back HTTP 500.
func setRaw(ctx context.Context, ip, path, roles string, val json.RawMessage) error {
	if err := ValidateHost(ip); err != nil {
		return fmt.Errorf("kef: %w", err)
	}
	body, err := json.Marshal(struct {
		Path  string          `json:"path"`
		Roles string          `json:"roles"`
		Value json.RawMessage `json:"value"`
	}{path, roles, val})
	if err != nil {
		return fmt.Errorf("kef: encode request: %w", err)
	}
	u := fmt.Sprintf("http://%s/api/setData", ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("kef: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	_, err = do(req, ip, path)
	return err
}

// do performs a request and returns the body, mapping transport and HTTP
// errors into messages that name the speaker and the path.
func do(req *http.Request, ip, path string) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kef: %s %s: %w", ip, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("kef: %s: read response: %w", ip, err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("kef: %s refused %s (HTTP %d)", ip, path, resp.StatusCode)
	}
	return raw, nil
}

// Typed helpers over getValue/setValue. Each one is a path plus its envelope
// type; everything above this line is shared.

func getInt(ctx context.Context, ip, path string) (int, error) {
	v, err := getValue(ctx, ip, path)
	if err != nil {
		return 0, err
	}
	var n int
	// Both integer widths appear in the tree and callers never care which.
	if v.Type == typeI64 {
		if err := v.decode(typeI64, &n); err != nil {
			return 0, err
		}
		return n, nil
	}
	if err := v.decode(typeI32, &n); err != nil {
		return 0, err
	}
	return n, nil
}

func setInt(ctx context.Context, ip, path string, n int) error {
	v, err := newValue(typeI32, n)
	if err != nil {
		return err
	}
	return setValue(ctx, ip, path, v)
}

func getBool(ctx context.Context, ip, path string) (bool, error) {
	v, err := getValue(ctx, ip, path)
	if err != nil {
		return false, err
	}
	var b bool
	return b, v.decode(typeBool, &b)
}

func setBool(ctx context.Context, ip, path string, b bool) error {
	v, err := newValue(typeBool, b)
	if err != nil {
		return err
	}
	return setValue(ctx, ip, path, v)
}

func getString(ctx context.Context, ip, path string) (string, error) {
	v, err := getValue(ctx, ip, path)
	if err != nil {
		return "", err
	}
	var s string
	return s, v.decode(typeString, &s)
}

func setString(ctx context.Context, ip, path string, s string) error {
	v, err := newValue(typeString, s)
	if err != nil {
		return err
	}
	return setValue(ctx, ip, path, v)
}

// getEnum reads a path whose envelope type is one of the speaker's own enum
// names (kefPhysicalSource, kefSpeakerStatus, …), all of which carry a
// string payload.
func getEnum(ctx context.Context, ip, path, typ string) (string, error) {
	v, err := getValue(ctx, ip, path)
	if err != nil {
		return "", err
	}
	var s string
	return s, v.decode(typ, &s)
}

func setEnum(ctx context.Context, ip, path, typ, s string) error {
	v, err := newValue(typ, s)
	if err != nil {
		return err
	}
	return setValue(ctx, ip, path, v)
}

// ── Paths ────────────────────────────────────────────────────────────────

const (
	pathDeviceName    = "settings:/deviceName"
	pathModel         = "settings:/kef/host/modelName"
	pathFirmware      = "settings:/version"
	pathRelease       = "settings:/releasetext"
	pathMAC           = "settings:/system/primaryMacAddress"
	pathSpeakerStatus = "settings:/kef/host/speakerStatus"
	pathSource        = "settings:/kef/play/physicalSource"
	pathStandbyMode   = "settings:/kef/host/standbyMode"
	pathMaxVolume     = "settings:/kef/host/maximumVolume"
	pathVolumeLimit   = "settings:/kef/host/volumeLimit"
	pathVolume        = "player:volume"
	pathMute          = "settings:/mediaPlayer/mute"
	pathPlayerData    = "player:player/data"
	pathPlayTime      = "player:player/data/playTime"
	pathControl       = "player:player/control"
)

// ── Device identity ──────────────────────────────────────────────────────

// Device is a speaker's identity, as the speaker reports it.
type Device struct {
	IP    string `json:"ip"`
	MAC   string `json:"mac"`   // primary MAC, lower-cased — the stable id
	Name  string `json:"name"`  // the speaker's own device name
	Model string `json:"model"` // e.g. "LS50 Wireless II"
}

// Describe reads a speaker's identity. Also the reachability probe behind
// "Test connection" and behind discovery's is-this-a-KEF check: the MAC and
// the device name both have to answer for the address to count as a speaker.
func Describe(ctx context.Context, ip string) (*Device, error) {
	name, err := getString(ctx, ip, pathDeviceName)
	if err != nil {
		return nil, fmt.Errorf("no KEF speaker found at %s: %w", ip, err)
	}
	mac, err := getString(ctx, ip, pathMAC)
	if err != nil {
		return nil, fmt.Errorf("device at %s does not look like a KEF speaker: %w", ip, err)
	}
	d := &Device{IP: ip, MAC: NormalizeMAC(mac), Name: strings.TrimSpace(name)}
	if d.MAC == "" {
		return nil, fmt.Errorf("device at %s does not look like a KEF speaker", ip)
	}
	d.Model = modelOf(ctx, ip)
	return d, nil
}

// modelOf resolves the product name. Newer firmware answers modelName
// directly; older firmware only carries the release string ("LS50W2_2.2.1"),
// whose prefix is a build code rather than a product name, so it is mapped
// to one. An unrecognised code is returned as-is — a wrong name is worse
// than an unfamiliar one.
func modelOf(ctx context.Context, ip string) string {
	if m, err := getString(ctx, ip, pathModel); err == nil {
		if m = strings.TrimSpace(m); m != "" {
			return m
		}
	}
	rel, err := getString(ctx, ip, pathRelease)
	if err != nil {
		return ""
	}
	code, _, _ := strings.Cut(strings.TrimSpace(rel), "_")
	return ModelFromCode(code)
}

// buildCodes maps the release string's leading build code to the product
// name KEF prints on the box.
var buildCodes = map[string]string{
	"LS50W2":  "LS50 Wireless II",
	"LSXII":   "LSX II",
	"LSX2":    "LSX II",
	"LSXIILT": "LSX II LT",
	"LS60":    "LS60 Wireless",
}

// ModelFromCode maps a firmware build code to a product name, returning the
// code unchanged when it isn't one we know.
func ModelFromCode(code string) string {
	if code == "" {
		return ""
	}
	if name, ok := buildCodes[strings.ToUpper(code)]; ok {
		return name
	}
	return code
}

// NormalizeMAC lower-cases a MAC and strips its separators, so the same
// speaker read through two firmware versions produces one id.
func NormalizeMAC(mac string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(strings.TrimSpace(mac)) {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			b.WriteRune(c)
		}
	}
	if b.Len() != 12 {
		return ""
	}
	return b.String()
}

// Firmware reads the speaker's firmware version and release string.
func Firmware(ctx context.Context, ip string) (version, release string) {
	version, _ = getString(ctx, ip, pathFirmware)
	release, _ = getString(ctx, ip, pathRelease)
	return strings.TrimSpace(version), strings.TrimSpace(release)
}

// ── Sources & power ──────────────────────────────────────────────────────

// Sources the speaker accepts on physicalSource. Not every model has every
// one — the LSX II has USB where the LS50 Wireless II has none — so the API
// reports the speaker's own list rather than this being an allow-list for
// the UI to render blindly.
const (
	SourceWiFi      = "wifi"
	SourceBluetooth = "bluetooth"
	SourceTV        = "tv"
	SourceOptical   = "optic"
	SourceCoaxial   = "coaxial"
	SourceAnalog    = "analog"
	SourceUSB       = "usb"
)

// AllSources is every source this bridge will send, in the order the KEF
// Connect app lists them.
var AllSources = []string{
	SourceWiFi, SourceBluetooth, SourceTV, SourceOptical,
	SourceCoaxial, SourceAnalog, SourceUSB,
}

// ValidSource reports whether s is a source name the speaker would accept.
func ValidSource(s string) bool {
	for _, v := range AllSources {
		if v == s {
			return true
		}
	}
	return false
}

// Standby modes. The speaker powers itself down after this much silence.
const (
	StandbyNever   = "standby_none"
	Standby20Min   = "standby_20mins"
	Standby60Min   = "standby_60mins"
	StatusPowerOn  = "powerOn"
	StatusStandby  = "standby"
	StatusPoweredM = "poweredMode" // some firmware reports this instead of powerOn
)

// ValidStandbyMode reports whether s is a standby mode the speaker accepts.
func ValidStandbyMode(s string) bool {
	return s == StandbyNever || s == Standby20Min || s == Standby60Min
}

// GetSource reads the speaker's current physical source.
func GetSource(ctx context.Context, ip string) (string, error) {
	return getEnum(ctx, ip, pathSource, typeSource)
}

// SetSource switches the speaker's input. Setting a source also wakes a
// speaker in standby, which is why powering on is the same call with "wifi".
func SetSource(ctx context.Context, ip, source string) error {
	if !ValidSource(source) {
		return fmt.Errorf("kef: %q is not a source this speaker accepts", source)
	}
	return setEnum(ctx, ip, pathSource, typeSource, source)
}

// GetPowerState reads whether the speaker is awake or in standby.
func GetPowerState(ctx context.Context, ip string) (string, error) {
	return getEnum(ctx, ip, pathSpeakerStatus, typeSpeakerStatus)
}

// SetStandby puts the speaker into standby, or wakes it. Waking goes through
// the source path rather than speakerStatus: the speaker takes "standby"
// there but ignores a write of "powerOn", and selecting an input is what the
// KEF Connect app's power button does too.
func SetStandby(ctx context.Context, ip string, standby bool) error {
	if !standby {
		return SetSource(ctx, ip, SourceWiFi)
	}
	return setEnum(ctx, ip, pathSpeakerStatus, typeSpeakerStatus, StatusStandby)
}

// GetStandbyMode reads the auto-standby timer.
func GetStandbyMode(ctx context.Context, ip string) (string, error) {
	return getEnum(ctx, ip, pathStandbyMode, typeStandbyMode)
}

// SetStandbyMode sets the auto-standby timer.
func SetStandbyMode(ctx context.Context, ip, mode string) error {
	if !ValidStandbyMode(mode) {
		return fmt.Errorf("kef: %q is not a standby mode", mode)
	}
	return setEnum(ctx, ip, pathStandbyMode, typeStandbyMode, mode)
}

// ── Volume & transport ───────────────────────────────────────────────────

// GetVolume reads the speaker's volume (0-100).
func GetVolume(ctx context.Context, ip string) (int, error) {
	return getInt(ctx, ip, pathVolume)
}

// SetVolume sets the speaker's volume (0-100).
func SetVolume(ctx context.Context, ip string, level int) error {
	return setInt(ctx, ip, pathVolume, clamp(level, 0, 100))
}

// GetMute reads the speaker's mute state.
func GetMute(ctx context.Context, ip string) (bool, error) {
	return getBool(ctx, ip, pathMute)
}

// SetMute mutes or unmutes the speaker.
func SetMute(ctx context.Context, ip string, muted bool) error {
	return setBool(ctx, ip, pathMute, muted)
}

// GetMaxVolume reads the volume ceiling configured on the speaker.
func GetMaxVolume(ctx context.Context, ip string) (int, error) {
	return getInt(ctx, ip, pathMaxVolume)
}

// SetMaxVolume sets the volume ceiling.
func SetMaxVolume(ctx context.Context, ip string, level int) error {
	return setInt(ctx, ip, pathMaxVolume, clamp(level, 0, 100))
}

// GetVolumeLimit reports whether the ceiling above is enforced.
func GetVolumeLimit(ctx context.Context, ip string) (bool, error) {
	return getBool(ctx, ip, pathVolumeLimit)
}

// SetVolumeLimit enables or disables the volume ceiling.
func SetVolumeLimit(ctx context.Context, ip string, on bool) error {
	return setBool(ctx, ip, pathVolumeLimit, on)
}

// Transport controls accepted on the control path. There are only three:
// the speaker has no "play" verb and no "stop" — ControlToggle flips
// between playing and paused, and anything else on this path is answered
// with HTTP 500.
const (
	ControlToggle   = "pause"
	ControlNext     = "next"
	ControlPrevious = "previous"
)

// control sends one transport action. Unlike every other path this is an
// "activate" role — the speaker treats it as a verb, not as stored state.
func control(ctx context.Context, ip, action string) error {
	raw, err := json.Marshal(map[string]string{"control": action})
	if err != nil {
		return fmt.Errorf("kef: encode control: %w", err)
	}
	return setRaw(ctx, ip, pathControl, "activate", raw)
}

// Play resumes playback on whatever source is selected.
func Play(ctx context.Context, ip string) error { return setPlaying(ctx, ip, true) }

// Pause pauses playback.
func Pause(ctx context.Context, ip string) error { return setPlaying(ctx, ip, false) }

// setPlaying drives the one control the speaker has towards a wanted state:
// read what the player is doing, and send the toggle only when it is on the
// wrong side — a blind send would pause the music of anyone whose UI was a
// beat out of date. A speaker that won't say what it is doing still gets the
// toggle: the caller pressed a button drawn from a reading of its own, and
// doing nothing at all is the worse answer.
func setPlaying(ctx context.Context, ip string, want bool) error {
	if status, err := PlaybackStatus(ctx, ip); err == nil && (status == StatusPlaying) == want {
		return nil
	}
	return control(ctx, ip, ControlToggle)
}

// PlaybackStatus reads the player's own word for what it is doing, without
// the rest of GetState's fan-out.
func PlaybackStatus(ctx context.Context, ip string) (string, error) {
	v, err := getValue(ctx, ip, pathPlayerData)
	if err != nil {
		return "", err
	}
	_, status, _ := ParsePlayerData(v.Raw)
	if status == "" {
		return "", fmt.Errorf("kef: %s did not report a playback status", ip)
	}
	return status, nil
}

// Next skips to the next track.
func Next(ctx context.Context, ip string) error { return control(ctx, ip, ControlNext) }

// Previous goes back one track.
func Previous(ctx context.Context, ip string) error { return control(ctx, ip, ControlPrevious) }

// ── Now playing ──────────────────────────────────────────────────────────

// Track is the now-playing metadata the speaker reports.
type Track struct {
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	// ArtURI is whatever the streaming service handed the speaker: an
	// absolute URL in practice, since KEF holds no artwork of its own.
	ArtURI string `json:"art_uri,omitempty"`
}

// State is one speaker's live playback state.
type State struct {
	// Status is the speaker's own word for what the player is doing:
	// playing | paused | stopped. Passed through rather than mapped so the
	// UI can tell "stopped" (no source) from "paused" (source, not running).
	Status  string `json:"status"`
	Playing bool   `json:"playing"`
	// Source is the selected input, and PoweredOn says whether the speaker
	// is awake at all — a speaker in standby answers, and reporting it as
	// merely "not playing" would hide why the transport does nothing.
	Source    string `json:"source"`
	PoweredOn bool   `json:"powered_on"`
	Volume    int    `json:"volume"`
	Muted     bool   `json:"muted"`
	Track     *Track `json:"track,omitempty"`
	// PositionMS and DurationMS are milliseconds. Duration is zero for live
	// streams and for the physical inputs, where there is nothing to seek.
	PositionMS int64 `json:"position_ms,omitempty"`
	DurationMS int64 `json:"duration_ms,omitempty"`
}

// playerData mirrors the subset of player:player/data we read. The speaker
// sends considerably more — queue ids, service tokens, per-service artwork
// variants — none of which this bridge exposes.
type playerData struct {
	// State is where the speaker puts "playing" / "paused" / "stopped".
	State string `json:"state"`
	// Status is the *track's* status rather than the player's: its duration
	// lives here. Some firmware also names the playback state under it, so
	// that is read as a fallback for State.
	Status struct {
		Name     string `json:"name"`
		Duration int64  `json:"duration"`
	} `json:"status"`
	TrackRoles struct {
		Title     string `json:"title"`
		Icon      string `json:"icon"`
		MediaData struct {
			MetaData struct {
				Artist   string `json:"artist"`
				Album    string `json:"album"`
				Duration int64  `json:"duration"`
			} `json:"metaData"`
			Resources []struct {
				Duration int64 `json:"duration"`
			} `json:"resources"`
		} `json:"mediaData"`
	} `json:"trackRoles"`
}

// Playback statuses the speaker reports.
const (
	StatusPlaying = "playing"
	StatusPaused  = "paused"
	StatusStopped = "stopped"
)

// ParsePlayerData turns a raw player:player/data payload into a Track and a
// status. Split out for testability — the shape is the one part of this API
// with enough nesting to get wrong.
func ParsePlayerData(raw []byte) (*Track, string, int64) {
	var d playerData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, "", 0
	}
	status := strings.ToLower(strings.TrimSpace(d.State))
	if status == "" {
		status = strings.ToLower(strings.TrimSpace(d.Status.Name))
	}
	// Duration, in the order the speaker actually fills it in.
	dur := d.Status.Duration
	if dur == 0 {
		dur = d.TrackRoles.MediaData.MetaData.Duration
	}
	if dur == 0 && len(d.TrackRoles.MediaData.Resources) > 0 {
		dur = d.TrackRoles.MediaData.Resources[0].Duration
	}
	t := &Track{
		Title:  strings.TrimSpace(d.TrackRoles.Title),
		Artist: strings.TrimSpace(d.TrackRoles.MediaData.MetaData.Artist),
		Album:  strings.TrimSpace(d.TrackRoles.MediaData.MetaData.Album),
		ArtURI: strings.TrimSpace(d.TrackRoles.Icon),
	}
	if t.Title == "" && t.Artist == "" && t.Album == "" {
		t = nil
	}
	return t, status, dur
}

// GetState gathers everything the Music surface shows for one speaker.
// Partial failures degrade rather than fail the whole read: a speaker in
// standby answers for its source and power state but not for a track, and
// that is a perfectly good answer to render.
func GetState(ctx context.Context, ip string) (*State, error) {
	// Power state first — it is the call that decides whether the speaker
	// is answering at all, so its error is the one worth returning.
	power, err := GetPowerState(ctx, ip)
	if err != nil {
		return nil, err
	}
	st := &State{
		PoweredOn: power != StatusStandby,
		Status:    StatusStopped,
	}
	if src, err := GetSource(ctx, ip); err == nil {
		st.Source = src
	}
	if v, err := GetVolume(ctx, ip); err == nil {
		st.Volume = v
	}
	if m, err := GetMute(ctx, ip); err == nil {
		st.Muted = m
	}
	if v, err := getValue(ctx, ip, pathPlayerData); err == nil && len(v.Raw) > 0 {
		track, status, dur := ParsePlayerData(v.Raw)
		st.Track = track
		if status != "" {
			st.Status = status
		}
		st.DurationMS = dur
	}
	st.Playing = st.Status == StatusPlaying
	if st.Playing || st.Status == StatusPaused {
		if pos, err := getInt(ctx, ip, pathPlayTime); err == nil && pos > 0 {
			st.PositionMS = int64(pos)
		}
	}
	return st, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
