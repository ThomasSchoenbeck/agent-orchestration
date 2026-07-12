package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// GET /api/tasks/{id}/memory returns {} before any memory is recorded, and the
// stored content once a memory row exists (Phase 6, T6.1).
func TestTaskMemoryEndpoint(t *testing.T) {
	srv, database := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)

	// No memory yet → empty object, not null.
	w := do(t, srv, http.MethodGet, "/api/tasks/"+taskID+"/memory", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("empty memory GET: %d %s", w.Code, w.Body.String())
	}
	if m := decodeMap(t, w.Body.Bytes()); m["content"] != nil {
		t.Errorf("expected empty object, got %v", m)
	}

	// Seed a memory row and read it back.
	if err := database.UpsertTaskMemory(context.Background(), &db.TaskMemory{
		TaskID:  taskID,
		Content: db.TaskMemoryContent{Summary: "did X", Progress: []string{"step 1"}},
	}); err != nil {
		t.Fatalf("UpsertTaskMemory: %v", err)
	}

	w = do(t, srv, http.MethodGet, "/api/tasks/"+taskID+"/memory", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("memory GET: %d %s", w.Code, w.Body.String())
	}
	m := decodeMap(t, w.Body.Bytes())
	content, _ := m["content"].(map[string]interface{})
	if content == nil || content["summary"] != "did X" {
		t.Errorf("expected content.summary 'did X', got %v", m["content"])
	}
}

// Non-GET methods on the memory endpoint are rejected.
func TestTaskMemoryEndpoint_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/memory", map[string]interface{}{}); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST memory: expected 405, got %d", w.Code)
	}
}
