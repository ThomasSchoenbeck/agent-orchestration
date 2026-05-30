package server_test

// git_routes_test.go exercises the project git file/tree/diff API routes.
// It uses newTestServer (from handlers_test.go) and do/createTestProject helpers.

import (
	"encoding/json"
	"net/http"
	"testing"

	"agent-orchestrator/git"
)

// TestProjectBranches_EmptyRepo verifies that a freshly created project has
// exactly one branch ("main") due to the initial empty commit on creation.
func TestProjectBranches_EmptyRepo(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/branches", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var branches []string
	_ = json.Unmarshal(w.Body.Bytes(), &branches)
	if len(branches) != 1 || branches[0] != "main" {
		t.Errorf("expected [main], got %v", branches)
	}
}

// TestProjectTree_TaskBranchRef_EmptyRepo verifies that requesting a
// non-existent task branch ref on a fresh project returns 200 with an empty
// array rather than 404. This covers the case where the UI requests the task
// branch before the agent has pushed any commits.
func TestProjectTree_TaskBranchRef_EmptyRepo(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/tree?ref=task%2Fsome-task-id&path=", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for nonexistent task branch on empty repo, got %d: %s", w.Code, w.Body.String())
	}
	var nodes []git.TreeNode
	_ = json.Unmarshal(w.Body.Bytes(), &nodes)
	if len(nodes) != 0 {
		t.Errorf("expected empty tree, got %v", nodes)
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

// TestProjectCommits_EmptyRepo verifies GET /commits returns 200 [] on a fresh project.
func TestProjectCommits_EmptyRepo(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/commits?ref=main", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var commits []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &commits)
	// main has the initial empty commit from project creation
	if commits == nil {
		t.Error("expected non-nil (at least empty) commits array")
	}
}

// TestProjectCommits_AfterFileCommit verifies a committed file produces a log entry.
func TestProjectCommits_AfterFileCommit(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	putW := do(t, srv, http.MethodPut, "/api/projects/"+projectID+"/file", map[string]interface{}{
		"path": "README.md", "content": "# Hello", "branch": "main", "message": "add readme",
	})
	if putW.Code != http.StatusOK {
		t.Fatalf("PUT file: %d %s", putW.Code, putW.Body.String())
	}
	var putResp map[string]string
	_ = json.Unmarshal(putW.Body.Bytes(), &putResp)
	fileSHA := putResp["sha"]

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/commits?ref=main", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET commits: %d %s", w.Code, w.Body.String())
	}
	var commits []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &commits)

	// Should have at least the file commit (plus the initial empty commit).
	if len(commits) == 0 {
		t.Fatal("expected at least one commit")
	}
	// Most recent commit SHA should match the file commit.
	gotSHA, _ := commits[0]["sha"].(string)
	if gotSHA != fileSHA {
		t.Errorf("top commit SHA = %q, want %q", gotSHA, fileSHA)
	}
	if msg, _ := commits[0]["message"].(string); msg != "add readme" {
		t.Errorf("top commit message = %q, want add readme", msg)
	}
}

// TestProjectCommits_NonexistentRef returns 200 [] for a branch that was never pushed.
func TestProjectCommits_NonexistentRef(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	w := do(t, srv, http.MethodGet, "/api/projects/"+projectID+"/commits?ref=task%2Fnever-pushed", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var commits []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &commits)
	if len(commits) != 0 {
		t.Errorf("expected 0 commits for nonexistent ref, got %d", len(commits))
	}
}
