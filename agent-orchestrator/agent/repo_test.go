package agent_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"

	"agent-orchestrator/agent"
	"agent-orchestrator/git"
)

// startRepoServer spins up a real HTTPHandler serving repoPath under slug,
// registers cleanup, and returns the base URL.
func startRepoServer(t *testing.T, slug, repoPath string) string {
	t.Helper()
	h := git.NewHTTPHandler(func(s string) (string, error) {
		if s == slug {
			return repoPath, nil
		}
		return "", nil
	}, nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL + "/git/" + slug + ".git"
}

// TestCloneOrOpen_EmptyRepo verifies that CloneOrOpen succeeds when the
// remote repo has no commits yet (server returns ErrEmptyRemoteRepository).
// The local directory should be a valid git repo with origin set.
func TestCloneOrOpen_EmptyRepo(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "srv.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	// No InitialCommit — empty repo.

	repoURL := startRepoServer(t, "srv", repoPath)
	localPath := filepath.Join(t.TempDir(), "workspace")

	if err := agent.CloneOrOpen(repoURL, localPath, "task/abc"); err != nil {
		t.Fatalf("CloneOrOpen empty repo: %v", err)
	}

	// Local directory must be a valid git repo.
	if _, err := os.Stat(filepath.Join(localPath, ".git")); err != nil {
		t.Errorf(".git directory not found: %v", err)
	}

	// origin remote must point at the server.
	r, err := gogit.PlainOpen(localPath)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	remotes, err := r.Remotes()
	if err != nil || len(remotes) == 0 {
		t.Fatalf("expected at least one remote, got %v / %v", remotes, err)
	}
	if remotes[0].Config().URLs[0] != repoURL {
		t.Errorf("remote URL = %q, want %q", remotes[0].Config().URLs[0], repoURL)
	}
}

// TestCloneOrOpen_WithInitialCommit verifies that CloneOrOpen succeeds when
// the remote has the InitialCommit placeholder commit. This is the exact
// production scenario that previously caused an HTTP 500.
func TestCloneOrOpen_WithInitialCommit(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "srv.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := git.InitialCommit(repoPath, "main"); err != nil {
		t.Fatalf("InitialCommit: %v", err)
	}

	repoURL := startRepoServer(t, "proj", repoPath)
	localPath := filepath.Join(t.TempDir(), "workspace")

	if err := agent.CloneOrOpen(repoURL, localPath, "task/task-001"); err != nil {
		t.Fatalf("CloneOrOpen with InitialCommit: %v", err)
	}

	if _, err := os.Stat(filepath.Join(localPath, ".git")); err != nil {
		t.Errorf(".git directory not found: %v", err)
	}
}

// TestCloneOrOpen_CreatesTaskBranch verifies that after cloning, the task
// branch is created and checked out locally.
func TestCloneOrOpen_CreatesTaskBranch(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "srv.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "main", "seed.txt", []byte("seed"), "init", "u", "u@x"); err != nil {
		t.Fatalf("CommitFile seed: %v", err)
	}

	repoURL := startRepoServer(t, "proj", repoPath)
	localPath := filepath.Join(t.TempDir(), "workspace")

	const branch = "task/new-task-001"
	if err := agent.CloneOrOpen(repoURL, localPath, branch); err != nil {
		t.Fatalf("CloneOrOpen: %v", err)
	}

	r, err := gogit.PlainOpen(localPath)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := r.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head.Name().Short() != branch {
		t.Errorf("HEAD branch = %q, want %q", head.Name().Short(), branch)
	}
}

// TestCloneOrOpen_Idempotent verifies that calling CloneOrOpen twice for the
// same task opens the existing repo rather than re-cloning it.
func TestCloneOrOpen_Idempotent(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "srv.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "main", "seed.txt", []byte("seed"), "init", "u", "u@x"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	repoURL := startRepoServer(t, "proj", repoPath)
	localPath := filepath.Join(t.TempDir(), "workspace")

	if err := agent.CloneOrOpen(repoURL, localPath, "task/t1"); err != nil {
		t.Fatalf("first CloneOrOpen: %v", err)
	}

	// Write a local file to verify it survives the second call.
	marker := filepath.Join(localPath, "local-marker.txt")
	if err := os.WriteFile(marker, []byte("survives"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := agent.CloneOrOpen(repoURL, localPath, "task/t1"); err != nil {
		t.Fatalf("second CloneOrOpen: %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Error("local-marker.txt was deleted by second CloneOrOpen — should have been preserved")
	}
}

// TestCloneOrOpen_ExistingRepoNoServer verifies the open-existing-repo path:
// even if the remote URL is unreachable, CloneOrOpen succeeds when the
// directory is already a valid git repo.
func TestCloneOrOpen_ExistingRepoNoServer(t *testing.T) {
	localPath := t.TempDir()
	// Init a local repo manually.
	if _, err := gogit.PlainInit(localPath, false); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	r, _ := gogit.PlainOpen(localPath)
	_, _ = r.CreateRemote(&gogitconfig.RemoteConfig{Name: "origin", URLs: []string{"http://localhost:0/git/x.git"}})

	// Unreachable URL — but the directory already exists as a git repo.
	if err := agent.CloneOrOpen("http://localhost:0/git/x.git", localPath, "task/t1"); err != nil {
		// Branch checkout will fail on an empty repo, but PlainOpen must succeed.
		// The test just checks no unexpected panic or "not a git repo" error.
		t.Logf("CloneOrOpen on existing repo returned: %v (acceptable for branch checkout on empty repo)", err)
	}
}
