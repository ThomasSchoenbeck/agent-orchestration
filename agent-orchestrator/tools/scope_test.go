package tools_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func sliceLen(t *testing.T, result map[string]interface{}, key string) int {
	t.Helper()
	raw, ok := result[key]
	if !ok || raw == nil {
		return 0
	}
	switch s := raw.(type) {
	case []string:
		return len(s)
	case []interface{}: // JSON-decoded from the HTTP response
		return len(s)
	default:
		t.Fatalf("expected %q to be a slice, got %T", key, raw)
		return 0
	}
}

func TestBootstrapProject_FromDescription(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	// Set a human description (the planner reads it and supplies derived items).
	p, _ := d.GetProject(ctx, projectID)
	p.Description = "A tool to track expenses with login and reports."
	if err := d.UpdateProject(ctx, p); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	callTool(t, reg, "bootstrap_project", map[string]interface{}{
		"project_id":   projectID,
		"requirements": `[{"title":"User login","body":"Authenticated access"}]`,
		"features":     `[{"title":"Reports","body":"Expense reports"}]`,
	})

	reqs, _ := d.ListRequirements(ctx, projectID)
	feats, _ := d.ListFeatures(ctx, projectID)
	if len(reqs) != 1 || len(feats) != 1 {
		t.Fatalf("expected 1 requirement + 1 feature, got %d/%d", len(reqs), len(feats))
	}
}

func TestBootstrapProject_SkipsWhenScopeExists(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	if err := d.CreateRequirement(ctx, &db.ProjectRequirement{ProjectID: projectID, Title: "Existing"}); err != nil {
		t.Fatalf("CreateRequirement: %v", err)
	}

	result := callTool(t, reg, "bootstrap_project", map[string]interface{}{
		"project_id":   projectID,
		"requirements": `[{"title":"New one","body":"x"}]`,
	})
	if skipped, _ := result["skipped"].(bool); !skipped {
		t.Error("expected skipped=true when scope already exists")
	}
	reqs, _ := d.ListRequirements(ctx, projectID)
	if len(reqs) != 1 {
		t.Errorf("bootstrap must not add to existing scope; got %d requirements", len(reqs))
	}
}

func TestSyncScope_AddsNewWithoutTouchingExisting(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	existing := &db.ProjectRequirement{ProjectID: projectID, Title: "Alpha", Status: db.ReqStatusAccepted}
	if err := d.CreateRequirement(ctx, existing); err != nil {
		t.Fatalf("CreateRequirement: %v", err)
	}

	result := callTool(t, reg, "sync_scope", map[string]interface{}{
		"project_id":   projectID,
		"requirements": `[{"title":"Alpha","body":"unchanged"},{"title":"Beta","body":"new"}]`,
	})
	if sliceLen(t, result, "added") != 1 {
		t.Errorf("expected 1 added, got %d", sliceLen(t, result, "added"))
	}

	reqs, _ := d.ListRequirements(ctx, projectID)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(reqs))
	}
	// Existing item keeps its ID and status (untouched).
	got, _ := d.GetRequirement(ctx, existing.ID)
	if got.Status != db.ReqStatusAccepted {
		t.Errorf("existing requirement status changed to %q, want untouched accepted", got.Status)
	}
}

func TestSyncScope_FlagsStaleAsNeedsReview(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	stale := &db.ProjectRequirement{ProjectID: projectID, Title: "Removed intent"}
	if err := d.CreateRequirement(ctx, stale); err != nil {
		t.Fatalf("CreateRequirement: %v", err)
	}

	callTool(t, reg, "sync_scope", map[string]interface{}{
		"project_id":   projectID,
		"requirements": `[{"title":"Something else","body":"x"}]`,
	})

	got, _ := d.GetRequirement(ctx, stale.ID)
	if got == nil {
		t.Fatal("stale requirement was deleted; must be preserved")
	}
	if got.Status != db.ScopeStatusNeedsReview {
		t.Errorf("stale requirement status = %q, want needs_review", got.Status)
	}
}

func TestSyncScope_PreservesTaskLinks(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	feat := &db.ProjectFeature{ProjectID: projectID, Title: "Legacy feature"}
	if err := d.CreateFeature(ctx, feat); err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	task := &db.Task{ProjectID: projectID, Role: "worker", Status: db.TaskStatusBacklog}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := d.AddTaskLink(ctx, task.ID, "feature", feat.ID); err != nil {
		t.Fatalf("AddTaskLink: %v", err)
	}

	// Sync with a desired set that does not include the feature → flag, not delete.
	callTool(t, reg, "sync_scope", map[string]interface{}{
		"project_id": projectID,
		"features":   `[{"title":"Brand new","body":"x"}]`,
	})

	links, _ := d.ListTaskLinks(ctx, task.ID)
	found := false
	for _, l := range links {
		if l.Kind == "feature" && l.TargetID == feat.ID {
			found = true
		}
	}
	if !found {
		t.Error("task link to flagged feature was lost; links must be preserved")
	}
}

func TestSyncScope_ClearsScopeDirty(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	if err := d.SetScopeDirty(ctx, projectID, true); err != nil {
		t.Fatalf("SetScopeDirty: %v", err)
	}
	callTool(t, reg, "sync_scope", map[string]interface{}{
		"project_id":   projectID,
		"requirements": `[{"title":"R","body":"x"}]`,
	})
	got, _ := d.GetProject(ctx, projectID)
	if got.ScopeDirty {
		t.Error("sync_scope should clear scope_dirty")
	}
}

func TestCompleteProject_RequiresScopeSatisfied(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	feat := &db.ProjectFeature{ProjectID: projectID, Title: "Core", Status: db.FeatStatusPlanned}
	if err := d.CreateFeature(ctx, feat); err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	// Not satisfied yet (feature is planned).
	callToolExpectError(t, reg, "complete_project", map[string]interface{}{"project_id": projectID}, "scope not satisfied")

	feat.Status = db.FeatStatusDone
	if err := d.UpdateFeature(ctx, feat); err != nil {
		t.Fatalf("UpdateFeature: %v", err)
	}
	result := callTool(t, reg, "complete_project", map[string]interface{}{"project_id": projectID})
	if success, _ := result["success"].(bool); !success {
		t.Error("expected complete_project success once scope satisfied")
	}
	got, _ := d.GetProject(ctx, projectID)
	if got.Status != "complete" {
		t.Errorf("project status = %q, want complete", got.Status)
	}
}
