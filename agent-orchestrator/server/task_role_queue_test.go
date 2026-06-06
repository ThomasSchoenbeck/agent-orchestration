package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// seedRole creates an enabled role definition and returns its generated id.
func seedRole(t *testing.T, database *db.Database, name string) string {
	t.Helper()
	rd := &db.RoleDefinition{Name: name, Label: name, Enabled: true}
	if err := database.CreateRoleDefinition(context.Background(), rd); err != nil {
		t.Fatalf("CreateRoleDefinition(%s): %v", name, err)
	}
	return rd.ID
}

func TestCreateWorkPackage_ResolvesRoleNameToID(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)
	workerID := seedRole(t, database, "worker")

	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/work-packages", map[string]interface{}{
		"title": "do a thing", "description": "details", "role": "worker",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	tasks, _ := database.ListTasks(context.Background(), db.TaskFilters{ProjectID: pid})
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Role != workerID {
		t.Errorf("task role = %q, want resolved id %q (agents match on id)", tasks[0].Role, workerID)
	}
}

func TestUpdateTask_ChangesRole(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)
	reviewerID := seedRole(t, database, "reviewer")

	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusBacklog}
	if err := database.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	w := do(t, srv, http.MethodPut, "/api/tasks/"+task.ID, map[string]interface{}{"role": "reviewer"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	got, _ := database.GetTask(context.Background(), task.ID)
	if got.Role != reviewerID {
		t.Errorf("task role = %q, want resolved id %q", got.Role, reviewerID)
	}
}

func TestUnqueue_SetsUnqueuedStatus(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)
	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusBacklog}
	if err := database.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	w := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/unqueue", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	got, _ := database.GetTask(context.Background(), task.ID)
	if got.Status != db.TaskStatusUnqueued {
		t.Errorf("status = %q, want %q", got.Status, db.TaskStatusUnqueued)
	}
}

func TestQueue_FromUnqueued_SetsBacklog(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)
	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusUnqueued}
	if err := database.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	w := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/queue", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	got, _ := database.GetTask(context.Background(), task.ID)
	if got.Status != db.TaskStatusBacklog {
		t.Errorf("status = %q, want %q", got.Status, db.TaskStatusBacklog)
	}
}
