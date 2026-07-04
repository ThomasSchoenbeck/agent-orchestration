package server_test

import (
	"net/http"
	"testing"
)

// Git read handlers run against the bare repo that createTestProject seeds
// (InitBare + a .gitkeep commit on main) — all go-git, no external binary.
func TestGitReadHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)
	base := "/api/projects/" + pid

	// Branches list includes main.
	w := do(t, srv, http.MethodGet, base+"/branches", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("branches: %d %s", w.Code, w.Body.String())
	}
	foundMain := false
	for _, b := range decodeList(t, w.Body.Bytes()) {
		if b == "main" {
			foundMain = true
		}
	}
	if !foundMain {
		t.Errorf("branches should include main, got %s", w.Body.String())
	}

	if w := do(t, srv, http.MethodGet, base+"/tree?ref=main", nil); w.Code != http.StatusOK {
		t.Errorf("tree: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodGet, base+"/commits?ref=main", nil); w.Code != http.StatusOK {
		t.Errorf("commits: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodGet, base+"/file?ref=main&path=.gitkeep", nil); w.Code != http.StatusOK {
		t.Errorf("file: %d %s", w.Code, w.Body.String())
	}
	// file without ?path → 400.
	if w := do(t, srv, http.MethodGet, base+"/file", nil); w.Code != http.StatusBadRequest {
		t.Errorf("file without path: expected 400, got %d", w.Code)
	}
	// DELETE branch without ?name → 400.
	if w := do(t, srv, http.MethodDelete, base+"/branches", nil); w.Code != http.StatusBadRequest {
		t.Errorf("delete branch without name: expected 400, got %d", w.Code)
	}
}

// Provider + role detail 404 branches.
func TestProviderRoleDetail404(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/providers/missing", nil); w.Code != http.StatusNotFound {
		t.Errorf("GET missing provider: expected 404, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPut, "/api/providers/missing", map[string]interface{}{"name": "x", "type": "openai_compatible"}); w.Code != http.StatusNotFound {
		t.Errorf("PUT missing provider: expected 404, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/roles/missing", nil); w.Code != http.StatusNotFound {
		t.Errorf("GET missing role: expected 404, got %d", w.Code)
	}
}

// Context save/query method guards + explicit type.
func TestContextHandlerMethods(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)

	// Wrong methods.
	if w := do(t, srv, http.MethodGet, "/api/context/save", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/context/save: expected 405, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodPost, "/api/context/query", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/context/query: expected 405, got %d", w.Code)
	}
	// Save with an explicit type, then query it back.
	if w := do(t, srv, http.MethodPost, "/api/context/save", map[string]interface{}{
		"project_id": pid, "type": "snippet", "content": "func main() {}",
	}); w.Code != http.StatusCreated {
		t.Errorf("save context: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodGet, "/api/context/query?project_id="+pid+"&query=main", nil); w.Code != http.StatusOK {
		t.Errorf("query context: %d", w.Code)
	}
}
