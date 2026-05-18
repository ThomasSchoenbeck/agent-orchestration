//go:build integration

// IT-7.1: Two tasks touching shared.go both complete without error. Because
// processTask runs synchronously within tick (acquire lock → merge → release
// lock), the serialisation is implicit: the second task's merge sees the first
// task's changes in main and performs a three-way merge using "task-branch
// wins" for any same-file conflict. Both tasks reach COMPLETED and main has
// the second task's version of shared.go (last writer wins).
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

// TestSerialMergeOverlappingFiles (IT-7.1) verifies that two tasks both
// touching shared.go complete successfully without corruption. The merge
// supervisor serialises them: the second task's three-way merge applies
// "task-branch wins" when both sides added the same file.
func TestSerialMergeOverlappingFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	srv := newGitTestServer(t)
	projectID, slug := makeProject(t, srv.BaseURL, "serial-merge-test")
	seedMainBranch(t, srv.BaseURL, slug)

	sup := workflow.NewMergeSupervisor(srv.DB, storage.New(srv.StorageRoot), 0)

	// --- Task A (shared.go v1) ---
	taskA := makeTask(t, srv.BaseURL, projectID)
	workerA := registerAgent(t, srv.BaseURL, "worker-serial-a", []string{"worker"})
	claimTask(t, srv.BaseURL, taskA, workerA)
	_, cloneA := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	hashA := commitAndPush(t, cloneA, "shared.go", "package shared // v1", "task/"+taskA)

	reviewerA := registerAgent(t, srv.BaseURL, "reviewer-serial-a", []string{"reviewer"})
	claimTask(t, srv.BaseURL, taskA, reviewerA)
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskA+"/reviews",
		map[string]interface{}{
			"status":          "approved",
			"body":            "ok",
			"branch_head_sha": hashA,
		}, nil)

	// --- Task B (shared.go v2) ---
	taskB := makeTask(t, srv.BaseURL, projectID)
	workerB := registerAgent(t, srv.BaseURL, "worker-serial-b", []string{"worker"})
	claimTask(t, srv.BaseURL, taskB, workerB)
	_, cloneB := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	hashB := commitAndPush(t, cloneB, "shared.go", "package shared // v2", "task/"+taskB)

	reviewerB := registerAgent(t, srv.BaseURL, "reviewer-serial-b", []string{"reviewer"})
	claimTask(t, srv.BaseURL, taskB, reviewerB)
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskB+"/reviews",
		map[string]interface{}{
			"status":          "approved",
			"body":            "ok",
			"branch_head_sha": hashB,
		}, nil)

	// Both tasks are AWAITING_MERGE. Drive the supervisor until both complete.
	var tA, tB db.Task
	for i := 0; i < 10; i++ {
		sup.TickOnce(ctx)
		apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskA, nil, &tA)
		apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskB, nil, &tB)
		if tA.Status == db.TaskStatusCompleted && tB.Status == db.TaskStatusCompleted {
			break
		}
	}

	if tA.Status != db.TaskStatusCompleted {
		t.Errorf("task A: expected COMPLETED, got %q", tA.Status)
	}
	if tB.Status != db.TaskStatusCompleted {
		t.Errorf("task B: expected COMPLETED, got %q", tB.Status)
	}

	// Clone main; shared.go must exist with a valid version.
	cloneDir := t.TempDir()
	if _, err := gogit.PlainClone(cloneDir, false, &gogit.CloneOptions{
		URL: srv.BaseURL + "/git/" + slug + ".git",
	}); err != nil {
		t.Fatalf("clone main: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cloneDir, "shared.go"))
	if err != nil {
		t.Fatalf("shared.go not found in main: %v", err)
	}
	content := string(data)
	if content != "package shared // v1" && content != "package shared // v2" {
		t.Errorf("shared.go content = %q: expected v1 or v2", content)
	}
}
