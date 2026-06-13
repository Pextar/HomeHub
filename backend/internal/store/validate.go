package store

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"rf-socket-controller/internal/tasmota"
)

// ValidateSchedule normalizes and validates a schedule. Caller must
// hold Mu (read lock at minimum) so target existence can be checked.
func (s *Store) ValidateSchedule(sch *Schedule) error {
	sch.SocketID = strings.TrimSpace(sch.SocketID)
	sch.TargetType = strings.ToLower(strings.TrimSpace(sch.TargetType))
	sch.TargetID = strings.TrimSpace(sch.TargetID)
	sch.Action = strings.ToLower(strings.TrimSpace(sch.Action))
	sch.TimeMode = strings.ToLower(strings.TrimSpace(sch.TimeMode))
	sch.Time = strings.TrimSpace(sch.Time)

	// Backwards compat: socket_id alone implies a socket target.
	if sch.TargetType == "" && sch.SocketID != "" {
		sch.TargetType = "socket"
		sch.TargetID = sch.SocketID
	}
	if sch.TargetType == "socket" {
		sch.SocketID = sch.TargetID
	} else {
		sch.SocketID = ""
	}

	switch sch.TargetType {
	case "socket":
		if sch.TargetID == "" {
			return errors.New("socket_id (or target_id) is required")
		}
		if _, ok := s.Sockets[sch.TargetID]; !ok {
			return errors.New("target socket does not exist")
		}
		if sch.Action != "on" && sch.Action != "off" && sch.Action != "toggle" {
			return errors.New("socket action must be on/off/toggle")
		}
	case "group":
		if sch.TargetID == "" {
			return errors.New("target_id is required for group schedules")
		}
		if _, ok := s.Groups[sch.TargetID]; !ok {
			return errors.New("target group does not exist")
		}
		if sch.Action != "on" && sch.Action != "off" && sch.Action != "toggle" {
			return errors.New("group action must be on/off/toggle")
		}
	case "room":
		if sch.TargetID == "" {
			return errors.New("target_id is required for room schedules")
		}
		if _, ok := s.Rooms[sch.TargetID]; !ok {
			return errors.New("target room does not exist")
		}
		if sch.Action != "on" && sch.Action != "off" && sch.Action != "toggle" {
			return errors.New("room action must be on/off/toggle")
		}
	case "scene":
		if sch.TargetID == "" {
			return errors.New("target_id is required for scene schedules")
		}
		if _, ok := s.Scenes[sch.TargetID]; !ok {
			return errors.New("target scene does not exist")
		}
		sch.Action = "activate"
	default:
		return errors.New("target_type must be socket, group, room, or scene")
	}

	switch sch.TimeMode {
	case "", ModeFixed:
		sch.TimeMode = ModeFixed
		sch.SolarOffsetMinutes = 0
		if _, err := time.Parse("15:04", sch.Time); err != nil {
			return errors.New("time must be in HH:MM format")
		}
	case ModeSunrise, ModeSunset:
		// Time isn't used for solar schedules; drop any stale value so
		// the persisted record doesn't suggest otherwise.
		sch.Time = ""
		if sch.SolarOffsetMinutes < -120 || sch.SolarOffsetMinutes > 120 {
			return errors.New("solar_offset_minutes must be between -120 and 120")
		}
	default:
		return errors.New("time_mode must be fixed, sunrise, or sunset")
	}
	for _, d := range sch.Days {
		if d < 0 || d > 6 {
			return errors.New("days values must be 0-6 (Sun-Sat)")
		}
	}
	if sch.RandomOffsetMinutes < 0 || sch.RandomOffsetMinutes > 120 {
		return errors.New("random_offset_minutes must be between 0 and 120")
	}
	return nil
}

// ValidateAutomation normalizes and validates an automation: its name and one
// or more rules, each with a trigger, optional conditions, and ordered actions.
// Referenced sockets/groups/scenes/sensors must exist. Caller must hold Mu
// (read lock at minimum).
func (s *Store) ValidateAutomation(a *Automation) error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return errors.New("name is required")
	}
	if len(a.Rules) == 0 {
		return errors.New("at least one rule is required")
	}
	for i := range a.Rules {
		if err := s.validateRule(&a.Rules[i]); err != nil {
			return err
		}
	}
	return nil
}

