package integration_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-orchestrator/db"
	"agent-orchestrator/git"

	gogit "github.com/go-git/go-git/v5"
)

// bareRepoPath returns the on-disk path of a project's bare repo.
func bareRepoPath(srv *gitTestServer, projectID string) string {
	return filepath.Join(srv.StorageRoot, "repos", projectID+".git")
}

// firstPRID lists a task's pull requests and returns the newest one's ID.
func firstPRID(t *testing.T, srv *gitTestServer, taskID string) string {
	t.Helper()
	var prs []db.PullRequest
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID+"/pull-requests", nil, &prs)
	if len(prs) == 0 {
		t.Fatalf("no pull requests for task %q", taskID)
	}
	return prs[0].ID
}

// setupApprovedPR drives a task to AWAITING_MERGE with an open PR: the worker
// pushes app.go on its branch, the reviewer approves, which opens the PR.
// Returns (projectID, slug, taskID, branch, prID).
func setupApprovedPR(t *testing.T, srv *gitTestServer, name string) (string, string, string, string, string) {
	t.Helper()
	projectID, slug := makeProject(t, srv.BaseURL, name)
	seedMainBranch(t, srv.BaseURL, slug)

	taskID := makeTask(t, srv.BaseURL, projectID)
	workerID := registerAgent(t, srv.BaseURL, name+"-worker", []string{"worker"})
	branch := claimTask(t, srv.BaseURL, taskID, workerID)

	_, clonePath := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	commitAndPush(t, clonePath, "app.go", "package app", branch)

	reviewerID := registerAgent(t, srv.BaseURL, name+"-reviewer", []string{"reviewer"})
	claimTask(t, srv.BaseURL, taskID, reviewerID)
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/reviews",
		map[string]interface{}{"status": "approved", "body": "LGTM"}, nil)

	// The PR opens when the merge phase is picked up; open it here via the
	// (human) Create-PR endpoint so the merge-decision assertions have a PR.
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/pull-requests", nil, nil)

	return projectID, slug, taskID, branch, firstPRID(t, srv, taskID)
}

func getTask(t *testing.T, srv *gitTestServer, taskID string) db.Task {
	t.Helper()
	var task db.Task
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID, nil, &task)
	return task
}

func getPR(t *testing.T, srv *gitTestServer, taskID, prID string) db.PullRequest {
	t.Helper()
	var prs []db.PullRequest
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID+"/pull-requests", nil, &prs)
	for _, pr := range prs {
		if pr.ID == prID {
			return pr
		}
	}
	t.Fatalf("PR %q not found for task %q", prID, taskID)
	return db.PullRequest{}
}

func TestReviewApproval_OpensPRAndAwaitsMerge(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	_, _, taskID, branch, prID := setupApprovedPR(t, srv, "pr-open")

	task := getTask(t, srv, taskID)
	if task.Status != db.TaskStatusAwaitingMerge {
		t.Fatalf("task status = %q, want %q", task.Status, db.TaskStatusAwaitingMerge)
	}
	pr := getPR(t, srv, taskID, prID)
	if pr.Status != "open" {
		t.Errorf("PR status = %q, want open", pr.Status)
	}
	if pr.Branch != branch {
		t.Errorf("PR branch = %q, want %q", pr.Branch, branch)
	}
}

func TestPR_NotAutoApproved(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	_, _, taskID, _, prID := setupApprovedPR(t, srv, "pr-noauto")

	// Give any (incorrectly-running) background merger a chance to act.
	time.Sleep(500 * time.Millisecond)

	task := getTask(t, srv, taskID)
	if task.Status != db.TaskStatusAwaitingMerge {
		t.Errorf("task status = %q, want %q (must not auto-merge)", task.Status, db.TaskStatusAwaitingMerge)
	}
	pr := getPR(t, srv, taskID, prID)
	if pr.Status != "open" {
		t.Errorf("PR status = %q, want open (no decision yet)", pr.Status)
	}
}

func TestDeployerClaimsAndApproves_Merges(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	projectID, _, taskID, branch, prID := setupApprovedPR(t, srv, "pr-merge")

	// Deployer claims the merge gate (AWAITING_MERGE → MERGING).
	deployerID := registerAgent(t, srv.BaseURL, "pr-merge-deployer", []string{"deployer"})
	claimTask(t, srv.BaseURL, taskID, deployerID)
	if s := getTask(t, srv, taskID).Status; s != db.TaskStatusMerging {
		t.Fatalf("after deployer claim: status = %q, want %q", s, db.TaskStatusMerging)
	}

	status := apiJSON(t, "POST", srv.BaseURL,
		"/api/tasks/"+taskID+"/pull-requests/"+prID+"/approve",
		map[string]interface{}{"decider_id": deployerID, "body": "ship it"}, nil)
	if status != 200 {
		t.Fatalf("approve: expected 200, got %d", status)
	}

	if s := getTask(t, srv, taskID).Status; s != db.TaskStatusCompleted {
		t.Fatalf("after approve: status = %q, want %q", s, db.TaskStatusCompleted)
	}
	if pr := getPR(t, srv, taskID, prID); pr.Status != "merged" {
		t.Errorf("PR status = %q, want merged", pr.Status)
	}

	// Branch deleted; main contains app.go.
	repoPath := bareRepoPath(srv, projectID)
	branches, _ := git.ListBranches(repoPath)
	for _, b := range branches {
		if b == branch {
			t.Errorf("branch %q should have been deleted; branches: %v", branch, branches)
		}
	}
	if _, err := git.ReadFile(repoPath, "main", "app.go"); err != nil {
		t.Errorf("app.go missing on main after merge: %v", err)
	}
}

