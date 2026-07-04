package server_test

import (
	"net/http"
	"testing"
)

// handleLLMChat: role routing with no providers registered → error (not 200).
func TestLLMChat_RoutingError(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/llm/chat", map[string]interface{}{
		"role":     "worker",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if w.Code < 400 {
		t.Errorf("llm chat with no provider: expected an error status, got %d %s", w.Code, w.Body.String())
	}
}

// handleProjectFile: a missing path on a real ref → 404.
func TestProjectFileNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	if w := do(t, srv, http.MethodGet, "/api/projects/"+pid+"/file?ref=main&path=nope.txt", nil); w.Code != http.StatusNotFound {
		t.Errorf("missing file: expected 404, got %d %s", w.Code, w.Body.String())
	}
}

// handleTaskChecklistIterations: cloning when the task has no items is a no-op → 200.
func TestChecklistIterationsNoItems(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/checklist/iterations", nil); w.Code != http.StatusOK {
		t.Errorf("clone iteration with no items: expected 200, got %d %s", w.Code, w.Body.String())
	}
}
