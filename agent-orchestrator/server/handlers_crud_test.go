package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// decodeMap unmarshals a response body into a map for assertions.
func decodeMap(t *testing.T, b []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode map: %v (body=%s)", err, b)
	}
	return m
}

func decodeList(t *testing.T, b []byte) []interface{} {
	t.Helper()
	var l []interface{}
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatalf("decode list: %v (body=%s)", err, b)
	}
	return l
}

// --- Providers CRUD ---

func TestProviders_CRUD(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create.
	w := do(t, srv, http.MethodPost, "/api/providers", map[string]interface{}{
		"name": "p1", "type": "openai_compatible", "base_url": "http://localhost:7777/v1",
		"api_key": "sk-1", "enabled": true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create provider: %d %s", w.Code, w.Body.String())
	}
	created := decodeMap(t, w.Body.Bytes())
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("created provider missing id")
	}
	if _, present := created["api_key"]; present {
		t.Error("create response must not echo api_key")
	}

	// Missing required fields.
	if w := do(t, srv, http.MethodPost, "/api/providers", map[string]interface{}{"name": "x"}); w.Code != http.StatusBadRequest {
		t.Errorf("create without type: expected 400, got %d", w.Code)
	}

	// List.
	if w := do(t, srv, http.MethodGet, "/api/providers", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list providers: code=%d", w.Code)
	}

	// Get detail.
	if w := do(t, srv, http.MethodGet, "/api/providers/"+id, nil); w.Code != http.StatusOK {
		t.Errorf("get provider: %d", w.Code)
	}

	// Update.
	if w := do(t, srv, http.MethodPut, "/api/providers/"+id, map[string]interface{}{
		"name": "p1", "type": "openai_compatible", "base_url": "http://localhost:7777/v1", "enabled": false,
	}); w.Code != http.StatusOK {
		t.Errorf("update provider: %d %s", w.Code, w.Body.String())
	}

	// Delete, then 404.
	if w := do(t, srv, http.MethodDelete, "/api/providers/"+id, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete provider: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/providers/"+id, nil); w.Code != http.StatusNotFound {
		t.Errorf("get deleted provider: expected 404, got %d", w.Code)
	}
}

// --- Roles CRUD ---

func TestRoles_CRUD(t *testing.T) {
	srv, _ := newTestServer(t)

	w := do(t, srv, http.MethodPost, "/api/roles", map[string]interface{}{
		"name": "planner", "label": "Planner", "capabilities": []string{"creates_tasks"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create role: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)
	if id == "" {
		t.Fatal("created role missing id")
	}

	if w := do(t, srv, http.MethodPost, "/api/roles", map[string]interface{}{"label": "no name"}); w.Code != http.StatusBadRequest {
		t.Errorf("create role without name: expected 400, got %d", w.Code)
	}

	if w := do(t, srv, http.MethodGet, "/api/roles", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list roles: code=%d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/roles/"+id, nil); w.Code != http.StatusOK {
		t.Errorf("get role: %d", w.Code)
	}
	if w := do(t, srv, http.MethodDelete, "/api/roles/"+id, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete role: %d", w.Code)
	}
}

// --- Context save/query ---

func TestContext_SaveAndQuery(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)

	w := do(t, srv, http.MethodPost, "/api/context/save", map[string]interface{}{
		"project_id": pid, "type": "note", "content": "microservices architecture",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("save context: %d %s", w.Code, w.Body.String())
	}

	// Missing content → 400.
	if w := do(t, srv, http.MethodPost, "/api/context/save", map[string]interface{}{"project_id": pid, "type": "note"}); w.Code != http.StatusBadRequest {
		t.Errorf("save context without content: expected 400, got %d", w.Code)
	}

	if w := do(t, srv, http.MethodGet, "/api/context/query?project_id="+pid+"&query=microservices", nil); w.Code != http.StatusOK {
		t.Errorf("query context: %d", w.Code)
	}
}

// --- Meta enumerations ---

func TestMetaTools(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/api/meta/tools", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("meta tools: %d", w.Code)
	}
	if len(decodeList(t, w.Body.Bytes())) == 0 {
		t.Error("expected a non-empty tool list")
	}
}
