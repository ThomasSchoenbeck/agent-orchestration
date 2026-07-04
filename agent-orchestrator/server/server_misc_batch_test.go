package server_test

import (
	"net/http"
	"testing"
	"time"
)

func TestLogDeletesAndMethods(t *testing.T) {
	srv, _ := newTestServer(t)
	before := time.Now().UTC().Format(time.RFC3339)

	if w := do(t, srv, http.MethodDelete, "/api/task-logs?before="+before, nil); w.Code != http.StatusOK {
		t.Errorf("delete task logs: %d", w.Code)
	}
	if w := do(t, srv, http.MethodDelete, "/api/agent-logs?before="+before, nil); w.Code != http.StatusOK {
		t.Errorf("delete agent logs: %d", w.Code)
	}
	if w := do(t, srv, http.MethodDelete, "/api/agent-logs", nil); w.Code != http.StatusOK {
		t.Errorf("delete agent logs (no before): %d", w.Code)
	}
	if w := do(t, srv, http.MethodPut, "/api/task-logs", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT task-logs: expected 405, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPut, "/api/agent-logs", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT agent-logs: expected 405, got %d", w.Code)
	}
}

func TestProjectDiff(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	base := "/api/projects/" + pid + "/diff"

	if w := do(t, srv, http.MethodGet, base+"?base=main&head=main", nil); w.Code != http.StatusOK {
		t.Errorf("diff main..main: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodGet, base, nil); w.Code != http.StatusBadRequest {
		t.Errorf("diff without base/head: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, base+"?base=main&head=main", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST diff: expected 405, got %d", w.Code)
	}
}

func TestSyncScope(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)
	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/sync-scope", map[string]interface{}{
		"requirements": []map[string]interface{}{{"title": "R", "body": "b"}},
		"features":     []map[string]interface{}{{"title": "F", "body": "b"}},
	})
	if w.Code != http.StatusOK {
		t.Errorf("sync-scope: %d %s", w.Code, w.Body.String())
	}
}

func TestSkillDetailDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/skills", map[string]interface{}{"name": "data", "label": "Data"})
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)
	if w := do(t, srv, http.MethodDelete, "/api/skills/"+id, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete skill: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/skills/"+id, nil); w.Code != http.StatusNotFound {
		t.Errorf("get deleted skill: expected 404, got %d", w.Code)
	}
}

func TestTaskLinkRemoveMissing(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	taskID := mkTask(t, srv, pid)
	if w := do(t, srv, http.MethodDelete, "/api/tasks/"+taskID+"/links", map[string]interface{}{
		"kind": "requirement", "target_id": "nonexistent",
	}); w.Code != http.StatusNotFound {
		t.Errorf("remove missing link: expected 404, got %d", w.Code)
	}
}