// validateRule normalizes and validates one trigger → conditions → actions
// rule. Caller must hold Mu (read lock at minimum).
func (s *Store) validateRule(a *AutomationRule) error {
	// ── Trigger ──────────────────────────────────────────────
	t := &a.Trigger
	t.Type = strings.ToLower(strings.TrimSpace(t.Type))
	switch t.Type {
	case "time":
		t.TimeMode = strings.ToLower(strings.TrimSpace(t.TimeMode))
		t.Time = strings.TrimSpace(t.Time)
		switch t.TimeMode {
		case "", ModeFixed:
			t.TimeMode = ModeFixed
			t.SolarOffsetMinutes = 0
			if _, err := time.Parse("15:04", t.Time); err != nil {
				return errors.New("trigger time must be in HH:MM format")
			}
		case ModeSunrise, ModeSunset:
			t.Time = ""
			if t.SolarOffsetMinutes < -120 || t.SolarOffsetMinutes > 120 {
				return errors.New("solar_offset_minutes must be between -120 and 120")
			}
		default:
			return errors.New("trigger time_mode must be fixed, sunrise, or sunset")
		}
		for _, d := range t.Days {
			if d < 0 || d > 6 {
				return errors.New("trigger days must be 0-6 (Sun-Sat)")
			}
		}
	case "sensor":
		t.SensorID = strings.TrimSpace(t.SensorID)
		t.Op = strings.ToLower(strings.TrimSpace(t.Op))
		if _, ok := s.Sensors[t.SensorID]; !ok {
			return errors.New("trigger sensor does not exist")
		}
		if t.Op != "above" && t.Op != "below" {
			return errors.New("sensor trigger op must be above or below")
		}
	case "device":
		t.SocketID = strings.TrimSpace(t.SocketID)
		t.ToState = strings.ToLower(strings.TrimSpace(t.ToState))
		if _, ok := s.Sockets[t.SocketID]; !ok {
			return errors.New("trigger device does not exist")
		}
		if t.ToState != "on" && t.ToState != "off" {
			return errors.New("device trigger to_state must be on or off")
		}
	default:
		return errors.New("trigger type must be time, sensor, or device")
	}

	// ── Conditions (optional, AND) ───────────────────────────
	for i := range a.Conditions {
		c := &a.Conditions[i]
		c.Type = strings.ToLower(strings.TrimSpace(c.Type))
		switch c.Type {
		case "device":
			c.SocketID = strings.TrimSpace(c.SocketID)
			c.State = strings.ToLower(strings.TrimSpace(c.State))
			if _, ok := s.Sockets[c.SocketID]; !ok {
				return errors.New("condition device does not exist")
			}
			if c.State != "on" && c.State != "off" {
				return errors.New("device condition state must be on or off")
			}
		case "time_range":
			if _, err := time.Parse("15:04", strings.TrimSpace(c.After)); err != nil {
				return errors.New("condition after must be in HH:MM format")
			}
			if _, err := time.Parse("15:04", strings.TrimSpace(c.Before)); err != nil {
				return errors.New("condition before must be in HH:MM format")
			}
			c.After = strings.TrimSpace(c.After)
			c.Before = strings.TrimSpace(c.Before)
		default:
			return errors.New("condition type must be device or time_range")
		}
	}

	// ── Actions (ordered, at least one) ──────────────────────
	if len(a.Actions) == 0 {
		return errors.New("at least one action is required")
	}
	for i := range a.Actions {
		act := &a.Actions[i]
		act.TargetType = strings.ToLower(strings.TrimSpace(act.TargetType))
		act.TargetID = strings.TrimSpace(act.TargetID)
		act.Action = strings.ToLower(strings.TrimSpace(act.Action))
		if err := s.VerifyTarget(act.TargetType, act.TargetID); err != nil {
			return err
		}
		if act.TargetType == "scene" {
			act.Action = "activate"
		} else if act.Action != "on" && act.Action != "off" && act.Action != "toggle" {
			return errors.New("action must be on/off/toggle")
		}
		// Level/colour apply to smart sockets, groups, and rooms being switched on.
		// Scheduler fans out QueueLight to each smart socket inside the target.
		if act.Action == "on" && (act.TargetType == "socket" || act.TargetType == "group" || act.TargetType == "room") {
			if act.Level != nil {
				if *act.Level < 1 || *act.Level > 100 {
					return errors.New("action level must be between 1 and 100")
				}
			}
			act.Color = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(act.Color)), "#")
			if act.Color != "" && !isHex6(act.Color) {
				return errors.New("action color must be a 6-digit RRGGBB hex")
			}
		} else {
			act.Level = nil
			act.Color = ""
		}
	}
	return nil
}

// ValidateSettings normalizes and bounds-checks the settings struct.
func (s *Store) ValidateSettings(set *Settings) error {
	set.LocationName = strings.TrimSpace(set.LocationName)
	if set.Latitude < -90 || set.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}
	if set.Longitude < -180 || set.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}
	return nil
}

