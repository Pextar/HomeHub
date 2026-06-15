package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"rf-socket-controller/internal/llm"
	"rf-socket-controller/internal/store"
)

// assistant_tools.go is the bridge between the model's tool calls and the
// store. Every executor reuses an existing do* helper (doControlSocket,
// doRoomSetState, doGroupAction, doActivateScene, doBulkSetState) so the
// Mu/staged/off-lock discipline is never re-implemented here. Executors return
// a plain result string fed back to the model as the tool's reply; resolution
// or device errors are returned as text (not Go errors) so the agent loop
// keeps running and the model can react.
//
// v1 is control + Q&A only. Creating/deleting schedules, scenes, and groups is
// deferred to v2 — the existing forms are faster and can't hallucinate.

// assistantTool pairs a model-facing spec with its executor and a flag marking
// bulk/destructive tools that must be confirmed by the user before running.
type assistantTool struct {
	Spec         llm.Tool
	NeedsConfirm bool
	Execute      func(user *store.User, args map[string]any) string
}

// assistantTools builds the tool registry. It is cheap to call per request.
func (s *Server) assistantTools() map[string]assistantTool {
	tools := map[string]assistantTool{
		"get_state": {
			Spec: fnTool("get_state",
				"Re-read the current state of the home. Use after an action to confirm the result, or when you need fresh data. Returns the requested slice as JSON.",
				objSchema(map[string]any{
					"kind": enumProp("Which slice to read", []string{"devices", "rooms", "scenes", "groups", "sensors", "all"}),
				}, nil)),
			Execute: s.toolGetState,
		},
		"get_sensor_readings": {
			Spec: fnTool("get_sensor_readings",
				"Read a sensor's recent readings (temperature, humidity, etc.) to answer questions about trends or current values.",
				objSchema(map[string]any{
					"sensor":        stringProp("Sensor name or id."),
					"since_minutes": intProp("How far back to look, in minutes. Defaults to 60."),
				}, []string{"sensor"})),
			Execute: s.toolGetSensorReadings,
		},
		"control_device": {
			Spec: fnTool("control_device",
				"Turn a single device on or off, or toggle it.",
				objSchema(map[string]any{
					"device": stringProp("Device name or id, e.g. \"kitchen lamp\"."),
					"action": enumProp("What to do", []string{"on", "off", "toggle"}),
				}, []string{"device", "action"})),
			Execute: s.toolControlDevice,
		},
		"activate_scene": {
			Spec: fnTool("activate_scene",
				"Activate a scene, driving its devices to the preset states.",
				objSchema(map[string]any{
					"scene": stringProp("Scene name or id, e.g. \"movie night\"."),
				}, []string{"scene"})),
			Execute: s.toolActivateScene,
		},
		"control_room": {
			Spec: fnTool("control_room",
				"Turn every device in a room on or off. Affects multiple devices, so it requires user confirmation.",
				objSchema(map[string]any{
					"room":   stringProp("Room name, e.g. \"living room\"."),
					"action": enumProp("What to do", []string{"on", "off"}),
				}, []string{"room", "action"})),
			NeedsConfirm: true,
			Execute:      s.toolControlRoom,
		},
		"control_group": {
			Spec: fnTool("control_group",
				"Turn every device in a group on or off, or toggle them. Affects multiple devices, so it requires user confirmation.",
				objSchema(map[string]any{
					"group":  stringProp("Group name or id."),
					"action": enumProp("What to do", []string{"on", "off", "toggle"}),
				}, []string{"group", "action"})),
			NeedsConfirm: true,
			Execute:      s.toolControlGroup,
		},
		"all_devices": {
			Spec: fnTool("all_devices",
				"Turn every device in the home on or off at once. Affects everything, so it requires user confirmation.",
				objSchema(map[string]any{
					"action": enumProp("What to do", []string{"on", "off"}),
				}, []string{"action"})),
			NeedsConfirm: true,
			Execute:      s.toolAllDevices,
		},
	}
	return tools
}

// specsFor returns the model-facing tool specs (the Execute funcs stripped).
func specsFor(tools map[string]assistantTool) []llm.Tool {
	specs := make([]llm.Tool, 0, len(tools))
	for _, t := range tools {
		specs = append(specs, t.Spec)
	}
	return specs
}

// --- executors ---

func (s *Server) toolGetState(user *store.User, args map[string]any) string {
	snap := s.buildSnapshot(user)
	kind := strings.ToLower(argString(args, "kind"))
	var v any
	switch kind {
	case "devices":
		v = map[string]any{"devices": snap.Devices}
	case "rooms":
		v = map[string]any{"rooms": snap.Rooms}
	case "scenes":
		v = map[string]any{"scenes": snap.Scenes}
	case "groups":
		v = map[string]any{"groups": snap.Groups}
	case "sensors":
		v = map[string]any{"sensors": snap.Sensors}
	default:
		v = snap
	}
	return toJSON(v)
}

