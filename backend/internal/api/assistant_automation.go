package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"rf-socket-controller/internal/llm"
	"rf-socket-controller/internal/store"
)

// assistant_automation.go adds the one tool that lets the model build a
// persistent automation from plain language: create_automation. Unlike the
// control tools (which reuse a do* helper), this one assembles a
// store.Automation, resolving every human name the model emits to an entity id
// OFF the store lock, then validates and saves it under Mu — the same path the
// REST createAutomation handler uses. It is confirm-gated: the model never
// writes config without the user approving the summary card first.

// The *Input structs mirror store.Automation but reference entities by NAME
// instead of id, since that is what the model has from the system-prompt
// snapshot. They are populated by JSON round-tripping the tool-call args.
type automationTriggerInput struct {
	Type string `json:"type"` // "time" | "sensor" | "device"

	// time
	Time               string `json:"time,omitempty"`      // "HH:MM" for a fixed time
	TimeMode           string `json:"time_mode,omitempty"` // "fixed" | "sunrise" | "sunset"
	SolarOffsetMinutes int    `json:"solar_offset_minutes,omitempty"`
	Days               []int  `json:"days,omitempty"` // 0=Sun..6=Sat; empty == every day

	// sensor
	Sensor string  `json:"sensor,omitempty"` // sensor name or id
	Op     string  `json:"op,omitempty"`     // "above" | "below"
	Value  float64 `json:"value,omitempty"`

	// device
	Device  string `json:"device,omitempty"`   // device name or id
	ToState string `json:"to_state,omitempty"` // "on" | "off"
}

type automationConditionInput struct {
	Type string `json:"type"` // "device" | "time_range" | "time_before" | "time_after"

	Device string `json:"device,omitempty"` // device name or id
	State  string `json:"state,omitempty"`  // "on" | "off"
	After  string `json:"after,omitempty"`  // "HH:MM"
	Before string `json:"before,omitempty"` // "HH:MM"
}

type automationActionInput struct {
	TargetType string `json:"target_type"` // "device" | "group" | "room" | "scene"
	Target     string `json:"target"`      // name or id of the target
	Action     string `json:"action"`      // "on" | "off" | "toggle" (scene => activate)
	Level      *int   `json:"level,omitempty"`
	Color      string `json:"color,omitempty"`
}

type automationRuleInput struct {
	Trigger    automationTriggerInput     `json:"trigger"`
	Conditions []automationConditionInput `json:"conditions,omitempty"`
	Actions    []automationActionInput    `json:"actions"`
}

type createAutomationInput struct {
	Name  string                `json:"name"`
	Rules []automationRuleInput `json:"rules"`
}

