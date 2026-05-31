package integration_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProjectCreationInitializesBareRepo (IT-1.1) asserts that
// POST /api/projects writes a bare git repo under {storageRoot}/repos/{id}.git
// and that HEAD points at refs/heads/main.
func TestProjectCreationInitializesBareRepo(t *testing.T) {
	t.Parallel()

	srv := newGitTestServer(t)
	projectID, _ := makeProject(t, srv.BaseURL, "myproject")

	// Assert the HEAD file exists on disk.
	headPath := filepath.Join(srv.StorageRoot, "repos", projectID+".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		t.Fatalf("HEAD not found at %s: %v", headPath, err)
	}

	got := string(data)
	want := "ref: refs/heads/main\n"
	if got != want {
		t.Errorf("HEAD = %q, want %q", got, want)
	}
}
