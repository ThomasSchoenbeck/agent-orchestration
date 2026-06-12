package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"agent-orchestrator/api"
	"agent-orchestrator/branchname"
	"agent-orchestrator/db"
)

// releaseTaskResources frees port pool entries held by a task that is leaving
// an execution state. Safe to call even if the task has no port assigned.
func (s *Server) releaseTaskResources(task *db.Task) {
	if task.AssignedPort > 0 {
		s.portPool.Release(task.AssignedPort)
	}
}

// prepareClaimResponse builds a ClaimTaskResponse for a task that has just been
// dequeued. It provisions workspace access (repo URL + branch) and persists
// those values back to the task row.
func (s *Server) prepareClaimResponse(ctx context.Context, task *db.Task, agentID string) *api.ClaimTaskResponse {
	resp := &api.ClaimTaskResponse{Task: task}

	// Allocate a port from the pool only if the task does not already have one
	// (tasks/next provisions the port when it first dequeues the task; the
	// explicit /claim endpoint may be called afterwards and must not double-allocate).
	if task.AssignedPort == 0 {
		port, err := s.portPool.Acquire()
		if err != nil {
			log.Printf("claim: port pool exhausted for task %q: %v", task.ID, err)
		} else {
			task.AssignedPort = port
		}
	}
	resp.AssignedPort = task.AssignedPort

	// Generate the human-readable branch once, at the start of work, and persist
	// it (immutable thereafter). Re-claims reuse the stored branch.
	branchName := task.Branch
	if branchName == "" {
		branchName = s.resolveTaskBranch(ctx, task)
		task.Branch = branchName
	}
	task.AssignedAgentID = agentID

	// A review claimed with no explicit reviewer ("any reviewer") records the
	// claiming agent's review role, so the agent routes the review with a reviewer
	// persona — the executor's effectiveRole needs review_role set to know it is a
	// review rather than running it as the task's own (worker) role.
	if (task.Status == db.TaskStatusReviewing || task.Status == db.TaskStatusMerging) && task.ReviewRole == "" {
		if rr := s.reviewRoleForAgent(ctx, agentID); rr != "" {
			task.ReviewRole = rr
		}
	}

	// Open the pull request when the merge phase is picked up — a task in
	// AWAITING_MERGE claimed into MERGING. Idempotent: it reuses an existing open
	// PR (e.g. one a human opened earlier via the Create-PR button).
	if task.Status == db.TaskStatusMerging {
		s.ensurePRForTask(ctx, task, agentID, "")
	}

	s.provisionWorkspace(ctx, task, branchName, resp)

	// Persist agent assignment and port back to DB.
	now := time.Now().UTC()
	task.StartedAt = &now
	if err := s.db.UpdateTask(ctx, task); err != nil {
		log.Printf("claim: UpdateTask workspace fields for %q: %v", task.ID, err)
	}

	return resp
}

// reviewRoleForAgent returns the id of the claiming agent's first role that
// carries the handles_review capability, or "" when it has none.
func (s *Server) reviewRoleForAgent(ctx context.Context, agentID string) string {
	agent, err := s.db.GetAgent(ctx, agentID)
	if err != nil || agent == nil {
		return ""
	}
	for _, roleRef := range agent.Roles {
		rd := s.roleDefByRef(ctx, roleRef)
		if rd == nil {
			continue
		}
		for _, c := range rd.Capabilities {
			if c == "handles_review" {
				return rd.ID
			}
		}
	}
	return ""
}

// resolveTaskBranch builds the human-readable branch an agent will work on, from
// the task type's template, ensuring it is unique within the project.
func (s *Server) resolveTaskBranch(ctx context.Context, task *db.Task) string {
	var template, typeKey string
	if tt := s.taskTypeFor(ctx, task); tt != nil {
		template = tt.BranchTemplate
		typeKey = tt.Key
	}
	title, _ := task.Payload["title"].(string)
	branch := branchname.Generate(template, title, task.ID, typeKey)

	// Uniqueness within the project: if another task already owns this branch,
	// append a short id suffix so the git-hook lookup stays 1:1.
	if existing, err := s.db.GetTaskByBranch(ctx, task.ProjectID, branch); err == nil &&
		existing != nil && existing.ID != task.ID {
		short := task.ID
		if len(short) > 8 {
			short = short[:8]
		}
		branch = branch + "-" + short
	}
	return branch
}

// taskTypeFor returns the task's configured type, falling back to the default
// task type, or nil when none is defined.
func (s *Server) taskTypeFor(ctx context.Context, task *db.Task) *db.TaskType {
	if task.TaskTypeID != "" {
		if tt, err := s.db.GetTaskType(ctx, task.TaskTypeID); err == nil {
			return tt
		}
	}
	if tt, err := s.db.GetDefaultTaskType(ctx); err == nil {
		return tt
	}
	return nil
}

// provisionWorkspace builds the RepoURL and Branch that the agent will use to
// clone and push. All agents — regardless of where they run — use the server's
// embedded git HTTP endpoint; there is no colocated filesystem shortcut.
func (s *Server) provisionWorkspace(ctx context.Context, task *db.Task, branchName string, resp *api.ClaimTaskResponse) {
	project, err := s.db.GetProject(ctx, task.ProjectID)
	if err != nil {
		log.Printf("claim: GetProject %q: %v", task.ProjectID, err)
		return
	}

	slug := project.Slug
	if slug == "" {
		slug = project.ID
	}

	scheme := "https"
	if s.cfg.Server.Insecure {
		scheme = "http"
	}
	resp.RepoURL = fmt.Sprintf("%s://%s:%d/git/%s.git",
		scheme, s.cfg.Server.PublicHost(), s.cfg.Server.Port, slug)
	resp.Branch = branchName

	// Log and notify so the user can see the branch immediately after claim.
	logEntry := &db.LogEntry{
		AgentID:   task.AssignedAgentID,
		TaskID:    task.ID,
		ProjectID: project.ID,
		Level:     "info",
		Message:   fmt.Sprintf("Workspace provisioned: %s branch %s", resp.RepoURL, branchName),
	}
	if lerr := s.db.CreateLog(ctx, logEntry); lerr != nil {
		log.Printf("claim: CreateLog task %q: %v", task.ID, lerr)
	}
	comment := &db.TaskComment{
		TaskID:     task.ID,
		AuthorType: "agent",
		AuthorID:   task.AssignedAgentID,
		Body:       fmt.Sprintf("Repository ready at `%s` on branch `%s`. Agent is starting.", resp.RepoURL, branchName),
	}
	if cerr := s.db.CreateComment(ctx, comment); cerr != nil {
		log.Printf("claim: CreateComment task %q: %v", task.ID, cerr)
	}
}