// createAutomationTool is the model-facing spec. Kept verbose so a small model
// gets the nested shape right; the description spells out each trigger kind.
func createAutomationTool() llm.Tool {
	trigger := objSchema(map[string]any{
		"type":                 enumProp("Trigger kind: \"time\" (clock or sunrise/sunset), \"sensor\" (a reading crosses a threshold), or \"device\" (a device turns on/off).", []string{"time", "sensor", "device"}),
		"time":                 stringProp("For a time trigger with a fixed clock time: \"HH:MM\" (24h)."),
		"time_mode":            enumProp("For a time trigger: \"fixed\" (use time), \"sunrise\", or \"sunset\". Defaults to fixed.", []string{"fixed", "sunrise", "sunset"}),
		"solar_offset_minutes": intProp("Offset from sunrise/sunset in minutes, -120..120 (e.g. -15 = 15 min before)."),
		"days":                 map[string]any{"type": "array", "description": "Days the trigger may fire, 0=Sun..6=Sat. Empty means every day.", "items": map[string]any{"type": "integer"}},
		"sensor":               stringProp("For a sensor trigger: the sensor name."),
		"op":                   enumProp("For a sensor trigger: fire when the reading goes above or below value.", []string{"above", "below"}),
		"value":                map[string]any{"type": "number", "description": "For a sensor trigger: the threshold value."},
		"device":               stringProp("For a device trigger: the device name to watch."),
		"to_state":             enumProp("For a device trigger: fire when the device turns to this state.", []string{"on", "off"}),
	}, []string{"type"})

	condition := objSchema(map[string]any{
		"type":   enumProp("Condition kind. All conditions must hold for the actions to run.", []string{"device", "time_range", "time_before", "time_after"}),
		"device": stringProp("For a device condition: the device name."),
		"state":  enumProp("For a device condition: the device must currently be in this state.", []string{"on", "off"}),
		"after":  stringProp("\"HH:MM\". Start of a time_range, or the lower bound for time_after."),
		"before": stringProp("\"HH:MM\". End of a time_range, or the upper bound for time_before."),
	}, []string{"type"})

	action := objSchema(map[string]any{
		"target_type": enumProp("What the action targets.", []string{"device", "group", "room", "scene"}),
		"target":      stringProp("Name of the device, group, room, or scene to act on."),
		"action":      enumProp("What to do. Use \"activate\" only for a scene; on/off/toggle for everything else.", []string{"on", "off", "toggle", "activate"}),
		"level":       intProp("Optional brightness 1-100, smart lights only, when turning on."),
		"color":       stringProp("Optional colour \"RRGGBB\", smart lights only, when turning on."),
	}, []string{"target_type", "target", "action"})

	rule := objSchema(map[string]any{
		"trigger":    trigger,
		"conditions": map[string]any{"type": "array", "description": "Optional gating conditions (logical AND).", "items": condition},
		"actions":    map[string]any{"type": "array", "description": "Ordered actions to run when the rule fires. At least one.", "items": action},
	}, []string{"trigger", "actions"})

	params := objSchema(map[string]any{
		"name":  stringProp("A short human name for the automation, e.g. \"Evening lights\"."),
		"rules": map[string]any{"type": "array", "description": "One or more independent trigger -> conditions -> actions rules.", "items": rule},
	}, []string{"name", "rules"})

	return fnTool("create_automation",
		"Create a new automation that runs actions automatically when a trigger fires (a time/sunrise/sunset, a sensor crossing a threshold, or a device changing state), optionally gated by conditions. Use this when the user wants something to happen automatically rather than right now. The app shows the user a confirmation card before it is saved.",
		params)
}

// toolCreateAutomation builds, validates, and persists the automation. Name
// resolution happens off-lock; validation + Save run under Mu, mirroring the
// REST createAutomation handler. Any failure is returned as text so the agent
// loop can relay it and let the model retry with corrected names.
func (s *Server) toolCreateAutomation(user *store.User, args map[string]any) string {
	in, err := decodeAutomationInput(args)
	if err != nil {
		return "could not read the automation: " + err.Error()
	}
	auto, reason := s.buildAutomation(user, in)
	if reason != "" {
		return reason
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()
	if err := s.Store.ValidateAutomation(auto); err != nil {
		return "the automation is not valid: " + err.Error()
	}
	auto.ID = fmt.Sprintf("automation_%d", time.Now().UnixNano())
	s.Store.Automations[auto.ID] = auto
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Automations, auto.ID)
		return "failed to save the automation: " + err.Error()
	}
	return toJSON(map[string]any{
		"created": auto.Name,
		"id":      auto.ID,
		"rules":   len(auto.Rules),
		"enabled": auto.Enabled,
	})
}

// decodeAutomationInput re-marshals the loosely-typed tool args into the typed
// input shape. Round-tripping through JSON is simpler and more robust than
// hand-walking the nested map[string]any the model produced.
func decodeAutomationInput(args map[string]any) (createAutomationInput, error) {
	var in createAutomationInput
	raw, err := json.Marshal(args)
	if err != nil {
		return in, err
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, err
	}
	return in, nil
}

