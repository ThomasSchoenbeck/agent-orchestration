package db_test

// Feature 3 / Bug 9: capability-driven GetNextTask routing.
//   - AWAITING_REVIEW is claimable only by an agent whose role matches the
//     task's review_role AND carries handles_review.
//   - AWAITING_MERGE is claimable by any agent role carrying handles_merge.
//   - CreateTask resolves a default review_role.

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func seedRole(t *testing.T, d *db.Database, name string, caps []string) {
	t.Helper()
	if err := d.CreateRoleDefinition(context.Background(), &db.RoleDefinition{
		Name: name, Label: name, Capabilities: caps, Enabled: true,
	}); err != nil {
		t.Fatalf("seed role %q: %v", name, err)
	}
}

func TestGetNextTask_CapabilityReviewRouting(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	seedRole(t, d, "reviewer", []string{"handles_review"})

	task := &db.Task{
		ProjectID:  "p1",
		Type:       "implement",
		Role:       "worker",
		ReviewRole: "reviewer",
		Status:     db.TaskStatusAwaitingReview,
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// A reviewer (has handles_review, matches review_role) claims it.
	got, err := d.GetNextTask(ctx, []string{"reviewer"})
	if err != nil {
		t.Fatalf("GetNextTask reviewer: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("reviewer should claim the AWAITING_REVIEW task, got %v", got)
	}

	// A worker (no handles_review) does not claim it.
	none, err := d.GetNextTask(ctx, []string{"worker"})
	if err != nil {
		t.Fatalf("GetNextTask worker: %v", err)
	}
	if none != nil {
		t.Errorf("worker must not claim an AWAITING_REVIEW task, got %v", none)
	}
}

func TestGetNextTask_ReviewRoleWithoutCapability(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	// worker exists but lacks handles_review.
	seedRole(t, d, "worker", nil)

	task := &db.Task{
		ProjectID:  "p1",
		Type:       "implement",
		Role:       "worker",
		ReviewRole: "worker", // review pinned to a role without the capability
		Status:     db.TaskStatusAwaitingReview,
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	none, err := d.GetNextTask(ctx, []string{"worker"})
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
	seedRole(t, d, "deployer", []string{"handles_merge"})

	task := &db.Task{
		ProjectID: "p1",
		Type:      "implement",
		Role:      "worker",
		Status:    db.TaskStatusAwaitingMerge,
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Deployer (handles_merge) claims the AWAITING_MERGE task.
	got, err := d.GetNextTask(ctx, []string{"deployer"})
	if err != nil {
		t.Fatalf("GetNextTask deployer: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("deployer should claim the AWAITING_MERGE task, got %v", got)
	}

	// An agent without handles_merge does not.
	none, err := d.GetNextTask(ctx, []string{"worker"})
	if err != nil {
		t.Fatalf("GetNextTask worker: %v", err)
	}
	if none != nil {
		t.Errorf("agent without handles_merge must not claim AWAITING_MERGE, got %v", none)
	}
}

func TestCreateTask_ResolvesDefaultReviewRole(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	seedRole(t, d, "reviewer", []string{"handles_review"})

	task := &db.Task{ProjectID: "p1", Type: "implement", Role: "worker"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ReviewRole != "reviewer" {
		t.Errorf("default ReviewRole = %q, want %q", task.ReviewRole, "reviewer")
	}
}

func TestCreateTask_DefaultReviewRoleFallback(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	// No roles seeded → fallback to the literal "reviewer".
	task := &db.Task{ProjectID: "p1", Type: "implement", Role: "worker"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ReviewRole != "reviewer" {
		t.Errorf("fallback ReviewRole = %q, want %q", task.ReviewRole, "reviewer")
	}
}
