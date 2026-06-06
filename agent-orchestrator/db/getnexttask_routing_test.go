package db_test

// Feature 3 / Task 9: capability-driven GetNextTask routing, id-based.
//   - AWAITING_REVIEW is claimable only by an agent whose role id matches the
//     task's review_role AND carries handles_review.
//   - AWAITING_MERGE is claimable by any agent role carrying handles_merge.
//   - CreateTask resolves a default review_role to a role id.

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

func TestCreateTask_ResolvesDefaultReviewRole(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	reviewer := seedRole(t, d, "reviewer", []string{"handles_review"})

	task := &db.Task{ProjectID: "p1", Role: "worker"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ReviewRole != reviewer.ID {
		t.Errorf("default ReviewRole = %q, want reviewer id %q", task.ReviewRole, reviewer.ID)
	}
}

func TestCreateTask_DefaultReviewRoleFallback(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	// No review-capable role seeded → default review_role resolves to empty.
	task := &db.Task{ProjectID: "p1", Role: "worker"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ReviewRole != "" {
		t.Errorf("fallback ReviewRole = %q, want empty", task.ReviewRole)
	}
}