// buildAutomation assembles a store.Automation with all names resolved to ids,
// or returns a non-empty reason the model can act on. New automations are
// enabled immediately. Caller must NOT hold Mu.
func (s *Server) buildAutomation(user *store.User, in createAutomationInput) (*store.Automation, string) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, "the automation needs a name"
	}
	if len(in.Rules) == 0 {
		return nil, "the automation needs at least one rule (a trigger and an action)"
	}
	auto := &store.Automation{Name: name, Enabled: true}
	for ri := range in.Rules {
		r := in.Rules[ri]
		rule := store.AutomationRule{}

		trig, reason := s.buildTrigger(user, r.Trigger)
		if reason != "" {
			return nil, fmt.Sprintf("rule %d: %s", ri+1, reason)
		}
		rule.Trigger = trig

		for _, c := range r.Conditions {
			cond, reason := s.buildCondition(user, c)
			if reason != "" {
				return nil, fmt.Sprintf("rule %d: %s", ri+1, reason)
			}
			rule.Conditions = append(rule.Conditions, cond)
		}

		if len(r.Actions) == 0 {
			return nil, fmt.Sprintf("rule %d needs at least one action", ri+1)
		}
		for _, a := range r.Actions {
			act, reason := s.buildAction(user, a)
			if reason != "" {
				return nil, fmt.Sprintf("rule %d: %s", ri+1, reason)
			}
			rule.Actions = append(rule.Actions, act)
		}
		auto.Rules = append(auto.Rules, rule)
	}
	return auto, ""
}

func (s *Server) buildTrigger(user *store.User, t automationTriggerInput) (store.AutomationTrigger, string) {
	switch strings.ToLower(strings.TrimSpace(t.Type)) {
	case "time":
		return store.AutomationTrigger{
			Type:               "time",
			TimeMode:           strings.ToLower(strings.TrimSpace(t.TimeMode)),
			Time:               strings.TrimSpace(t.Time),
			SolarOffsetMinutes: t.SolarOffsetMinutes,
			Days:               t.Days,
		}, ""
	case "sensor":
		id, _, ok, reason := s.resolveSensor(t.Sensor)
		if !ok {
			return store.AutomationTrigger{}, reason
		}
		return store.AutomationTrigger{
			Type:     "sensor",
			SensorID: id,
			Op:       strings.ToLower(strings.TrimSpace(t.Op)),
			Value:    t.Value,
		}, ""
	case "device":
		sock, ok, reason := s.resolveSocket(user, t.Device)
		if !ok {
			return store.AutomationTrigger{}, reason
		}
		return store.AutomationTrigger{
			Type:     "device",
			SocketID: sock.ID,
			ToState:  strings.ToLower(strings.TrimSpace(t.ToState)),
		}, ""
	default:
		return store.AutomationTrigger{}, "trigger type must be time, sensor, or device"
	}
}

func (s *Server) buildCondition(user *store.User, c automationConditionInput) (store.AutomationCondition, string) {
	switch strings.ToLower(strings.TrimSpace(c.Type)) {
	case "device":
		sock, ok, reason := s.resolveSocket(user, c.Device)
		if !ok {
			return store.AutomationCondition{}, reason
		}
		return store.AutomationCondition{Type: "device", SocketID: sock.ID, State: strings.ToLower(strings.TrimSpace(c.State))}, ""
	case "time_range":
		return store.AutomationCondition{Type: "time_range", After: strings.TrimSpace(c.After), Before: strings.TrimSpace(c.Before)}, ""
	case "time_before":
		return store.AutomationCondition{Type: "time_before", Before: strings.TrimSpace(c.Before)}, ""
	case "time_after":
		return store.AutomationCondition{Type: "time_after", After: strings.TrimSpace(c.After)}, ""
	default:
		return store.AutomationCondition{}, "condition type must be device, time_range, time_before, or time_after"
	}
}

