//go:build integration

package integration_test

import (
	"testing"

	"agent-orchestrator/db"
)

// TestReviewerChangesRequestedTransition (IT-4.1) verifies:
//   - reviewer claims AWAITING_REVIEW → REVIEWING
//   - POST changes_requested → AWAITING_REVISION
//   - the review row records the branch_head_sha from the push
func TestReviewerChangesRequestedTransition(t *testing.T) {
	t.Parallel()

	srv := newGitTestServer(t)
	projectID, slug := makeProject(t, srv.BaseURL, "review-test-1")
	taskID := makeTask(t, srv.BaseURL, projectID)
	workerID := registerAgent(t, srv.BaseURL, "worker-r1", []string{"worker"})
	claimTask(t, srv.BaseURL, taskID, workerID)

	_, clonePath := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	commitHash := commitAndPush(t, clonePath, "main.go", "package main", "task/"+taskID)

	// Reviewer claims the AWAITING_REVIEW task → REVIEWING.
	reviewerID := registerAgent(t, srv.BaseURL, "reviewer-r1", []string{"reviewer"})
	claimTask(t, srv.BaseURL, taskID, reviewerID)

	var reviewingTask db.Task
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID, nil, &reviewingTask)
	if reviewingTask.Status != db.TaskStatusReviewing {
		t.Fatalf("after reviewer claim: status = %q, want %q", reviewingTask.Status, db.TaskStatusReviewing)
	}

	// Post changes_requested review.
	var rev db.TaskReview
	status := apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/reviews",
		map[string]interface{}{
			"status":          "changes_requested",
			"body":            "fix error handling",
			"branch_head_sha": commitHash,
		}, &rev)
	if status != 201 {
		t.Fatalf("POST review: expected 201, got %d", status)
	}

	// Task must be AWAITING_REVISION.
	var task db.Task
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID, nil, &task)
	if task.Status != db.TaskStatusAwaitingRevision {
		t.Errorf("status = %q, want %q", task.Status, db.TaskStatusAwaitingRevision)
	}

	// The review row must carry the branch_head_sha.
	var reviews []db.TaskReview
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID+"/reviews", nil, &reviews)
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].BranchHeadSHA != commitHash {
		t.Errorf("review branch_head_sha = %q, want %q", reviews[0].BranchHeadSHA, commitHash)
	}
}

// TestSecondPushAfterRevisionUpdatesSHA (IT-4.2) verifies that a second push
// after revision updates branch_head_sha and transitions back to AWAITING_REVIEW.
func TestSecondPushAfterRevisionUpdatesSHA(t *testing.T) {
	t.Parallel()

	// --- Set up: full flow to AWAITING_REVISION ---
	srv := newGitTestServer(t)
	projectID, slug := makeProject(t, srv.BaseURL, "review-test-2")
	taskID := makeTask(t, srv.BaseURL, projectID)
	workerID := registerAgent(t, srv.BaseURL, "worker-r2", []string{"worker"})
	claimTask(t, srv.BaseURL, taskID, workerID)

	_, clonePath := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	firstHash := commitAndPush(t, clonePath, "main.go", "package main", "task/"+taskID)

	reviewerID := registerAgent(t, srv.BaseURL, "reviewer-r2", []string{"reviewer"})
	claimTask(t, srv.BaseURL, taskID, reviewerID)
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/reviews",
		map[string]interface{}{
			"status":          "changes_requested",
			"body":            "fix it",
			"branch_head_sha": firstHash,
		}, nil)

	// Worker re-claims AWAITING_REVISION → DEVELOPING.
	claimTask(t, srv.BaseURL, taskID, workerID)
	var devTask db.Task
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID, nil, &devTask)
	if devTask.Status != db.TaskStatusDeveloping {
		t.Fatalf("after re-claim: status = %q, want %q", devTask.Status, db.TaskStatusDeveloping)
	}

	// Second push — different content.
	secondHash := commitAndPush(t, clonePath, "main.go", "package main // v2", "task/"+taskID)
	if secondHash == firstHash {
		t.Fatal("second commit hash must differ from first")
	}

	// Task must be back to AWAITING_REVIEW.
	var task db.Task
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID, nil, &task)
	if task.Status != db.TaskStatusAwaitingReview {
		t.Errorf("status = %q, want %q", task.Status, db.TaskStatusAwaitingReview)
	}

	// branch_head_sha must reflect the second commit.
	if task.BranchHeadSHA != secondHash {
		t.Errorf("branch_head_sha = %q, want %q", task.BranchHeadSHA, secondHash)
	}
}