func TestDeployerRejects_ReturnsToRevision(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	projectID, _, taskID, branch, prID := setupApprovedPR(t, srv, "pr-reject")

	deployerID := registerAgent(t, srv.BaseURL, "pr-reject-deployer", []string{"deployer"})
	claimTask(t, srv.BaseURL, taskID, deployerID)

	status := apiJSON(t, "POST", srv.BaseURL,
		"/api/tasks/"+taskID+"/pull-requests/"+prID+"/reject",
		map[string]interface{}{"decider_id": deployerID, "body": "not deployable"}, nil)
	if status != 200 {
		t.Fatalf("reject: expected 200, got %d", status)
	}

	if s := getTask(t, srv, taskID).Status; s != db.TaskStatusAwaitingRevision {
		t.Fatalf("after reject: status = %q, want %q", s, db.TaskStatusAwaitingRevision)
	}
	if pr := getPR(t, srv, taskID, prID); pr.Status != "rejected" {
		t.Errorf("PR status = %q, want rejected", pr.Status)
	}
	// Branch is kept so the worker can revise and resubmit.
	branches, _ := git.ListBranches(bareRepoPath(srv, projectID))
	found := false
	for _, b := range branches {
		if b == branch {
			found = true
		}
	}
	if !found {
		t.Errorf("branch %q should be kept after reject; branches: %v", branch, branches)
	}
}

func TestApprove_MergeConflict_ReturnsToRevision(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	projectID, _, taskID, _, prID := setupApprovedPR(t, srv, "pr-conflict")

	// Advance main with a conflicting version of app.go (task added it as
	// "package app"; main now adds it differently → three-way conflict).
	repoPath := bareRepoPath(srv, projectID)
	if _, err := git.CommitFile(repoPath, "main", "app.go", []byte("package main // conflict"), "diverge", "u", "u@x"); err != nil {
		t.Fatalf("CommitFile main: %v", err)
	}

	status := apiJSON(t, "POST", srv.BaseURL,
		"/api/tasks/"+taskID+"/pull-requests/"+prID+"/approve",
		map[string]interface{}{"decider_id": "human", "body": "approve"}, nil)
	if status != 200 {
		t.Fatalf("approve: expected 200, got %d", status)
	}

	if s := getTask(t, srv, taskID).Status; s != db.TaskStatusAwaitingRevision {
		t.Fatalf("after conflicting approve: status = %q, want %q", s, db.TaskStatusAwaitingRevision)
	}
	if pr := getPR(t, srv, taskID, prID); pr.Status != "rejected" {
		t.Errorf("PR status = %q, want rejected", pr.Status)
	}
}

func TestHumanApprovePR_Merges(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	projectID, _, taskID, _, prID := setupApprovedPR(t, srv, "pr-human")

	// Human approves directly from AWAITING_MERGE — no deployer claim.
	status := apiJSON(t, "POST", srv.BaseURL,
		"/api/tasks/"+taskID+"/pull-requests/"+prID+"/approve",
		map[string]interface{}{"decider_id": "human", "body": "approved by human"}, nil)
	if status != 200 {
		t.Fatalf("human approve: expected 200, got %d", status)
	}

	if s := getTask(t, srv, taskID).Status; s != db.TaskStatusCompleted {
		t.Fatalf("after human approve: status = %q, want %q", s, db.TaskStatusCompleted)
	}
	if pr := getPR(t, srv, taskID, prID); pr.Status != "merged" {
		t.Errorf("PR status = %q, want merged", pr.Status)
	}
	if _, err := git.ReadFile(bareRepoPath(srv, projectID), "main", "app.go"); err != nil {
		t.Errorf("app.go missing on main after human merge: %v", err)
	}
}

// TestFullWorkflow_E2E exercises the full Feature 2 chain:
// worker push → reviewer approves (PR open) → deployer reviews + approves →
// merged, task COMPLETED, branch deleted, main carries the work.
func TestFullWorkflow_E2E(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	projectID, slug, taskID, _, prID := setupApprovedPR(t, srv, "pr-e2e")

	if s := getTask(t, srv, taskID).Status; s != db.TaskStatusAwaitingMerge {
		t.Fatalf("after reviewer approval: status = %q, want %q", s, db.TaskStatusAwaitingMerge)
	}

	deployerID := registerAgent(t, srv.BaseURL, "pr-e2e-deployer", []string{"deployer"})
	claimTask(t, srv.BaseURL, taskID, deployerID)
	apiJSON(t, "POST", srv.BaseURL,
		"/api/tasks/"+taskID+"/pull-requests/"+prID+"/approve",
		map[string]interface{}{"decider_id": deployerID, "body": "deploy approved"}, nil)

	if s := getTask(t, srv, taskID).Status; s != db.TaskStatusCompleted {
		t.Fatalf("after deployer approve: status = %q, want %q", s, db.TaskStatusCompleted)
	}

	// main (cloned fresh) must carry app.go.
	cloneDir := t.TempDir()
	if _, err := gogit.PlainClone(cloneDir, false, &gogit.CloneOptions{
		URL: srv.BaseURL + "/git/" + slug + ".git",
	}); err != nil {
		t.Fatalf("clone main: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(cloneDir, "app.go")); err != nil {
		t.Errorf("app.go not present in merged main: %v", err)
	}
	_ = projectID
}