func (s *Server) buildAction(user *store.User, a automationActionInput) (store.AutomationAction, string) {
	switch strings.ToLower(strings.TrimSpace(a.TargetType)) {
	case "device", "socket":
		sock, ok, reason := s.resolveSocket(user, a.Target)
		if !ok {
			return store.AutomationAction{}, reason
		}
		return store.AutomationAction{TargetType: "socket", TargetID: sock.ID, Action: normalizeAction(a.Action), Level: a.Level, Color: a.Color}, ""
	case "group":
		id, _, ok, reason := s.resolveGroup(a.Target)
		if !ok {
			return store.AutomationAction{}, reason
		}
		return store.AutomationAction{TargetType: "group", TargetID: id, Action: normalizeAction(a.Action), Level: a.Level, Color: a.Color}, ""
	case "room":
		id, _, ok, reason := s.resolveRoomID(a.Target)
		if !ok {
			return store.AutomationAction{}, reason
		}
		return store.AutomationAction{TargetType: "room", TargetID: id, Action: normalizeAction(a.Action), Level: a.Level, Color: a.Color}, ""
	case "scene":
		id, _, ok, reason := s.resolveScene(a.Target)
		if !ok {
			return store.AutomationAction{}, reason
		}
		return store.AutomationAction{TargetType: "scene", TargetID: id, Action: "activate"}, ""
	default:
		return store.AutomationAction{}, "action target_type must be device, group, room, or scene"
	}
}

// --- confirmation summary ---

// summarizeAutomation renders the human review sentence and the list of action
// targets for the confirmation card, working from the raw (name-based) input so
// it does not need the store. A malformed payload still yields a safe sentence.
func summarizeAutomation(args map[string]any) (string, []string) {
	in, err := decodeAutomationInput(args)
	if err != nil {
		return "Create this automation?", nil
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "(unnamed)"
	}
	var parts []string
	var affected []string
	for _, r := range in.Rules {
		var dos []string
		for _, a := range r.Actions {
			dos = append(dos, describeAction(a))
			if t := strings.TrimSpace(a.Target); t != "" {
				affected = append(affected, t)
			}
		}
		seg := "when " + describeTrigger(r.Trigger)
		if len(dos) > 0 {
			seg += ", " + strings.Join(dos, " and ")
		}
		if len(r.Conditions) > 0 {
			var cs []string
			for _, c := range r.Conditions {
				cs = append(cs, describeCondition(c))
			}
			seg += " (if " + strings.Join(cs, " and ") + ")"
		}
		parts = append(parts, seg)
	}
	summary := fmt.Sprintf("Create automation %s — %s. It will be enabled right away.", quote(name), strings.Join(parts, "; "))
	return summary, affected
}

func describeTrigger(t automationTriggerInput) string {
	switch strings.ToLower(strings.TrimSpace(t.Type)) {
	case "time":
		switch strings.ToLower(strings.TrimSpace(t.TimeMode)) {
		case "sunrise":
			return "at sunrise" + offsetSuffix(t.SolarOffsetMinutes)
		case "sunset":
			return "at sunset" + offsetSuffix(t.SolarOffsetMinutes)
		default:
			return "at " + strings.TrimSpace(t.Time)
		}
	case "sensor":
		return fmt.Sprintf("%s goes %s %s", strings.TrimSpace(t.Sensor), strings.TrimSpace(t.Op), trimNum(t.Value))
	case "device":
		return fmt.Sprintf("%s turns %s", strings.TrimSpace(t.Device), strings.TrimSpace(t.ToState))
	default:
		return "triggered"
	}
}

func describeCondition(c automationConditionInput) string {
	switch strings.ToLower(strings.TrimSpace(c.Type)) {
	case "device":
		return fmt.Sprintf("%s is %s", strings.TrimSpace(c.Device), strings.TrimSpace(c.State))
	case "time_range":
		return fmt.Sprintf("between %s and %s", strings.TrimSpace(c.After), strings.TrimSpace(c.Before))
	case "time_before":
		return "before " + strings.TrimSpace(c.Before)
	case "time_after":
		return "after " + strings.TrimSpace(c.After)
	default:
		return "condition"
	}
}

func describeAction(a automationActionInput) string {
	target := strings.TrimSpace(a.Target)
	if strings.ToLower(strings.TrimSpace(a.TargetType)) == "scene" {
		return "activate " + target
	}
	return fmt.Sprintf("turn %s %s", normalizeAction(a.Action), target)
}

func offsetSuffix(mins int) string {
	switch {
	case mins > 0:
		return fmt.Sprintf(" +%dm", mins)
	case mins < 0:
		return fmt.Sprintf(" %dm", mins)
	default:
		return ""
	}
}

// trimNum formats a float without a trailing ".0" so thresholds read cleanly.
func trimNum(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}
