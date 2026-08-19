package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/control"
	"homehub/internal/store"
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
	var b []byte
	var err error
	s.Store.View(func() {
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
		b, err = json.Marshal(result)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) createAutomation(w http.ResponseWriter, r *http.Request) {
	var a store.Automation
	if !decodeBody(w, r, &a) {
		return
	}

	if !s.updateOr(w, func() { delete(s.Store.Automations, a.ID) }, func() error {
		if err := s.Store.ValidateAutomation(&a); err != nil {
			return errInvalid(err)
		}
		if a.ID == "" {
			a.ID = fmt.Sprintf("automation_%d", time.Now().UnixNano())
		} else if _, exists := s.Store.Automations[a.ID]; exists {
			// A client-supplied ID must not silently replace an existing record.
			return errStatus(http.StatusConflict, "an automation with that id already exists")
		}
		s.Store.Automations[a.ID] = &a
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) updateAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updated store.Automation
	if !decodeBody(w, r, &updated) {
		return
	}

	var existing *store.Automation
	if !s.update(w, func() error {
		var ok bool
		existing, ok = s.Store.Automations[id]
		if !ok {
			return errStatus(http.StatusNotFound, "automation not found")
		}

		// Full-object replace: the editor always sends the complete automation.
		// Preserve identity and run history; everything else comes from the body.
		updated.ID = id
		updated.LastFiredAt = existing.LastFiredAt
		updated.RunCount = existing.RunCount
		if err := s.Store.ValidateAutomation(&updated); err != nil {
			return errInvalid(err)
		}
		*existing = updated
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) deleteAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if !s.update(w, func() error {
		if _, ok := s.Store.Automations[id]; !ok {
			return errStatus(http.StatusNotFound, "automation not found")
		}
		delete(s.Store.Automations, id)
		return nil
	}) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// runAutomation fires every rule's actions immediately, ignoring triggers and
// conditions — the list view's "Run now" quick action. For an automation whose
// rules conflict (e.g. on then off), the actions run in order.
func (s *Server) runAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var ok bool
	var name string
	var actions []store.AutomationAction
	var music []store.MusicAction
	s.Store.View(func() {
		var a *store.Automation
		if a, ok = s.Store.Automations[id]; ok {
			name = a.Name
			for _, rl := range a.Rules {
				actions = append(actions, rl.Actions...)
				music = append(music, rl.Music...)
			}
		}
	})
	if !ok {
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}
	s.runAutomationActions(w, id, name, actions, music)
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

	var ok bool
	var name string
	var actions []store.AutomationAction
	var music []store.MusicAction
	s.Store.View(func() {
		a, found := s.Store.Automations[id]
		if found && idx < len(a.Rules) {
			ok = true
			name = a.Name
			actions = append(actions, a.Rules[idx].Actions...)
			music = append(music, a.Rules[idx].Music...)
		}
	})
	if !ok {
		writeError(w, http.StatusNotFound, "automation or rule not found")
		return
	}
	s.runAutomationActions(w, id, name, actions, music)
}

// runAutomationActions transmits a set of actions immediately and records the
// run against the automation. Shared by the whole-automation and per-rule run
// endpoints.
//
// The run itself belongs to internal/control, which is also what the scheduler
// fires an automation through: pressing "Run" and the trigger matching at
// 07:00 are the same thing happening, and this is where they were most likely
// to drift apart.
func (s *Server) runAutomationActions(w http.ResponseWriter, id, name string, actions []store.AutomationAction, music []store.MusicAction) {
	// Marshalled inside the same transaction that recorded the run, so the
	// response carries the RunCount the run produced rather than whatever a
	// concurrent request has made of it since.
	var body []byte
	res, err := s.Control.Automation(control.AutomationRun{
		ID: id, Name: name, Actions: actions, Music: music,
		Source: control.SourceAutomation,
		Recorded: func(a *store.Automation, _ control.Result) {
			if a != nil {
				body, _ = json.Marshal(a)
			}
		},
	})
	if err == nil {
		err = res.Err()
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if body == nil {
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}
	writeJSONBytes(w, http.StatusOK, body)
}
