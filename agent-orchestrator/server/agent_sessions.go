package server

import (
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// =========================================================================
// Agent sessions (Session checkpoint feature)
// =========================================================================

// handleAgentSessions serves GET (list by task_id) and POST (create checkpoint).
func (s *Server) handleAgentSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		taskID := r.URL.Query().Get("task_id")
		if taskID == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "task_id is required")
			return
		}
		sessions, err := s.db.ListAgentSessionsByTask(r.Context(), taskID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if sessions == nil {
			sessions = []*db.AgentSession{}
		}
		api.WriteJSON(w, http.StatusOK, sessions)

	case http.MethodPost:
		var sess db.AgentSession
		if !s.decodeJSON(w, r, &sess) {
			return
		}
		if sess.TaskID == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "task_id is required")
			return
		}
		if err := s.db.CreateAgentSession(r.Context(), &sess); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, sess)

	default:
		methodNotAllowed(w)
	}
}
