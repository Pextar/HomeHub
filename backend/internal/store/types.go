package store

import "time"

// Socket represents a controllable socket / smart device.
type Socket struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Code     string `json:"code"`               // 433MHz code, Tasmota IP, or Matter node id
	Protocol string `json:"protocol"`           // e.g., "nexa", "kaku", "intertechno", "tasmota", "matter", "matter-thread"
	State    bool   `json:"state"`              // true = on, false = off
	Room     string `json:"room"`               // room/location
	Favorite bool   `json:"favorite,omitempty"` // pinned to dashboard
	Emoji    string `json:"emoji,omitempty"`    // shown big in kid mode; admin-picked
}

// Schedule represents a recurring timer for a socket, group, or scene.
//
// Targets:
//   - target_type "socket": fires action ("on"|"off"|"toggle") on a socket
//   - target_type "group":  fires action ("on"|"off"|"toggle") on every member
//   - target_type "scene":  activates the scene (action ignored, treated as "activate")
//
// For backwards compatibility, schedules with socket_id set and no
// target_type are treated as target_type="socket", target_id=socket_id.
//
// TimeMode picks how the trigger time is derived:
//   - "fixed" (default): fire at the wall-clock Time ("HH:MM").
//   - "sunrise": fire at today's sunrise + SolarOffsetMinutes.
//   - "sunset":  fire at today's sunset + SolarOffsetMinutes.
//
// Sunrise/sunset modes require Settings.Latitude/Longitude to be set;
// without a location the scheduler skips them silently.
type Schedule struct {
	ID                  string    `json:"id"`
	SocketID            string    `json:"socket_id,omitempty"`
	TargetType          string    `json:"target_type,omitempty"`
	TargetID            string    `json:"target_id,omitempty"`
	Action              string    `json:"action"`                         // "on" | "off" | "toggle" | "activate"
	TimeMode            string    `json:"time_mode,omitempty"`            // "fixed" | "sunrise" | "sunset" (empty == "fixed")
	Time                string    `json:"time"`                           // "HH:MM" format (used when TimeMode is "fixed")
	SolarOffsetMinutes  int       `json:"solar_offset_minutes,omitempty"` // -120..120, used when TimeMode is sunrise/sunset
	Days                []int     `json:"days"`                           // 0=Sun, 1=Mon, etc
	Enabled             bool      `json:"enabled"`
	RandomOffsetMinutes int       `json:"random_offset_minutes,omitempty"` // fire at a random time 0..N minutes after the trigger time
	LastFiredAt         time.Time `json:"last_fired_at,omitempty"`
}

// AutomationTrigger fires an automation. Type selects which fields apply:
//   - "time":   wall-clock / solar time, like a Schedule (Time/TimeMode/Days).
//   - "sensor": a sensor reading crosses a threshold (SensorID Op Value).
//   - "device": a socket changes to a given state (SocketID -> ToState).
type AutomationTrigger struct {
	Type string `json:"type"` // "time" | "sensor" | "device"

	// time
	TimeMode           string `json:"time_mode,omitempty"` // "fixed" | "sunrise" | "sunset" (empty == "fixed")
	Time               string `json:"time,omitempty"`      // "HH:MM" when TimeMode is fixed
	SolarOffsetMinutes int    `json:"solar_offset_minutes,omitempty"`
	Days               []int  `json:"days,omitempty"` // 0=Sun..6=Sat; empty == every day

	// sensor
	SensorID string  `json:"sensor_id,omitempty"`
	Op       string  `json:"op,omitempty"`    // "above" | "below"
	Value    float64 `json:"value,omitempty"` // threshold for Op

	// device
	SocketID string `json:"socket_id,omitempty"`
	ToState  string `json:"to_state,omitempty"` // "on" | "off"
}

// AutomationCondition optionally gates a trigger. All conditions on an
// automation must hold (logical AND) for its actions to run.
//   - "device":     a socket must currently be on/off.
//   - "time_range": local time must fall within [After, Before] (may wrap midnight).
type AutomationCondition struct {
	Type string `json:"type"` // "device" | "time_range"

	// device
	SocketID string `json:"socket_id,omitempty"`
	State    string `json:"state,omitempty"` // "on" | "off"

	// time_range
	After  string `json:"after,omitempty"`  // "HH:MM"
	Before string `json:"before,omitempty"` // "HH:MM"
}

