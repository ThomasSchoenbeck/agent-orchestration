package server

import (
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// handleTaskTypes serves the task-type collection: GET (list) and POST (create).
func (s *Server) handleTaskTypes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		types, err := s.db.ListTaskTypes(r.Context())
		if err != nil {
			s.internalError(w, err)
			return
		}
		if types == nil {
			types = []*db.TaskType{}
		}
		api.WriteJSON(w, http.StatusOK, types)

	case http.MethodPost:
		var tt db.TaskType
		if !s.decodeJSON(w, r, &tt) {
			return
		}
		if tt.Key == "" || tt.Label == "" || tt.BranchTemplate == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput,
				"key, label and branch_template are required")
			return
		}
		if err := s.db.CreateTaskType(r.Context(), &tt); err != nil {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusCreated, tt)

	default:
		methodNotAllowed(w)
	}
}

// handleTaskTypeDetail serves a single task type: GET, PUT, DELETE by id.
func (s *Server) handleTaskTypeDetail(w http.ResponseWriter, r *http.Request) {
	id := pathSegment(r.URL.Path, "/api/task-types/", 0)
	if id == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		tt, err := s.db.GetTaskType(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, tt)

	case http.MethodPut:
		existing, err := s.db.GetTaskType(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		var req db.TaskType
		if !s.decodeJSON(w, r, &req) {
			return
		}
		existing.Key = req.Key
		existing.Label = req.Label
		existing.BranchTemplate = req.BranchTemplate
		existing.IsDefault = req.IsDefault
		existing.SortOrder = req.SortOrder
		if existing.Key == "" || existing.Label == "" || existing.BranchTemplate == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput,
				"key, label and branch_template are required")
			return
		}
		if err := s.db.UpdateTaskType(r.Context(), existing); err != nil {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, existing)

	case http.MethodDelete:
		tt, err := s.db.GetTaskType(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if tt.IsDefault {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput,
				"cannot delete the default task type")
			return
		}
		if n, _ := s.db.CountTasksUsingType(r.Context(), id); n > 0 {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput,
				"cannot delete a task type that is in use by tasks")
			return
		}
		if err := s.db.DeleteTaskType(r.Context(), id); err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}
