package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// --- Task cost (handleTaskDetail "cost" case) ---

func TestTaskCost(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)

	if w := do(t, srv, http.MethodGet, "/api/tasks/"+taskID+"/cost", nil); w.Code != http.StatusOK {
		t.Errorf("get task cost: expected 200, got %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/cost", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST cost: expected 405, got %d", w.Code)
	}
}

// --- Pull requests: list, reject, and error paths (approve touches git → integration) ---

func TestTaskPullRequests_ListAndReject(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()
	pid := newProject(t, database)

	// A task in review, so a PR can be opened.
	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusAwaitingReview, Branch: "feature/x"}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Empty list initially.
	if w := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/pull-requests", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 0 {
		t.Errorf("empty PR list: code=%d", w.Code)
	}

	// Open a PR (moves task to AWAITING_MERGE).
	w := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/pull-requests", nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create PR: %d %s", w.Code, w.Body.String())
	}
	prID, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	// List now has one PR.
	if w := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/pull-requests", nil); len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Error("PR list should contain 1 entry")
	}

	// Reject it (no git involved): PR → rejected, task → AWAITING_REVISION.
	w = do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/pull-requests/"+prID+"/reject", map[string]interface{}{"decider_id": "human", "body": "no"})
	if w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["status"] != "rejected" {
		t.Fatalf("reject PR: %d %s", w.Code, w.Body.String())
	}
	if got, _ := database.GetTask(ctx, task.ID); got.Status != db.TaskStatusAwaitingRevision {
		t.Errorf("task status after reject = %q, want AWAITING_REVISION", got.Status)
	}

	// Unknown action → 404.
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/pull-requests/"+prID+"/bogus", nil); w.Code != http.StatusNotFound {
		t.Errorf("unknown PR action: expected 404, got %d", w.Code)
	}
}

func TestCreatePR_WrongStatusReturns409(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid) // fresh task is in BACKLOG, not in review

	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/pull-requests", nil); w.Code != http.StatusConflict {
		t.Errorf("create PR on non-review task: expected 409, got %d %s", w.Code, w.Body.String())
	}
}
