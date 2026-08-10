package sonos

// Per-speaker device settings — the half of the Sonos app that isn't playback.
//
// Three services carry them, and which one owns a setting decides its scope:
//
//   - RenderingControl: bass, treble, loudness and the EQ block (night mode,
//     speech enhancement, sub, surround). Per *speaker*, and the EQ block is
//     model-dependent: a Sonos One has no sub and no night mode, so asking it
//     returns a UPnP fault. That fault is the capability probe — there is no
//     "what can you do" action to call, so Load() asks and records what
//     answered (see Capabilities).
//   - DeviceProperties: the status LED and the touch/button lock, plus the
//     read-only serial/firmware/MAC block. Per speaker.
//   - AVTransport: the sleep timer. Per *group*, like shuffle and repeat — it
//     lives on the coordinator, and setting it on a follower is meaningless.
//
// Everything here is read on demand when the settings surface opens rather
// than folded into the status poll: a speaker's bass doesn't change on its
// own, and eleven extra SOAP calls every five seconds to find that out would
// be absurd.

import (
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

var deviceProperties = service{
	path: "/DeviceProperties/Control",
	urn:  "urn:schemas-upnp-org:service:DeviceProperties:1",
}

// EQ types Sonos accepts on RenderingControl's Get/SetEQ. Only the ones a
// given model supports answer; the rest fault, which is how Capabilities is
// worked out.
const (
	eqNightMode   = "NightMode"
	eqDialogLevel = "DialogLevel"
	eqSubEnable   = "SubEnable"
	eqSubGain     = "SubGain"
	eqSurround    = "SurroundEnable"
)

// Bass, treble and sub-gain ranges as Sonos defines them.
const (
	ToneMin    = -10
	ToneMax    = 10
	SubGainMin = -15
	SubGainMax = 15
)

// MaxSleepMinutes caps the sleep timer. Sonos itself accepts more, but a
// timer measured in days is a mis-tap, not an intent.
const MaxSleepMinutes = 600

// Capabilities records which of the model-dependent controls this speaker
// actually answered for. The UI renders only what is true here — a control
// that would fault is worse than a control that isn't there.
type Capabilities struct {
	Bass      bool `json:"bass"`
	Treble    bool `json:"treble"`
	Loudness  bool `json:"loudness"`
	NightMode bool `json:"night_mode"`
	// DialogLevel is Sonos' name for what the app calls speech enhancement.
	DialogLevel bool `json:"dialog_level"`
	Sub         bool `json:"sub"`
	Surround    bool `json:"surround"`
}

// ZoneInfo is the read-only identity block from DeviceProperties. Useful for
// support ("which firmware is the kitchen on?") and nothing else, so it is
// never editable.
type ZoneInfo struct {
	SerialNumber    string `json:"serial_number,omitempty"`
	SoftwareVersion string `json:"software_version,omitempty"`
	HardwareVersion string `json:"hardware_version,omitempty"`
	MACAddress      string `json:"mac_address,omitempty"`
	// ZoneName is the speaker's own room name as the Sonos app shows it,
	// which can drift from the name HomeHub stores for it.
	ZoneName string `json:"zone_name,omitempty"`
}

// Settings is one speaker's full settings snapshot. Every adjustable field is
// a pointer: absent means "this model doesn't have it", which is a different
// statement from a zero value, and the UI has to be able to tell them apart.
type Settings struct {
	Capabilities Capabilities `json:"capabilities"`

	Bass        *int  `json:"bass,omitempty"`   // -10…10
	Treble      *int  `json:"treble,omitempty"` // -10…10
	Loudness    *bool `json:"loudness,omitempty"`
	NightMode   *bool `json:"night_mode,omitempty"`
	DialogLevel *bool `json:"dialog_level,omitempty"`
	SubEnabled  *bool `json:"sub_enabled,omitempty"`
	SubGain     *int  `json:"sub_gain,omitempty"` // -15…15
	Surround    *bool `json:"surround,omitempty"`

	// LED is the status light on the speaker's face; ButtonLock is the
	// touch/button lock. Both are plain on/off and present on every model.
	LED        *bool `json:"led,omitempty"`
	ButtonLock *bool `json:"button_lock,omitempty"`

	// SleepMinutes is the group sleep timer's remaining whole minutes, 0 when
	// none is set. Group-scoped: only meaningful read from a coordinator.
	SleepMinutes int `json:"sleep_minutes"`

	Info ZoneInfo `json:"info"`

	// Model identity from the device description, which carries a finer
	// answer than the status poll's model name (a number like "S13", and the
	// short display name the speaker calls itself).
	ModelNumber string `json:"model_number,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	// HasImage says the speaker publishes a picture of itself, so the UI can
	// fall back to the striped placeholder instead of a broken <img>.
	HasImage bool `json:"has_image"`

	// Battery is present only on the portable models (battery.go). Nil is
	// "this speaker runs on mains", which is a different statement from a
	// flat battery and has to stay distinguishable — the same rule the
	// pointer fields above follow.
	Battery *Battery `json:"battery,omitempty"`
}

// LoadSettings reads everything above from one speaker.
//
// The reads run in four parallel branches — tone, EQ, device, transport —
// because a full snapshot is eleven SOAP round trips and doing them one after
// another is what makes a settings sheet feel slow to open. Each branch is
// sequential inside itself, which keeps a single speaker to four concurrent
// requests rather than eleven.
//
// Partial failure never fails the whole read: a control whose Get faulted is
// simply reported as unsupported. Only an unreachable speaker is an error,
// and the tone branch is the one that decides that, since bass exists on
// every Sonos ever made.
func LoadSettings(ctx context.Context, ip string) (*Settings, error) {
	if err := ValidateHost(ip); err != nil {
		return nil, fmt.Errorf("sonos: %w", err)
	}

	var (
		s      Settings
		mu     sync.Mutex // guards s across the branches below
		wg     sync.WaitGroup
		toneEr error
	)

	set := func(fn func(*Settings)) {
		mu.Lock()
		defer mu.Unlock()
		fn(&s)
	}

	wg.Add(4)

	// Tone — present on every model, so its failure is the reachability
	// verdict for the whole read.
	go func() {
		defer wg.Done()
		bass, err := getIntTag(ctx, ip, renderingControl, "GetBass",
			[]arg{{"InstanceID", instance0}}, "CurrentBass")
		if err != nil {
			toneEr = err
			return
		}
		set(func(s *Settings) { s.Bass = &bass; s.Capabilities.Bass = true })

		if treble, err := getIntTag(ctx, ip, renderingControl, "GetTreble",
			[]arg{{"InstanceID", instance0}}, "CurrentTreble"); err == nil {
			set(func(s *Settings) { s.Treble = &treble; s.Capabilities.Treble = true })
		}
		if loud, err := getBoolTag(ctx, ip, renderingControl, "GetLoudness",
			[]arg{{"InstanceID", instance0}, {"Channel", "Master"}}, "CurrentLoudness"); err == nil {
			set(func(s *Settings) { s.Loudness = &loud; s.Capabilities.Loudness = true })
		}
	}()

	// EQ — the model-dependent block. Every Get here doubles as the probe for
	// its own control.
	go func() {
		defer wg.Done()
		if v, err := getEQ(ctx, ip, eqNightMode); err == nil {
			on := v != 0
			set(func(s *Settings) { s.NightMode = &on; s.Capabilities.NightMode = true })
		}
		if v, err := getEQ(ctx, ip, eqDialogLevel); err == nil {
			on := v != 0
			set(func(s *Settings) { s.DialogLevel = &on; s.Capabilities.DialogLevel = true })
		}
		if v, err := getEQ(ctx, ip, eqSubEnable); err == nil {
			on := v != 0
			set(func(s *Settings) { s.SubEnabled = &on; s.Capabilities.Sub = true })
			// Gain is only worth asking for on a speaker that has a sub.
			if g, err := getEQ(ctx, ip, eqSubGain); err == nil {
				set(func(s *Settings) { s.SubGain = &g })
			}
		}
		if v, err := getEQ(ctx, ip, eqSurround); err == nil {
			on := v != 0
			set(func(s *Settings) { s.Surround = &on; s.Capabilities.Surround = true })
		}
	}()

	// Device — LED, button lock and the identity block.
	go func() {
		defer wg.Done()
		if led, err := GetLED(ctx, ip); err == nil {
			set(func(s *Settings) { s.LED = &led })
		}
		if lock, err := GetButtonLock(ctx, ip); err == nil {
			set(func(s *Settings) { s.ButtonLock = &lock })
		}
		if info, err := GetZoneInfo(ctx, ip); err == nil {
			set(func(s *Settings) { s.Info = *info })
		}
		if body, err := soapCall(ctx, ip, deviceProperties, "GetZoneAttributes", nil); err == nil {
			if name := extractTag(body, "CurrentZoneName"); name != "" {
				set(func(s *Settings) { s.Info.ZoneName = name })
			}
		}
	}()

	// Transport (sleep timer) and the device description, which is plain HTTP
	// rather than SOAP but belongs on its own round trip either way.
	go func() {
		defer wg.Done()
		if mins, err := GetSleepTimer(ctx, ip); err == nil {
			set(func(s *Settings) { s.SleepMinutes = mins })
		}
		if info, err := DescribeFull(ctx, ip); err == nil {
			set(func(s *Settings) {
				s.ModelNumber = info.ModelNumber
				s.DisplayName = info.DisplayName
				s.HasImage = info.IconPath != ""
			})
		}
		// Portable models only, and the ask *is* the probe — there is no
		// action that says which models have one. A mains speaker answers
		// nothing here and stays nil, which is the answer (battery.go).
		if bat, err := GetBattery(ctx, ip); err == nil && bat != nil {
			set(func(s *Settings) { s.Battery = bat })
		}
	}()

	wg.Wait()
	if toneEr != nil {
		return nil, toneEr
	}
	return &s, nil
}

// ── Tone (RenderingControl) ──────────────────────────────────────────────

// SetBass sets bass, clamped to Sonos' -10…10.
func SetBass(ctx context.Context, ip string, level int) error {
	_, err := soapCall(ctx, ip, renderingControl, "SetBass",
		[]arg{{"InstanceID", instance0}, {"DesiredBass", strconv.Itoa(clamp(level, ToneMin, ToneMax))}})
	return err
}

// SetTreble sets treble, clamped to Sonos' -10…10.
func SetTreble(ctx context.Context, ip string, level int) error {
	_, err := soapCall(ctx, ip, renderingControl, "SetTreble",
		[]arg{{"InstanceID", instance0}, {"DesiredTreble", strconv.Itoa(clamp(level, ToneMin, ToneMax))}})
	return err
}

// SetLoudness turns the loudness contour on or off.
func SetLoudness(ctx context.Context, ip string, on bool) error {
	_, err := soapCall(ctx, ip, renderingControl, "SetLoudness",
		[]arg{{"InstanceID", instance0}, {"Channel", "Master"}, {"DesiredLoudness", boolArg(on)}})
	return err
}

// ── EQ block (RenderingControl) ──────────────────────────────────────────

// SetNightMode turns night mode on or off. Home-theatre models only.
func SetNightMode(ctx context.Context, ip string, on bool) error {
	return setEQ(ctx, ip, eqNightMode, boolInt(on))
}

// SetDialogLevel turns speech enhancement on or off. Home-theatre models only.
func SetDialogLevel(ctx context.Context, ip string, on bool) error {
	return setEQ(ctx, ip, eqDialogLevel, boolInt(on))
}

// SetSubEnabled turns a paired Sonos Sub on or off.
func SetSubEnabled(ctx context.Context, ip string, on bool) error {
	return setEQ(ctx, ip, eqSubEnable, boolInt(on))
}

// SetSubGain sets the sub's level, clamped to Sonos' -15…15.
func SetSubGain(ctx context.Context, ip string, gain int) error {
	return setEQ(ctx, ip, eqSubGain, clamp(gain, SubGainMin, SubGainMax))
}

// SetSurround turns paired surround speakers on or off.
func SetSurround(ctx context.Context, ip string, on bool) error {
	return setEQ(ctx, ip, eqSurround, boolInt(on))
}

func getEQ(ctx context.Context, ip, eqType string) (int, error) {
	return getIntTag(ctx, ip, renderingControl, "GetEQ",
		[]arg{{"InstanceID", instance0}, {"EQType", eqType}}, "CurrentValue")
}

func setEQ(ctx context.Context, ip, eqType string, value int) error {
	_, err := soapCall(ctx, ip, renderingControl, "SetEQ", []arg{
		{"InstanceID", instance0},
		{"EQType", eqType},
		{"DesiredValue", strconv.Itoa(value)},
	})
	return err
}

// ── Device properties ────────────────────────────────────────────────────

// GetLED reads whether the status light is lit.
func GetLED(ctx context.Context, ip string) (bool, error) {
	body, err := soapCall(ctx, ip, deviceProperties, "GetLEDState", nil)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(extractTag(body, "CurrentLEDState"), "On"), nil
}

// SetLED turns the status light on or off.
func SetLED(ctx context.Context, ip string, on bool) error {
	_, err := soapCall(ctx, ip, deviceProperties, "SetLEDState",
		[]arg{{"DesiredLEDState", onOff(on)}})
	return err
}

// GetButtonLock reports whether the speaker's touch controls are locked.
// Sonos words the underlying state as the lock being "On", which is the
// opposite polarity from the Sonos app's "Touch controls" switch — the
// mapping is kept here rather than in the UI.
func GetButtonLock(ctx context.Context, ip string) (bool, error) {
	body, err := soapCall(ctx, ip, deviceProperties, "GetButtonLockState", nil)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(extractTag(body, "CurrentButtonLockState"), "On"), nil
}

// SetButtonLock locks (true) or unlocks (false) the speaker's touch controls.
func SetButtonLock(ctx context.Context, ip string, locked bool) error {
	_, err := soapCall(ctx, ip, deviceProperties, "SetButtonLockState",
		[]arg{{"DesiredButtonLockState", onOff(locked)}})
	return err
}

// GetZoneInfo reads the speaker's serial number, firmware and MAC.
func GetZoneInfo(ctx context.Context, ip string) (*ZoneInfo, error) {
	body, err := soapCall(ctx, ip, deviceProperties, "GetZoneInfo", nil)
	if err != nil {
		return nil, err
	}
	return ParseZoneInfo(body), nil
}

// ParseZoneInfo pulls the identity fields out of a GetZoneInfo response.
// Split out for testability. DisplaySoftwareVersion is the version Sonos
// shows its own users ("16.1"), so it wins over the internal build string.
func ParseZoneInfo(body string) *ZoneInfo {
	sw := extractTag(body, "DisplaySoftwareVersion")
	if sw == "" {
		sw = extractTag(body, "SoftwareVersion")
	}
	return &ZoneInfo{
		SerialNumber:    extractTag(body, "SerialNumber"),
		SoftwareVersion: sw,
		HardwareVersion: extractTag(body, "HardwareVersion"),
		MACAddress:      extractTag(body, "MACAddress"),
	}
}

// ── Sleep timer (AVTransport, group-scoped) ──────────────────────────────

// GetSleepTimer returns the whole minutes left on the group's sleep timer,
// rounded up so a timer with 30 seconds on it still reads as 1 rather than
// disappearing. Zero means no timer is set.
func GetSleepTimer(ctx context.Context, ip string) (int, error) {
	body, err := soapCall(ctx, ip, avTransport, "GetRemainingSleepTimerDuration",
		[]arg{{"InstanceID", instance0}})
	if err != nil {
		return 0, err
	}
	return clockToMinutes(extractTag(body, "RemainingSleepTimerDuration")), nil
}

// SetSleepTimer starts a sleep timer for the group led by this speaker.
// Zero minutes cancels it, which Sonos expresses as an empty duration.
func SetSleepTimer(ctx context.Context, ip string, minutes int) error {
	if minutes < 0 || minutes > MaxSleepMinutes {
		return fmt.Errorf("sonos: sleep timer must be between 0 and %d minutes", MaxSleepMinutes)
	}
	_, err := soapCall(ctx, ip, avTransport, "ConfigureSleepTimer",
		[]arg{{"InstanceID", instance0}, {"NewSleepTimerDuration", minutesToClock(minutes)}})
	return err
}

// minutesToClock renders minutes as the HH:MM:SS Sonos expects. Zero is the
// empty string, which is how the timer is cancelled.
func minutesToClock(minutes int) string {
	if minutes <= 0 {
		return ""
	}
	return fmt.Sprintf("%02d:%02d:00", minutes/60, minutes%60)
}

// clockToMinutes parses a H:MM:SS duration into whole minutes, rounding any
// remaining seconds up. Unset timers ("", "NOT_IMPLEMENTED", all-zero) are 0.
func clockToMinutes(clock string) int {
	clock = strings.TrimSpace(clock)
	if clock == "" || clock == "NOT_IMPLEMENTED" {
		return 0
	}
	parts := strings.Split(clock, ":")
	if len(parts) != 3 {
		return 0
	}
	var secs int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0
		}
		secs = secs*60 + n
	}
	if secs <= 0 {
		return 0
	}
	return (secs + 59) / 60
}

// ── Device description: model detail and the speaker's own picture ───────

// DeviceInfo is the device description read in full: identity plus the model
// detail and self-portrait that only the settings surface needs.
type DeviceInfo struct {
	Device
	ModelNumber string `json:"model_number,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	// IconPath is a speaker-relative path to a picture of this model, as
	// published by the speaker itself (e.g. /img/icon-S13.png). Empty when
	// the description lists none — nothing here invents an image.
	IconPath string `json:"icon_path,omitempty"`
}

// DescribeFull fetches and parses a speaker's whole device description.
func DescribeFull(ctx context.Context, ip string) (*DeviceInfo, error) {
	raw, err := fetchDescription(ctx, ip)
	if err != nil {
		return nil, err
	}
	info := ParseDeviceInfo(raw)
	if info.UUID == "" {
		return nil, fmt.Errorf("device at %s does not look like a Sonos speaker", ip)
	}
	info.IP = ip
	return info, nil
}

// descriptionXML mirrors the parts of device_description.xml we read. Only
// the root <device> is described; the MediaRenderer and MediaServer
// sub-devices carry their own iconLists, which are UPnP plumbing rather than
// pictures of the speaker.
type descriptionXML struct {
	Device struct {
		ModelName   string `xml:"modelName"`
		ModelNumber string `xml:"modelNumber"`
		DisplayName string `xml:"displayName"`
		RoomName    string `xml:"roomName"`
		UDN         string `xml:"UDN"`
		IconList    struct {
			Icons []struct {
				MimeType string `xml:"mimetype"`
				Width    int    `xml:"width"`
				Height   int    `xml:"height"`
				URL      string `xml:"url"`
			} `xml:"icon"`
		} `xml:"iconList"`
	} `xml:"device"`
}

// ParseDeviceInfo parses a device description document. Split out for
// testability; tolerant by design, since the fields beyond identity are all
// nice-to-have.
func ParseDeviceInfo(body string) *DeviceInfo {
	info := &DeviceInfo{Device: *ParseDescription(body)}

	var parsed descriptionXML
	if err := xml.Unmarshal([]byte(body), &parsed); err != nil {
		return info
	}
	info.ModelNumber = strings.TrimSpace(parsed.Device.ModelNumber)
	info.DisplayName = strings.TrimSpace(parsed.Device.DisplayName)

	// Largest icon wins: the list is usually one 48px PNG, but where a model
	// offers more the biggest is the one worth showing.
	best := 0
	for _, ic := range parsed.Device.IconList.Icons {
		if !safeIconPath(ic.URL) {
			continue
		}
		size := ic.Width
		if ic.Height > size {
			size = ic.Height
		}
		if info.IconPath == "" || size > best {
			info.IconPath, best = ic.URL, size
		}
	}
	return info
}

// safeIconPath reports whether an icon URL is a speaker-relative path we can
// proxy. Anything absolute or traversing is dropped: the icon path comes off
// the network, and it ends up in a URL we fetch ourselves.
func safeIconPath(p string) bool {
	p = strings.TrimSpace(p)
	return strings.HasPrefix(p, "/") &&
		!strings.HasPrefix(p, "//") &&
		!strings.Contains(p, "..") &&
		!strings.ContainsAny(p, "\\ \t\r\n")
}

// ── shared helpers ───────────────────────────────────────────────────────

// getIntTag runs a SOAP action and parses one tag out of the response as an
// integer. A non-numeric value is an error, so a fault that somehow returns
// 200 doesn't read as a legitimate zero.
func getIntTag(ctx context.Context, ip string, svc service, action string, args []arg, tag string) (int, error) {
	body, err := soapCall(ctx, ip, svc, action, args)
	if err != nil {
		return 0, err
	}
	raw := strings.TrimSpace(extractTag(body, tag))
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("sonos: %s returned %q for %s", action, raw, tag)
	}
	return v, nil
}

// getBoolTag is getIntTag for UPnP's 0/1 booleans.
func getBoolTag(ctx context.Context, ip string, svc service, action string, args []arg, tag string) (bool, error) {
	v, err := getIntTag(ctx, ip, svc, action, args, tag)
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

// boolArg renders a bool as UPnP's 0/1.
func boolArg(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// boolInt renders a bool as the 0/1 SetEQ takes as its DesiredValue.
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// onOff renders a bool as the "On"/"Off" DeviceProperties takes.
func onOff(v bool) string {
	if v {
		return "On"
	}
	return "Off"
}
