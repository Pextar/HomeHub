package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// mqttStatus handles GET /api/mqtt/status. It lets the frontend tell
// whether the MQTT protocol is available (and currently connected) so it
// can surface it in the socket/sensor editors.
func (s *Server) mqttStatus(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]interface{}{"enabled": s.MQTT.Enabled()}
	if s.MQTT.Enabled() {
		resp["broker"] = s.MQTT.BrokerURL
		resp["connected"] = s.MQTT.Connected()
	}
	writeJSON(w, http.StatusOK, resp)
}

// mqttPublish handles POST /api/mqtt/publish with {topic, payload?}. It
// powers the socket editor's "Send test signal" button: publishing a value
// to a command topic so the user can confirm the device reacts before
// saving. Defaults the payload to "ON" when omitted.
func (s *Server) mqttPublish(w http.ResponseWriter, r *http.Request) {
	if !s.MQTT.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "MQTT is not configured (set MQTT_BROKER_URL)")
		return
	}
	var body struct {
		Topic   string `json:"topic"`
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	topic := strings.TrimSpace(body.Topic)
	if topic == "" {
		writeError(w, http.StatusBadRequest, "topic is required")
		return
	}
	payload := body.Payload
	if payload == "" {
		payload = "ON"
	}
	if err := s.MQTT.Publish(topic, payload); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "topic": topic})
}