// ValidateGroup normalizes a group, dedupes its socket IDs and verifies
// every member exists. Caller must hold Mu.
func (s *Store) ValidateGroup(g *Group) error {
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" {
		return errors.New("name is required")
	}
	seen := make(map[string]bool, len(g.SocketIDs))
	out := make([]string, 0, len(g.SocketIDs))
	for _, sid := range g.SocketIDs {
		sid = strings.TrimSpace(sid)
		if sid == "" || seen[sid] {
			continue
		}
		if _, ok := s.Sockets[sid]; !ok {
			return fmt.Errorf("unknown socket %q", sid)
		}
		seen[sid] = true
		out = append(out, sid)
	}
	g.SocketIDs = out
	return nil
}

// ValidateScene checks that every socket referenced by the scene exists
// and that each action is on/off. The same socket may appear in different
// steps but is deduplicated within a single step. Caller must hold Mu.
func (s *Store) ValidateScene(sc *Scene) error {
	sc.Name = strings.TrimSpace(sc.Name)
	if sc.Name == "" {
		return errors.New("name is required")
	}

	// Optional tile identity. Icon is a frontend icon-set name (format-checked
	// only — unknown names simply don't render); colour is one of a fixed set
	// of accent presets that map to design tokens on the client.
	sc.Icon = strings.TrimSpace(sc.Icon)
	if sc.Icon != "" && !isIconName(sc.Icon) {
		return errors.New("scene icon must be a short alphanumeric name")
	}
	sc.Color = strings.ToLower(strings.TrimSpace(sc.Color))
	if sc.Color != "" && !sceneAccents[sc.Color] {
		return errors.New("scene color must be one of: amber, cool, violet, orange, green, gold")
	}

	// Migrate legacy flat-actions format to steps so the rest of the
	// validation only has to handle one shape.
	if len(sc.Steps) == 0 && len(sc.Actions) > 0 {
		sc.Steps = []SceneStep{{DelayMinutes: 0, Actions: sc.Actions}}
		sc.Actions = nil
	}

	// Steps are optional — a scene without steps can still be useful as a
	// named container for automated rules. Validate each step that is present.
	for si := range sc.Steps {
		step := &sc.Steps[si]
		if step.DelayMinutes < 0 {
			return errors.New("delay_minutes must be zero or positive")
		}

		// Deduplicate socket IDs within this step (same socket can appear
		// in different steps but not twice in the same step).
		seen := make(map[string]bool, len(step.Actions))
		out := make([]SceneAction, 0, len(step.Actions))
		for _, a := range step.Actions {
			a.SocketID = strings.TrimSpace(a.SocketID)
			a.Action = strings.ToLower(strings.TrimSpace(a.Action))
			if a.SocketID == "" || seen[a.SocketID] {
				continue
			}
			if a.Action != "on" && a.Action != "off" {
				return errors.New("scene action must be on or off")
			}
			if _, ok := s.Sockets[a.SocketID]; !ok {
				return fmt.Errorf("unknown socket %q", a.SocketID)
			}
			// Brightness/colour only make sense for a light being switched on.
			if a.Action != "on" {
				a.Level = nil
				a.Color = ""
			} else {
				if a.Level != nil {
					if *a.Level < 1 || *a.Level > 100 {
						return errors.New("scene level must be between 1 and 100")
					}
				}
				a.Color = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(a.Color)), "#")
				if a.Color != "" && !isHex6(a.Color) {
					return errors.New("scene color must be a 6-digit RRGGBB hex")
				}
			}
			seen[a.SocketID] = true
			out = append(out, a)
		}
		step.Actions = out
	}

	// Drop empty steps (steps that had only blank/duplicate socket IDs).
	active := sc.Steps[:0]
	for _, step := range sc.Steps {
		if len(step.Actions) > 0 {
			active = append(active, step)
		}
	}
	sc.Steps = active

	return nil
}

// ValidateSensor normalizes and validates a sensor. Caller must hold Mu.
// Allowed kinds: temperature, humidity, motion, light, power, custom.
func (s *Store) ValidateSensor(sn *Sensor) error {
	sn.Name = strings.TrimSpace(sn.Name)
	sn.Kind = strings.ToLower(strings.TrimSpace(sn.Kind))
	sn.Unit = strings.TrimSpace(sn.Unit)
	sn.Code = strings.TrimSpace(sn.Code)
	sn.Protocol = strings.TrimSpace(sn.Protocol)
	sn.Field = strings.TrimSpace(sn.Field)
	sn.Room = strings.TrimSpace(sn.Room)

	if sn.Name == "" {
		return errors.New("name is required")
	}
	switch sn.Kind {
	case "temperature", "humidity", "motion", "light", "power", "custom":
	case "":
		sn.Kind = "custom"
	default:
		return errors.New("kind must be temperature, humidity, motion, light, power, or custom")
	}
	if sn.Code == "" {
		return errors.New("code is required")
	}
	if sn.Unit == "" {
		sn.Unit = defaultUnitForKind(sn.Kind)
	}
	return nil
}

