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

	"homehub/internal/store"
)

func (s *Server) getSensors(w http.ResponseWriter, r *http.Request) {
	var b []byte
	var err error
	s.Store.View(func() {
		result := make([]*store.Sensor, 0, len(s.Store.Sensors))
		for _, sn := range s.Store.Sensors {
			result = append(result, sn)
		}
		sort.Slice(result, func(i, j int) bool {
			if result[i].Room != result[j].Room {
				return result[i].Room < result[j].Room
			}
			return result[i].Name < result[j].Name
		})
		b, err = json.Marshal(result)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}

	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) createSensor(w http.ResponseWriter, r *http.Request) {
	var sn store.Sensor
	if !decodeBody(w, r, &sn) {
		return
	}

	if !s.updateOr(w, func() { delete(s.Store.Sensors, sn.ID) }, func() error {
		if err := s.Store.ValidateSensor(&sn); err != nil {
			return errInvalid(err)
		}
		if sn.ID == "" {
			sn.ID = fmt.Sprintf("sensor_%d", time.Now().UnixNano())
		} else if _, exists := s.Store.Sensors[sn.ID]; exists {
			// A client-supplied ID must not silently replace an existing record.
			return errStatus(http.StatusConflict, "a sensor with that id already exists")
		}
		s.Store.Sensors[sn.ID] = &sn
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusCreated, sn)
}

func (s *Server) updateSensor(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updates store.Sensor
	if !decodeBody(w, r, &updates) {
		return
	}

	var existing *store.Sensor
	if !s.update(w, func() error {
		var ok bool
		existing, ok = s.Store.Sensors[id]
		if !ok {
			return errStatus(http.StatusNotFound, "sensor not found")
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
			return errInvalid(err)
		}
		*existing = merged
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) deleteSensor(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if !s.update(w, func() error {
		if _, ok := s.Store.Sensors[id]; !ok {
			return errStatus(http.StatusNotFound, "sensor not found")
		}
		delete(s.Store.Sensors, id)
		delete(s.Store.Readings, id)
		s.Store.PruneAutomationsForSensor(id)
		return nil
	}) {
		return
	}

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

	var until time.Time
	s.Store.Mutate(func() { until = s.Store.StartDiscovery(time.Duration(secs) * time.Second) })

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
	var active bool
	var until time.Time
	var candidates []*store.DiscoveryCandidate
	s.Store.View(func() {
		active, until, candidates = s.Store.DiscoverySnapshot()
	})

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

	// Copied out under the read lock so the filtering below runs unlocked.
	var readings []store.SensorReading
	var found bool
	s.Store.View(func() {
		if _, found = s.Store.Sensors[id]; !found {
			return
		}
		src := s.Store.Readings[id]
		readings = make([]store.SensorReading, len(src))
		copy(readings, src)
	})
	if !found {
		writeError(w, http.StatusNotFound, "sensor not found")
		return
	}

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
	if !decodeBody(w, r, &body) {
		return
	}
	t := time.Now().UTC()
	if body.Time != nil {
		t = body.Time.UTC()
	}

	// Mutate, not Update: AppendReading arms a debounced sensor-only save of
	// its own, so a full Save here would rewrite every store file on every
	// incoming reading.
	var appendErr error
	s.Store.Mutate(func() {
		appendErr = s.Store.AppendReading(id, store.SensorReading{Time: t, Value: body.Value})
	})
	if appendErr != nil {
		if strings.Contains(appendErr.Error(), "not found") {
			writeError(w, http.StatusNotFound, appendErr.Error())
		} else {
			writeError(w, http.StatusInternalServerError, appendErr.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"time": t, "value": body.Value})
}
