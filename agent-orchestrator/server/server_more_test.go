package server_test

import (
	"net/http"
	"testing"
)

// handleChecklistTemplateDetail (was 0%): PUT / DELETE / 404 / 405.
func TestChecklistTemplateDetail(t *testing.T) {
	srv, _ := newTestServer(t)

	w := do(t, srv, http.MethodPost, "/api/checklist-templates", map[string]interface{}{"name": "t1"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create template: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	// PUT updates name + items.
	w = do(t, srv, http.MethodPut, "/api/checklist-templates/"+id, map[string]interface{}{"name": "t2", "items_json": `["a","b"]`})
	if w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["name"] != "t2" {
		t.Errorf("PUT template: %d %s", w.Code, w.Body.String())
	}
	// PUT on a missing id → 404.
	if w := do(t, srv, http.MethodPut, "/api/checklist-templates/nope", map[string]interface{}{"name": "x"}); w.Code != http.StatusNotFound {
		t.Errorf("PUT missing template: expected 404, got %d", w.Code)
	}
	// GET is not allowed on the detail route → 405.
	if w := do(t, srv, http.MethodGet, "/api/checklist-templates/"+id, nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET template detail: expected 405, got %d", w.Code)
	}
	// DELETE then DELETE-again → 204 then 404.
	if w := do(t, srv, http.MethodDelete, "/api/checklist-templates/"+id, nil); w.Code != http.StatusNoContent {
		t.Errorf("DELETE template: %d", w.Code)
	}
	if w := do(t, srv, http.MethodDelete, "/api/checklist-templates/"+id, nil); w.Code != http.StatusNotFound {
		t.Errorf("DELETE missing template: expected 404, got %d", w.Code)
	}
}

// handleAgentDetail sub-routes (heartbeat/stop/offline) over a real DB.
func TestAgentDetailSubRoutes(t *testing.T) {
	srv, _ := newTestServer(t)
	rw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{"name": "a1", "roles": []string{"worker"}})
	if rw.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rw.Code, rw.Body.String())
	}
	id, _ := decodeMap(t, rw.Body.Bytes())["agent_id"].(string)

	if w := do(t, srv, http.MethodPost, "/api/agents/"+id+"/heartbeat", nil); w.Code != http.StatusOK {
		t.Errorf("heartbeat: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/agents/"+id+"/stop", nil); w.Code != http.StatusOK {
		t.Errorf("stop: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/agents/"+id+"/offline", nil); w.Code != http.StatusOK {
		t.Errorf("offline: %d %s", w.Code, w.Body.String())
	}
	// Wrong method on a sub-route → 405.
	if w := do(t, srv, http.MethodGet, "/api/agents/"+id+"/heartbeat", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET heartbeat: expected 405, got %d", w.Code)
	}
}

// Meta endpoints with seeded definitions exercise the enabled-defs loop
// (the existing tests only cover the built-in fallback path).
func TestMetaTaskRoles_WithDefinitions(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodPost, "/api/roles", map[string]interface{}{"name": "customrole", "label": "Custom"}); w.Code != http.StatusCreated {
		t.Fatalf("create role: %d %s", w.Code, w.Body.String())
	}
	w := do(t, srv, http.MethodGet, "/api/meta/task-roles", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("meta task-roles: %d", w.Code)
	}
	found := false
	for _, it := range decodeList(t, w.Body.Bytes()) {
		if m, ok := it.(map[string]interface{}); ok && m["value"] == "customrole" {
			found = true
		}
	}
	if !found {
		t.Error("meta task-roles should include the seeded custom role")
	}
}

func TestMetaSkills_WithDefinitions(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodPost, "/api/skills", map[string]interface{}{"name": "backend", "label": "Backend"}); w.Code != http.StatusCreated {
		t.Fatalf("create skill: %d %s", w.Code, w.Body.String())
	}
	w := do(t, srv, http.MethodGet, "/api/meta/skills", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("meta skills: %d", w.Code)
	}
	found := false
	for _, it := range decodeList(t, w.Body.Bytes()) {
		if m, ok := it.(map[string]interface{}); ok && m["value"] == "backend" {
			found = true
		}
	}
	if !found {
		t.Error("meta skills should include the seeded skill")
	}
}
