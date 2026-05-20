package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

func (s *Server) getSensors(w http.ResponseWriter, r *http.Request) {
	s.Store.Mu.RLock()
	result := make([]*store.Sensor, 0, len(s.Store.Sensors))
	for _, sn := range s.Store.Sensors {
		result = append(result, sn)
	}
	s.Store.Mu.RUnlock()

	sort.Slice(result, func(i, j int) bool {
		if result[i].Room != result[j].Room {
			return result[i].Room < result[j].Room
		}
		return result[i].Name < result[j].Name
	})

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createSensor(w http.ResponseWriter, r *http.Request) {
	var sn store.Sensor
	if err := json.NewDecoder(r.Body).Decode(&sn); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if err := s.Store.ValidateSensor(&sn); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if sn.ID == "" {
		sn.ID = fmt.Sprintf("sensor_%d", time.Now().UnixNano())
	}
	s.Store.Sensors[sn.ID] = &sn
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Sensors, sn.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sn)
}

func (s *Server) updateSensor(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updates store.Sensor
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	existing, ok := s.Store.Sensors[id]
	if !ok {
		writeError(w, http.StatusNotFound, "sensor not found")
		return
	}

	merged := *existing
	if v := strings.TrimSpace(updates.Name); v != "" {
		merged.Name = v
	}
	if v := strings.TrimSpace(updates.Kind); v != "" {
		merged.Kind = v
	}
	if v := strings.TrimSpace(updates.Unit); v != "" {
		merged.Unit = v
	}
	if v := strings.TrimSpace(updates.Code); v != "" {
		merged.Code = v
	}
	if v := strings.TrimSpace(updates.Protocol); v != "" {
		merged.Protocol = v
	}
	// Field and Room are allowed to be cleared, so always overwrite.
	merged.Field = strings.TrimSpace(updates.Field)
	merged.Room = strings.TrimSpace(updates.Room)
	// Thresholds are pointers — nil means "clear it", so always overwrite.
	merged.AlertMin = updates.AlertMin
	merged.AlertMax = updates.AlertMax

	if err := s.Store.ValidateSensor(&merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	*existing = merged
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) deleteSensor(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	if _, ok := s.Store.Sensors[id]; !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "sensor not found")
		return
	}
	delete(s.Store.Sensors, id)
	delete(s.Store.Readings, id)
	if err := s.Store.Save(); err != nil {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// startSensorPair opens a discovery window in which the RX listener
// records every unknown 433MHz emitter it hears. The frontend then polls
// listDiscoveryCandidates to show the user candidates they can adopt.
//
// This mirrors learnSocket conceptually but in the opposite direction:
// sockets learn a code we transmit; sensors transmit a code we capture.
func (s *Server) startSensorPair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Seconds int `json:"seconds"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	secs := body.Seconds
	if secs <= 0 {
		secs = 60
	}
	if secs > 300 {
		secs = 300
	}

	s.Store.Mu.Lock()
	until := s.Store.StartDiscovery(time.Duration(secs) * time.Second)
	s.Store.Mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active":  true,
		"until":   until.UTC(),
		"seconds": secs,
	})
}

// listDiscoveryCandidates returns the current state of the pair window:
// whether it's still open, when it closes, and every unknown emitter
// heard so far (with sample numeric fields).
func (s *Server) listDiscoveryCandidates(w http.ResponseWriter, _ *http.Request) {
	s.Store.Mu.RLock()
	active, until, candidates := s.Store.DiscoverySnapshot()
	s.Store.Mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active":     active,
		"until":      until.UTC(),
		"candidates": candidates,
	})
}

// getSensorReadings returns the rolling window of readings for one sensor.
// Optional query params:
//   - since_minutes=N: only readings from the last N minutes
//   - limit=N: cap to N most-recent readings
func (s *Server) getSensorReadings(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	q := r.URL.Query()

	s.Store.Mu.RLock()
	if _, ok := s.Store.Sensors[id]; !ok {
		s.Store.Mu.RUnlock()
		writeError(w, http.StatusNotFound, "sensor not found")
		return
	}
	src := s.Store.Readings[id]
	readings := make([]store.SensorReading, len(src))
	copy(readings, src)
	s.Store.Mu.RUnlock()

	if v := q.Get("since_minutes"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cutoff := time.Now().Add(-time.Duration(n) * time.Minute)
			out := readings[:0]
			for _, r := range readings {
				if !r.Time.Before(cutoff) {
					out = append(out, r)
				}
			}
			readings = out
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < len(readings) {
			readings = readings[len(readings)-n:]
		}
	}
	writeJSON(w, http.StatusOK, readings)
}

// postSensorReading ingests a single reading for a sensor. Used by the
// RX listener internally and as an HTTP escape hatch for testing or for
// devices that push readings over a non-433MHz transport.
func (s *Server) postSensorReading(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Value float64    `json:"value"`
		Time  *time.Time `json:"time,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	t := time.Now().UTC()
	if body.Time != nil {
		t = body.Time.UTC()
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if err := s.Store.AppendReading(id, store.SensorReading{Time: t, Value: body.Value}); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"time": t, "value": body.Value})
}
