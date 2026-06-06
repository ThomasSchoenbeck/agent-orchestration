package server

import (
	"context"
	"fmt"
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
	"agent-orchestrator/git"
)

// handleTaskPullRequests serves the PR sub-routes of a task:
//
//	GET  /api/tasks/{id}/pull-requests
//	POST /api/tasks/{id}/pull-requests/{prID}/approve
//	POST /api/tasks/{id}/pull-requests/{prID}/reject
//
// parts is the path split after "/api/tasks/": [taskID, "pull-requests", prID?, action?].
func (s *Server) handleTaskPullRequests(w http.ResponseWriter, r *http.Request, taskID string, parts []string) {
	// List: no PR id.
	if len(parts) < 3 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		prs, err := s.db.ListPRsForTask(r.Context(), taskID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if prs == nil {
			prs = []*db.PullRequest{}
		}
		api.WriteJSON(w, http.StatusOK, prs)
		return
	}

	prID := parts[2]
	action := ""
	if len(parts) > 3 {
		action = parts[3]
	}

	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req struct {
		DeciderID string `json:"decider_id"`
		Body      string `json:"body"`
	}
	_ = s.decodeJSONOptional(r, &req)

	switch action {
	case "approve":
		s.approvePR(w, r, taskID, prID, req.DeciderID, req.Body)
	case "reject":
		s.rejectPR(w, r, taskID, prID, req.DeciderID, req.Body)
	default:
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "unknown pull-request action")
	}
}

