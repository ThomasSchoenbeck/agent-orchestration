package server_test

import (
	"net/http"
	"testing"
)

func TestPreparedPrompts_CreateAndListByTask(t *testing.T) {
	srv, _ := newTestServer(t)

	w := do(t, srv, http.MethodPost, "/api/prepared-prompts", map[string]interface{}{
		"task_id": "t1", "session_id": "s1", "round": 0, "prompt": "synthesized",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create prepared prompt: %d %s", w.Code, w.Body.String())
	}

	if w := do(t, srv, http.MethodPost, "/api/prepared-prompts", map[string]interface{}{"prompt": "no task"}); w.Code != http.StatusBadRequest {
		t.Errorf("prepared prompt without task_id: expected 400, got %d", w.Code)
	}

	if w := do(t, srv, http.MethodGet, "/api/prepared-prompts?task_id=t1", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list prepared prompts by task: %d %s", w.Code, w.Body.String())
	}

	if w := do(t, srv, http.MethodGet, "/api/prepared-prompts", nil); w.Code != http.StatusBadRequest {
		t.Errorf("list without task_id: expected 400, got %d", w.Code)
	}
}
