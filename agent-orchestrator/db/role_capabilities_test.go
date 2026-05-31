package db_test

// Feature 3 foundation: capability flags on role definitions and the review_role
// routing key on tasks round-trip through the DB.

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestRoleDefinition_CapabilitiesRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	r := &db.RoleDefinition{
		Name:         "reviewer",
		Label:        "Reviewer",
		Capabilities: []string{"handles_review"},
		Enabled:      true,
	}
	if err := d.CreateRoleDefinition(ctx, r); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	got, err := d.GetRoleDefinitionByName(ctx, "reviewer")
	if err != nil {
		t.Fatalf("GetRoleDefinitionByName: %v", err)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0] != "handles_review" {
		t.Errorf("Capabilities = %v, want [handles_review]", got.Capabilities)
	}

	// Update adds a second capability and must round-trip.
	got.Capabilities = []string{"handles_review", "handles_merge"}
	if err := d.UpdateRoleDefinition(ctx, got); err != nil {
		t.Fatalf("UpdateRoleDefinition: %v", err)
	}
	again, err := d.GetRoleDefinitionByName(ctx, "reviewer")
	if err != nil {
		t.Fatalf("GetRoleDefinitionByName (2): %v", err)
	}
	if len(again.Capabilities) != 2 {
		t.Errorf("Capabilities after update = %v, want 2 entries", again.Capabilities)
	}
}

func TestRoleDefinition_CapabilitiesDefaultEmpty(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// A role created without capabilities round-trips as an empty (non-nil) list.
	r := &db.RoleDefinition{Name: "worker", Label: "Worker", Enabled: true}
	if err := d.CreateRoleDefinition(ctx, r); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}
	got, err := d.GetRoleDefinitionByName(ctx, "worker")
	if err != nil {
		t.Fatalf("GetRoleDefinitionByName: %v", err)
	}
	if len(got.Capabilities) != 0 {
		t.Errorf("Capabilities = %v, want empty", got.Capabilities)
	}
}

func TestTask_ReviewRoleRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	task := &db.Task{ProjectID: "p1", Type: "implement", Role: "worker", ReviewRole: "reviewer"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	got, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ReviewRole != "reviewer" {
		t.Errorf("ReviewRole = %q, want %q", got.ReviewRole, "reviewer")
	}
}
