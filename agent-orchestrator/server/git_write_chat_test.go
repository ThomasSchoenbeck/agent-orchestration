package server_test

import (
	"net/http"
	"testing"
)

// handleProjectInitRepo + handleProjectFiles (go-git, in-process against the seeded repo).
func TestProjectInitRepoAndFiles(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	base := "/api/projects/" + pid

	if w := do(t, srv, http.MethodPost, base+"/init-repo", nil); w.Code != http.StatusOK {
		t.Errorf("init-repo: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodGet, base+"/init-repo", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET init-repo: expected 405, got %d", w.Code)
	}

	// Commit files to main.
	w := do(t, srv, http.MethodPost, base+"/files", map[string]interface{}{
		"branch": "main", "message": "add a file", "author_name": "T", "author_email": "t@x",
		"files": []map[string]interface{}{{"path": "hello.txt", "content": "hi"}},
	})
	if w.Code != http.StatusOK {
		t.Errorf("commit files: %d %s", w.Code, w.Body.String())
	}
	// Missing required fields → 400.
	if w := do(t, srv, http.MethodPost, base+"/files", map[string]interface{}{"branch": "main"}); w.Code != http.StatusBadRequest {
		t.Errorf("commit files without files: expected 400, got %d", w.Code)
	}
}

// handleProviderTest: a provider pointing at a dead URL returns ok:false (not an error).
func TestProviderTest(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/providers", map[string]interface{}{
		"name": "dead", "type": "openai_compatible", "base_url": "http://127.0.0.1:1/v1", "enabled": true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create provider: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	w = do(t, srv, http.MethodPost, "/api/providers/"+id+"/test", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("provider test: %d %s", w.Code, w.Body.String())
	}
	if ok, _ := decodeMap(t, w.Body.Bytes())["ok"].(bool); ok {
		t.Error("provider test against a dead URL should report ok=false")
	}
	// Test on a missing provider → 404.
	if w := do(t, srv, http.MethodPost, "/api/providers/missing/test", nil); w.Code != http.StatusNotFound {
		t.Errorf("test missing provider: expected 404, got %d", w.Code)
	}
}

// handleProjectChat / handleTaskChat early validation (the LLM call itself is integration).
func TestChatHandlersValidation(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)

	// Project chat: method, missing message, unknown project.
	if w := do(t, srv, http.MethodGet, "/api/projects/"+pid+"/chat", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET project chat: expected 405, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, "/api/projects/"+pid+"/chat", map[string]interface{}{"message": ""}); w.Code != http.StatusBadRequest {
		t.Errorf("project chat without message: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, "/api/projects/missing/chat", map[string]interface{}{"message": "hi"}); w.Code != http.StatusNotFound {
		t.Errorf("project chat unknown project: expected 404, got %d", w.Code)
	}

	// Task chat: missing message, unknown task.
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/chat", map[string]interface{}{"message": ""}); w.Code != http.StatusBadRequest {
		t.Errorf("task chat without message: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, "/api/tasks/missing/chat", map[string]interface{}{"message": "hi"}); w.Code != http.StatusNotFound {
		t.Errorf("task chat unknown task: expected 404, got %d", w.Code)
	}
}
