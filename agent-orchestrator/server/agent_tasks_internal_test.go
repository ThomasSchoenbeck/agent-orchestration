package server_test

import (
	"net/http"
	"testing"
)

// handleAgentDetail bare GET + PATCH (live roles/skills update; the UI uses PATCH
// — see ui api.js updateAgent).
func TestAgentDetailGetAndPatch(t *testing.T) {
	srv, _ := newTestServer(t)
	rw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{"name": "a1", "roles": []string{"worker"}})
	id, _ := decodeMap(t, rw.Body.Bytes())["agent_id"].(string)

	if w := do(t, srv, http.MethodGet, "/api/agents/"+id, nil); w.Code != http.StatusOK {
		t.Errorf("get agent: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPatch, "/api/agents/"+id, map[string]interface{}{"roles": []string{"worker", "reviewer"}}); w.Code != http.StatusOK {
		t.Errorf("update agent: %d %s", w.Code, w.Body.String())
	}
}

// handleTasks GET with the full set of filter query params.
func TestTasksFilters(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	_ = mkTask(t, srv, pid)
	url := "/api/tasks?status=BACKLOG&role=worker&project_id=" + pid + "&agent_id=&limit=10"
	if w := do(t, srv, http.MethodGet, url, nil); w.Code != http.StatusOK {
		t.Errorf("list tasks with filters: %d %s", w.Code, w.Body.String())
	}
}

// handleInternalProviders only accepts GET → POST is 405. The endpoint is
// agent-namespaced (secrets-only), so it is reached at /api/agent/internal/providers.
func TestInternalProvidersMethod(t *testing.T) {
	srv, _ := newAgentTestServer(t, "")
	if w := agentPost(t, srv, "/api/agent/internal/providers", map[string]interface{}{}); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST internal providers: expected 405, got %d", w.Code)
	}
}