// approvePR is the single place that touches git: a merge can only follow an
// explicit approval. It merges the PR branch into main, deletes the branch,
// marks the PR merged, and completes the task. On merge conflict the PR is
// rejected and the task returns to AWAITING_REVISION with the branch kept.
func (s *Server) approvePR(w http.ResponseWriter, r *http.Request, taskID, prID, deciderID, body string) {
	ctx := r.Context()
	pr, err := s.db.GetPR(ctx, prID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
		return
	}
	task, err := s.db.GetTask(ctx, taskID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
		return
	}
	if task.Status != db.TaskStatusAwaitingMerge && task.Status != db.TaskStatusMerging {
		api.WriteError(w, http.StatusConflict, api.ErrCodeConflict,
			fmt.Sprintf("task %q is not awaiting merge (status %q)", taskID, task.Status))
		return
	}

	project, err := s.db.GetProject(ctx, task.ProjectID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	repoPath := s.storage.RepoPath(project.ID)

	var sha string
	var mergeErr error
	if s.cfg.Merge.ShouldSquash() {
		msg, name, email := s.squashCommitInfo(ctx, task, deciderID)
		sha, mergeErr = git.SquashMerge(repoPath, pr.Base, pr.Branch, msg, name, email)
	} else {
		sha, mergeErr = git.MergeBranch(repoPath, pr.Base, pr.Branch)
	}
	if mergeErr != nil {
		// Conflict or merge failure: reject the PR and send the task back for
		// revision. The branch is kept so the worker can resolve and resubmit.
		decision := body
		if decision != "" {
			decision += "\n\n"
		}
		decision += fmt.Sprintf("Merge failed: %v", mergeErr)
		_ = s.db.SetPRDecision(ctx, prID, "rejected", deciderID, decision)
		_ = s.db.TransitionTaskState(ctx, taskID, task.Status,
			db.TaskStatusAwaitingRevision, deciderID, "merge conflict — revision requested")
		s.logPREvent(ctx, task, deciderID, "pr_rejected", task.Status,
			db.TaskStatusAwaitingRevision, fmt.Sprintf("PR %s rejected: merge failed", prID))
		updated, _ := s.db.GetPR(ctx, prID)
		api.WriteJSON(w, http.StatusOK, updated)
		return
	}

	// Merge succeeded: optionally delete the source branch, record the SHA, complete.
	if s.cfg.Merge.ShouldDeleteBranch() {
		if derr := git.DeleteBranch(repoPath, pr.Branch); derr != nil {
			// Non-fatal: the merge already landed; log and continue.
			s.logPREvent(ctx, task, deciderID, "pr_branch_delete_failed", task.Status, task.Status,
				fmt.Sprintf("branch %s delete failed: %v", pr.Branch, derr))
		}
	}
	_ = s.db.SetPRDecision(ctx, prID, "merged", deciderID, body)

	task.BranchHeadSHA = sha
	_ = s.db.UpdateTask(ctx, task)

	fromStatus := task.Status
	_ = s.db.TransitionTaskState(ctx, taskID, fromStatus,
		db.TaskStatusCompleted, deciderID, "pr merged")
	s.logPREvent(ctx, task, deciderID, "pr_merged", fromStatus,
		db.TaskStatusCompleted, fmt.Sprintf("PR %s merged (sha=%s)", prID, sha))

	// Mirror upstream if configured (non-blocking).
	if project.RemoteURL != "" {
		go func(p *db.Project) {
			if err := git.PushMain(s.storage.RepoPath(p.ID), "upstream", ""); err != nil {
				_ = s.db.CreateLog(context.Background(), &db.LogEntry{
					ProjectID: p.ID,
					Level:     "warn",
					Message:   fmt.Sprintf("upstream push failed: %v", err),
					Metadata:  map[string]interface{}{"event": "project_upstream_sync_failed"},
				})
			}
		}(project)
	}

	updated, _ := s.db.GetPR(ctx, prID)
	api.WriteJSON(w, http.StatusOK, updated)
}

// rejectPR records a merge-review rejection: the PR is marked rejected and the
// task returns to AWAITING_REVISION with its branch kept. No git is touched.
func (s *Server) rejectPR(w http.ResponseWriter, r *http.Request, taskID, prID, deciderID, body string) {
	ctx := r.Context()
	if _, err := s.db.GetPR(ctx, prID); err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
		return
	}
	task, err := s.db.GetTask(ctx, taskID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
		return
	}
	if task.Status != db.TaskStatusAwaitingMerge && task.Status != db.TaskStatusMerging {
		api.WriteError(w, http.StatusConflict, api.ErrCodeConflict,
			fmt.Sprintf("task %q is not awaiting merge (status %q)", taskID, task.Status))
		return
	}

	_ = s.db.SetPRDecision(ctx, prID, "rejected", deciderID, body)
	_ = s.db.TransitionTaskState(ctx, taskID, task.Status,
		db.TaskStatusAwaitingRevision, deciderID, "merge review rejected")
	s.logPREvent(ctx, task, deciderID, "pr_rejected", task.Status,
		db.TaskStatusAwaitingRevision, fmt.Sprintf("PR %s rejected", prID))

	updated, _ := s.db.GetPR(ctx, prID)
	api.WriteJSON(w, http.StatusOK, updated)
}

// squashCommitInfo derives the squash commit message and author for a merged
// task: the message is "task/<id>: <title>" and the author is the agent that
// performed the merge (deciderID), resolved to its name when available.
func (s *Server) squashCommitInfo(ctx context.Context, task *db.Task, deciderID string) (message, name, email string) {
	title, _ := task.Payload["title"].(string)
	if title == "" {
		title = "merge work"
	}
	message = fmt.Sprintf("task/%s: %s\n", task.ID, title)
	name = deciderID
	if a, err := s.db.GetAgent(ctx, deciderID); err == nil && a != nil && a.Name != "" {
		name = a.Name
	}
	if name == "" {
		name = "Agent Orchestrator"
	}
	return message, name, "agent@agent-orchestrator"
}

func (s *Server) logPREvent(ctx context.Context, task *db.Task, agentID, event, from, to, desc string) {
	_ = s.db.CreateTaskLog(ctx, &db.TaskLog{
		TaskID:      task.ID,
		ProjectID:   task.ProjectID,
		AgentID:     agentID,
		EventType:   event,
		OldStatus:   from,
		NewStatus:   to,
		Description: desc,
	})
}
