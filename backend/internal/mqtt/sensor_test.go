package mqtt

import "testing"

func TestTopicMatches(t *testing.T) {
	cases := []struct {
		filter string
		topic  string
		want   bool
	}{
		{"zigbee2mqtt/sensor", "zigbee2mqtt/sensor", true},
		{"zigbee2mqtt/sensor", "zigbee2mqtt/other", false},
		{"zigbee2mqtt/+/state", "zigbee2mqtt/lamp/state", true},
		{"zigbee2mqtt/+/state", "zigbee2mqtt/lamp/level", false},
		{"zigbee2mqtt/+/state", "zigbee2mqtt/a/b/state", false},
		{"zigbee2mqtt/#", "zigbee2mqtt/lamp/state", true},
		{"zigbee2mqtt/#", "zigbee2mqtt", true},
		{"#", "anything/at/all", true},
		{"a/b", "a/b/c", false},
		{"a/b/c", "a/b", false},
	}
	for _, c := range cases {
		if got := topicMatches(c.filter, c.topic); got != c.want {
			t.Errorf("topicMatches(%q, %q) = %v, want %v", c.filter, c.topic, got, c.want)
		}
	}
}

func TestExtractValue(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		field   string
		want    float64
		ok      bool
	}{
		{"bare number", "21.5", "", 21.5, true},
		{"quoted number", `"42"`, "", 42, true},
		{"on state", "ON", "", 1, true},
		{"off state", "off", "", 0, true},
		{"json field", `{"temperature":19.4,"humidity":55}`, "temperature", 19.4, true},
		{"json missing field", `{"humidity":55}`, "temperature", 0, false},
		{"json auto first numeric", `{"id":"abc","temperature":18}`, "", 18, true},
		{"json string state field", `{"state":"ON"}`, "state", 1, true},
		{"json bool field", `{"occupancy":true}`, "occupancy", 1, true},
		{"empty", "", "", 0, false},
		{"unparseable", "hello", "", 0, false},
		{"bad json", "{not json", "", 0, false},
	}
	for _, c := range cases {
		got, ok := extractValue([]byte(c.payload), c.field)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: extractValue(%q, %q) = (%v, %v), want (%v, %v)",
				c.name, c.payload, c.field, got, ok, c.want, c.ok)
		}
	}
}