// sceneAccents is the allow-list of scene tile accent presets. Each key maps
// to a design token on the client (amber→--on, cool→--cool, etc.), so the
// stored value stays theme-aware rather than baking in a hex.
var sceneAccents = map[string]bool{
	"amber": true, "cool": true, "violet": true,
	"orange": true, "green": true, "gold": true,
}

// isIconName reports whether s is a plausible frontend icon name: it must start
// with a letter and contain only letters/digits, max 32 chars. The actual icon
// set lives in the frontend; we only guard against junk being persisted.
func isIconName(s string) bool {
	if len(s) == 0 || len(s) > 32 {
		return false
	}
	for i, c := range s {
		isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isDigit := c >= '0' && c <= '9'
		if i == 0 && !isLetter {
			return false
		}
		if !isLetter && !isDigit {
			return false
		}
	}
	return true
}

func isHex6(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func defaultUnitForKind(kind string) string {
	switch kind {
	case "temperature":
		return "°C"
	case "humidity":
		return "%"
	case "light":
		return "lux"
	case "power":
		return "W"
	case "motion":
		return ""
	}
	return ""
}

// ValidateRoom normalizes and validates a room. Caller must hold Mu.
func (s *Store) ValidateRoom(r *Room) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("name is required")
	}
	for _, existing := range s.Rooms {
		if existing.ID != r.ID && strings.EqualFold(existing.Name, r.Name) {
			return errors.New("a room with that name already exists")
		}
	}
	return nil
}

// ValidateSocket normalizes and validates a socket. The Nexa/Proove protocol
// requires the code to be in "houseID:unit" format; this is checked so that
// malformed codes are rejected at save time rather than discovered later when
// the first toggle command fails with a cryptic parse error.
func (s *Store) ValidateSocket(sock *Socket) error {
	sock.Name = strings.TrimSpace(sock.Name)
	sock.Code = strings.TrimSpace(sock.Code)
	sock.Protocol = strings.ToLower(strings.TrimSpace(sock.Protocol))
	sock.Room = strings.TrimSpace(sock.Room)
	sock.Emoji = strings.TrimSpace(sock.Emoji)

	if sock.Name == "" {
		return errors.New("name is required")
	}
	if sock.Code == "" {
		return errors.New("code is required")
	}
	switch sock.Protocol {
	case "nexa":
		if err := validateNexaCode(sock.Code); err != nil {
			return err
		}
	case "tasmota":
		// Code is the device host/IP, interpolated into a server-side URL.
		// Reject anything that could point the request at a non-device host.
		if err := tasmota.ValidateHost(sock.Code); err != nil {
			return err
		}
	case "matter", "matter-thread":
		// Code is the Matter node id (a decimal string). Keep it numeric so it
		// can't smuggle path segments into the bridge URL, even though it's
		// already path-escaped at request time.
		if _, err := strconv.ParseUint(sock.Code, 10, 64); err != nil {
			return errors.New("matter node id must be a decimal number")
		}
	}
	return nil
}

// ValidateNexaCode checks that code is in "houseID:unit" format with values
// within the protocol's 26-bit / 4-bit ranges. The same constraints are
// enforced at transmit time by nexa_tx.py; surfacing them here produces a
// clear error at save time instead of a confusing failure when the socket is
// first toggled.
//
// Exported so the api package can call it when re-using an existing code for
// the learnSocket "resend same code" path.
func ValidateNexaCode(code string) error {
	return validateNexaCode(code)
}

func validateNexaCode(code string) error {
	parts := strings.SplitN(code, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf(
			`Nexa/Proove code must be "houseID:unit", e.g. "12345678:0" — ` +
				`use the "Pair with socket" button to generate one automatically`,
		)
	}
	house, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("Nexa house ID %q must be a number", parts[0])
	}
	unit, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("Nexa unit %q must be a number", parts[1])
	}
	if house < 0 || house >= (1<<26) {
		return fmt.Errorf("Nexa house ID %d out of range (0–67108863)", house)
	}
	if unit < 0 || unit > 15 {
		return fmt.Errorf("Nexa unit %d out of range (0–15)", unit)
	}
	return nil
}

// VerifyTarget checks that a target_type/target_id pair refers to an
// existing entity. Caller must hold Mu.
func (s *Store) VerifyTarget(tt, tid string) error {
	switch tt {
	case "socket":
		if _, ok := s.Sockets[tid]; !ok {
			return errors.New("target socket does not exist")
		}
	case "group":
		if _, ok := s.Groups[tid]; !ok {
			return errors.New("target group does not exist")
		}
	case "room":
		if _, ok := s.Rooms[tid]; !ok {
			return errors.New("target room does not exist")
		}
	case "scene":
		if _, ok := s.Scenes[tid]; !ok {
			return errors.New("target scene does not exist")
		}
	default:
		return errors.New("invalid target_type")
	}
	return nil
}