// AutomationAction is one step run when an automation fires. Targets and
// actions mirror Schedule/Timer semantics and go through ExecuteAction.
type AutomationAction struct {
	TargetType string `json:"target_type"` // "socket" | "group" | "scene"
	TargetID   string `json:"target_id"`
	Action     string `json:"action"`          // "on" | "off" | "toggle" | "activate"
	Level      *int   `json:"level,omitempty"` // 1-100, smart lights only
	Color      string `json:"color,omitempty"` // "RRGGBB", smart lights only
}

// Automation is a trigger → optional conditions → ordered actions rule.
// Unlike a Schedule (time-only), an automation can react to sensor
// thresholds and device-state changes. Evaluated by the scheduler tick.
type Automation struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Enabled     bool                  `json:"enabled"`
	Trigger     AutomationTrigger     `json:"trigger"`
	Conditions  []AutomationCondition `json:"conditions,omitempty"`
	Actions     []AutomationAction    `json:"actions"`
	LastFiredAt time.Time             `json:"last_fired_at,omitempty"`
	RunCount    int                   `json:"run_count,omitempty"`
}

// NotifPrefs controls which event categories trigger push notifications for
// a user. The boolean categories default to true when a user first subscribes
// (set explicitly in the subscribe handler). A user who has never subscribed
// will have the zero value (all false) — no notifications sent.
//
// QuietHours, when enabled, suppresses every category EXCEPT SensorAlerts
// (which can be safety-critical) between QuietStart and QuietEnd local time.
// The window may wrap past midnight (e.g. 22:00–07:00).
//
// MutedSocketIDs / MutedSensorIDs let a user opt specific devices out of
// notifications while keeping the category enabled for everything else.
type NotifPrefs struct {
	SensorAlerts   bool     `json:"sensor_alerts"`
	StateChanges   bool     `json:"state_changes"`
	ScheduleFired  bool     `json:"schedule_fired"`
	DeviceOffline  bool     `json:"device_offline"`
	QuietHours     bool     `json:"quiet_hours,omitempty"`
	QuietStart     string   `json:"quiet_start,omitempty"` // "HH:MM"
	QuietEnd       string   `json:"quiet_end,omitempty"`   // "HH:MM"
	MutedSocketIDs []string `json:"muted_socket_ids,omitempty"`
	MutedSensorIDs []string `json:"muted_sensor_ids,omitempty"`
}

// User is a login profile. Admins have unrestricted access; non-admins
// may only see and control the sockets listed in SocketIDs. PasswordHash
// is a bcrypt hash — it is persisted to disk but the API layer never
// returns a raw User to clients (see api.userView).
//
// There is exactly one Owner — the user bootstrapped from AUTH_USER/AUTH_PASS.
// The owner cannot be deleted or demoted. Additional admin-level users
// (managers) are created via a one-time invite link; they set their own
// password through that link rather than having one chosen for them.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`        // admins; empty for code-only users or pending invites
	LoginCode    string    `json:"login_code,omitempty"` // limited users; a short numeric code, the only credential
	Admin        bool      `json:"admin"`
	Owner        bool      `json:"owner,omitempty"` // true for the one bootstrapped admin; cannot be demoted
	Kid          bool      `json:"kid,omitempty"`   // limited users; renders the playful kid layout
	SocketIDs    []string  `json:"socket_ids"`
	CreatedAt    time.Time `json:"created_at"`
	// TokenVersion is bumped whenever the user's credentials change
	// (password or login code). Session cookies embed the version they
	// were minted with, so changing a credential invalidates every
	// existing session for that user.
	TokenVersion int `json:"token_version,omitempty"`
	// Invite fields are set when a new admin user is created and cleared
	// once they accept the invite and set their password.
	InviteToken  string     `json:"invite_token,omitempty"`
	InviteExpiry time.Time  `json:"invite_expiry,omitempty"`
	NotifPrefs   NotifPrefs `json:"notif_prefs,omitempty"`
}

// CanAccessSocket reports whether this user may see/control the given
// socket. Admins can access everything; others are limited to SocketIDs.
func (u *User) CanAccessSocket(socketID string) bool {
	if u == nil {
		return false
	}
	if u.Admin {
		return true
	}
	for _, id := range u.SocketIDs {
		if id == socketID {
			return true
		}
	}
	return false
}

// Settings holds app-wide preferences, currently just the controller's
// location used to compute sunrise/sunset for solar-based schedules.
type Settings struct {
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	LocationName string  `json:"location_name,omitempty"` // free-form label for the UI ("Home", "Stockholm")
}

