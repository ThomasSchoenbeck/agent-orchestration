package tools_test

import (
	"context"
	"path/filepath"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/tools"
)

// openPlanDB opens a temporary database and returns it with a Registry
// that has plan tools registered. It also creates and returns a test project.
func openPlanDB(t *testing.T) (*db.Database, *tools.Registry, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan_test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	reg := tools.NewRegistry()
	if err := tools.RegisterPlanTools(reg, d); err != nil {
		t.Fatalf("RegisterPlanTools: %v", err)
	}

	// Create a project so tests have a valid project_id.
	p := &db.Project{Name: "Plan Test Project"}
	if err := d.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return d, reg, p.ID
}

// callTool invokes a tool by name and returns the result map.
func callTool(t *testing.T, reg *tools.Registry, name string, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	result, err := reg.Execute(context.Background(), name, args)
	if err != nil {
		t.Fatalf("tool %q returned error: %v", name, err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result from tool %q, got %T", name, result)
	}
	return m
}

// callToolExpectError invokes a tool expecting it to return an error.
func callToolExpectError(t *testing.T, reg *tools.Registry, name string, args map[string]interface{}) error {
	t.Helper()
	_, err := reg.Execute(context.Background(), name, args)
	if err == nil {
		t.Fatalf("expected error from tool %q, got nil", name)
	}
	return err
}

// taskIDsFromResult extracts the []string task_ids from a plan_project result.
// Handlers return native Go types (not JSON-decoded), so the slice is []string.
func taskIDsFromResult(t *testing.T, result map[string]interface{}) []string {
	t.Helper()
	raw, ok := result["task_ids"]
	if !ok {
		t.Fatal("result missing 'task_ids' key")
	}
	ids, ok := raw.([]string)
	if !ok {
		t.Fatalf("expected task_ids to be []string, got %T", raw)
	}
	return ids
}

// intResult extracts an int value from a result map key.
func intResult(t *testing.T, result map[string]interface{}, key string) int {
	t.Helper()
	raw, ok := result[key]
	if !ok {
		t.Fatalf("result missing %q key", key)
	}
	switch v := raw.(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		t.Fatalf("expected %q to be int or float64, got %T", key, raw)
	}
	return 0
}

// --- plan_project ---

func TestPlanProject_Basic(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	workPackages := `[
		{"title":"Set up database","description":"Create schema","role":"worker","priority":8},
		{"title":"Implement API","description":"REST endpoints","role":"worker","priority":6}
	]`

	result := callTool(t, reg, "plan_project", map[string]interface{}{
		"project_id":    projectID,
		"architecture":  "Microservices with SQLite",
		"work_packages": workPackages,
	})

	if success, _ := result["success"].(bool); !success {
		t.Error("expected success=true")
	}
	if count := intResult(t, result, "task_count"); count != 2 {
		t.Errorf("expected task_count=2, got %d", count)
	}

	taskIDs := taskIDsFromResult(t, result)
	if len(taskIDs) != 2 {
		t.Fatalf("expected 2 task_ids, got %d", len(taskIDs))
	}

	// Verify both tasks exist in the database.
	ctx := context.Background()
	for _, taskID := range taskIDs {
		task, err := d.GetTask(ctx, taskID)
		if err != nil {
			t.Errorf("GetTask(%q): %v", taskID, err)
			continue
		}
		if task.Status != db.TaskStatusBacklog {
			t.Errorf("task %q: expected status %s, got %q", taskID, db.TaskStatusBacklog, task.Status)
		}
		if task.ProjectID != projectID {
			t.Errorf("task %q: expected project_id %q, got %q", taskID, projectID, task.ProjectID)
		}
	}
}

func TestPlanProject_SingleObject(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	// A single JSON object (not an array) should create 1 task.
	result := callTool(t, reg, "plan_project", map[string]interface{}{
		"project_id":    projectID,
		"architecture":  "Monolith",
		"work_packages": `{"title":"Bootstrap","description":"Initial setup","role":"worker","priority":5}`,
	})

	if count := intResult(t, result, "task_count"); count != 1 {
		t.Errorf("expected task_count=1, got %d", count)
	}

	taskIDs := taskIDsFromResult(t, result)
	if len(taskIDs) != 1 {
		t.Fatalf("expected 1 task_id, got %d", len(taskIDs))
	}

	task, err := d.GetTask(context.Background(), taskIDs[0])
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Role != "worker" {
		t.Errorf("expected role worker, got %q", task.Role)
	}
}

func TestPlanProject_DefaultPriorityAndRole(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	// Omit role and priority — should default to worker/5.
	result := callTool(t, reg, "plan_project", map[string]interface{}{
		"project_id":    projectID,
		"architecture":  "TBD",
		"work_packages": `[{"title":"Task","description":"Do stuff"}]`,
	})

	taskIDs := taskIDsFromResult(t, result)
	if len(taskIDs) == 0 {
		t.Fatal("expected at least one task_id")
	}
	task, err := d.GetTask(context.Background(), taskIDs[0])
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Role != "worker" {
		t.Errorf("expected default role worker, got %q", task.Role)
	}
	if task.Priority != 5 {
		t.Errorf("expected default priority 5, got %d", task.Priority)
	}
}

func TestPlanProject_InvalidProject(t *testing.T) {
	_, reg, _ := openPlanDB(t)
	callToolExpectError(t, reg, "plan_project", map[string]interface{}{
		"project_id":    "nonexistent-project-id",
		"architecture":  "N/A",
		"work_packages": `[{"title":"x","description":"y"}]`,
	})
}

func TestPlanProject_BadJSON(t *testing.T) {
	_, reg, projectID := openPlanDB(t)
	callToolExpectError(t, reg, "plan_project", map[string]interface{}{
		"project_id":    projectID,
		"architecture":  "N/A",
		"work_packages": `not valid json {{{`,
	})
}

func TestPlanProject_EmptyWorkPackages(t *testing.T) {
	_, reg, projectID := openPlanDB(t)
	callToolExpectError(t, reg, "plan_project", map[string]interface{}{
		"project_id":    projectID,
		"architecture":  "N/A",
		"work_packages": `[]`,
	})
}

func TestPlanProject_MissingProjectID(t *testing.T) {
	_, reg, _ := openPlanDB(t)
	callToolExpectError(t, reg, "plan_project", map[string]interface{}{
		"architecture":  "N/A",
		"work_packages": `[{"title":"x","description":"y"}]`,
	})
}

func TestPlanProject_ArchitectureStoredAsContext(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	callTool(t, reg, "plan_project", map[string]interface{}{
		"project_id":    projectID,
		"architecture":  "Event-driven microservices",
		"work_packages": `[{"title":"x","description":"y"}]`,
	})

	// The architecture should be saved as a context entry.
	entries, err := d.QueryContext(context.Background(), projectID, "Event-driven", 5)
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected architecture to be saved as context entry")
	}
}