func (s *Server) toolGetSensorReadings(user *store.User, args map[string]any) string {
	id, name, ok, reason := s.resolveSensor(argString(args, "sensor"))
	if !ok {
		return reason
	}
	since := time.Duration(argInt(args, "since_minutes", 60)) * time.Minute
	cutoff := time.Now().Add(-since)

	s.Store.Mu.RLock()
	all := s.Store.Readings[id]
	type point struct {
		Time  time.Time
		Value float64
	}
	out := make([]point, 0, len(all))
	for _, rd := range all {
		if rd.Time.After(cutoff) {
			out = append(out, point{Time: rd.Time, Value: rd.Value})
		}
	}
	sn := s.Store.Sensors[id]
	unit := ""
	var last *float64
	if sn != nil {
		unit = sn.Unit
		last = sn.LastValue
	}
	s.Store.Mu.RUnlock()

	result := map[string]any{
		"sensor": name,
		"unit":   unit,
		"latest": last,
		"window": fmt.Sprintf("last %d min", int(since.Minutes())),
		"count":  len(out),
	}
	// Summarise instead of dumping every point with a full RFC3339 timestamp —
	// the raw list bloats the next round's prompt and slows the model. Report
	// min/max/avg over the window plus the few most recent points with short
	// HH:MM times.
	if len(out) > 0 {
		min, max, sum := out[0].Value, out[0].Value, 0.0
		for _, p := range out {
			if p.Value < min {
				min = p.Value
			}
			if p.Value > max {
				max = p.Value
			}
			sum += p.Value
		}
		result["min"] = min
		result["max"] = max
		result["avg"] = sum / float64(len(out))

		const maxPts = 6
		recent := out
		if len(recent) > maxPts {
			recent = recent[len(recent)-maxPts:]
		}
		pts := make([]map[string]any, len(recent))
		for i, p := range recent {
			pts[i] = map[string]any{"t": p.Time.Format("15:04"), "v": p.Value}
		}
		result["recent"] = pts
	}
	return toJSON(result)
}

func (s *Server) toolControlDevice(user *store.User, args map[string]any) string {
	action := normalizeAction(argString(args, "action"))
	sock, ok, reason := s.resolveSocket(user, argString(args, "device"))
	if !ok {
		return reason
	}
	updated, found, err := s.doControlSocket(sock.ID, action)
	if !found {
		return "device no longer exists"
	}
	if err != nil {
		return fmt.Sprintf("failed to turn %s %s: %s", sock.Name, action, err.Error())
	}
	return toJSON(map[string]any{
		"device": updated.Name,
		"state":  onOff(updated.State),
	})
}

func (s *Server) toolActivateScene(user *store.User, args map[string]any) string {
	id, name, ok, reason := s.resolveScene(argString(args, "scene"))
	if !ok {
		return reason
	}
	_, okCount, failures, found, err := s.doActivateScene(id)
	if !found {
		return "scene no longer exists"
	}
	if err != nil {
		return "failed to activate scene: " + err.Error()
	}
	return toJSON(map[string]any{"scene": name, "devices_updated": okCount, "failures": len(failures)})
}

func (s *Server) toolControlRoom(user *store.User, args map[string]any) string {
	action := normalizeAction(argString(args, "action"))
	room, ok, reason := s.resolveRoom(argString(args, "room"))
	if !ok {
		return reason
	}
	okCount, failures, found, err := s.doRoomSetState(user, room, action == "on")
	if !found {
		return "no devices in " + room
	}
	if err != nil {
		return "failed to control room: " + err.Error()
	}
	return toJSON(map[string]any{"room": room, "action": action, "devices_updated": okCount, "failures": len(failures)})
}

func (s *Server) toolControlGroup(user *store.User, args map[string]any) string {
	action := normalizeAction(argString(args, "action"))
	id, name, ok, reason := s.resolveGroup(argString(args, "group"))
	if !ok {
		return reason
	}
	_, okCount, failures, found, err := s.doGroupAction(id, action)
	if !found {
		return "group no longer exists"
	}
	if err != nil {
		return "failed to control group: " + err.Error()
	}
	return toJSON(map[string]any{"group": name, "action": action, "devices_updated": okCount, "failures": len(failures)})
}

func (s *Server) toolAllDevices(user *store.User, args map[string]any) string {
	action := normalizeAction(argString(args, "action"))
	okCount, failures, err := s.doBulkSetState(user, action == "on")
	if err != nil {
		return "failed to control all devices: " + err.Error()
	}
	return toJSON(map[string]any{"action": action, "devices_updated": okCount, "failures": len(failures)})
}

// --- schema + arg helpers ---

func fnTool(name, desc string, params map[string]any) llm.Tool {
	return llm.Tool{
		Type:     "function",
		Function: llm.ToolFunction{Name: name, Description: desc, Parameters: params},
	}
}

func objSchema(props map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func enumProp(desc string, values []string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

// argString extracts a trimmed string argument, tolerating the model passing a
// number or bool where a string was asked for.
func argString(args map[string]any, key string) string {
	switch v := args[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	case bool:
		return fmt.Sprintf("%v", v)
	default:
		return ""
	}
}

// argInt extracts an integer argument, accepting a JSON number or a numeric
// string, falling back to def.
func argInt(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err == nil {
			return n
		}
	}
	return def
}

// normalizeAction lowercases and maps common synonyms to the canonical verbs.
func normalizeAction(a string) string {
	switch strings.ToLower(strings.TrimSpace(a)) {
	case "on", "turn_on", "turn on", "enable", "activate":
		return "on"
	case "off", "turn_off", "turn off", "disable":
		return "off"
	case "toggle", "flip", "switch":
		return "toggle"
	default:
		return strings.ToLower(strings.TrimSpace(a))
	}
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("{\"error\":%q}", err.Error())
	}
	return string(b)
}
