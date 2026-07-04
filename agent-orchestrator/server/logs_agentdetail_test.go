package server_test

import (
	"net/http"
	"testing"
	"time"
)

// handleLogs (GET/POST/DELETE) + handleTaskLogs/handleAgentLogs GET.
func TestLogsHandlers(t *testing.T) {
	srv, _ := newTestServer(t)

	// POST a log, then list it.
	if w := do(t, srv, http.MethodPost, "/api/logs", map[string]interface{}{"level": "info", "message": "hello"}); w.Code != http.StatusCreated {
		t.Fatalf("create log: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/logs", map[string]interface{}{}); w.Code != http.StatusBadRequest {
		t.Errorf("log without message: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/logs?level=info&limit=10", nil); w.Code != http.StatusOK {
		t.Errorf("list logs: %d", w.Code)
	}
	// DELETE old logs (before now+).
	before := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if w := do(t, srv, http.MethodDelete, "/api/logs?before="+before, nil); w.Code != http.StatusOK {
		t.Errorf("delete old logs: %d", w.Code)
	}

	// Task/agent logs lists (empty without the partitioned log DB) → 200.
	if w := do(t, srv, http.MethodGet, "/api/task-logs?task_id=t1", nil); w.Code != http.StatusOK {
		t.Errorf("task logs: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/agent-logs?agent_id=a1", nil); w.Code != http.StatusOK {
		t.Errorf("agent logs: %d", w.Code)
	}
}

// handleAgentDetail: poll-status / stats / logs sub-routes.
func TestAgentDetailMoreSubRoutes(t *testing.T) {
	srv, _ := newTestServer(t)
	rw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{"name": "a1", "roles": []string{"worker"}})
	if rw.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rw.Code, rw.Body.String())
	}
	id, _ := decodeMap(t, rw.Body.Bytes())["agent_id"].(string)

	for _, sub := range []string{"/poll-status", "/stats", "/logs"} {
		if w := do(t, srv, http.MethodGet, "/api/agents/"+id+sub, nil); w.Code != http.StatusOK {
			t.Errorf("GET /api/agents/{id}%s: %d %s", sub, w.Code, w.Body.String())
		}
	}
}
