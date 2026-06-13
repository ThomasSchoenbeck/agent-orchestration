package git_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-orchestrator/git"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// initWorktreeRepo creates a non-bare repo with a single base commit, then
// branches "main" and "feature" off it. Returns the worktree dir and the repo.
func initWorktreeRepo(t *testing.T, baseFiles map[string]string) (string, *gogit.Repository, *gogit.Worktree) {
	t.Helper()
	dir := filepath.Join(tempDir(t), "wt")
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	for name, content := range baseFiles {
		wcommitFile(t, wt, dir, name, content)
	}
	c0 := wcommit(t, wt, "c0")
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), c0)); err != nil {
		t.Fatalf("set main: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), c0)); err != nil {
		t.Fatalf("set feature: %v", err)
	}
	return dir, repo, wt
}

func wcommitFile(t *testing.T, wt *gogit.Worktree, dir, file, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
	if _, err := wt.Add(file); err != nil {
		t.Fatalf("add %s: %v", file, err)
	}
}

func wcommit(t *testing.T, wt *gogit.Worktree, msg string) plumbing.Hash {
	t.Helper()
	sig := &object.Signature{Name: "dev", Email: "dev@x", When: time.Now()}
	h, err := wt.Commit(msg, &gogit.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatalf("commit %q: %v", msg, err)
	}
	return h
}

func checkout(t *testing.T, wt *gogit.Worktree, branch string) {
	t.Helper()
	if err := wt.Checkout(&gogit.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(branch)}); err != nil {
		t.Fatalf("checkout %s: %v", branch, err)
	}
}

func TestMergeIntoFeature_UpToDate(t *testing.T) {
	dir, _, wt := initWorktreeRepo(t, map[string]string{"base.txt": "base\n"})
	// feature gains a commit; main stays at c0 (feature already contains main).
	checkout(t, wt, "feature")
	wcommitFile(t, wt, dir, "f.txt", "feat\n")
	wcommit(t, wt, "feature commit")

	st, err := git.MergeIntoFeature(dir, "main", "feature")
	if err != nil {
		t.Fatalf("MergeIntoFeature: %v", err)
	}
	if !st.UpToDate || !st.Clean {
		t.Errorf("expected up-to-date clean merge, got %+v", st)
	}
}

func TestMergeIntoFeature_FastForward(t *testing.T) {
	dir, _, wt := initWorktreeRepo(t, map[string]string{"base.txt": "base\n"})
	// main advances; feature stays at c0 → fast-forward.
	checkout(t, wt, "main")
	wcommitFile(t, wt, dir, "m.txt", "main\n")
	mainTip := wcommit(t, wt, "main commit")
	checkout(t, wt, "feature")

	st, err := git.MergeIntoFeature(dir, "main", "feature")
	if err != nil {
		t.Fatalf("MergeIntoFeature: %v", err)
	}
	if !st.Clean || st.MergedSHA != mainTip.String() {
		t.Errorf("expected fast-forward to %s, got %+v", mainTip, st)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.txt")); err != nil {
		t.Errorf("main's file missing from worktree after fast-forward: %v", err)
	}
}

func TestMergeIntoFeature_CleanThreeWay(t *testing.T) {
	dir, repo, wt := initWorktreeRepo(t, map[string]string{"base.txt": "base\n"})
	// main edits a.txt; feature adds b.txt — non-overlapping.
	checkout(t, wt, "main")
	wcommitFile(t, wt, dir, "a.txt", "a-main\n")
	wcommit(t, wt, "main edit")
	checkout(t, wt, "feature")
	wcommitFile(t, wt, dir, "b.txt", "b-feat\n")
	wcommit(t, wt, "feature add")

	st, err := git.MergeIntoFeature(dir, "main", "feature")
	if err != nil {
		t.Fatalf("MergeIntoFeature: %v", err)
	}
	if !st.Clean || st.MergedSHA == "" {
		t.Fatalf("expected clean merge commit, got %+v", st)
	}
	// Worktree must hold both files.
	for _, f := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s missing from worktree after merge: %v", f, err)
		}
	}
	// The new feature HEAD must be a two-parent merge commit.
	h := plumbing.NewHash(st.MergedSHA)
	c, err := repo.CommitObject(h)
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	if c.NumParents() != 2 {
		t.Errorf("merge commit parents = %d, want 2", c.NumParents())
	}
}

func TestMergeIntoFeature_Conflict(t *testing.T) {
	dir, _, wt := initWorktreeRepo(t, map[string]string{"shared.txt": "base\n"})
	checkout(t, wt, "main")
	wcommitFile(t, wt, dir, "shared.txt", "main-version\n")
	wcommit(t, wt, "main edit shared")
	checkout(t, wt, "feature")
	wcommitFile(t, wt, dir, "shared.txt", "feature-version\n")
	wcommit(t, wt, "feature edit shared")

	st, err := git.MergeIntoFeature(dir, "main", "feature")
	if err != nil {
		t.Fatalf("MergeIntoFeature: %v", err)
	}
	if st.Clean {
		t.Fatalf("expected conflict, got clean merge %+v", st)
	}
	if len(st.ConflictPaths) != 1 || st.ConflictPaths[0] != "shared.txt" {
		t.Fatalf("ConflictPaths = %v, want [shared.txt]", st.ConflictPaths)
	}
	// The worktree file must carry conflict markers with both sides.
	data, err := os.ReadFile(filepath.Join(dir, "shared.txt"))
	if err != nil {
		t.Fatalf("read shared.txt: %v", err)
	}
	body := string(data)
	for _, want := range []string{"<<<<<<< feature", "feature-version", "=======", "main-version", ">>>>>>> main"} {
		if !strings.Contains(body, want) {
			t.Errorf("conflict file missing %q; got:\n%s", want, body)
		}
	}
}

func TestCommitMerge_TwoParentCommit(t *testing.T) {
	dir, repo, wt := initWorktreeRepo(t, map[string]string{"shared.txt": "base\n"})
	checkout(t, wt, "main")
	wcommitFile(t, wt, dir, "shared.txt", "main-version\n")
	wcommit(t, wt, "main edit")
	checkout(t, wt, "feature")
	wcommitFile(t, wt, dir, "shared.txt", "feature-version\n")
	wcommit(t, wt, "feature edit")

	if _, err := git.MergeIntoFeature(dir, "main", "feature"); err != nil {
		t.Fatalf("MergeIntoFeature: %v", err)
	}
	// Resolve the conflict by hand, then finalise.
	if err := os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolution: %v", err)
	}
	sha, err := git.CommitMerge(dir, "feature", "main", "merge resolved", "Agent", "agent@system")
	if err != nil {
		t.Fatalf("CommitMerge: %v", err)
	}
	c, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	if c.NumParents() != 2 {
		t.Errorf("resolved merge parents = %d, want 2", c.NumParents())
	}
	data, _ := os.ReadFile(filepath.Join(dir, "shared.txt"))
	if string(data) != "resolved\n" {
		t.Errorf("resolved content = %q, want resolved", data)
	}
}
