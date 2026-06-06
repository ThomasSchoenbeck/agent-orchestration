package git_test

import (
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"agent-orchestrator/git"
)

func TestSquashMerge_CollapsesToSingleCommit(t *testing.T) {
	repoPath, c0 := newRepoWithMain(t)
	if err := git.EnsureBranchRef(repoPath, "task/1", c0); err != nil {
		t.Fatalf("EnsureBranchRef: %v", err)
	}
	// Two commits on the branch — squashing must collapse them into one.
	if _, err := git.CommitFile(repoPath, "task/1", "f1.txt", []byte("one\n"), "c1", "dev", "dev@x"); err != nil {
		t.Fatalf("CommitFile c1: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "task/1", "f2.txt", []byte("two\n"), "c2", "dev", "dev@x"); err != nil {
		t.Fatalf("CommitFile c2: %v", err)
	}

	sha, err := git.SquashMerge(repoPath, "main", "task/1", "task/abc: my feature\n", "agent-x", "agent@x")
	if err != nil {
		t.Fatalf("SquashMerge: %v", err)
	}

	for _, f := range []string{"f1.txt", "f2.txt"} {
		if _, err := git.ReadFile(repoPath, "main", f); err != nil {
			t.Errorf("%s missing on main after squash: %v", f, err)
		}
	}

	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	c, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	if c.NumParents() != 1 {
		t.Errorf("squash commit parents = %d, want 1", c.NumParents())
	}
	if len(c.ParentHashes) != 1 || c.ParentHashes[0].String() != c0 {
		t.Errorf("squash parent = %v, want base %s", c.ParentHashes, c0)
	}
	if c.Message != "task/abc: my feature\n" {
		t.Errorf("message = %q", c.Message)
	}
	if c.Author.Name != "agent-x" {
		t.Errorf("author = %q, want agent-x", c.Author.Name)
	}
}

func TestSquashMerge_Conflict(t *testing.T) {
	repoPath, c0 := newRepoWithMain(t)
	if _, err := git.CommitFile(repoPath, "main", "shared.txt", []byte("main\n"), "main add", "dev", "dev@x"); err != nil {
		t.Fatalf("CommitFile main: %v", err)
	}
	if err := git.EnsureBranchRef(repoPath, "task/3", c0); err != nil {
		t.Fatalf("EnsureBranchRef: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "task/3", "shared.txt", []byte("task\n"), "task add", "dev", "dev@x"); err != nil {
		t.Fatalf("CommitFile task: %v", err)
	}
	if _, err := git.SquashMerge(repoPath, "main", "task/3", "m", "a", "a@x"); err == nil {
		t.Error("expected a conflict error for divergent edits to shared.txt")
	}
}
