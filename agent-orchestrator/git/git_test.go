package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"agent-orchestrator/git"
)

// tempDir creates a temporary directory and registers cleanup.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestInitBare_CreatesRepo(t *testing.T) {
	path := filepath.Join(tempDir(t), "myproject.git")
	if err := git.InitBare(path); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	// Bare repos contain a HEAD file.
	if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
		t.Fatalf("HEAD not found after InitBare: %v", err)
	}
}

func TestInitBare_Idempotent(t *testing.T) {
	path := filepath.Join(tempDir(t), "myproject.git")
	if err := git.InitBare(path); err != nil {
		t.Fatalf("first InitBare: %v", err)
	}
	if err := git.InitBare(path); err != nil {
		t.Fatalf("second InitBare (idempotent): %v", err)
	}
}

func TestInitBare_SetsMainBranch(t *testing.T) {
	path := filepath.Join(tempDir(t), "myproject.git")
	if err := git.InitBare(path); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	// HEAD should reference refs/heads/main
	headBytes, err := os.ReadFile(filepath.Join(path, "HEAD"))
	if err != nil {
		t.Fatalf("reading HEAD: %v", err)
	}
	head := string(headBytes)
	const want = "ref: refs/heads/main\n"
	if head != want {
		t.Errorf("HEAD = %q, want %q", head, want)
	}
}

func TestOpenBare_ExistingRepo(t *testing.T) {
	path := filepath.Join(tempDir(t), "myproject.git")
	if err := git.InitBare(path); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	repo, err := git.OpenBare(path)
	if err != nil {
		t.Fatalf("OpenBare: %v", err)
	}
	if repo == nil {
		t.Fatal("OpenBare returned nil repo")
	}
}

func TestOpenBare_MissingRepo(t *testing.T) {
	path := filepath.Join(tempDir(t), "nonexistent.git")
	_, err := git.OpenBare(path)
	if err == nil {
		t.Fatal("expected error opening non-existent repo, got nil")
	}
}

func TestAddRemote_AddsAndIdempotent(t *testing.T) {
	path := filepath.Join(tempDir(t), "myproject.git")
	if err := git.InitBare(path); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	const url = "https://github.com/example/repo.git"
	if err := git.AddRemote(path, "origin", url); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}
	// Second call should be a no-op (idempotent).
	if err := git.AddRemote(path, "origin", url); err != nil {
		t.Fatalf("AddRemote (idempotent): %v", err)
	}
}

func TestEnsureBranchRef_CreatesRef(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "repo.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	sha, err := git.CommitFile(repoPath, "main", "f.txt", []byte("x"), "init", "dev", "dev@localhost")
	if err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	if err := git.EnsureBranchRef(repoPath, "feature", sha); err != nil {
		t.Fatalf("EnsureBranchRef: %v", err)
	}

	branches, _ := git.ListBranches(repoPath)
	found := false
	for _, b := range branches {
		if b == "feature" {
			found = true
		}
	}
	if !found {
		t.Errorf("branch 'feature' not found after EnsureBranchRef; branches: %v", branches)
	}
}

func TestEnsureBranchRef_NoopIfExists(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "repo.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	sha, _ := git.CommitFile(repoPath, "main", "f.txt", []byte("x"), "init", "dev", "dev@localhost")

	// Create the ref once.
	if err := git.EnsureBranchRef(repoPath, "main", sha); err != nil {
		t.Fatalf("first EnsureBranchRef: %v", err)
	}
	// Add a second commit that advances main.
	sha2, _ := git.CommitFile(repoPath, "main", "g.txt", []byte("y"), "second", "dev", "dev@localhost")

	// EnsureBranchRef on an already-existing branch must be a no-op (not overwrite it).
	if err := git.EnsureBranchRef(repoPath, "main", sha2); err != nil {
		t.Fatalf("second EnsureBranchRef: %v", err)
	}
	// main should still point at the latest commit (set by CommitFile, not overwritten).
	data, err := git.ReadFile(repoPath, "main", "g.txt")
	if err != nil {
		t.Fatalf("ReadFile g.txt after no-op EnsureBranchRef: %v", err)
	}
	if string(data) != "y" {
		t.Errorf("g.txt = %q, want y", data)
	}
}
