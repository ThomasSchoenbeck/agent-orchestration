package git_test

import (
	"path/filepath"
	"testing"

	"agent-orchestrator/git"
)

// newRepoWithMain creates a bare repo with one commit on main and returns the
// repo path and the main commit SHA.
func newRepoWithMain(t *testing.T) (string, string) {
	t.Helper()
	repoPath := filepath.Join(tempDir(t), "merge.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	sha, err := git.CommitFile(repoPath, "main", "base.txt", []byte("base\n"), "c0", "dev", "dev@x")
	if err != nil {
		t.Fatalf("CommitFile c0: %v", err)
	}
	return repoPath, sha
}

func TestMergeBranch_FastForward(t *testing.T) {
	repoPath, c0 := newRepoWithMain(t)

	// Branch task at c0, then advance it with a new commit (main untouched).
	if err := git.EnsureBranchRef(repoPath, "task/1", c0); err != nil {
		t.Fatalf("EnsureBranchRef: %v", err)
	}
	c1, err := git.CommitFile(repoPath, "task/1", "feature.txt", []byte("feat\n"), "c1", "dev", "dev@x")
	if err != nil {
		t.Fatalf("CommitFile c1: %v", err)
	}

	sha, err := git.MergeBranch(repoPath, "main", "task/1")
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if sha != c1 {
		t.Errorf("ff merge sha = %q, want branch tip %q", sha, c1)
	}
	// main must now contain feature.txt.
	if _, err := git.ReadFile(repoPath, "main", "feature.txt"); err != nil {
		t.Errorf("feature.txt missing on main after ff merge: %v", err)
	}
}

func TestMergeBranch_MergeCommit(t *testing.T) {
	repoPath, c0 := newRepoWithMain(t)

	// Diverge: main modifies a.txt; task (branched at c0) adds b.txt.
	if _, err := git.CommitFile(repoPath, "main", "a.txt", []byte("a-main\n"), "main edit", "dev", "dev@x"); err != nil {
		t.Fatalf("CommitFile main a: %v", err)
	}
	if err := git.EnsureBranchRef(repoPath, "task/2", c0); err != nil {
		t.Fatalf("EnsureBranchRef: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "task/2", "b.txt", []byte("b-task\n"), "task add b", "dev", "dev@x"); err != nil {
		t.Fatalf("CommitFile task b: %v", err)
	}

	sha, err := git.MergeBranch(repoPath, "main", "task/2")
	if err != nil {
		t.Fatalf("MergeBranch (non-ff, no conflict): %v", err)
	}
	if sha == "" {
		t.Fatal("expected non-empty merge commit sha")
	}
	// Merged main must contain both the main edit and the task's new file.
	if _, err := git.ReadFile(repoPath, "main", "a.txt"); err != nil {
		t.Errorf("a.txt missing after merge: %v", err)
	}
	if _, err := git.ReadFile(repoPath, "main", "b.txt"); err != nil {
		t.Errorf("b.txt missing after merge: %v", err)
	}
}

func TestMergeBranch_Conflict(t *testing.T) {
	repoPath, c0 := newRepoWithMain(t)

	// Both main and task modify shared.txt differently relative to c0.
	if _, err := git.CommitFile(repoPath, "main", "shared.txt", []byte("main-version\n"), "main edit", "dev", "dev@x"); err != nil {
		t.Fatalf("CommitFile main shared: %v", err)
	}
	if err := git.EnsureBranchRef(repoPath, "task/3", c0); err != nil {
		t.Fatalf("EnsureBranchRef: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "task/3", "shared.txt", []byte("task-version\n"), "task edit", "dev", "dev@x"); err != nil {
		t.Fatalf("CommitFile task shared: %v", err)
	}

	if _, err := git.MergeBranch(repoPath, "main", "task/3"); err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}

func TestDeleteBranch(t *testing.T) {
	repoPath, c0 := newRepoWithMain(t)
	if err := git.EnsureBranchRef(repoPath, "task/4", c0); err != nil {
		t.Fatalf("EnsureBranchRef: %v", err)
	}

	if err := git.DeleteBranch(repoPath, "task/4"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	branches, _ := git.ListBranches(repoPath)
	for _, b := range branches {
		if b == "task/4" {
			t.Errorf("branch task/4 still present after DeleteBranch: %v", branches)
		}
	}
	// Deleting again is a no-op (no error).
	if err := git.DeleteBranch(repoPath, "task/4"); err != nil {
		t.Errorf("second DeleteBranch should be no-op, got: %v", err)
	}
}
