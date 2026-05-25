package server_test

// git_routes_test.go exercises the project git file/tree/diff API routes.
// It uses newTestServer (from handlers_test.go) and do/createTestProject helpers.

import (
	"encoding/json"
	"net/http"
	"testing"

	"agent-orchestrator/git"
)

// TestProjectBranches_EmptyRepo verifies that listing branches on a freshly
// created project (empty bare repo) returns an empty array.
func TestProjectBranches_EmptyRepo(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/branches", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var branches []string
	_ = json.Unmarshal(w.Body.Bytes(), &branches)
	if len(branches) != 0 {
		t.Errorf("expected 0 branches, got %v", branches)
	}
}

// TestProjectTree_EmptyRepo verifies ReadTree on an empty repo returns [].
func TestProjectTree_EmptyRepo(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/tree?ref=main", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var nodes []git.TreeNode
	_ = json.Unmarshal(w.Body.Bytes(), &nodes)
	if len(nodes) != 0 {
		t.Errorf("expected empty tree, got %v", nodes)
	}
}

// TestProjectFile_CommitAndRead exercises the full PUT→GET file round-trip.
func TestProjectFile_CommitAndRead(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	// PUT /file — commit a new file.
	putW := do(t, srv, http.MethodPut, "/api/projects/"+projectID+"/file", map[string]interface{}{
		"path":    "README.md",
		"content": "# Hello\n",
		"branch":  "main",
		"message": "add README",
	})
	if putW.Code != http.StatusOK {
		t.Fatalf("PUT file: expected 200, got %d: %s", putW.Code, putW.Body.String())
	}
	var putResp map[string]string
	_ = json.Unmarshal(putW.Body.Bytes(), &putResp)
	if putResp["sha"] == "" {
		t.Error("expected non-empty sha in response")
	}
	if putResp["path"] != "README.md" {
		t.Errorf("path = %q, want README.md", putResp["path"])
	}

	// GET /file — read it back.
	getW := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/file?ref=main&path=README.md", nil)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET file: expected 200, got %d: %s", getW.Code, getW.Body.String())
	}
	var getResp map[string]string
	_ = json.Unmarshal(getW.Body.Bytes(), &getResp)
	if getResp["content"] != "# Hello\n" {
		t.Errorf("content = %q, want \"# Hello\\n\"", getResp["content"])
	}
	if getResp["encoding"] != "utf8" {
		t.Errorf("encoding = %q, want utf8", getResp["encoding"])
	}
}

// TestProjectTree_AfterCommit verifies ReadTree returns the committed file.
func TestProjectTree_AfterCommit(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	do(t, srv, http.MethodPut, "/api/projects/"+projectID+"/file", map[string]interface{}{
		"path": "src/main.go", "content": "package main", "branch": "main", "message": "init",
	})

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/tree?ref=main", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET tree: %d: %s", w.Code, w.Body.String())
	}
	var nodes []git.TreeNode
	_ = json.Unmarshal(w.Body.Bytes(), &nodes)
	if len(nodes) != 1 || nodes[0].Name != "src" {
		t.Errorf("expected root to contain [src], got %+v", nodes)
	}
}

// TestProjectFile_MissingPath returns 400 when path is omitted.
func TestProjectFile_MissingPath(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/file?ref=main", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestProjectDiff_NoChanges returns an empty patch list when base==head.
func TestProjectDiff_NoChanges(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	// Commit something first so main exists.
	do(t, srv, http.MethodPut, "/api/projects/"+projectID+"/file", map[string]interface{}{
		"path": "a.txt", "content": "aaa", "branch": "main", "message": "init",
	})

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/diff?base=main&head=main", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET diff: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var patches []git.FilePatch
	_ = json.Unmarshal(w.Body.Bytes(), &patches)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for base==head, got %d", len(patches))
	}
}

// TestProjectBranches_AfterCommit verifies the branch appears after first commit.
func TestProjectBranches_AfterCommit(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	do(t, srv, http.MethodPut, "/api/projects/"+projectID+"/file", map[string]interface{}{
		"path": "f.txt", "content": "x", "branch": "main", "message": "init",
	})

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/branches", nil)
	var branches []string
	_ = json.Unmarshal(w.Body.Bytes(), &branches)
	if len(branches) != 1 || branches[0] != "main" {
		t.Errorf("expected [main], got %v", branches)
	}
}