// HasLocation reports whether a real location has been configured.
// A latitude/longitude of exactly (0, 0) is treated as "not set" — the
// Null Island corner case is unlikely to matter for home automation.
func (s *Settings) HasLocation() bool {
	if s == nil {
		return false
	}
	return s.Latitude != 0 || s.Longitude != 0
}

// Group is a manually curated collection of sockets that can be
// controlled together. A socket may belong to any number of groups.
type Group struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	SocketIDs []string `json:"socket_ids"`
}

// SceneAction sets one socket to a specific state when its scene fires.
// For smart lights (Tasmota/Matter) turned on, an optional Level (1-100%)
// and/or Color (RRGGBB hex) are applied after switching on. RF sockets
// ignore Level/Color.
type SceneAction struct {
	SocketID string `json:"socket_id"`
	Action   string `json:"action"`          // "on" | "off"
	Level    *int   `json:"level,omitempty"` // 1-100, smart lights only
	Color    string `json:"color,omitempty"` // "RRGGBB", smart lights only
}

// SceneStep is one time-phased stage within a multi-step scene.
// DelayMinutes=0 means "run immediately on scene activation".
// Subsequent steps fire DelayMinutes after the scene was activated,
// allowing the same socket to be driven to different states over time
// (e.g. on at 30 % immediately, then 70 % an hour later).
type SceneStep struct {
	DelayMinutes int           `json:"delay_minutes"`
	Actions      []SceneAction `json:"actions"`
}

// Scene is a named preset that drives sockets through one or more
// time-phased steps. The same socket may appear in multiple steps
// with different settings (e.g. dim low at step 1, brighter at step 2).
//
// Legacy scenes saved before multi-step support used a flat Actions
// slice. On first load those are migrated to a single step with
// DelayMinutes=0; the Actions field is then cleared.
type Scene struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Steps []SceneStep `json:"steps"`
	// Actions is the legacy single-step field kept for on-disk
	// backward-compatibility. Populated by old scenes; migrated to
	// Steps on first load. Omitted when empty so new scenes don't carry it.
	Actions []SceneAction `json:"actions,omitempty"`
}

// Timer fires once at FiresAt and is then deleted. Used for "off in 30
// minutes" style quick actions. Persisted so they survive restarts.
type Timer struct {
	ID         string    `json:"id"`
	TargetType string    `json:"target_type"` // "socket" | "group" | "scene"
	TargetID   string    `json:"target_id"`
	Action     string    `json:"action"` // "on" | "off" | "toggle" | "activate"
	FiresAt    time.Time `json:"fires_at"`
	CreatedAt  time.Time `json:"created_at"`
	Note       string    `json:"note,omitempty"` // human-friendly description
}

// Sensor is a 433MHz device that reports a numeric value (temperature,
// humidity, motion, light, etc.). Predefined Kinds get tailored UI; "custom"
// covers anything else.
//
// The (Protocol, Code, Field) triple identifies how the receiver should
// match incoming packets:
//   - Protocol: which decoder produced the packet (e.g. "rtl_433")
//   - Code:     stable per-device identifier (e.g. "Acurite-Tower:1234")
//   - Field:    which JSON key to read as the numeric value (e.g.
//     "temperature_C", "humidity"). Empty means "the only
//     numeric field in the packet" — useful for simple sensors.
type Sensor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`     // temperature|humidity|motion|light|power|custom
	Unit     string `json:"unit"`     // "°C", "%", "lux", "W", ...
	Code     string `json:"code"`     // 433MHz device identifier
	Protocol string `json:"protocol"` // decoder/source label
	Field    string `json:"field,omitempty"`
	Room     string `json:"room,omitempty"`
	// Optional alert thresholds. When set, the UI flags the sensor whenever
	// its latest reading falls below AlertMin or above AlertMax.
	AlertMin      *float64   `json:"alert_min,omitempty"`
	AlertMax      *float64   `json:"alert_max,omitempty"`
	LastValue     *float64   `json:"last_value,omitempty"`
	LastReadingAt *time.Time `json:"last_reading_at,omitempty"`
	// Alerting is true while the latest reading is outside the configured
	// thresholds. Not persisted — resets to false on restart. Used to
	// detect the rising edge of an alert so push notifications are sent
	// only once per threshold breach, not on every subsequent reading.
	Alerting bool `json:"alerting,omitempty"`
}

// SensorReading is one timestamped value for a sensor. Stored in a
// rolling window per sensor (see ReadingsHistorySize).
type SensorReading struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}
