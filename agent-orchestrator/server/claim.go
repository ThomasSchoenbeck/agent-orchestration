package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"agent-orchestrator/api"
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

	branchName := fmt.Sprintf("task/%s", task.ID)
	task.AssignedAgentID = agentID

	s.provisionWorkspace(ctx, task, branchName, resp)

	// Persist agent assignment and port back to DB.
	now := time.Now().UTC()
	task.StartedAt = &now
	if err := s.db.UpdateTask(ctx, task); err != nil {
		log.Printf("claim: UpdateTask workspace fields for %q: %v", task.ID, err)
	}

	return resp
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
