package git_test

import (
	"path/filepath"
	"strings"
	"testing"

	"agent-orchestrator/git"
)

func TestFileDiff_ModifiedFile(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "test.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	// Commit v1 to main.
	if _, err := git.CommitFile(repoPath, "main", "hello.txt", []byte("line1\nline2\n"), "v1", "dev", "dev@example.com"); err != nil {
		t.Fatalf("CommitFile v1: %v", err)
	}
	// Commit v2 to a feature branch.
	if _, err := git.CommitFile(repoPath, "feat", "hello.txt", []byte("line1\nline2\nline3\n"), "v2", "dev", "dev@example.com"); err != nil {
		t.Fatalf("CommitFile v2: %v", err)
	}

	diff, err := git.FileDiff(repoPath, "main", "feat", "hello.txt")
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}
	if !strings.Contains(diff, "line3") {
		t.Errorf("expected diff to mention line3, got:\n%s", diff)
	}
}

func TestFileDiff_NoChange(t *testing.T) {
	repoPath := initRepoWithFile(t, "same.txt", []byte("content"))
	// Both refs point to the same commit so diff should be empty.
	diff, err := git.FileDiff(repoPath, "main", "main", "same.txt")
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for same ref, got: %s", diff)
	}
}

func TestBranchDiff_ReturnsPatches(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "test.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	if _, err := git.CommitFile(repoPath, "main", "a.txt", []byte("aaa\n"), "base", "dev", "dev@example.com"); err != nil {
		t.Fatalf("commit base: %v", err)
	}
	// feat branch: modify a.txt and add b.txt.
	if _, err := git.CommitFile(repoPath, "feat", "a.txt", []byte("aaa\nbbb\n"), "change a", "dev", "dev@example.com"); err != nil {
		t.Fatalf("commit feat a: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "feat", "b.txt", []byte("new file\n"), "add b", "dev", "dev@example.com"); err != nil {
		t.Fatalf("commit feat b: %v", err)
	}

	patches, err := git.BranchDiff(repoPath, "main", "feat")
	if err != nil {
		t.Fatalf("BranchDiff: %v", err)
	}
	if len(patches) == 0 {
		t.Fatal("expected at least one FilePatch")
	}

	byPath := map[string]git.FilePatch{}
	for _, p := range patches {
		byPath[p.Path] = p
	}

	if _, ok := byPath["b.txt"]; !ok {
		t.Errorf("expected b.txt in patches, got: %+v", patches)
	}
	if p, ok := byPath["b.txt"]; ok && p.Status != "added" {
		t.Errorf("b.txt status = %q, want added", p.Status)
	}
}

func TestHasOverlap(t *testing.T) {
	tests := []struct {
		name   string
		a      []string
		b      []string
		expect bool
	}{
		{name: "no overlap", a: []string{"a.go", "b.go"}, b: []string{"c.go"}, expect: false},
		{name: "exact overlap", a: []string{"a.go"}, b: []string{"a.go"}, expect: true},
		{name: "partial overlap", a: []string{"a.go", "b.go"}, b: []string{"b.go", "c.go"}, expect: true},
		{name: "empty a", a: []string{}, b: []string{"a.go"}, expect: false},
		{name: "empty b", a: []string{"a.go"}, b: []string{}, expect: false},
		{name: "both empty", a: []string{}, b: []string{}, expect: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := git.HasOverlap(tc.a, tc.b)
			if got != tc.expect {
				t.Errorf("HasOverlap(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.expect)
			}
		})
	}
}
