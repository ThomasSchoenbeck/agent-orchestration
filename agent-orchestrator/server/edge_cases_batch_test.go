package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// handleAgentDetail "tasks/next": no-task and matched-task (claim) paths.
func TestAgentTasksNext(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()
	pid := createTestProject(t, srv)

	rw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{"name": "w", "roles": []string{"worker"}})
	id, _ := decodeMap(t, rw.Body.Bytes())["agent_id"].(string)

	// No matching task → 200 (nil body).
	if w := do(t, srv, http.MethodGet, "/api/agents/"+id+"/tasks/next", nil); w.Code != http.StatusOK {
		t.Errorf("tasks/next (no task): %d", w.Code)
	}

	// A BACKLOG worker task is found and claimed.
	if err := database.CreateTask(ctx, &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusBacklog}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if w := do(t, srv, http.MethodGet, "/api/agents/"+id+"/tasks/next", nil); w.Code != http.StatusOK {
		t.Errorf("tasks/next (match): %d %s", w.Code, w.Body.String())
	}
}

// handleProjectDiff: file-specific diff (path param) against the seeded repo.
func TestProjectDiffFile(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	if w := do(t, srv, http.MethodGet, "/api/projects/"+pid+"/diff?base=main&head=main&path=.gitkeep", nil); w.Code != http.StatusOK {
		t.Errorf("file diff: %d %s", w.Code, w.Body.String())
	}
}

// handleBootstrapProject: when the project already has scope, bootstrap skips (200).
func TestBootstrapSkip(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)
	if err := database.CreateRequirement(context.Background(), &db.ProjectRequirement{ProjectID: pid, Title: "existing"}); err != nil {
		t.Fatalf("CreateRequirement: %v", err)
	}
	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/bootstrap", map[string]interface{}{
		"requirements": []map[string]interface{}{{"title": "new", "body": "b"}},
	})
	if w.Code != http.StatusOK {
		t.Errorf("bootstrap skip: %d %s", w.Code, w.Body.String())
	}
}

// handleConversationMessages POST validation: role and content are both required.
func TestConversationMessageValidation(t *testing.T) {
	srv, _ := newTestServer(t)
	cw := do(t, srv, http.MethodPost, "/api/conversations", map[string]interface{}{"title": "c"})
	id, _ := decodeMap(t, cw.Body.Bytes())["id"].(string)
	base := "/api/conversations/" + id + "/messages"

	if w := do(t, srv, http.MethodPost, base, map[string]interface{}{"role": "user"}); w.Code != http.StatusBadRequest {
		t.Errorf("message without content: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, base, map[string]interface{}{"content": "hi"}); w.Code != http.StatusBadRequest {
		t.Errorf("message without role: expected 400, got %d", w.Code)
	}
}
