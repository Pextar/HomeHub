package store

import "time"

// Socket represents a 433MHz controllable socket.
type Socket struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Code     string `json:"code"`     // 433MHz code (e.g., "12345")
	Protocol string `json:"protocol"` // e.g., "nexa", "kaku", "intertechno"
	State    bool   `json:"state"`    // true = on, false = off
	Room     string `json:"room"`     // room/location
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
type Schedule struct {
	ID                   string    `json:"id"`
	SocketID             string    `json:"socket_id,omitempty"`
	TargetType           string    `json:"target_type,omitempty"`
	TargetID             string    `json:"target_id,omitempty"`
	Action               string    `json:"action"` // "on" | "off" | "toggle" | "activate"
	Time                 string    `json:"time"`   // "HH:MM" format
	Days                 []int     `json:"days"`   // 0=Sun, 1=Mon, etc
	Enabled              bool      `json:"enabled"`
	RandomOffsetMinutes  int       `json:"random_offset_minutes,omitempty"` // fire at a random time 0..N minutes after Time
	LastFiredAt          time.Time `json:"last_fired_at,omitempty"`
}

// Group is a manually curated collection of sockets that can be
// controlled together. A socket may belong to any number of groups.
type Group struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	SocketIDs []string `json:"socket_ids"`
}

// SceneAction sets one socket to a specific state when its scene fires.
type SceneAction struct {
	SocketID string `json:"socket_id"`
	Action   string `json:"action"` // "on" | "off"
}

// Scene is a named preset that drives a specific set of sockets to
// specific states ("movie night": lamp ON, ceiling OFF).
type Scene struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Actions []SceneAction `json:"actions"`
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
//               "temperature_C", "humidity"). Empty means "the only
//               numeric field in the packet" — useful for simple sensors.
type Sensor struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`     // temperature|humidity|motion|light|power|custom
	Unit          string     `json:"unit"`     // "°C", "%", "lux", "W", ...
	Code          string     `json:"code"`     // 433MHz device identifier
	Protocol      string     `json:"protocol"` // decoder/source label
	Field         string     `json:"field,omitempty"`
	Room          string     `json:"room,omitempty"`
	LastValue     *float64   `json:"last_value,omitempty"`
	LastReadingAt *time.Time `json:"last_reading_at,omitempty"`
}

// SensorReading is one timestamped value for a sensor. Stored in a
// rolling window per sensor (see ReadingsHistorySize).
type SensorReading struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}
