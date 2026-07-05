package server

import (
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// =========================================================================
// Prepared prompts (Phase 4, prompt_prep)
// =========================================================================

// handlePreparedPrompts serves GET (list by task_id) and POST (record a
// synthesized prompt).
func (s *Server) handlePreparedPrompts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		taskID := r.URL.Query().Get("task_id")
		if taskID == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "task_id is required")
			return
		}
		prompts, err := s.db.ListPreparedPrompts(r.Context(), taskID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if prompts == nil {
			prompts = []*db.PreparedPrompt{}
		}
		api.WriteJSON(w, http.StatusOK, prompts)

	case http.MethodPost:
		var p db.PreparedPrompt
		if !s.decodeJSON(w, r, &p) {
			return
		}
		if p.TaskID == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "task_id is required")
			return
		}
		if err := s.db.CreatePreparedPrompt(r.Context(), &p); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, p)

	default:
		methodNotAllowed(w)
	}
}
