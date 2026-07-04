package server_test

import (
	"net/http"
	"testing"
)

// A sweep of method-not-allowed / missing-id / not-found branches across handlers.
func TestMethodAndErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)

	// handleTaskComments: DELETE without a comment id → 400; POST to a comment id → 405.
	if w := do(t, srv, http.MethodDelete, "/api/tasks/"+taskID+"/comments", nil); w.Code != http.StatusBadRequest {
		t.Errorf("delete comment without id: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/comments/c1", map[string]interface{}{"body": "x"}); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST to comment id: expected 405, got %d", w.Code)
	}

	// handleProjectFiles only accepts POST.
	if w := do(t, srv, http.MethodGet, "/api/projects/"+pid+"/files", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET project files: expected 405, got %d", w.Code)
	}

	// handleSettingDetail accepts only GET/PUT → DELETE is 405.
	if w := do(t, srv, http.MethodDelete, "/api/settings/some.key", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE setting: expected 405, got %d", w.Code)
	}

	// Skill / subagent-skill PUT on a missing id → 404.
	if w := do(t, srv, http.MethodPut, "/api/skills/missing", map[string]interface{}{"name": "x"}); w.Code != http.StatusNotFound {
		t.Errorf("PUT missing skill: expected 404, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPut, "/api/subagent-skills/missing", map[string]interface{}{"name": "x"}); w.Code != http.StatusNotFound {
		t.Errorf("PUT missing subagent skill: expected 404, got %d", w.Code)
	}

	// handleProviderDetail GET detail (success).
	cw := do(t, srv, http.MethodPost, "/api/providers", map[string]interface{}{"name": "p", "type": "openai_compatible", "base_url": "http://x/v1", "enabled": true})
	pidProv, _ := decodeMap(t, cw.Body.Bytes())["id"].(string)
	if w := do(t, srv, http.MethodGet, "/api/providers/"+pidProv, nil); w.Code != http.StatusOK {
		t.Errorf("GET provider detail: %d", w.Code)
	}
}
