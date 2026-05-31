package integration_test

import (
	"testing"

	"agent-orchestrator/db"
)

// TestCommentsThreadUnderReview (IT-5.1) verifies comment threading via the
// existing /comments and /reviews endpoints (no unified /feed endpoint exists).
//
// Assertions:
//   - 2 comments with review_id=reviewID are returned by GET /comments?review_id=…
//   - 1 standalone comment (no review_id) is returned by GET /comments (no param)
//   - All comments are ordered by created_at ascending
//   - The standalone comment has an empty review_id
func TestCommentsThreadUnderReview(t *testing.T) {
	t.Parallel()

	srv := newGitTestServer(t)
	projectID, slug := makeProject(t, srv.BaseURL, "feed-test")
	taskID := makeTask(t, srv.BaseURL, projectID)
	workerID := registerAgent(t, srv.BaseURL, "worker-f1", []string{"worker"})
	claimTask(t, srv.BaseURL, taskID, workerID)

	_, clonePath := cloneRepo(t, srv.BaseURL+"/git/"+slug+".git")
	commitHash := commitAndPush(t, clonePath, "main.go", "package main", "task/"+taskID)

	reviewerID := registerAgent(t, srv.BaseURL, "reviewer-f1", []string{"reviewer"})
	claimTask(t, srv.BaseURL, taskID, reviewerID)

	// Post changes_requested review.
	var rev db.TaskReview
	status := apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/reviews",
		map[string]interface{}{
			"status":          "changes_requested",
			"body":            "needs work",
			"branch_head_sha": commitHash,
		}, &rev)
	if status != 201 {
		t.Fatalf("POST review: expected 201, got %d", status)
	}
	reviewID := rev.ID

	// Post two comments threaded under the review.
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/comments",
		map[string]interface{}{
			"body":      "reply 1",
			"review_id": reviewID,
		}, nil)
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/comments",
		map[string]interface{}{
			"body":      "reply 2",
			"review_id": reviewID,
		}, nil)

	// Post one standalone comment (no review_id).
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks/"+taskID+"/comments",
		map[string]interface{}{"body": "standalone"}, nil)

	// Assert: threaded comments under the review.
	var threaded []db.TaskComment
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID+"/comments?review_id="+reviewID, nil, &threaded)
	if len(threaded) != 2 {
		t.Errorf("threaded comments: expected 2, got %d", len(threaded))
	}
	for _, c := range threaded {
		if c.ReviewID != reviewID {
			t.Errorf("threaded comment review_id = %q, want %q", c.ReviewID, reviewID)
		}
	}
	if len(threaded) == 2 && !threaded[0].CreatedAt.Before(threaded[1].CreatedAt) &&
		threaded[0].CreatedAt != threaded[1].CreatedAt {
		t.Error("threaded comments not in ascending created_at order")
	}

	// Assert: standalone comments (no review_id). This includes the one user-posted
	// "standalone" comment plus any system comments posted by provisionWorkspace on
	// claim — so we check for presence of the specific comment, not an exact count.
	var standalone []db.TaskComment
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID+"/comments", nil, &standalone)

	found := false
	for _, c := range standalone {
		if c.ReviewID != "" {
			t.Errorf("standalone list contains comment with review_id %q", c.ReviewID)
		}
		if c.Body == "standalone" {
			found = true
		}
	}
	if !found {
		t.Errorf("standalone comment with body %q not found; got %d standalone comments", "standalone", len(standalone))
	}

	// Total: 1 review + 2 threaded comments (review-scoped) must be present.
	// Standalone count varies due to system comments; we verified the user comment above.
	var reviews []db.TaskReview
	apiJSON(t, "GET", srv.BaseURL, "/api/tasks/"+taskID+"/reviews", nil, &reviews)
	if len(reviews) != 1 {
		t.Errorf("reviews: expected 1, got %d", len(reviews))
	}
	if len(threaded) != 2 {
		t.Errorf("threaded: expected 2, got %d", len(threaded))
	}
}
