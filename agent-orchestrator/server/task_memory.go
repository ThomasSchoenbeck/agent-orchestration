package server

import (
	"net/http"

	"agent-orchestrator/api"
)

// =========================================================================
// Task memory (Multi-session orchestration — Phase 6, T6.1)
// =========================================================================

// handleTaskMemory serves GET /api/tasks/{id}/memory — the task's durable
// running memory. Returns an empty object when the task has no memory row yet.
func (s *Server) handleTaskMemory(w http.ResponseWriter, r *http.Request, taskID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	mem, err := s.db.GetTaskMemory(r.Context(), taskID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if mem == nil {
		// No memory recorded yet — return {} rather than null so clients can
		// render an empty panel without special-casing.
		api.WriteJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	api.WriteJSON(w, http.StatusOK, mem)
}
