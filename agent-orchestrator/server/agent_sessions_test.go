package server_test

import (
	"net/http"
	"testing"
)

func TestAgentSessions_CreateAndListByTask(t *testing.T) {
	srv, _ := newTestServer(t)

	w := do(t, srv, http.MethodPost, "/api/agent-sessions", map[string]interface{}{
		"task_id": "t1", "agent_id": "a1", "summary": "did stuff", "round": 5,
		"messages": `[{"role":"system","content":"s"}]`,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create session: %d %s", w.Code, w.Body.String())
	}

	if w := do(t, srv, http.MethodPost, "/api/agent-sessions", map[string]interface{}{"summary": "no task"}); w.Code != http.StatusBadRequest {
		t.Errorf("session without task_id: expected 400, got %d", w.Code)
	}

	if w := do(t, srv, http.MethodGet, "/api/agent-sessions?task_id=t1", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list sessions by task: %d %s", w.Code, w.Body.String())
	}

	if w := do(t, srv, http.MethodGet, "/api/agent-sessions", nil); w.Code != http.StatusBadRequest {
		t.Errorf("list without task_id: expected 400, got %d", w.Code)
	}
}
