package server_test

import (
	"net/http"
	"testing"
)

// With a valid message but no LLM provider configured, the chat handlers run
// their full body (load history, build context + system prompt, assemble
// messages) and then fail at provider routing with 503 — no live LLM needed.
func TestProjectChat_NoProvider(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	w := do(t, srv, http.MethodPost, "/api/projects/"+pid+"/chat", map[string]interface{}{"message": "hello"})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("project chat without provider: expected 503, got %d %s", w.Code, w.Body.String())
	}
}

func TestTaskChat_NoProvider(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)
	w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/chat", map[string]interface{}{"message": "hello"})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("task chat without provider: expected 503, got %d %s", w.Code, w.Body.String())
	}
}
