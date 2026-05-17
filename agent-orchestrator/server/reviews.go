package server

import (
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// handleTaskReviews serves GET/POST /api/tasks/{id}/reviews
// and GET /api/tasks/{id}/reviews/{reviewID}.
func (s *Server) handleTaskReviews(w http.ResponseWriter, r *http.Request, taskID string, parts []string) {
	reviewID := ""
	if len(parts) > 2 {
		reviewID = parts[2]
	}

	if reviewID != "" {
		// Single review.
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		rev, err := s.db.GetTaskReview(r.Context(), reviewID)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, rev)
		return
	}

	switch r.Method {
	case http.MethodGet:
		reviews, err := s.db.ListTaskReviews(r.Context(), taskID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if reviews == nil {
			reviews = []*db.TaskReview{}
		}
		api.WriteJSON(w, http.StatusOK, reviews)

	case http.MethodPost:
		var req struct {
			AuthorType    string `json:"author_type"`
			AuthorRole    string `json:"author_role"`
			AuthorID      string `json:"author_id"`
			Status        string `json:"status"`
			Body          string `json:"body"`
			BranchHeadSHA string `json:"branch_head_sha"`
		}
		if !s.decodeJSON(w, r, &req) {
			return
		}
		if req.Status == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "status is required")
			return
		}
		if req.Body == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "body is required")
			return
		}
		rev := &db.TaskReview{
			TaskID:        taskID,
			AuthorType:    req.AuthorType,
			AuthorRole:    req.AuthorRole,
			AuthorID:      req.AuthorID,
			Status:        req.Status,
			Body:          req.Body,
			BranchHeadSHA: req.BranchHeadSHA,
		}
		if err := s.db.CreateTaskReview(r.Context(), rev); err != nil {
			s.internalError(w, err)
			return
		}

		// Drive the state machine based on review status.
		task, err := s.db.GetTask(r.Context(), taskID)
		if err == nil && task.Status == db.TaskStatusReviewing {
			switch req.Status {
			case "approved":
				_ = s.db.TransitionTaskState(r.Context(), taskID,
					db.TaskStatusReviewing, db.TaskStatusAwaitingMerge,
					req.AuthorID, "review approved")
			case "changes_requested", "revision_requested":
				_ = s.db.TransitionTaskState(r.Context(), taskID,
					db.TaskStatusReviewing, db.TaskStatusAwaitingRevision,
					req.AuthorID, "reviewer requested changes")
			}
		}

		api.WriteJSON(w, http.StatusCreated, rev)

	default:
		methodNotAllowed(w)
	}
}
