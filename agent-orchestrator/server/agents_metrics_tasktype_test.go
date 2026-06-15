package server_test

import (
	"net/http"
	"testing"
)

// --- handleAgents (GET /api/agents) ---

func TestAgentsList(t *testing.T) {
	srv, _ := newTestServer(t)

	// Register an agent so the list is non-empty.
	if w := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "a1", "roles": []string{"worker"},
	}); w.Code != http.StatusCreated {
		t.Fatalf("register agent: %d %s", w.Code, w.Body.String())
	}

	if w := do(t, srv, http.MethodGet, "/api/agents", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) < 1 {
		t.Errorf("list agents: code=%d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, "/api/agents", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/agents: expected 405, got %d", w.Code)
	}
}

// --- handleMetricsCostOptions (GET /api/metrics/costs/options) ---

func TestMetricsCostOptions(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/metrics/costs/options", nil); w.Code != http.StatusOK {
		t.Errorf("cost options: expected 200, got %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/metrics/costs/options", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST cost options: expected 405, got %d", w.Code)
	}
}

// --- handleTaskTypeDetail: GET / PUT / 404 (delete + defaults covered elsewhere) ---

func TestTaskTypeDetail_GetPutAndMissing(t *testing.T) {
	srv, _ := newTestServer(t)

	w := do(t, srv, http.MethodPost, "/api/task-types", map[string]interface{}{
		"key": "spike", "label": "Spike", "branch_template": "spike/{slug}",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create task type: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	// GET detail.
	if w := do(t, srv, http.MethodGet, "/api/task-types/"+id, nil); w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["key"] != "spike" {
		t.Errorf("get task type: %d", w.Code)
	}

	// PUT update.
	w = do(t, srv, http.MethodPut, "/api/task-types/"+id, map[string]interface{}{
		"key": "spike", "label": "Spike v2", "branch_template": "spike/{slug}",
	})
	if w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["label"] != "Spike v2" {
		t.Errorf("update task type: %d %s", w.Code, w.Body.String())
	}

	// PUT with a missing required field → 400.
	if w := do(t, srv, http.MethodPut, "/api/task-types/"+id, map[string]interface{}{"key": "spike", "label": "x"}); w.Code != http.StatusBadRequest {
		t.Errorf("update without branch_template: expected 400, got %d", w.Code)
	}

	// GET a non-existent task type → 404.
	if w := do(t, srv, http.MethodGet, "/api/task-types/nope", nil); w.Code != http.StatusNotFound {
		t.Errorf("missing task type: expected 404, got %d", w.Code)
	}
}
