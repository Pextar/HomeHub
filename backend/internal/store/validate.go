package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
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
	case "scene":
		if sch.TargetID == "" {
			return errors.New("target_id is required for scene schedules")
		}
		if _, ok := s.Scenes[sch.TargetID]; !ok {
			return errors.New("target scene does not exist")
		}
		sch.Action = "activate"
	default:
		return errors.New("target_type must be socket, group, or scene")
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

// ValidateSettings normalizes and bounds-checks the settings struct.
func (s *Store) ValidateSettings(set *Settings) error {
	set.LocationName = strings.TrimSpace(set.LocationName)
	if set.Latitude < -90 || set.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}
	if set.Longitude < -180 || set.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}
	set.HueBridgeIP = strings.TrimSpace(set.HueBridgeIP)
	set.HueUsername = strings.TrimSpace(set.HueUsername)
	if (set.HueBridgeIP == "") != (set.HueUsername == "") {
		return errors.New("hue_bridge_ip and hue_username must both be set or both be empty")
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
// and that each action is on/off. Caller must hold Mu.
func (s *Store) ValidateScene(sc *Scene) error {
	sc.Name = strings.TrimSpace(sc.Name)
	if sc.Name == "" {
		return errors.New("name is required")
	}
	seen := make(map[string]bool, len(sc.Actions))
	out := make([]SceneAction, 0, len(sc.Actions))
	for _, a := range sc.Actions {
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
		seen[a.SocketID] = true
		out = append(out, a)
	}
	sc.Actions = out
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
	case "scene":
		if _, ok := s.Scenes[tid]; !ok {
			return errors.New("target scene does not exist")
		}
	default:
		return errors.New("invalid target_type")
	}
	return nil
}
