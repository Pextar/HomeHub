package mqtt

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"rf-socket-controller/internal/store"
)

// reconcileInterval controls how often the listener re-reads the store to
// (un)subscribe topics as MQTT sensors are added or removed at runtime.
const reconcileInterval = 15 * time.Second

// SensorListener subscribes to the topics of sensors whose protocol is
// "mqtt" and records incoming payloads as readings. It mirrors the rtl_433
// rx.Listener but sources data from a broker instead of a subprocess.
//
// For an MQTT sensor:
//   - Code is the topic filter to subscribe to (wildcards allowed)
//   - Field is the JSON key to read from object payloads (empty = first
//     numeric field; also handles bare-number and ON/OFF scalar payloads)
type SensorListener struct {
	Client *Client
}

// Run blocks until ctx is cancelled. It subscribes to the topics of all
// MQTT sensors, reconciling the subscription set every reconcileInterval so
// sensors added or removed at runtime are picked up without a restart, and
// re-subscribing on every broker (re)connection. Spawn it in a goroutine.
func (l SensorListener) Run(ctx context.Context, st *store.Store) {
	if !l.Client.Enabled() {
		log.Printf("mqtt: broker not configured — sensor ingestion disabled")
		return
	}

	var mu sync.Mutex
	subscribed := make(map[string]bool)

	reconcile := func() {
		desired := desiredTopics(st)
		mu.Lock()
		defer mu.Unlock()
		for topic := range desired {
			if subscribed[topic] {
				continue
			}
			if err := l.Client.Subscribe(topic, func(topic string, payload []byte) {
				dispatch(st, topic, payload)
			}); err != nil {
				log.Printf("mqtt: subscribe %q: %v", topic, err)
				continue
			}
			subscribed[topic] = true
			log.Printf("mqtt: subscribed to sensor topic %q", topic)
		}
		for topic := range subscribed {
			if desired[topic] {
				continue
			}
			if err := l.Client.Unsubscribe(topic); err != nil {
				log.Printf("mqtt: unsubscribe %q: %v", topic, err)
			}
			delete(subscribed, topic)
		}
	}

	// A clean-session reconnect wipes our server-side subscriptions; forget
	// what we think is subscribed so reconcile re-establishes everything.
	l.Client.OnConnect(func() {
		mu.Lock()
		for k := range subscribed {
			delete(subscribed, k)
		}
		mu.Unlock()
		reconcile()
	})

	reconcile()
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

// desiredTopics returns the set of topic filters the listener should be
// subscribed to, derived from the current MQTT sensors.
func desiredTopics(st *store.Store) map[string]bool {
	st.Mu.RLock()
	defer st.Mu.RUnlock()
	out := make(map[string]bool)
	for _, sensor := range st.Sensors {
		if strings.EqualFold(sensor.Protocol, "mqtt") && sensor.Code != "" {
			out[sensor.Code] = true
		}
	}
	return out
}

// dispatch matches an incoming message against MQTT sensors and records a
// reading for each one whose topic filter matches and whose value parses.
func dispatch(st *store.Store, topic string, payload []byte) {
	st.Mu.Lock()
	defer st.Mu.Unlock()
	for _, sensor := range st.Sensors {
		if !strings.EqualFold(sensor.Protocol, "mqtt") {
			continue
		}
		if !topicMatches(sensor.Code, topic) {
			continue
		}
		value, ok := extractValue(payload, sensor.Field)
		if !ok {
			continue
		}
		reading := store.SensorReading{Time: time.Now().UTC(), Value: value}
		if err := st.AppendReading(sensor.ID, reading); err != nil {
			log.Printf("mqtt: append reading for %s: %v", sensor.ID, err)
		}
	}
}

// topicMatches reports whether an MQTT topic matches a subscription filter,
// honoring the '+' (single level) and '#' (multi level, trailing) wildcards.
func topicMatches(filter, topic string) bool {
	if filter == topic {
		return true
	}
	fs := strings.Split(filter, "/")
	ts := strings.Split(topic, "/")
	for i, f := range fs {
		if f == "#" {
			return true
		}
		if i >= len(ts) {
			return false
		}
		if f == "+" {
			continue
		}
		if f != ts[i] {
			return false
		}
	}
	return len(fs) == len(ts)
}

// extractValue reads a numeric value from a payload. JSON object payloads
// (Zigbee2MQTT, Tasmota SENSOR, etc.) are decoded and the requested field
// read — or, when field is empty, the first non-identifier numeric field.
// Scalar payloads are parsed as a bare number or an ON/OFF-style state.
func extractValue(payload []byte, field string) (float64, bool) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return 0, false
	}
	if trimmed[0] == '{' {
		var obj map[string]interface{}
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			return 0, false
		}
		if field != "" {
			v, ok := obj[field]
			if !ok {
				return 0, false
			}
			return toFloat(v)
		}
		return pickNumeric(obj)
	}
	return parseScalar(string(trimmed))
}

// preferredFields ranks the measurement names Zigbee2MQTT/Tasmota commonly
// emit. Without this, multi-field payloads (temperature + humidity +
// battery…) would record a random field per message, since Go map iteration
// order is randomized.
var preferredFields = []string{
	"temperature_C", "temperature_F", "temperature",
	"humidity", "moisture", "pressure_hPa", "pressure",
	"illuminance", "lux", "power", "energy",
}

// pickNumeric deterministically selects a measurement when no field is
// configured: well-known measurement names first, then the alphabetically
// first remaining non-identifier numeric field.
func pickNumeric(obj map[string]interface{}) (float64, bool) {
	for _, k := range preferredFields {
		if v, ok := obj[k]; ok {
			if f, ok := toFloat(v); ok {
				return f, true
			}
		}
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		if !isIdentifierField(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if f, ok := toFloat(obj[k]); ok {
			return f, true
		}
	}
	return 0, false
}

// parseScalar parses a non-JSON payload as a number, or maps a common
// boolean-ish state string (ON/OFF, true/false, open/closed) to 1/0.
func parseScalar(s string) (float64, bool) {
	s = strings.Trim(strings.TrimSpace(s), `"`)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, true
	}
	switch strings.ToLower(s) {
	case "on", "true", "open", "yes", "detected":
		return 1, true
	case "off", "false", "closed", "no", "clear":
		return 0, true
	}
	return 0, false
}

func toFloat(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		return parseScalar(x)
	}
	return 0, false
}

func isIdentifierField(k string) bool {
	switch strings.ToLower(k) {
	case "id", "model", "time", "channel", "subtype", "protocol", "device", "linkquality", "last_seen":
		return true
	}
	return false
}
