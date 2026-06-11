package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

type automationResponse struct {
	*store.Automation
	// EffectiveTriggerTime is the resolved HH:MM for solar time triggers
	// (sunrise/sunset + offset). Empty when the trigger is not solar or
	// the location is not configured.
	EffectiveTriggerTime string `json:"effective_trigger_time,omitempty"`
}

func (s *Server) getAutomations(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	s.Store.Mu.RLock()
	list := make([]*store.Automation, 0, len(s.Store.Automations))
	effective := make(map[string]string, len(s.Store.Automations))
	for _, a := range s.Store.Automations {
		list = append(list, a)
		if eff, ok := store.TriggerEffectiveHHMM(&a.Trigger, now, s.Store.Settings); ok {
			effective[a.ID] = eff
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return list[i].Name < list[j].Name
		}
		return list[i].ID < list[j].ID
	})

	result := make([]automationResponse, len(list))
	for i, a := range list {
		result[i] = automationResponse{Automation: a, EffectiveTriggerTime: effective[a.ID]}
	}
	// Snapshot under the lock — result still holds live *store.Automation
	// pointers that writers mutate in place.
	b, err := json.Marshal(result)
	s.Store.Mu.RUnlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) createAutomation(w http.ResponseWriter, r *http.Request) {
	var a store.Automation
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	if err := s.Store.ValidateAutomation(&a); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.ID == "" {
		a.ID = fmt.Sprintf("automation_%d", time.Now().UnixNano())
	}
	s.Store.Automations[a.ID] = &a
	if err := s.Store.Save(); err != nil {
		delete(s.Store.Automations, a.ID)
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) updateAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updated store.Automation
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.Store.Mu.Lock()
	defer s.Store.Mu.Unlock()

	existing, ok := s.Store.Automations[id]
	if !ok {
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}

	// Full-object replace: the editor always sends the complete automation.
	// Preserve identity and run history; everything else comes from the body.
	updated.ID = id
	updated.LastFiredAt = existing.LastFiredAt
	updated.RunCount = existing.RunCount
	if err := s.Store.ValidateAutomation(&updated); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	*existing = updated
	if err := s.Store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) deleteAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	if _, ok := s.Store.Automations[id]; !ok {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}
	delete(s.Store.Automations, id)
	if err := s.Store.Save(); err != nil {
		s.Store.Mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist data: "+err.Error())
		return
	}
	s.Store.Mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// runAutomation fires an automation's actions immediately, ignoring its
// trigger and conditions — the "Run now" / test button in the editor.
func (s *Server) runAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.Lock()
	a, ok := s.Store.Automations[id]
	var kind, name string
	var staged []store.StagedSend
	if ok {
		name = a.Name
		kind = "bulk"
		if len(a.Actions) == 1 {
			kind = a.Actions[0].TargetType
		}
		staged = s.Store.StageAutomationActions(a.Actions)
	}
	s.Store.Mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}

	s.Store.SendStaged(staged)

	s.Store.Mu.Lock()
	s.Store.SuppressStateChange = true
	firstErr := s.Store.ApplyStaged(staged)
	s.Store.SuppressStateChange = false

	entry := store.ActivityEntry{Kind: kind, Source: "automation", Action: "run", Label: name}
	if firstErr != nil {
		entry.Status = "error"
		entry.Error = firstErr.Error()
	}
	s.Store.Activity.Add(entry)
	// Re-fetch: the automation may have been deleted while sends were in flight.
	var result *store.Automation
	if cur, still := s.Store.Automations[id]; still {
		cur.LastFiredAt = time.Now().UTC()
		cur.RunCount++
		result = cur
	}
	var body []byte
	if result != nil {
		body, _ = json.Marshal(result)
	}
	if err := s.Store.Save(); err != nil && firstErr == nil {
		firstErr = err
	}
	s.Store.Mu.Unlock()
	s.Store.FlushLights()
	if firstErr != nil {
		writeError(w, http.StatusInternalServerError, firstErr.Error())
		return
	}
	if body == nil {
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}
	writeJSONBytes(w, http.StatusOK, body)
}
