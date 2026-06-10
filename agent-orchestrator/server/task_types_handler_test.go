package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

func TestTaskTypes_CRUDAndValidation(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()

	// Missing required fields → 400.
	if w := do(t, srv, http.MethodPost, "/api/task-types", map[string]interface{}{"key": "x"}); w.Code != http.StatusBadRequest {
		t.Fatalf("create missing fields: got %d, want 400", w.Code)
	}

	// Create a default type.
	w := do(t, srv, http.MethodPost, "/api/task-types", map[string]interface{}{
		"key": "bug", "label": "Bug", "branch_template": "bug/{slug}", "is_default": true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d (%s)", w.Code, w.Body.String())
	}
	var bug db.TaskType
	_ = json.Unmarshal(w.Body.Bytes(), &bug)
	if bug.ID == "" {
		t.Fatal("expected a generated id")
	}

	// Duplicate key → 400 (unique constraint).
	if w := do(t, srv, http.MethodPost, "/api/task-types", map[string]interface{}{
		"key": "bug", "label": "Dup", "branch_template": "bug/{slug}",
	}); w.Code != http.StatusBadRequest {
		t.Errorf("duplicate key: got %d, want 400", w.Code)
	}

	// List → 1.
	lw := do(t, srv, http.MethodGet, "/api/task-types", nil)
	var list []db.TaskType
	_ = json.Unmarshal(lw.Body.Bytes(), &list)
	if lw.Code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: got %d len=%d, want 200/1", lw.Code, len(list))
	}

	// Cannot delete the default.
	if w := do(t, srv, http.MethodDelete, "/api/task-types/"+bug.ID, nil); w.Code != http.StatusBadRequest {
		t.Errorf("delete default: got %d, want 400", w.Code)
	}

	// Second, non-default type.
	w2 := do(t, srv, http.MethodPost, "/api/task-types", map[string]interface{}{
		"key": "hotfix", "label": "Hotfix", "branch_template": "hotfix/{slug}",
	})
	var hotfix db.TaskType
	_ = json.Unmarshal(w2.Body.Bytes(), &hotfix)

	// In use → cannot delete.
	pid := newProject(t, database)
	task := &db.Task{ProjectID: pid, Role: "worker", TaskTypeID: hotfix.ID, Status: db.TaskStatusBacklog}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if w := do(t, srv, http.MethodDelete, "/api/task-types/"+hotfix.ID, nil); w.Code != http.StatusBadRequest {
		t.Errorf("delete in-use: got %d, want 400", w.Code)
	}

	// Free the type, then delete succeeds.
	if err := database.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if w := do(t, srv, http.MethodDelete, "/api/task-types/"+hotfix.ID, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete unused: got %d, want 204", w.Code)
	}
}

// TestCreateTask_DefaultsTaskType verifies a created task with no explicit
// task_type_id is assigned the default type.
func TestCreateTask_DefaultsTaskType(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()

	do(t, srv, http.MethodPost, "/api/task-types", map[string]interface{}{
		"key": "normal", "label": "Normal", "branch_template": "feature/{slug}", "is_default": true,
	})
	pid := newProject(t, database)

	w := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": pid, "role": "worker",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create task: %d (%s)", w.Code, w.Body.String())
	}
	var task db.Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)

	def, err := database.GetDefaultTaskType(ctx)
	if err != nil {
		t.Fatalf("GetDefaultTaskType: %v", err)
	}
	if task.TaskTypeID != def.ID {
		t.Errorf("task TaskTypeID = %q, want default %q", task.TaskTypeID, def.ID)
	}
}
