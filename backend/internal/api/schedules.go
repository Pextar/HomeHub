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

func (s *Server) getSchedules(w http.ResponseWriter, r *http.Request) {
	s.Store.Mu.RLock()
	result := make([]*store.Schedule, 0, len(s.Store.Schedules))
	for _, sch := range s.Store.Schedules {
		result = append(result, sch)
	}
	s.Store.Mu.RUnlock()

	sort.Slice(result, func(i, j int) bool {
		if result[i].Time != result[j].Time {
			return result[i].Time < result[j].Time
		}
		return result[i].ID < result[j].ID
	})

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createSchedule(w http.ResponseWriter, r *http.Request) {
	var schedule store.Schedule
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
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
	if v := strings.TrimSpace(updates.Time); v != "" {
		merged.Time = v
	}
	if updates.Days != nil {
		merged.Days = updates.Days
	}
	merged.Enabled = updates.Enabled

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

func (s *Server) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	if _, ok := s.Store.Schedules[id]; !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "schedule not found")
		return
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
