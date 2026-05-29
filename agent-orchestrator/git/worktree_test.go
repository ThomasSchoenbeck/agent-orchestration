package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"agent-orchestrator/git"
)

func TestCreateWorktree_EmptyRepo(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "empty.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	worktreePath := filepath.Join(tempDir(t), "wt")
	sha, err := git.CreateWorktree(repoPath, worktreePath, "task/abc", "main")
	if err != nil {
		t.Fatalf("CreateWorktree on empty repo: %v", err)
	}
	// Empty repo returns blank SHA, not the zero hash string.
	if sha != "" {
		t.Errorf("expected empty SHA for empty repo, got %q", sha)
	}
	// Worktree directory must exist and be a valid git repo.
	if _, statErr := os.Stat(filepath.Join(worktreePath, ".git")); statErr != nil {
		t.Errorf("worktree .git dir missing: %v", statErr)
	}
}

func TestCreateWorktree_RepoWithCommits(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "repo.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	// Seed a commit on main so the repo is non-empty.
	commitSHA, err := git.CommitFile(repoPath, "main", "readme.txt", []byte("hello"), "init", "dev", "dev@example.com")
	if err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	worktreePath := filepath.Join(tempDir(t), "wt")
	sha, err := git.CreateWorktree(repoPath, worktreePath, "task/xyz", "main")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if sha != commitSHA {
		t.Errorf("SHA = %q, want %q", sha, commitSHA)
	}
	// Seeded file must be present in the worktree.
	data, readErr := os.ReadFile(filepath.Join(worktreePath, "readme.txt"))
	if readErr != nil {
		t.Fatalf("readme.txt missing in worktree: %v", readErr)
	}
	if string(data) != "hello" {
		t.Errorf("readme.txt = %q, want hello", data)
	}
}

func TestCreateWorktree_IdempotentOnExistingWorktree(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "repo.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "main", "f.txt", []byte("x"), "init", "dev", "dev@localhost"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	worktreePath := filepath.Join(tempDir(t), "wt")
	if _, err := git.CreateWorktree(repoPath, worktreePath, "task/dup", "main"); err != nil {
		t.Fatalf("first CreateWorktree: %v", err)
	}
	// Calling again must not error (ErrRepositoryAlreadyExists is swallowed).
	if _, err := git.CreateWorktree(repoPath, worktreePath, "task/dup", "main"); err != nil {
		t.Fatalf("second CreateWorktree (idempotent): %v", err)
	}
}

func TestRemoveWorktree_DeletesDirectory(t *testing.T) {
	dir := filepath.Join(tempDir(t), "wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := git.RemoveWorktree(dir); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("expected directory to be gone after RemoveWorktree")
	}
}

func TestRemoveWorktree_NoErrorIfMissing(t *testing.T) {
	dir := filepath.Join(tempDir(t), "nonexistent")
	if err := git.RemoveWorktree(dir); err != nil {
		t.Fatalf("RemoveWorktree on missing dir: %v", err)
	}
}
