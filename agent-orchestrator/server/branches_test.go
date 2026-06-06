package server_test

import (
	"net/http"
	"testing"
)

func TestProjectBranches_ListAndDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv) // initialises a bare repo with a main branch
	base := "/api/projects/" + pid + "/branches"

	// List includes the seeded main branch.
	w := do(t, srv, http.MethodGet, base, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list branches: %d %s", w.Code, w.Body.String())
	}
	branches := decodeList(t, w.Body.Bytes())
	hasMain := false
	for _, b := range branches {
		if b == "main" {
			hasMain = true
		}
	}
	if !hasMain {
		t.Errorf("expected main in branches, got %v", branches)
	}

	// Guards: missing name and base-branch deletion are rejected.
	if w := do(t, srv, http.MethodDelete, base, nil); w.Code != http.StatusBadRequest {
		t.Errorf("delete without name: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodDelete, base+"?name=main", nil); w.Code != http.StatusBadRequest {
		t.Errorf("delete base branch: expected 400, got %d", w.Code)
	}

	// Deleting a non-existent branch is a no-op (git.DeleteBranch tolerates it).
	if w := do(t, srv, http.MethodDelete, base+"?name=task/none", nil); w.Code != http.StatusNoContent {
		t.Errorf("delete missing branch: expected 204, got %d %s", w.Code, w.Body.String())
	}
}
