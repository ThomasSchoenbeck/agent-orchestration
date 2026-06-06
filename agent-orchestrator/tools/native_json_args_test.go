package tools_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

// Smaller models (e.g. gemma) emit structured JSON for array/object parameters
// instead of a JSON-encoded string. These tests assert the planning tools accept
// native arrays/objects, not just stringified JSON.

func TestPlanProject_AcceptsNativeWorkPackages(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	result := callTool(t, reg, "plan_project", map[string]interface{}{
		"project_id":   projectID,
		"architecture": "files per topic",
		"work_packages": []map[string]interface{}{
			{"title": "Business jokes", "description": "20 jokes", "role": "worker", "priority": 5},
			{"title": "Finance jokes", "description": "20 jokes", "role": "worker", "priority": 5},
		},
	})
	if n := len(taskIDsFromResult(t, result)); n != 2 {
		t.Fatalf("expected 2 tasks created, got %d", n)
	}
	tasks, _ := d.ListTasks(context.Background(), db.TaskFilters{ProjectID: projectID})
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks in db, got %d", len(tasks))
	}
}

func TestPlanProject_AcceptsOverEscapedWorkPackages(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	// Some models stringify the array but escape the inner quotes a second
	// time, producing `[{\"title\":...}]` — invalid JSON at the top level.
	overEscaped := `[{\"title\": \"Identify topics\", \"description\": \"list them\", \"role\": \"worker\", \"priority\": 5}]`

	result := callTool(t, reg, "plan_project", map[string]interface{}{
		"project_id":    projectID,
		"architecture":  "files per topic",
		"work_packages": overEscaped,
	})
	if n := len(taskIDsFromResult(t, result)); n != 1 {
		t.Fatalf("expected 1 task created, got %d", n)
	}
	tasks, _ := d.ListTasks(context.Background(), db.TaskFilters{ProjectID: projectID})
	if len(tasks) != 1 || tasks[0].Role != "worker" {
		t.Errorf("expected 1 worker task, got %+v", tasks)
	}
}

func TestSyncScope_AcceptsNativeArrays(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	result := callTool(t, reg, "sync_scope", map[string]interface{}{
		"project_id": projectID,
		"requirements": []map[string]interface{}{
			{"title": "15 topics", "body": "fifteen joke topics"},
		},
		"features": []map[string]interface{}{
			{"title": "Business Jokes File", "body": "a file of business jokes"},
			{"title": "Finance Jokes File", "body": "a file of finance jokes"},
		},
	})
	if sliceLen(t, result, "added") != 3 {
		t.Errorf("expected 3 added, got %d", sliceLen(t, result, "added"))
	}
	reqs, _ := d.ListRequirements(ctx, projectID)
	feats, _ := d.ListFeatures(ctx, projectID)
	if len(reqs) != 1 || len(feats) != 2 {
		t.Errorf("expected 1 requirement + 2 features, got %d/%d", len(reqs), len(feats))
	}
}

func TestBootstrapProject_AcceptsNativeArrays(t *testing.T) {
	d, reg, projectID := openPlanDB(t)
	ctx := context.Background()

	callTool(t, reg, "bootstrap_project", map[string]interface{}{
		"project_id": projectID,
		"requirements": []map[string]interface{}{
			{"title": "User login", "body": "Authenticated access"},
		},
		"features": []map[string]interface{}{
			{"title": "Reports", "body": "Expense reports"},
		},
	})
	reqs, _ := d.ListRequirements(ctx, projectID)
	feats, _ := d.ListFeatures(ctx, projectID)
	if len(reqs) != 1 || len(feats) != 1 {
		t.Fatalf("expected 1 requirement + 1 feature, got %d/%d", len(reqs), len(feats))
	}
}
