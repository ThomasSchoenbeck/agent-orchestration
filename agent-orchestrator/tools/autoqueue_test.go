package tools_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/tools"
)

// satisfyScope gives a project a single done feature so its scope is satisfied.
func satisfyScope(t *testing.T, d *db.Database, projectID string) {
	t.Helper()
	if err := d.CreateFeature(context.Background(), &db.ProjectFeature{
		ProjectID: projectID, Title: "Done feature", Status: db.FeatStatusDone,
	}); err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
}

func TestCompleteProject_DisarmsAutoQueue(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	// Arm the project, then satisfy scope.
	p, _ := d.GetProject(ctx, projectID)
	p.AutoQueue = true
	p.Status = "active"
	if err := d.UpdateProject(ctx, p); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	satisfyScope(t, d, projectID)

	callTool(t, reg, "complete_project", map[string]interface{}{
		"project_id": projectID,
		"summary":    "All features delivered",
	})

	got, _ := d.GetProject(ctx, projectID)
	if got.Status != "complete" {
		t.Errorf("status = %q, want complete", got.Status)
	}
	if got.AutoQueue {
		t.Error("auto_queue should be disarmed after complete_project")
	}
}

func TestCompleteProject_RejectedWithOpenTasks(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()
	satisfyScope(t, d, projectID)

	// An open task means the project is not actually done.
	if err := d.CreateTask(ctx, &db.Task{ProjectID: projectID, Role: "worker", Status: db.TaskStatusDeveloping}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	callToolExpectError(t, reg, "complete_project", map[string]interface{}{"project_id": projectID}, "non-terminal")
}

func TestCompleteProject_RequiresCreatesTasks(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()
	satisfyScope(t, d, projectID)

	// A caller without creates_tasks is rejected even when scope is satisfied.
	noCreate := tools.WithCapabilities(ctx, []string{"handles_review"})
	if _, err := reg.Execute(noCreate, "complete_project", map[string]interface{}{"project_id": projectID}); err == nil {
		t.Fatal("expected error for caller without creates_tasks")
	}

	// With the capability it succeeds.
	withCreate := tools.WithCapabilities(ctx, []string{"creates_tasks"})
	if _, err := reg.Execute(withCreate, "complete_project", map[string]interface{}{"project_id": projectID}); err != nil {
		t.Fatalf("expected success with creates_tasks, got %v", err)
	}
	got, _ := d.GetProject(ctx, projectID)
	if got.Status != "complete" {
		t.Errorf("status = %q, want complete", got.Status)
	}
}