// --- create_work_package ---

func TestCreateWorkPackage_Defaults(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	result := callTool(t, reg, "create_work_package", map[string]interface{}{
		"project_id":  projectID,
		"title":       "My Task",
		"description": "Do something useful",
	})

	if success, _ := result["success"].(bool); !success {
		t.Error("expected success=true")
	}
	taskID, _ := result["task_id"].(string)
	if taskID == "" {
		t.Fatal("expected non-empty task_id")
	}

	task, err := d.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Role != "worker" {
		t.Errorf("expected default role worker, got %q", task.Role)
	}
	if task.Priority != 5 {
		t.Errorf("expected default priority 5, got %d", task.Priority)
	}
	if task.Type != "implement" {
		t.Errorf("expected default type implement, got %q", task.Type)
	}
}

func TestCreateWorkPackage_Custom(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	result := callTool(t, reg, "create_work_package", map[string]interface{}{
		"project_id":  projectID,
		"title":       "Code Review",
		"description": "Review the implementation",
		"role":        "reviewer",
		"priority":    float64(8), // float64 matches intArgOpt's JSON-decode path
		"task_type":   "review",
	})

	taskID, _ := result["task_id"].(string)
	if taskID == "" {
		t.Fatal("expected non-empty task_id")
	}
	task, err := d.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Role != "reviewer" {
		t.Errorf("expected role reviewer, got %q", task.Role)
	}
	if task.Priority != 8 {
		t.Errorf("expected priority 8, got %d", task.Priority)
	}
	if task.Type != "review" {
		t.Errorf("expected type review, got %q", task.Type)
	}
}

func TestCreateWorkPackage_MissingTitle(t *testing.T) {
	_, reg, projectID := openPlanDB(t)
	callToolExpectError(t, reg, "create_work_package", map[string]interface{}{
		"project_id":  projectID,
		"description": "Missing title",
	})
}

func TestCreateWorkPackage_MissingProjectID(t *testing.T) {
	_, reg, _ := openPlanDB(t)
	callToolExpectError(t, reg, "create_work_package", map[string]interface{}{
		"title":       "Task",
		"description": "Missing project",
	})
}

func TestCreateWorkPackage_PayloadContainsTitleAndDescription(t *testing.T) {
	d, reg, projectID := openPlanDB(t)

	callTool(t, reg, "create_work_package", map[string]interface{}{
		"project_id":  projectID,
		"title":       "Implement Auth",
		"description": "JWT-based authentication",
	})

	tasks, err := d.ListTasks(context.Background(), db.TaskFilters{ProjectID: projectID})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	payload := tasks[0].Payload
	if payload["title"] != "Implement Auth" {
		t.Errorf("payload title mismatch: %v", payload["title"])
	}
	if payload["description"] != "JWT-based authentication" {
		t.Errorf("payload description mismatch: %v", payload["description"])
	}
}
