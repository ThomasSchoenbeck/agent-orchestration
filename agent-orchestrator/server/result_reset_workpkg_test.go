package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// handleTaskDetail "result": failed submission + completed-redirect-to-review.
func TestTaskResult(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()
	pid := createTestProject(t, srv)

	// FAILED submission goes straight through SubmitTaskResult.
	t1 := mkTask(t, srv, pid)
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+t1+"/result", map[string]interface{}{
		"status": db.TaskStatusFailed, "result": map[string]interface{}{"output": "broke"},
	}); w.Code != http.StatusOK {
		t.Errorf("submit failed result: %d %s", w.Code, w.Body.String())
	}

	// A code (worker) task submitting COMPLETED is redirected to review.
	dev := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusDeveloping}
	if err := database.CreateTask(ctx, dev); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+dev.ID+"/result", map[string]interface{}{
		"status": db.TaskStatusCompleted, "result": map[string]interface{}{"output": "done"},
	}); w.Code != http.StatusOK {
		t.Errorf("submit completed (worker→review): %d %s", w.Code, w.Body.String())
	}
	if got, _ := database.GetTask(ctx, dev.ID); got.Status != db.TaskStatusAwaitingReview {
		t.Errorf("worker COMPLETED should redirect to AWAITING_REVIEW, got %q", got.Status)
	}
}

// handleAgentDetail "reset".
func TestAgentReset(t *testing.T) {
	srv, _ := newTestServer(t)
	rw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{"name": "a1", "roles": []string{"worker"}})
	id, _ := decodeMap(t, rw.Body.Bytes())["agent_id"].(string)
	if w := do(t, srv, http.MethodPost, "/api/agents/"+id+"/reset", nil); w.Code != http.StatusOK {
		t.Errorf("reset agent: %d %s", w.Code, w.Body.String())
	}
}

// handleCreateWorkPackage (agent route): success + validation.
func TestCreateWorkPackage(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)
	base := "/api/agent/projects/" + pid + "/work-packages"

	if w := agentPost(t, srv, base, map[string]interface{}{
		"title": "Build API", "description": "the api", "role": "worker", "priority": 4,
	}); w.Code != http.StatusCreated {
		t.Errorf("create work package: %d %s", w.Code, w.Body.String())
	}
	if w := agentPost(t, srv, base, map[string]interface{}{"title": "no desc"}); w.Code != http.StatusBadRequest {
		t.Errorf("work package without description: expected 400, got %d", w.Code)
	}
}
