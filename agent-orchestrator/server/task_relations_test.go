package server_test

import (
	"net/http"
	"testing"

	"agent-orchestrator/server"
)

// mkTask creates a task in the given project and returns its id.
func mkTask(t *testing.T, srv *server.Server, pid string) string {
	t.Helper()
	w := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": pid, "role": "worker", "priority": 5,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create task: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)
	if id == "" {
		t.Fatal("created task missing id")
	}
	return id
}

// --- Task dependencies (handleTaskDependencies) ---

func TestTaskDependencies(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	a := mkTask(t, srv, pid)
	b := mkTask(t, srv, pid)

	// Add a dependency a -> b.
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+a+"/dependencies", map[string]interface{}{"depends_on_id": b}); w.Code != http.StatusCreated {
		t.Fatalf("add dependency: %d %s", w.Code, w.Body.String())
	}

	// Missing depends_on_id → 400.
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+a+"/dependencies", map[string]interface{}{}); w.Code != http.StatusBadRequest {
		t.Errorf("add without depends_on_id: expected 400, got %d", w.Code)
	}

	// Self-dependency → 400.
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+a+"/dependencies", map[string]interface{}{"depends_on_id": a}); w.Code != http.StatusBadRequest {
		t.Errorf("self dependency: expected 400, got %d", w.Code)
	}

	// List shows the one dependency.
	if w := do(t, srv, http.MethodGet, "/api/tasks/"+a+"/dependencies", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list dependencies: code=%d", w.Code)
	}

	// Delete it, then deleting again → 404.
	if w := do(t, srv, http.MethodDelete, "/api/tasks/"+a+"/dependencies", map[string]interface{}{"depends_on_id": b}); w.Code != http.StatusNoContent {
		t.Errorf("delete dependency: %d", w.Code)
	}
	if w := do(t, srv, http.MethodDelete, "/api/tasks/"+a+"/dependencies", map[string]interface{}{"depends_on_id": b}); w.Code != http.StatusNotFound {
		t.Errorf("delete missing dependency: expected 404, got %d", w.Code)
	}
}

// --- Task links (handleTaskLinks) ---

func TestTaskLinks(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)

	// A requirement in the same project to link to.
	rw := do(t, srv, http.MethodPost, "/api/projects/"+pid+"/requirements", map[string]interface{}{"title": "R"})
	rid, _ := decodeMap(t, rw.Body.Bytes())["id"].(string)

	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/links", map[string]interface{}{"kind": "requirement", "target_id": rid}); w.Code != http.StatusCreated {
		t.Fatalf("add link: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodGet, "/api/tasks/"+taskID+"/links", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list links: code=%d", w.Code)
	}

	// Invalid kind → 400.
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/links", map[string]interface{}{"kind": "bogus", "target_id": rid}); w.Code != http.StatusBadRequest {
		t.Errorf("invalid kind: expected 400, got %d", w.Code)
	}
	// Unknown target → 400.
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/links", map[string]interface{}{"kind": "requirement", "target_id": "nope"}); w.Code != http.StatusBadRequest {
		t.Errorf("unknown target: expected 400, got %d", w.Code)
	}

	if w := do(t, srv, http.MethodDelete, "/api/tasks/"+taskID+"/links", map[string]interface{}{"kind": "requirement", "target_id": rid}); w.Code != http.StatusNoContent {
		t.Errorf("delete link: %d", w.Code)
	}
}

// --- Task checklist (handleTaskChecklist / Item / Iterations) ---

func TestTaskChecklist(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)
	base := "/api/tasks/" + taskID + "/checklist"

	w := do(t, srv, http.MethodPost, base, map[string]interface{}{"label": "step 1"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create checklist item: %d %s", w.Code, w.Body.String())
	}
	itemID, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	if w := do(t, srv, http.MethodPost, base, map[string]interface{}{"group_label": "g"}); w.Code != http.StatusBadRequest {
		t.Errorf("create without label: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, base, nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list checklist: code=%d", w.Code)
	}

	// Patch status.
	if w := do(t, srv, http.MethodPatch, base+"/"+itemID, map[string]interface{}{"status": "done"}); w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["status"] != "done" {
		t.Errorf("patch item: %d", w.Code)
	}
	// Patch a non-existent item → 404.
	if w := do(t, srv, http.MethodPatch, base+"/nope", map[string]interface{}{"status": "done"}); w.Code != http.StatusNotFound {
		t.Errorf("patch missing item: expected 404, got %d", w.Code)
	}

	// Clone iteration.
	if w := do(t, srv, http.MethodPost, base+"/iterations", nil); w.Code != http.StatusOK {
		t.Errorf("clone iteration: %d %s", w.Code, w.Body.String())
	}

	// Delete the item.
	if w := do(t, srv, http.MethodDelete, base+"/"+itemID, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete item: %d", w.Code)
	}
}
