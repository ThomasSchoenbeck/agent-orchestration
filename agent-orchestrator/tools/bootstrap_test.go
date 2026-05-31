package tools_test

import (
	"context"
	"testing"
)

func TestBootstrapProject_WritesRequirementsAndFeatures(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	result := callTool(t, reg, "bootstrap_project", map[string]interface{}{
		"project_id": projectID,
		"requirements": `[
			{"title":"Must authenticate users","body":"JWT-based login"},
			{"title":"Audit logging","body":"Record all writes"}
		]`,
		"features": `[{"title":"Dashboard","body":"Overview page"}]`,
	})

	if success, _ := result["success"].(bool); !success {
		t.Error("expected success=true")
	}
	if c := intResult(t, result, "requirement_count"); c != 2 {
		t.Errorf("requirement_count = %d, want 2", c)
	}
	if c := intResult(t, result, "feature_count"); c != 1 {
		t.Errorf("feature_count = %d, want 1", c)
	}

	ctx := context.Background()
	reqs, err := d.ListRequirements(ctx, projectID)
	if err != nil {
		t.Fatalf("ListRequirements: %v", err)
	}
	if len(reqs) != 2 {
		t.Errorf("expected 2 requirements persisted, got %d", len(reqs))
	}
	feats, err := d.ListFeatures(ctx, projectID)
	if err != nil {
		t.Fatalf("ListFeatures: %v", err)
	}
	if len(feats) != 1 {
		t.Errorf("expected 1 feature persisted, got %d", len(feats))
	}
}

func TestBootstrapProject_RequirementsOnly(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	callTool(t, reg, "bootstrap_project", map[string]interface{}{
		"project_id":   projectID,
		"requirements": `{"title":"Single requirement","body":"As an object, not array"}`,
	})

	reqs, err := d.ListRequirements(context.Background(), projectID)
	if err != nil {
		t.Fatalf("ListRequirements: %v", err)
	}
	if len(reqs) != 1 {
		t.Errorf("expected 1 requirement, got %d", len(reqs))
	}
}

func TestBootstrapProject_InvalidProject(t *testing.T) {
	_, reg, _ := openPlanDB(t)
	callToolExpectError(t, reg, "bootstrap_project", map[string]interface{}{
		"project_id":   "nonexistent",
		"requirements": `[{"title":"x","body":"y"}]`,
	})
}

func TestBootstrapProject_EmptyScope(t *testing.T) {
	_, reg, projectID := openPlanDB(t)
	// No requirements or features → error.
	callToolExpectError(t, reg, "bootstrap_project", map[string]interface{}{
		"project_id": projectID,
	})
}

func TestBootstrapProject_MissingProjectID(t *testing.T) {
	_, reg, _ := openPlanDB(t)
	callToolExpectError(t, reg, "bootstrap_project", map[string]interface{}{
		"requirements": `[{"title":"x","body":"y"}]`,
	})
}
