//go:build integration

package integration_test

import (
	"path/filepath"
	"testing"

	"agent-orchestrator/db"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// TestPushTransitionsToAwaitingReview (IT-3.1) verifies that when a worker
// agent pushes to refs/heads/task/{id}, the post-receive hook fires,
// branch_head_sha is recorded, and the task transitions to AWAITING_REVIEW.
func TestPushTransitionsToAwaitingReview(t *testing.T) {
	t.Parallel()

	srv := newGitTestServer(t)
	projectID, slug := makeProject(t, srv.BaseURL, "push-test")
	taskID := makeTask(t, srv.BaseURL, projectID)
	workerID := registerAgent(t, srv.BaseURL, "worker-1", []string{"worker"})
	claimTask(t, srv.BaseURL, taskID, workerID)

	repoURL := srv.BaseURL + "/git/" + slug + ".git"
	_, clonePath := cloneRepo(t, repoURL)

	commitHash := commitAndPush(t, clonePath, "main.go", "package main", "task/"+taskID)

	// 1. Task must be AWAITING_REVIEW.
	var task db.Task
	status := apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID, nil, &task)
	if status != 200 {
		t.Fatalf("GET task: expected 200, got %d", status)
	}
	if task.Status != db.TaskStatusAwaitingReview {
		t.Errorf("status = %q, want %q", task.Status, db.TaskStatusAwaitingReview)
	}

	// 2. branch_head_sha must match the pushed commit.
	if task.BranchHeadSHA != commitHash {
		t.Errorf("branch_head_sha = %q, want %q", task.BranchHeadSHA, commitHash)
	}

	// 3. The bare repo must have refs/heads/task/{id} pointing at the same hash.
	repoPath := filepath.Join(srv.StorageRoot, "repos", projectID+".git")
	bareRepo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		t.Fatalf("open bare repo: %v", err)
	}
	ref, err := bareRepo.Reference(plumbing.NewBranchReferenceName("task/"+taskID), true)
	if err != nil {
		t.Fatalf("resolve task branch in bare repo: %v", err)
	}
	if ref.Hash().String() != commitHash {
		t.Errorf("bare repo task branch = %q, want %q", ref.Hash().String(), commitHash)
	}
}
