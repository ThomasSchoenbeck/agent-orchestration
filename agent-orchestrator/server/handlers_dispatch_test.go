package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// handleProjects collection: list, validation, method.
func TestProjectsCollection(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/projects", nil); w.Code != http.StatusOK {
		t.Errorf("list projects: %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, "/api/projects", map[string]interface{}{}); w.Code != http.StatusBadRequest {
		t.Errorf("create project without name: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPut, "/api/projects", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT projects: expected 405, got %d", w.Code)
	}
}

// handleProjectDetail bare branches: GET / GET-404 / tasks / PUT / DELETE.
func TestProjectDetailBare(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)

	if w := do(t, srv, http.MethodGet, "/api/projects/"+pid, nil); w.Code != http.StatusOK {
		t.Errorf("get project: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/projects/missing", nil); w.Code != http.StatusNotFound {
		t.Errorf("get missing project: expected 404, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/projects/"+pid+"/tasks", nil); w.Code != http.StatusOK {
		t.Errorf("list project tasks: %d", w.Code)
	}
	if w := do(t, srv, http.MethodPut, "/api/projects/"+pid, map[string]interface{}{"name": "Renamed", "description": "d"}); w.Code != http.StatusOK {
		t.Errorf("update project: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodDelete, "/api/projects/"+pid, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete project: %d", w.Code)
	}
}

// handleTaskDetail bare branches + queue/unqueue/submit-for-review/transitions.
func TestTaskDetailBareAndSubroutes(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()
	pid := createTestProject(t, srv)
	t1 := mkTask(t, srv, pid)

	if w := do(t, srv, http.MethodGet, "/api/tasks/"+t1, nil); w.Code != http.StatusOK {
		t.Errorf("get task: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/tasks/missing", nil); w.Code != http.StatusNotFound {
		t.Errorf("get missing task: expected 404, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPut, "/api/tasks/"+t1, map[string]interface{}{"priority": 9}); w.Code != http.StatusOK {
		t.Errorf("update task: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodGet, "/api/tasks/"+t1+"/transitions", nil); w.Code != http.StatusOK {
		t.Errorf("task transitions: %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+t1+"/queue", nil); w.Code != http.StatusOK {
		t.Errorf("queue task: %d %s", w.Code, w.Body.String())
	}

	t2 := mkTask(t, srv, pid)
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+t2+"/unqueue", nil); w.Code != http.StatusOK {
		t.Errorf("unqueue task: %d %s", w.Code, w.Body.String())
	}

	t3 := mkTask(t, srv, pid)
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+t3+"/submit-for-review", nil); w.Code != http.StatusOK {
		t.Errorf("submit-for-review: %d %s", w.Code, w.Body.String())
	}

	// Queuing a completed task is rejected.
	completed := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusCompleted}
	if err := database.CreateTask(ctx, completed); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+completed.ID+"/queue", nil); w.Code != http.StatusBadRequest {
		t.Errorf("queue completed task: expected 400, got %d", w.Code)
	}

	if w := do(t, srv, http.MethodDelete, "/api/tasks/"+t1, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete task: %d", w.Code)
	}
}
