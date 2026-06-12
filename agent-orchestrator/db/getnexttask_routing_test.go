package db_test

// Feature 3 / Task 9: capability-driven GetNextTask routing, id-based.
//   - AWAITING_REVIEW is claimable by an agent carrying handles_review when the
//     task's review_role is unset ("any reviewer") OR matches one of the agent's
//     review roles (id/name-expanded). (B2/B4)
//   - AWAITING_MERGE is claimable by any agent role carrying handles_merge.
//   - CreateTask leaves an unset review_role empty (no auto default reviewer).

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

// seedRole creates a role definition and returns it (with its assigned id).
func seedRole(t *testing.T, d *db.Database, name string, caps []string) *db.RoleDefinition {
	t.Helper()
	rd := &db.RoleDefinition{Name: name, Label: name, Capabilities: caps, Enabled: true}
	if err := d.CreateRoleDefinition(context.Background(), rd); err != nil {
		t.Fatalf("seed role %q: %v", name, err)
	}
	return rd
}

func TestGetNextTask_CapabilityReviewRouting(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	reviewer := seedRole(t, d, "reviewer", []string{"handles_review"})

	task := &db.Task{
		ProjectID:  "p1",
		Role:       "worker",
		ReviewRole: reviewer.ID, // review pinned by role id
		Status:     db.TaskStatusAwaitingReview,
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// A reviewer (has handles_review, id matches review_role) claims it.
	got, err := d.GetNextTask(ctx, []string{reviewer.ID})
	if err != nil {
		t.Fatalf("GetNextTask reviewer: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("reviewer should claim the AWAITING_REVIEW task, got %v", got)
	}

	// An agent without handles_review does not claim it.
	none, err := d.GetNextTask(ctx, []string{"some-other-role"})
	if err != nil {
		t.Fatalf("GetNextTask other: %v", err)
	}
	if none != nil {
		t.Errorf("non-reviewer must not claim an AWAITING_REVIEW task, got %v", none)
	}
}

// TestGetNextTask_AssignedAwaitingReviewHiddenUntilCleared reproduces the
// production bug: an AWAITING_REVIEW task that still carries the worker's
// assignment is invisible to a polling reviewer (the poll query excludes
// assigned tasks). Transitioning into a queue state must clear the assignment.
func TestGetNextTask_AssignedAwaitingReviewHiddenUntilCleared(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	reviewer := seedRole(t, d, "reviewer", []string{"handles_review"})

	// AWAITING_REVIEW but still assigned to the worker → not pollable.
	task := &db.Task{
		ProjectID: "p1", Role: "worker", ReviewRole: reviewer.ID,
		Status: db.TaskStatusAwaitingReview, AssignedAgentID: "worker-1",
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if got, err := d.GetNextTask(ctx, []string{reviewer.ID}); err != nil {
		t.Fatalf("GetNextTask: %v", err)
	} else if got != nil {
		t.Fatalf("reviewer must NOT poll an AWAITING_REVIEW task still assigned to the worker, got %v", got)
	}

	// Simulate the real path: worker in DEVELOPING, then the git hook transitions
	// to AWAITING_REVIEW — which must release the worker's claim.
	task.Status = db.TaskStatusDeveloping
	task.AssignedAgentID = "worker-1"
	if err := d.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if err := d.TransitionTaskState(ctx, task.ID,
		db.TaskStatusDeveloping, db.TaskStatusAwaitingReview, "", "branch pushed"); err != nil {
		t.Fatalf("TransitionTaskState: %v", err)
	}
	if got, _ := d.GetTask(ctx, task.ID); got.AssignedAgentID != "" {
		t.Errorf("assigned_agent_id = %q, want cleared after entering AWAITING_REVIEW", got.AssignedAgentID)
	}
	got, err := d.GetNextTask(ctx, []string{reviewer.ID})
	if err != nil {
		t.Fatalf("GetNextTask after transition: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("reviewer should poll the task once the assignment is cleared, got %v", got)
	}
}

func TestGetNextTask_ReviewRoleWithoutCapability(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	worker := seedRole(t, d, "worker", nil) // exists but lacks handles_review

	task := &db.Task{
		ProjectID:  "p1",
		Role:       "worker",
		ReviewRole: worker.ID, // review pinned to a role without the capability
		Status:     db.TaskStatusAwaitingReview,
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	none, err := d.GetNextTask(ctx, []string{worker.ID})
	if err != nil {
		t.Fatalf("GetNextTask: %v", err)
	}
	if none != nil {
		t.Errorf("AWAITING_REVIEW must not be claimable when review_role lacks handles_review, got %v", none)
	}
}

func TestGetNextTask_MergeRoutingByCapability(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	deployer := seedRole(t, d, "deployer", []string{"handles_merge"})

	task := &db.Task{
		ProjectID: "p1",
		Role:      "worker",
		Status:    db.TaskStatusAwaitingMerge,
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Deployer (handles_merge) claims the AWAITING_MERGE task.
	got, err := d.GetNextTask(ctx, []string{deployer.ID})
	if err != nil {
		t.Fatalf("GetNextTask deployer: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("deployer should claim the AWAITING_MERGE task, got %v", got)
	}

	// An agent without handles_merge does not.
	none, err := d.GetNextTask(ctx, []string{"some-other-role"})
	if err != nil {
		t.Fatalf("GetNextTask other: %v", err)
	}
	if none != nil {
		t.Errorf("agent without handles_merge must not claim AWAITING_MERGE, got %v", none)
	}
}

// TestCreateTask_EmptyReviewRoleStaysEmpty: the auto default reviewer was removed
// (B2/B4). An unset review_role is left empty — meaning "any review-capable
// agent" — rather than being resolved to a specific reviewer at creation.
func TestCreateTask_EmptyReviewRoleStaysEmpty(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	seedRole(t, d, "reviewer", []string{"handles_review"})

	task := &db.Task{ProjectID: "p1", Role: "worker"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ReviewRole != "" {
		t.Errorf("ReviewRole = %q, want empty (any reviewer)", task.ReviewRole)
	}
}

// TestGetNextTask_EmptyReviewRoleClaimableByAnyReviewer: an AWAITING_REVIEW task
// with an unset review_role is claimable by any handles_review agent, and not by
// a non-review agent (B2/B4 "any reviewer" default).
func TestGetNextTask_EmptyReviewRoleClaimableByAnyReviewer(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	reviewer := seedRole(t, d, "reviewer", []string{"handles_review"})
	worker := seedRole(t, d, "worker", nil)

	task := &db.Task{ProjectID: "p1", Role: worker.ID, ReviewRole: "", Status: db.TaskStatusAwaitingReview}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	got, err := d.GetNextTask(ctx, []string{reviewer.ID})
	if err != nil {
		t.Fatalf("GetNextTask reviewer: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("any reviewer should claim an empty-review_role task, got %v", got)
	}

	none, err := d.GetNextTask(ctx, []string{worker.ID})
	if err != nil {
		t.Fatalf("GetNextTask worker: %v", err)
	}
	if none != nil {
		t.Errorf("non-review agent must not claim an AWAITING_REVIEW task, got %v", none)
	}
}

// TestGetNextTask_ReviewRoleNameMatchesIDReviewer: a review_role stored as a NAME
// (legacy/pre-migration) is still claimable by a reviewer agent whose roles are
// ids, via id/name expansion of the review clause (B2 — the bug fix).
func TestGetNextTask_ReviewRoleNameMatchesIDReviewer(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	reviewer := seedRole(t, d, "reviewer", []string{"handles_review"})

	task := &db.Task{ProjectID: "p1", Role: "worker", ReviewRole: "reviewer", Status: db.TaskStatusAwaitingReview}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	got, err := d.GetNextTask(ctx, []string{reviewer.ID})
	if err != nil {
		t.Fatalf("GetNextTask: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("reviewer (id) should claim a task whose review_role is the name form, got %v", got)
	}
}
