package server

import (
	"context"
	"fmt"
	"log"
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
				// Work review approved: open a PR and move to the merge gate
				// rather than completing. Merge only follows an explicit
				// approval decision on the PR (deployer or human).
				// Approval just moves the task to the merge gate. The pull request
				// is opened when the merge phase is picked up (MERGING) or when a
				// human opens it via the Create-PR button.
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

// ensurePRForTask returns the task's open pull request, creating one (feature →
// main) if none exists. Idempotent: a reviewer claim and the later approval both
// call it, and re-claims after a revision reuse the same PR. Failures are logged
// but never fatal. Returns nil only when creation fails.
func (s *Server) ensurePRForTask(ctx context.Context, task *db.Task, authorID, authorName string) *db.PullRequest {
	if prs, err := s.db.ListPRsForTask(ctx, task.ID); err == nil {
		for _, pr := range prs {
			if pr.Status == "open" {
				return pr
			}
		}
	}
	title, _ := task.Payload["title"].(string)
	if title == "" {
		title = "Task " + task.ID
	}
	branch := task.Branch
	if branch == "" {
		branch = fmt.Sprintf("task/%s", task.ID)
	}
	pr := &db.PullRequest{
		TaskID:     task.ID,
		ProjectID:  task.ProjectID,
		Branch:     branch,
		Base:       "main",
		Title:      title,
		Status:     "open",
		AuthorID:   authorID,
		AuthorName: authorName,
	}
	if err := s.db.CreatePR(ctx, pr); err != nil {
		log.Printf("ensurePRForTask %q: %v", task.ID, err)
		return nil
	}
	_ = s.db.CreateTaskLog(ctx, &db.TaskLog{
		TaskID:      task.ID,
		ProjectID:   task.ProjectID,
		AgentID:     authorID,
		EventType:   "pr_opened",
		OldStatus:   task.Status,
		NewStatus:   task.Status,
		Description: fmt.Sprintf("Pull request %s opened", pr.ID),
	})
	return pr
}
