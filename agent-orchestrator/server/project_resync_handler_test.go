package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

func TestProjectResync_CreatesOrchestratorTaskWithDefaultPrompt(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)

	w := do(t, srv, http.MethodPost, "/api/projects/"+pid+"/resync", nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("resync: expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	tasks, err := database.ListTasks(context.Background(), db.TaskFilters{ProjectID: pid})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.Role != "orchestrator" {
		t.Errorf("role = %q, want orchestrator", task.Role)
	}
	if task.Priority != 8 {
		t.Errorf("priority = %d, want 8", task.Priority)
	}
	if got, _ := task.Payload["mode"].(string); got != "sync" {
		t.Errorf("payload.mode = %q, want sync", got)
	}
	if got, _ := task.Payload["description"].(string); got != db.DefaultResyncPrompt {
		t.Errorf("payload.description = %q, want default prompt", got)
	}
}

func TestProjectResync_UsesRoleResyncPrompt(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)

	const custom = "custom re-sync instructions"
	if err := database.CreateRoleDefinition(context.Background(), &db.RoleDefinition{
		Name: "orchestrator", Label: "Orchestrator", ResyncPrompt: custom, Enabled: true,
	}); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	w := do(t, srv, http.MethodPost, "/api/projects/"+pid+"/resync", nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("resync: expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	tasks, _ := database.ListTasks(context.Background(), db.TaskFilters{ProjectID: pid})
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if got, _ := tasks[0].Payload["description"].(string); got != custom {
		t.Errorf("payload.description = %q, want %q", got, custom)
	}
}

func TestProjectResync_UnknownProject404(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/projects/does-not-exist/resync", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown project, got %d", w.Code)
	}
}
