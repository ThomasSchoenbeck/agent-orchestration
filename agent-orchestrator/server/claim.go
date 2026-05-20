package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
	"agent-orchestrator/git"
)

// releaseTaskResources frees port pool entries held by a task that is leaving
// an execution state. Safe to call even if the task has no port assigned.
func (s *Server) releaseTaskResources(task *db.Task) {
	if task.AssignedPort > 0 {
		s.portPool.Release(task.AssignedPort)
	}
}

// prepareClaimResponse builds a ClaimTaskResponse for a task that has just been
// dequeued. It provisions workspace (worktree or repo URL), allocates a port,
// and persists those values back to the task row.
func (s *Server) prepareClaimResponse(ctx context.Context, task *db.Task, agentID string) *api.ClaimTaskResponse {
	resp := &api.ClaimTaskResponse{Task: task}

	agent, err := s.db.GetAgent(ctx, agentID)
	if err != nil {
		log.Printf("claim: GetAgent %q: %v", agentID, err)
		// Continue with default (remote) behaviour.
	}

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

	isColocated := agent != nil && agent.Mode == "colocated"
	branchName := fmt.Sprintf("task/%s", task.ID)

	if isColocated {
		s.provisionColocatedWorktree(ctx, task, branchName, resp)
	} else {
		s.provisionRemoteAccess(task, branchName, resp)
	}

	// Persist workspace fields and port back to DB.
	now := time.Now().UTC()
	task.StartedAt = &now
	if err := s.db.UpdateTask(ctx, task); err != nil {
		log.Printf("claim: UpdateTask workspace fields for %q: %v", task.ID, err)
	}

	return resp
}

func (s *Server) provisionColocatedWorktree(ctx context.Context, task *db.Task, branchName string, resp *api.ClaimTaskResponse) {
	// Look up the project to find its bare repo.
	project, err := s.db.GetProject(ctx, task.ProjectID)
	if err != nil {
		log.Printf("claim: GetProject %q for worktree: %v", task.ProjectID, err)
		return
	}

	repoPath := s.storage.RepoPath(project.ID)
	worktreePath := s.storage.WorktreePath(task.ID)

	// Determine base branch: for AWAITING_REVISION the branch already exists.
	baseBranch := "main"
	sha, err := git.CreateWorktree(repoPath, worktreePath, branchName, baseBranch)
	if err != nil {
		log.Printf("claim: CreateWorktree task %q: %v", task.ID, err)
		return
	}

	task.WorktreePath = worktreePath
	if sha != "" && sha != "0000000000000000000000000000000000000000" {
		task.BranchHeadSHA = sha
	}

	resp.WorktreePath = worktreePath

	// Write .agent_context/ files into the worktree.
	if err := writeAgentContext(ctx, s.db, task, worktreePath); err != nil {
		log.Printf("claim: writeAgentContext task %q: %v", task.ID, err)
	}
}

func (s *Server) provisionRemoteAccess(task *db.Task, branchName string, resp *api.ClaimTaskResponse) {
	// Remote agents clone over the embedded git HTTP server.
	// Build the repo URL from server config.
	host := s.cfg.Server.Host
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	resp.RepoURL = fmt.Sprintf("http://%s:%d/git/%s.git",
		host, s.cfg.Server.Port, task.ProjectID)
	resp.Branch = branchName
}
