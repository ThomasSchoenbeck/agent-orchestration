package server

import (
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// =========================================================================
// Agent templates (Feature 8) — server-managed co-located agents
// =========================================================================

func (s *Server) handleAgentTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tpls, err := s.db.ListAgentTemplates(r.Context())
		if err != nil {
			s.internalError(w, err)
			return
		}
		if tpls == nil {
			tpls = []*db.AgentTemplate{}
		}
		api.WriteJSON(w, http.StatusOK, tpls)

	case http.MethodPost:
		var t db.AgentTemplate
		if !s.decodeJSON(w, r, &t) {
			return
		}
		if t.Name == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "name is required")
			return
		}
		if t.Replicas == 0 {
			t.Replicas = 1
		}
		t.Enabled = true
		if err := s.db.CreateAgentTemplate(r.Context(), &t); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, t)

	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAgentTemplateDetail(w http.ResponseWriter, r *http.Request) {
	id := pathSegment(r.URL.Path, "/api/agent-templates/", 0)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	action := pathSegment(r.URL.Path, "/api/agent-templates/", 1)

	switch action {
	case "scale":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var body struct {
			Replicas int `json:"replicas"`
		}
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if err := s.agentSup.ScaleTemplate(r.Context(), id, body.Replicas); err != nil {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, err.Error())
			return
		}
		t, _ := s.db.GetAgentTemplate(r.Context(), id)
		api.WriteJSON(w, http.StatusOK, t)
		return

	case "start":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if err := s.agentSup.StartTemplate(r.Context(), id); err != nil {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "started"})
		return

	case "stop":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.agentSup.StopTemplate(r.Context(), id)
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		t, err := s.db.GetAgentTemplate(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, t)

	case http.MethodPatch, http.MethodPut:
		t, err := s.db.GetAgentTemplate(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		oldReplicas := t.Replicas
		if !s.decodeJSON(w, r, t) {
			return
		}
		t.ID = id
		if err := s.db.UpdateAgentTemplate(r.Context(), t); err != nil {
			s.internalError(w, err)
			return
		}
		// A replica change reconciles the running instances.
		if t.Replicas != oldReplicas {
			if err := s.agentSup.ScaleTemplate(r.Context(), id, t.Replicas); err != nil {
				api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, err.Error())
				return
			}
		}
		updated, _ := s.db.GetAgentTemplate(r.Context(), id)
		api.WriteJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		if err := s.agentSup.DeleteTemplate(r.Context(), id); err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}
