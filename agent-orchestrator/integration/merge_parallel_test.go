//go:build integration

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

// TestParallelMergeNonOverlappingFiles (IT-8.1) verifies that two tasks
// touching different files both reach MERGING (or COMPLETED) in a single
// supervisor tick — no unnecessary serialisation.
func TestParallelMergeNonOverlappingFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	srv := newGitTestServer(t)
	projectID, slug := makeProject(t, srv.BaseURL, "parallel-merge-test")
	seedMainBranch(t, srv.BaseURL, slug)

	sup := workflow.NewMergeSupervisor(srv.DB, storage.New(srv.StorageRoot), 0)

	// --- Task A (touches a.go) ---
	taskA := makeTask(t, srv.BaseURL, projectID)
	workerA := registerAgent(t, srv.BaseURL, "worker-par-a", []string{"worker"})
	claimTask(t, srv.BaseURL, taskA, workerA)
	_, cloneA := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	hashA := commitAndPush(t, cloneA, "a.go", "package a", "task/"+taskA)

	reviewerA := registerAgent(t, srv.BaseURL, "reviewer-par-a", []string{"reviewer"})
	claimTask(t, srv.BaseURL, taskA, reviewerA)
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskA+"/reviews",
		map[string]interface{}{
			"status":          "approved",
			"body":            "ok",
			"branch_head_sha": hashA,
		}, nil)

	// --- Task B (touches b.go) ---
	taskB := makeTask(t, srv.BaseURL, projectID)
	workerB := registerAgent(t, srv.BaseURL, "worker-par-b", []string{"worker"})
	claimTask(t, srv.BaseURL, taskB, workerB)
	_, cloneB := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	hashB := commitAndPush(t, cloneB, "b.go", "package b", "task/"+taskB)

	reviewerB := registerAgent(t, srv.BaseURL, "reviewer-par-b", []string{"reviewer"})
	claimTask(t, srv.BaseURL, taskB, reviewerB)
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskB+"/reviews",
		map[string]interface{}{
			"status":          "approved",
			"body":            "ok",
			"branch_head_sha": hashB,
		}, nil)

	// Both tasks are AWAITING_MERGE. One tick should allow both to proceed.
	sup.TickOnce(ctx)

	var tA, tB db.Task
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskA, nil, &tA)
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskB, nil, &tB)

	// Both must have advanced past AWAITING_MERGE.
	for _, tk := range []db.Task{tA, tB} {
		if tk.Status == db.TaskStatusAwaitingMerge {
			t.Errorf("task %q still AWAITING_MERGE after TickOnce — non-overlapping files should not block each other", tk.ID)
		}
	}

	// Drive both to completion.
	for i := 0; i < 10; i++ {
		apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskA, nil, &tA)
		apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskB, nil, &tB)
		if tA.Status == db.TaskStatusCompleted && tB.Status == db.TaskStatusCompleted {
			break
		}
		sup.TickOnce(ctx)
	}

	if tA.Status != db.TaskStatusCompleted {
		t.Errorf("task A: expected COMPLETED, got %q", tA.Status)
	}
	if tB.Status != db.TaskStatusCompleted {
		t.Errorf("task B: expected COMPLETED, got %q", tB.Status)
	}

	// Clone main; both a.go and b.go must exist.
	cloneDir := t.TempDir()
	if _, err := gogit.PlainClone(cloneDir, false, &gogit.CloneOptions{
		URL: srv.BaseURL + "/git/" + slug + ".git",
	}); err != nil {
		t.Fatalf("clone main: %v", err)
	}
	for _, file := range []string{"a.go", "b.go"} {
		if _, err := os.Stat(filepath.Join(cloneDir, file)); err != nil {
			t.Errorf("%s not found in main after merge: %v", file, err)
		}
	}
}
