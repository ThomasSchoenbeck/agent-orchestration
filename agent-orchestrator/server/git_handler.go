package server

import (
	"context"
	"log"
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

	postReceive := func(slug, branchName, newSHA string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Resolve the project, then the task that owns this branch. Branches that
		// belong to no task (e.g. pushes to main) are ignored. Branches are now
		// human-readable (e.g. "feature/<slug>"), so we resolve by the persisted
		// branch name rather than parsing a "task/<id>" prefix.
		p, err := s.db.GetProjectBySlugOrID(ctx, slug)
		if err != nil {
			return
		}
		t, err := s.db.GetTaskByBranch(ctx, p.ID, branchName)
		if err != nil {
			return // not a task branch
		}

		// Record the pushed SHA and timestamp regardless of state transition.
		now := time.Now().UTC()
		t.BranchHeadSHA = newSHA
		t.LastPushAt = &now
		if err := s.db.UpdateTask(ctx, t); err != nil {
			log.Printf("git post-receive: UpdateTask SHA %q: %v", t.ID, err)
		}

		// Transition DEVELOPING → AWAITING_REVIEW on push.
		if t.Status == db.TaskStatusDeveloping {
			if err := s.db.TransitionTaskState(
				ctx, t.ID,
				db.TaskStatusDeveloping,
				db.TaskStatusAwaitingReview,
				"", // no agent actor — driven by git push
				"branch pushed by agent",
			); err != nil {
				log.Printf("git post-receive: TransitionTaskState %q: %v", t.ID, err)
			} else {
				log.Printf("git post-receive: task %q → AWAITING_REVIEW (sha=%s)", t.ID, newSHA)
			}
		}
	}

	return git.NewHTTPHandler(resolver, postReceive)
}
