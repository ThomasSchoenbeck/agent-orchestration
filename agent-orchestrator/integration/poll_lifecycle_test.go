package integration_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// pollNextTask hits the real poll endpoint (GET /api/agents/{id}/tasks/next) the
// way an autonomous agent discovers work — routing through GetNextTask and
// atomically claiming any match. Returns the claimed task (nil when nothing
// matched) and its branch. This is the door real agents use; the rest of the
// suite claims by id via /claim, which bypasses the poll query's filters.
func pollNextTask(t *testing.T, baseURL, agentID string) (*db.Task, string) {
	t.Helper()
	var resp struct {
		Task   *db.Task `json:"task"`
		Branch string   `json:"branch"`
	}
	status := apiJSON(t, "GET", baseURL, "/api/agents/"+agentID+"/tasks/next", nil, &resp)
	if status != http.StatusOK {
		t.Fatalf("poll next for %q: expected 200, got %d", agentID, status)
	}
	return resp.Task, resp.Branch
}

// TestFullLifecycle_ViaPolling drives a single task worker → reviewer → deployer
// entirely through the POLL endpoint each agent really uses, instead of claiming
// by id. It exercises capability routing, review-role matching, and assignment
// release at every hop — the regression guard for B2 (review-role match) and B5
// (a queue-state task must release the prior agent's claim so the next agent can
// poll it). Both bugs made the reviewer poll below return nothing.
func TestFullLifecycle_ViaPolling(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	ctx := context.Background()

	// Capability-bearing role definitions must exist for poll-based routing.
	if _, err := srv.DB.SeedRoleDefinitions(ctx, db.DefaultRoleDefinitions()); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	projectID, slug := makeProject(t, srv.BaseURL, "poll-lifecycle")
	seedMainBranch(t, srv.BaseURL, slug)
	taskID := makeTask(t, srv.BaseURL, projectID)

	// 1. Worker discovers the BACKLOG task by polling.
	workerID := registerAgent(t, srv.BaseURL, "poll-worker", []string{"worker"})
	claimed, branch := pollNextTask(t, srv.BaseURL, workerID)
	if claimed == nil || claimed.ID != taskID {
		t.Fatalf("worker poll should claim the BACKLOG task, got %v", claimed)
	}
	if branch == "" {
		t.Fatal("expected a generated branch from the claim")
	}

	// Worker pushes its branch → git hook → AWAITING_REVIEW (assignment released).
	_, clonePath := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	hash := commitAndPush(t, clonePath, "app.go", "package app", branch)

	// 2. Reviewer discovers the AWAITING_REVIEW task by polling — the exact hop
	//    that was broken (assignment still held by the worker + review-role match).
	reviewerID := registerAgent(t, srv.BaseURL, "poll-reviewer", []string{"reviewer"})
	rTask, _ := pollNextTask(t, srv.BaseURL, reviewerID)
	if rTask == nil || rTask.ID != taskID {
		t.Fatalf("reviewer poll should claim the AWAITING_REVIEW task, got %v", rTask)
	}
	if rTask.Status != db.TaskStatusReviewing {
		t.Errorf("after reviewer claim: status = %q, want REVIEWING", rTask.Status)
	}
	// The "any reviewer" task must record the claiming reviewer's role so the
	// agent routes the review with a reviewer persona (not the worker role).
	if rTask.ReviewRole == "" {
		t.Errorf("review_role is empty after reviewer claim; the executor would route the review as the worker role")
	}

	// Reviewer approves → AWAITING_MERGE + PR opened.
	if st := apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/reviews",
		map[string]interface{}{"status": "approved", "body": "LGTM", "branch_head_sha": hash}, nil); st != http.StatusOK && st != http.StatusCreated {
		t.Fatalf("review approve: got %d", st)
	}

	// 3. A reviewer (now owns handles_merge) discovers the AWAITING_MERGE task by
	//    polling — the merge gate is reviewer-owned; deployer = deployment only.
	mergeReviewerID := registerAgent(t, srv.BaseURL, "poll-merge-reviewer", []string{"reviewer"})
	mTask, _ := pollNextTask(t, srv.BaseURL, mergeReviewerID)
	if mTask == nil || mTask.ID != taskID {
		t.Fatalf("reviewer poll should claim the AWAITING_MERGE task, got %v", mTask)
	}

	// The merge-review approves the PR → merge → COMPLETED.
	prID := firstPRID(t, srv, taskID)
	if st := apiJSON(t, "POST", srv.BaseURL,
		"/api/tasks/"+taskID+"/pull-requests/"+prID+"/approve",
		map[string]interface{}{"decider_id": mergeReviewerID, "body": "ship it"}, nil); st != http.StatusOK {
		t.Fatalf("PR approve: expected 200, got %d", st)
	}

	var final db.Task
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID, nil, &final)
	if final.Status != db.TaskStatusCompleted {
		t.Fatalf("final status = %q, want COMPLETED", final.Status)
	}
}
