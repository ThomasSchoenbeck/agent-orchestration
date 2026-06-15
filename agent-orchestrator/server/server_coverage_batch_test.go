package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// --- agent_sessions.go ---

func TestAgentSessions(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)

	// GET without task_id → 400.
	if w := do(t, srv, http.MethodGet, "/api/agent-sessions", nil); w.Code != http.StatusBadRequest {
		t.Errorf("GET sessions without task_id: expected 400, got %d", w.Code)
	}
	// POST creates a session.
	if w := do(t, srv, http.MethodPost, "/api/agent-sessions", map[string]interface{}{"task_id": taskID}); w.Code != http.StatusCreated {
		t.Fatalf("create session: %d %s", w.Code, w.Body.String())
	}
	// POST without task_id → 400.
	if w := do(t, srv, http.MethodPost, "/api/agent-sessions", map[string]interface{}{}); w.Code != http.StatusBadRequest {
		t.Errorf("create session without task_id: expected 400, got %d", w.Code)
	}
	// GET by task → 200 with one session.
	if w := do(t, srv, http.MethodGet, "/api/agent-sessions?task_id="+taskID, nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list sessions: %d", w.Code)
	}
	// Wrong method → 405.
	if w := do(t, srv, http.MethodPut, "/api/agent-sessions", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT sessions: expected 405, got %d", w.Code)
	}
}

// --- reviews.go ---

func TestTaskReviews(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)
	ctx := context.Background()
	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusReviewing}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	base := "/api/tasks/" + task.ID + "/reviews"

	// POST an approval → 201 (and the task transitions to AWAITING_MERGE).
	w := do(t, srv, http.MethodPost, base, map[string]interface{}{
		"author_type": "agent", "author_role": "reviewer", "author_id": "rev1",
		"status": "approved", "body": "looks good",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create review: %d %s", w.Code, w.Body.String())
	}
	reviewID, _ := decodeMap(t, w.Body.Bytes())["id"].(string)
	if got, _ := database.GetTask(ctx, task.ID); got.Status != db.TaskStatusAwaitingMerge {
		t.Errorf("after approval task status = %q, want AWAITING_MERGE", got.Status)
	}

	// Validation: missing status / body → 400.
	if w := do(t, srv, http.MethodPost, base, map[string]interface{}{"body": "x"}); w.Code != http.StatusBadRequest {
		t.Errorf("review without status: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, base, map[string]interface{}{"status": "approved"}); w.Code != http.StatusBadRequest {
		t.Errorf("review without body: expected 400, got %d", w.Code)
	}

	// List + single fetch.
	if w := do(t, srv, http.MethodGet, base, nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list reviews: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, base+"/"+reviewID, nil); w.Code != http.StatusOK {
		t.Errorf("get single review: %d", w.Code)
	}
}

// --- checklist_handler.go: task comments review filter + delete ---

func TestTaskCommentsReviewAndDelete(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)
	ctx := context.Background()
	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusBacklog}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	review := &db.TaskReview{TaskID: task.ID, AuthorType: "agent", AuthorRole: "reviewer", Status: "changes_requested", Body: "fix"}
	if err := database.CreateTaskReview(ctx, review); err != nil {
		t.Fatalf("CreateTaskReview: %v", err)
	}
	base := "/api/tasks/" + task.ID + "/comments"

	// Comment tied to the review, with an author role.
	w := do(t, srv, http.MethodPost, base, map[string]interface{}{
		"body": "addressed", "author_type": "agent", "author_role": "worker", "review_id": review.ID,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create comment: %d %s", w.Code, w.Body.String())
	}
	commentID, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	// GET filtered by review_id returns it.
	if w := do(t, srv, http.MethodGet, base+"?review_id="+review.ID, nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list comments by review: %d", w.Code)
	}

	// DELETE then DELETE-again → 204, 404.
	if w := do(t, srv, http.MethodDelete, base+"/"+commentID, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete comment: %d", w.Code)
	}
	if w := do(t, srv, http.MethodDelete, base+"/"+commentID, nil); w.Code != http.StatusNotFound {
		t.Errorf("delete missing comment: expected 404, got %d", w.Code)
	}
}

