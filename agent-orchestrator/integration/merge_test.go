package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/storage"
	"agent-orchestrator/workflow"

	gogit "github.com/go-git/go-git/v5"
)

// TestFullLifecycleCompleted (IT-6.1) drives the full happy path:
//
//	push → approve → TickOnce → COMPLETED; bare repo main has the pushed file.
func TestFullLifecycleCompleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	srv := newGitTestServer(t)
	projectID, slug := makeProject(t, srv.BaseURL, "lifecycle-test")

	// Seed main so ChangedFiles can resolve the base commit.
	seedMainBranch(t, srv.BaseURL, slug)

	taskID := makeTask(t, srv.BaseURL, projectID)
	workerID := registerAgent(t, srv.BaseURL, "worker-lc", []string{"worker"})
	branch := claimTask(t, srv.BaseURL, taskID, workerID)

	_, clonePath := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	commitHash := commitAndPush(t, clonePath, "app.go", "package app", branch)

	// Reviewer approves → AWAITING_MERGE.
	reviewerID := registerAgent(t, srv.BaseURL, "reviewer-lc", []string{"reviewer"})
	claimTask(t, srv.BaseURL, taskID, reviewerID)
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/reviews",
		map[string]interface{}{
			"status":          "approved",
			"body":            "LGTM",
			"branch_head_sha": commitHash,
		}, nil)

	var awaitingTask db.Task
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID, nil, &awaitingTask)
	if awaitingTask.Status != db.TaskStatusAwaitingMerge {
		t.Fatalf("after approval: status = %q, want %q", awaitingTask.Status, db.TaskStatusAwaitingMerge)
	}

	// Drive the merge supervisor one cycle.
	sup := workflow.NewMergeSupervisor(srv.DB, storage.New(srv.StorageRoot, "", ""), 0)
	sup.TickOnce(ctx)

	// Task must be COMPLETED.
	var task db.Task
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID, nil, &task)
	if task.Status != db.TaskStatusCompleted {
		t.Fatalf("after TickOnce: status = %q, want %q", task.Status, db.TaskStatusCompleted)
	}

	// Clone main and assert app.go is present.
	cloneDir := t.TempDir()
	_, err := gogit.PlainClone(cloneDir, false, &gogit.CloneOptions{
		URL: srv.BaseURL + "/git/" + slug + ".git",
	})
	if err != nil {
		t.Fatalf("clone main: %v", err)
	}
	appGoPath := filepath.Join(cloneDir, "app.go")
	data, err := os.ReadFile(appGoPath)
	if err != nil {
		t.Fatalf("app.go not found in main after merge: %v", err)
	}
	if string(data) != "package app" {
		t.Errorf("app.go content = %q, want %q", string(data), "package app")
	}

	// Bare repo main must resolve to a non-zero SHA.
	repoPath := filepath.Join(srv.StorageRoot, "repos", projectID+".git")
	bareRepo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		t.Fatalf("open bare repo: %v", err)
	}
	mainRef, err := bareRepo.Head()
	if err != nil {
		t.Fatalf("bare repo HEAD: %v", err)
	}
	if mainRef.Hash().IsZero() {
		t.Error("bare repo main HEAD is zero hash after merge")
	}
}
