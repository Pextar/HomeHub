package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

func (s *Server) getTimers(w http.ResponseWriter, r *http.Request) {
	s.Store.Mu.RLock()
	out := make([]*store.Timer, 0, len(s.Store.Timers))
	for _, t := range s.Store.Timers {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FiresAt.Before(out[j].FiresAt) })
	b, err := json.Marshal(out)
	s.Store.Mu.RUnlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	writeJSONBytes(w, http.StatusOK, b)
}

// timerRequest is the JSON shape clients use to schedule a one-shot
// timer. Either FiresAt (RFC3339) or InSeconds must be set.
type timerRequest struct {
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Action     string    `json:"action"`
	FiresAt    time.Time `json:"fires_at,omitempty"`
	InSeconds  int       `json:"in_seconds,omitempty"`
	Note       string    `json:"note,omitempty"`
}

func (req *timerRequest) toTimer() (*store.Timer, error) {
	tt := strings.ToLower(strings.TrimSpace(req.TargetType))
	tid := strings.TrimSpace(req.TargetID)
	action := strings.ToLower(strings.TrimSpace(req.Action))

	if tt == "" || tid == "" {
		return nil, errors.New("target_type and target_id are required")
	}
	switch tt {
	case "socket", "group", "room":
		if action != "on" && action != "off" && action != "toggle" {
			return nil, errors.New("action must be on/off/toggle")
		}
	case "scene":
		action = "activate"
	default:
		return nil, errors.New("target_type must be socket, group, room, or scene")
	}

	var firesAt time.Time
	switch {
	case !req.FiresAt.IsZero():
		firesAt = req.FiresAt
	case req.InSeconds > 0:
		firesAt = time.Now().Add(time.Duration(req.InSeconds) * time.Second)
	default:
		return nil, errors.New("either fires_at or in_seconds is required")
	}
	if !firesAt.After(time.Now().Add(-time.Second)) {
		return nil, errors.New("fires_at must be in the future")
	}

	now := time.Now()
	return &store.Timer{
		ID:         fmt.Sprintf("timer_%d", now.UnixNano()),
		TargetType: tt,
		TargetID:   tid,
		Action:     action,
		FiresAt:    firesAt,
		CreatedAt:  now,
		Note:       strings.TrimSpace(req.Note),
	}, nil
}

func (s *Server) createTimer(w http.ResponseWriter, r *http.Request) {
	var req timerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	t, err := req.toTimer()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if err := s.Store.VerifyTarget(t.TargetType, t.TargetID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Store.Timers[t.ID] = t
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Timers, t.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// createSocketTimer is a convenience for "off in N seconds" from a
// socket card. The path supplies the socket id; the body supplies
// action and in_seconds (or fires_at).
func (s *Server) createSocketTimer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !s.requireSocketAccess(w, r, id) {
		return
	}

	var req timerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	req.TargetType = "socket"
	req.TargetID = id

	t, err := req.toTimer()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if _, ok := s.Store.Sockets[id]; !ok {
		writeError(w, http.StatusNotFound, "socket not found")
		return
	}
	s.Store.Timers[t.ID] = t
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Timers, t.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) deleteTimer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	if _, ok := s.Store.Timers[id]; !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "timer not found")
		return
	}
	delete(s.Store.Timers, id)
	if err := s.Store.Save(); err != nil {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
