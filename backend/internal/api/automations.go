package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"rf-socket-controller/internal/store"
)

type automationResponse struct {
	*store.Automation
	// EffectiveTriggerTimes holds the resolved HH:MM for each rule's solar time
	// trigger (sunrise/sunset + offset), index-aligned to Rules. An entry is
	// empty when that rule's trigger is not solar or location is not configured.
	EffectiveTriggerTimes []string `json:"effective_trigger_times,omitempty"`
}

func (s *Server) getAutomations(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	s.Store.Mu.RLock()
	list := make([]*store.Automation, 0, len(s.Store.Automations))
	for _, a := range s.Store.Automations {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return list[i].Name < list[j].Name
		}
		return list[i].ID < list[j].ID
	})

	result := make([]automationResponse, len(list))
	for i, a := range list {
		var effs []string
		any := false
		for ri := range a.Rules {
			if effs == nil {
				effs = make([]string, len(a.Rules))
			}
			if eff, ok := store.TriggerEffectiveHHMM(&a.Rules[ri].Trigger, now, s.Store.Settings); ok {
				effs[ri] = eff
				any = true
			}
		}
		if !any {
			effs = nil
		}
		result[i] = automationResponse{Automation: a, EffectiveTriggerTimes: effs}
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
	} else if _, exists := s.Store.Automations[a.ID]; exists {
		// A client-supplied ID must not silently replace an existing record.
		writeError(w, http.StatusConflict, "an automation with that id already exists")
		return
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

// runAutomation fires every rule's actions immediately, ignoring triggers and
// conditions — the list view's "Run now" quick action. For an automation whose
// rules conflict (e.g. on then off), the actions run in order.
func (s *Server) runAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	s.Store.Mu.RLock()
	a, ok := s.Store.Automations[id]
	var name string
	var actions []store.AutomationAction
	if ok {
		name = a.Name
		for _, rl := range a.Rules {
			actions = append(actions, rl.Actions...)
		}
	}
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}
	s.runAutomationActions(w, id, name, actions)
}

// runAutomationRule fires just one rule's actions immediately — the per-rule
// "Run" / test button in the editor.
func (s *Server) runAutomationRule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	idx, err := strconv.Atoi(vars["idx"])
	if err != nil || idx < 0 {
		writeError(w, http.StatusBadRequest, "invalid rule index")
		return
	}

	s.Store.Mu.RLock()
	a, ok := s.Store.Automations[id]
	var name string
	var actions []store.AutomationAction
	if ok && idx < len(a.Rules) {
		name = a.Name
		actions = append(actions, a.Rules[idx].Actions...)
	} else {
		ok = false
	}
	s.Store.Mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "automation or rule not found")
		return
	}
	s.runAutomationActions(w, id, name, actions)
}

// runAutomationActions transmits a set of actions immediately and records the
// run against the automation. Shared by the whole-automation and per-rule run
// endpoints.
func (s *Server) runAutomationActions(w http.ResponseWriter, id, name string, actions []store.AutomationAction) {
	s.Store.Mu.Lock()
	kind := "bulk"
	if len(actions) == 1 {
		kind = actions[0].TargetType
	}
	staged := s.Store.StageAutomationActions(actions)
	s.Store.Mu.Unlock()

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
	var body []byte
	if cur, still := s.Store.Automations[id]; still {
		cur.LastFiredAt = time.Now().UTC()
		cur.RunCount++
		body, _ = json.Marshal(cur)
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
