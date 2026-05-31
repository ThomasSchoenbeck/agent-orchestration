package server

import (
	"context"
	"log"
	"strings"
	"time"

	"agent-orchestrator/db"
	"agent-orchestrator/git"
)

// newGitHTTPHandler constructs the git smart-HTTP handler wired to the DB.
// It resolves project slugs to bare-repo paths and fires the post-receive
// hook that drives the DEVELOPING → AWAITING_REVIEW state transition.
func (s *Server) newGitHTTPHandler() *git.HTTPHandler {
	resolver := func(slug string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p, err := s.db.GetProjectBySlugOrID(ctx, slug)
		if err != nil {
			return "", err
		}
		return s.storage.RepoPath(p.ID), nil
	}

	postReceive := func(branchName, newSHA string) {
		// Only care about task branches: task/{taskID}
		if !strings.HasPrefix(branchName, "task/") {
			return
		}
		taskID := strings.TrimPrefix(branchName, "task/")
		if taskID == "" {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Load the task to check its current state.
		t, err := s.db.GetTask(ctx, taskID)
		if err != nil {
			log.Printf("git post-receive: GetTask %q: %v", taskID, err)
			return
		}

		// Record the pushed SHA and timestamp regardless of state transition.
		now := time.Now().UTC()
		t.BranchHeadSHA = newSHA
		t.LastPushAt = &now
		if err := s.db.UpdateTask(ctx, t); err != nil {
			log.Printf("git post-receive: UpdateTask SHA %q: %v", taskID, err)
		}

		// Transition DEVELOPING → AWAITING_REVIEW on push.
		if t.Status == db.TaskStatusDeveloping {
			if err := s.db.TransitionTaskState(
				ctx, taskID,
				db.TaskStatusDeveloping,
				db.TaskStatusAwaitingReview,
				"", // no agent actor — driven by git push
				"branch pushed by agent",
			); err != nil {
				log.Printf("git post-receive: TransitionTaskState %q: %v", taskID, err)
			} else {
				log.Printf("git post-receive: task %q → AWAITING_REVIEW (sha=%s)", taskID, newSHA)
			}
		}
	}

	return git.NewHTTPHandler(resolver, postReceive)
}
