package server_test

import (
	"net/http"
	"testing"
)

// handleAgentRegister: the re-registration (existing-agent override) branch and
// the validation branches (TestRegisterAgent only covers a fresh 201).
func TestAgentRegister_ReregisterAndValidation(t *testing.T) {
	srv, _ := newTestServer(t)

	w := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{"name": "dup", "roles": []string{"worker"}})
	if w.Code != http.StatusCreated {
		t.Fatalf("first register: %d %s", w.Code, w.Body.String())
	}
	id1, _ := decodeMap(t, w.Body.Bytes())["agent_id"].(string)

	// Re-register the same name → 200 (override), same id.
	w = do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{"name": "dup", "roles": []string{"worker", "reviewer"}})
	if w.Code != http.StatusOK {
		t.Fatalf("re-register: expected 200, got %d %s", w.Code, w.Body.String())
	}
	if id2, _ := decodeMap(t, w.Body.Bytes())["agent_id"].(string); id2 != id1 {
		t.Errorf("re-register agent_id = %q, want %q", id2, id1)
	}

	// Validation: missing roles → 400; missing name → 400.
	if w := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{"name": "x"}); w.Code != http.StatusBadRequest {
		t.Errorf("register without roles: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{"roles": []string{"worker"}}); w.Code != http.StatusBadRequest {
		t.Errorf("register without name: expected 400, got %d", w.Code)
	}
}

// handleMetricsCosts + parseCostFilter with all query parameters set.
func TestMetricsCostsWithFilters(t *testing.T) {
	srv, _ := newTestServer(t)
	url := "/api/metrics/costs?from=2024-01-01&to=2024-12-31&model=gpt-4o&agent_role=worker&source=chat&provider=p1"
	if w := do(t, srv, http.MethodGet, url, nil); w.Code != http.StatusOK {
		t.Errorf("metrics costs with filters: %d %s", w.Code, w.Body.String())
	}
}

// handleSubagentSkillDetail GET + 404 paths (CRUD test covers PUT/DELETE).
func TestSubagentSkillDetail_GetAnd404(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/subagent-skills", map[string]interface{}{"name": "inv", "label": "Inv", "prompt_template": "x"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create subagent skill: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	if w := do(t, srv, http.MethodGet, "/api/subagent-skills/"+id, nil); w.Code != http.StatusOK {
		t.Errorf("GET subagent skill: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/subagent-skills/nope", nil); w.Code != http.StatusNotFound {
		t.Errorf("GET missing subagent skill: expected 404, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPut, "/api/subagent-skills/nope", map[string]interface{}{"label": "x"}); w.Code != http.StatusNotFound {
		t.Errorf("PUT missing subagent skill: expected 404, got %d", w.Code)
	}
}

// handleSkillDetail GET + PUT (CRUD/seed tests cover create/list/delete/404).
func TestSkillDetail_GetAndPut(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/skills", map[string]interface{}{"name": "frontend", "label": "Frontend"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create skill: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	if w := do(t, srv, http.MethodGet, "/api/skills/"+id, nil); w.Code != http.StatusOK {
		t.Errorf("GET skill: %d", w.Code)
	}
	w = do(t, srv, http.MethodPut, "/api/skills/"+id, map[string]interface{}{"name": "frontend", "label": "Frontend v2"})
	if w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["label"] != "Frontend v2" {
		t.Errorf("PUT skill: %d %s", w.Code, w.Body.String())
	}
}
