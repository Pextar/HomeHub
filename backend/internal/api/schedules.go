package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

type scheduleResponse struct {
	*store.Schedule
	EffectiveTime string `json:"effective_time,omitempty"`
}

// scheduleSocketID returns the socket ID a schedule targets, handling both the
// legacy socket_id field and the newer target_type/target_id fields. Returns ""
// when the schedule does not target a plain socket.
func scheduleSocketID(s *store.Schedule) string {
	if s.TargetType == "socket" && s.TargetID != "" {
		return s.TargetID
	}
	// Legacy schedules had no TargetType; the socket_id field carried the target.
	if s.TargetType == "" && s.SocketID != "" {
		return s.SocketID
	}
	return ""
}

func (s *Server) getSchedules(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	admin := isAdmin(user)
	now := time.Now()

	s.Store.Mu.RLock()
	raw := make([]*store.Schedule, 0, len(s.Store.Schedules))
	keys := make(map[string]string, len(s.Store.Schedules))
	effective := make(map[string]string, len(s.Store.Schedules))
	for _, sch := range s.Store.Schedules {
		// Non-admins only see schedules targeting their own sockets.
		if !admin {
			sockID := scheduleSocketID(sch)
			if sockID == "" || !user.CanAccessSocket(sockID) {
				continue
			}
		}
		raw = append(raw, sch)
		k, ok := sch.EffectiveHHMM(now, s.Store.Settings)
		if !ok {
			// Unresolvable schedules (e.g. sunrise without a configured
			// location) sort to the end so the list still reads top-to-bottom
			// by trigger time.
			k = "~~"
		} else {
			effective[sch.ID] = k
		}
		keys[sch.ID] = k
	}
	sort.Slice(raw, func(i, j int) bool {
		ki, kj := keys[raw[i].ID], keys[raw[j].ID]
		if ki != kj {
			return ki < kj
		}
		return raw[i].ID < raw[j].ID
	})

	result := make([]scheduleResponse, len(raw))
	for i, sch := range raw {
		result[i] = scheduleResponse{Schedule: sch, EffectiveTime: effective[sch.ID]}
	}
	// Snapshot under the lock — result still holds live *store.Schedule
	// pointers that writers mutate in place.
	b, err := json.Marshal(result)
	s.Store.Mu.RUnlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}

	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) createSchedule(w http.ResponseWriter, r *http.Request) {
	var schedule store.Schedule
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	user := currentUser(r)
	if !isAdmin(user) {
		// Non-admins can only schedule their own sockets — not groups, scenes, or rooms.
		tt := strings.TrimSpace(schedule.TargetType)
		if tt != "" && tt != "socket" {
			writeError(w, http.StatusForbidden, "you can only schedule your own devices")
			return
		}
		sockID := strings.TrimSpace(schedule.TargetID)
		if sockID == "" {
			sockID = strings.TrimSpace(schedule.SocketID)
		}
		if !user.CanAccessSocket(sockID) {
			writeError(w, http.StatusForbidden, "you don't have access to that device")
			return
		}
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if err := s.Store.ValidateSchedule(&schedule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if schedule.ID == "" {
		schedule.ID = fmt.Sprintf("schedule_%d", time.Now().UnixNano())
	}

	s.Store.Schedules[schedule.ID] = &schedule
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Schedules, schedule.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, schedule)
}

func (s *Server) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates store.Schedule
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	existing, ok := s.Store.Schedules[id]
	if !ok {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}

	user := currentUser(r)
	if !isAdmin(user) {
		// The user must own the existing schedule (it must target their socket).
		if sockID := scheduleSocketID(existing); !user.CanAccessSocket(sockID) {
			writeError(w, http.StatusForbidden, "you don't own that schedule")
			return
		}
	}

	// Build merged schedule and validate it whole.
	merged := *existing
	if v := strings.TrimSpace(updates.SocketID); v != "" {
		merged.SocketID = v
	}
	if v := strings.TrimSpace(updates.TargetType); v != "" {
		merged.TargetType = v
	}
	if v := strings.TrimSpace(updates.TargetID); v != "" {
		merged.TargetID = v
	}
	if v := strings.TrimSpace(updates.Action); v != "" {
		merged.Action = v
	}
	if v := strings.TrimSpace(updates.TimeMode); v != "" {
		merged.TimeMode = v
	}
	if v := strings.TrimSpace(updates.Time); v != "" {
		merged.Time = v
	}
	if updates.Days != nil {
		merged.Days = updates.Days
	}
	merged.Enabled = updates.Enabled
	merged.RandomOffsetMinutes = updates.RandomOffsetMinutes
	merged.SolarOffsetMinutes = updates.SolarOffsetMinutes

	if !isAdmin(user) {
		// After merge, the target must still be the user's own socket.
		tt := strings.TrimSpace(merged.TargetType)
		if tt != "" && tt != "socket" {
			writeError(w, http.StatusForbidden, "you can only schedule your own devices")
			return
		}
		if sockID := scheduleSocketID(&merged); !user.CanAccessSocket(sockID) {
			writeError(w, http.StatusForbidden, "you don't have access to that device")
			return
		}
	}

	if err := s.Store.ValidateSchedule(&merged); err != nil {
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

// setAllSchedules flips every schedule's Enabled flag to the given value in
// one shot — the backend of the UI's "vacation mode" switch. Returns how
// many schedules ended up changed.
func (s *Server) setAllSchedules(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.Store.Mu.Lock()
		defer s.Store.Mu.Unlock()

		changed := 0
		for _, sch := range s.Store.Schedules {
			if sch.Enabled != enabled {
				sch.Enabled = enabled
				changed++
			}
		}
		if changed > 0 {
			if err := s.Store.Save(); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"enabled": enabled, "changed": changed})
	}
}

func (s *Server) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()

	sch, ok := s.Store.Schedules[id]
	if !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}

	user := currentUser(r)
	if !isAdmin(user) {
		sockID := scheduleSocketID(sch)
		if !user.CanAccessSocket(sockID) {
			s.Store.Mu.Unlock()
			writeError(w, http.StatusForbidden, "you don't own that schedule")
			return
		}
	}

	delete(s.Store.Schedules, id)
	if err := s.Store.Save(); err != nil {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}
