package agent_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// TestCommitPath_IgnoresAgentContext guards the "task memory is never merged into
// the repo" property: files under the gitignored .agent_context/ scratchpad
// (memory.md/memory.json etc.) must not be staged by the commit path, which
// stages via go-git AddWithOptions{All: true} after a Status() call — exactly
// what CommitAndPush does.
func TestCommitPath_IgnoresAgentContext(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	// Provisioning writes this ignore entry (see server.writeAgentContext).
	mustWrite(t, filepath.Join(dir, ".gitignore"), ".agent_context/\n")
	// Agent memory scratchpad — must NOT be committed.
	if err := os.MkdirAll(filepath.Join(dir, ".agent_context"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agent_context", "memory.md"), "# secret task memory\n")
	mustWrite(t, filepath.Join(dir, ".agent_context", "memory.json"), "{}")
	// Real work — must be committed.
	mustWrite(t, filepath.Join(dir, "foo.go"), "package main\n")

	if _, err := wt.Status(); err != nil { // mirror CommitAndPush: status before add
		t.Fatalf("Status: %v", err)
	}
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		t.Fatalf("AddWithOptions: %v", err)
	}
	h, err := wt.Commit("work", &gogit.CommitOptions{
		Author: &object.Signature{Name: "n", Email: "e@x", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	commit, err := repo.CommitObject(h)
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}

	if _, err := tree.File("foo.go"); err != nil {
		t.Errorf("expected foo.go to be committed, got %v", err)
	}
	for _, p := range []string{".agent_context/memory.md", ".agent_context/memory.json"} {
		if _, err := tree.File(p); err == nil {
			t.Errorf("%s must NOT be committed (it is agent-internal memory)", p)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