// --- conversations_handler.go: pagination, method, PUT-404, update ---

func TestConversationsExtra(t *testing.T) {
	srv, _ := newTestServer(t)

	// Pagination query params on the list.
	if w := do(t, srv, http.MethodGet, "/api/conversations?limit=5&offset=0", nil); w.Code != http.StatusOK {
		t.Errorf("list conversations w/ pagination: %d", w.Code)
	}
	// Wrong method on the collection → 405.
	if w := do(t, srv, http.MethodPut, "/api/conversations", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT conversations: expected 405, got %d", w.Code)
	}
	// PUT on a missing conversation → 404.
	if w := do(t, srv, http.MethodPut, "/api/conversations/missing", map[string]interface{}{"title": "x"}); w.Code != http.StatusNotFound {
		t.Errorf("PUT missing conversation: expected 404, got %d", w.Code)
	}

	cw := do(t, srv, http.MethodPost, "/api/conversations", map[string]interface{}{"title": "c"})
	id, _ := decodeMap(t, cw.Body.Bytes())["id"].(string)

	// Update provider_id.
	if w := do(t, srv, http.MethodPut, "/api/conversations/"+id, map[string]interface{}{"provider_id": "p1"}); w.Code != http.StatusOK {
		t.Errorf("update conversation provider: %d", w.Code)
	}
	// Detail with message_limit + messages pagination.
	if w := do(t, srv, http.MethodGet, "/api/conversations/"+id+"?message_limit=10", nil); w.Code != http.StatusOK {
		t.Errorf("conversation detail w/ message_limit: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/conversations/"+id+"/messages?limit=5&offset=0", nil); w.Code != http.StatusOK {
		t.Errorf("messages w/ pagination: %d", w.Code)
	}
}

// --- requirements_handler.go: PATCH all fields + 404, links/deps GET ---

func TestRequirementFeaturePatchAndLinks(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)
	ctx := context.Background()

	// Requirement PATCH with every field.
	rw := do(t, srv, http.MethodPost, "/api/projects/"+pid+"/requirements", map[string]interface{}{"title": "R"})
	rid, _ := decodeMap(t, rw.Body.Bytes())["id"].(string)
	w := do(t, srv, http.MethodPatch, "/api/projects/"+pid+"/requirements/"+rid, map[string]interface{}{
		"title": "R2", "body": "details", "status": "satisfied", "position": 3,
	})
	if w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["title"] != "R2" {
		t.Errorf("PATCH requirement all fields: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPatch, "/api/projects/"+pid+"/requirements/missing", map[string]interface{}{"title": "x"}); w.Code != http.StatusNotFound {
		t.Errorf("PATCH missing requirement: expected 404, got %d", w.Code)
	}

	// Feature PATCH with every field + 404.
	fw := do(t, srv, http.MethodPost, "/api/projects/"+pid+"/features", map[string]interface{}{"title": "F"})
	fid, _ := decodeMap(t, fw.Body.Bytes())["id"].(string)
	if w := do(t, srv, http.MethodPatch, "/api/projects/"+pid+"/features/"+fid, map[string]interface{}{
		"title": "F2", "body": "d", "status": "done", "position": 2,
	}); w.Code != http.StatusOK {
		t.Errorf("PATCH feature all fields: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPatch, "/api/projects/"+pid+"/features/missing", map[string]interface{}{"title": "x"}); w.Code != http.StatusNotFound {
		t.Errorf("PATCH missing feature: expected 404, got %d", w.Code)
	}

	// Links + dependencies GET (empty lists).
	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusBacklog}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if w := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/links", nil); w.Code != http.StatusOK {
		t.Errorf("GET task links: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/dependencies", nil); w.Code != http.StatusOK {
		t.Errorf("GET task dependencies: %d", w.Code)
	}
}
