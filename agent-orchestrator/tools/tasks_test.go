package tools_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/tools"
)

// openTaskToolDB returns an open DB and a Registry with task tools registered,
// plus a seeded project ID.
func openTaskToolDB(t *testing.T) (*db.Database, *tools.Registry, string) {
	t.Helper()
	d, _, projectID := openPlanDB(t)

	reg := tools.NewRegistry()
	if err := tools.RegisterTaskTools(reg, d); err != nil {
		t.Fatalf("RegisterTaskTools: %v", err)
	}
	return d, reg, projectID
}

// seedTask creates a task with the given status directly in the DB.
func seedTask(t *testing.T, d *db.Database, projectID, status string) *db.Task {
	t.Helper()
	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    status,
		Priority:  5,
		Payload:   map[string]interface{}{"title": "test task"},
	}
	if err := d.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask(%q): %v", status, err)
	}
	return task
}

// countFromResult extracts the integer "count" field from a tool result map.
// Handlers return native int, so we handle both int and float64 defensively.
func countFromResult(result map[string]interface{}) int {
	switch v := result["count"].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return -1
}

// --- list_tasks ---

func TestListTasks_Empty(t *testing.T) {
	_, reg, projectID := openTaskToolDB(t)

	result := callTool(t, reg, "list_tasks", map[string]interface{}{
		"project_id": projectID,
	})
	if n := countFromResult(result); n != 0 {
		t.Errorf("expected count=0, got %d", n)
	}
}

func TestListTasks_ReturnsAll(t *testing.T) {
	d, reg, projectID := openTaskToolDB(t)

	seedTask(t, d, projectID, "planned")
	seedTask(t, d, projectID, "in_progress")
	seedTask(t, d, projectID, "completed")

	result := callTool(t, reg, "list_tasks", map[string]interface{}{
		"project_id": projectID,
	})
	if n := countFromResult(result); n != 3 {
		t.Errorf("expected 3 tasks, got %d", n)
	}
}

func TestListTasks_FilterByStatus(t *testing.T) {
	d, reg, projectID := openTaskToolDB(t)

	seedTask(t, d, projectID, "planned")
	seedTask(t, d, projectID, "in_progress")
	seedTask(t, d, projectID, "completed")

	result := callTool(t, reg, "list_tasks", map[string]interface{}{
		"project_id": projectID,
		"status":     "planned",
	})
	if n := countFromResult(result); n != 1 {
		t.Errorf("expected 1 planned task, got %d", n)
	}
}

func TestListTasks_FilterByRole(t *testing.T) {
	d, reg, projectID := openTaskToolDB(t)

	workerTask := &db.Task{
		ProjectID: projectID, Type: "implement", Role: "worker", Status: "planned",
		Payload: map[string]interface{}{},
	}
	reviewerTask := &db.Task{
		ProjectID: projectID, Type: "review", Role: "reviewer", Status: "planned",
		Payload: map[string]interface{}{},
	}
	_ = d.CreateTask(context.Background(), workerTask)
	_ = d.CreateTask(context.Background(), reviewerTask)

	result := callTool(t, reg, "list_tasks", map[string]interface{}{
		"project_id": projectID,
		"role":       "reviewer",
	})
	if n := countFromResult(result); n != 1 {
		t.Errorf("expected 1 reviewer task, got %d", n)
	}
}

func TestListTasks_MissingProjectID(t *testing.T) {
	_, reg, _ := openTaskToolDB(t)
	callToolExpectError(t, reg, "list_tasks", map[string]interface{}{})
}

// --- submit_task_result ---

func TestSubmitTaskResult_Completed(t *testing.T) {
	d, reg, projectID := openTaskToolDB(t)
	task := seedTask(t, d, projectID, "in_progress")

	result := callTool(t, reg, "submit_task_result", map[string]interface{}{
		"task_id": task.ID,
		"status":  "completed",
		"output":  "all done",
	})

	if success, _ := result["success"].(bool); !success {
		t.Error("expected success=true")
	}

	updated, err := d.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if updated.Status != "completed" {
		t.Errorf("expected status completed, got %q", updated.Status)
	}
	if updated.Result["output"] != "all done" {
		t.Errorf("expected result.output='all done', got %v", updated.Result["output"])
	}
}

func TestSubmitTaskResult_Failed(t *testing.T) {
	d, reg, projectID := openTaskToolDB(t)
	task := seedTask(t, d, projectID, "in_progress")

	callTool(t, reg, "submit_task_result", map[string]interface{}{
		"task_id": task.ID,
		"status":  "failed",
		"output":  "partial work",
		"error":   "LLM timeout",
	})

	updated, err := d.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if updated.Status != "failed" {
		t.Errorf("expected status failed, got %q", updated.Status)
	}
	if updated.Result["error"] != "LLM timeout" {
		t.Errorf("expected result.error='LLM timeout', got %v", updated.Result["error"])
	}
}

func TestSubmitTaskResult_NeedsReview(t *testing.T) {
	d, reg, projectID := openTaskToolDB(t)
	task := seedTask(t, d, projectID, "in_progress")

	callTool(t, reg, "submit_task_result", map[string]interface{}{
		"task_id": task.ID,
		"status":  "needs_review",
		"output":  "implementation ready for review",
	})

	updated, _ := d.GetTask(context.Background(), task.ID)
	if updated.Status != "needs_review" {
		t.Errorf("expected status needs_review, got %q", updated.Status)
	}
}

func TestSubmitTaskResult_MissingTaskID(t *testing.T) {
	_, reg, _ := openTaskToolDB(t)
	callToolExpectError(t, reg, "submit_task_result", map[string]interface{}{
		"status": "completed",
		"output": "done",
	})
}

// --- get_next_task ---

func TestGetNextTask_NoTask(t *testing.T) {
	_, reg, _ := openTaskToolDB(t)

	result := callTool(t, reg, "get_next_task", map[string]interface{}{
		"roles": "worker",
	})
	if result["task"] != nil {
		t.Errorf("expected nil task, got %v", result["task"])
	}
}

func TestGetNextTask_ReturnsTask(t *testing.T) {
	d, reg, projectID := openTaskToolDB(t)
	seedTask(t, d, projectID, "planned")

	result := callTool(t, reg, "get_next_task", map[string]interface{}{
		"roles": "worker",
	})
	if result["task"] == nil {
		t.Error("expected a task, got nil")
	}
}

func TestGetNextTask_RoleNotMatched(t *testing.T) {
	d, reg, projectID := openTaskToolDB(t)
	seedTask(t, d, projectID, "planned") // role="worker"

	result := callTool(t, reg, "get_next_task", map[string]interface{}{
		"roles": "reviewer",
	})
	if result["task"] != nil {
		t.Errorf("expected nil task for non-matching role, got %v", result["task"])
	}
}

func TestGetNextTask_MultipleRoles(t *testing.T) {
	d, reg, projectID := openTaskToolDB(t)
	seedTask(t, d, projectID, "planned") // role="worker"

	result := callTool(t, reg, "get_next_task", map[string]interface{}{
		"roles": "reviewer,worker",
	})
	if result["task"] == nil {
		t.Error("expected task to be returned with multi-role query")
	}
}

func TestGetNextTask_MissingRoles(t *testing.T) {
	_, reg, _ := openTaskToolDB(t)
	callToolExpectError(t, reg, "get_next_task", map[string]interface{}{})
}
