package git_test

import (
	"path/filepath"
	"testing"

	"agent-orchestrator/git"
)

func TestCommitFile_FirstCommit(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "test.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	sha, err := git.CommitFile(repoPath, "main", "hello.txt", []byte("hello"), "add hello", "dev", "dev@example.com")
	if err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	if sha == "" {
		t.Error("expected non-empty SHA")
	}

	// Verify we can read it back.
	data, err := git.ReadFile(repoPath, "main", "hello.txt")
	if err != nil {
		t.Fatalf("ReadFile after commit: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want hello", string(data))
	}
}

func TestCommitFile_UpdateExisting(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "test.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	sha1, _ := git.CommitFile(repoPath, "main", "hello.txt", []byte("v1"), "add", "dev", "dev@example.com")
	sha2, err := git.CommitFile(repoPath, "main", "hello.txt", []byte("v2"), "update", "dev", "dev@example.com")
	if err != nil {
		t.Fatalf("second CommitFile: %v", err)
	}
	if sha1 == sha2 {
		t.Error("expected different SHAs for different content")
	}

	data, _ := git.ReadFile(repoPath, "main", "hello.txt")
	if string(data) != "v2" {
		t.Errorf("content = %q, want v2", string(data))
	}
}

func TestCommitFile_NestedPath(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "test.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	_, err := git.CommitFile(repoPath, "main", "src/pkg/code.go", []byte("package pkg"), "add nested", "", "")
	if err != nil {
		t.Fatalf("CommitFile nested: %v", err)
	}

	data, err := git.ReadFile(repoPath, "main", "src/pkg/code.go")
	if err != nil {
		t.Fatalf("ReadFile nested: %v", err)
	}
	if string(data) != "package pkg" {
		t.Errorf("content = %q, want \"package pkg\"", string(data))
	}
}

func TestCommitFile_ParentChain(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "test.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	sha1, _ := git.CommitFile(repoPath, "main", "a.txt", []byte("a"), "c1", "dev", "dev@example.com")
	sha2, _ := git.CommitFile(repoPath, "main", "b.txt", []byte("b"), "c2", "dev", "dev@example.com")

	// Both files should be present after second commit.
	dataA, err := git.ReadFile(repoPath, "main", "a.txt")
	if err != nil {
		t.Fatalf("ReadFile a.txt: %v", err)
	}
	dataB, err := git.ReadFile(repoPath, "main", "b.txt")
	if err != nil {
		t.Fatalf("ReadFile b.txt: %v", err)
	}
	if string(dataA) != "a" || string(dataB) != "b" {
		t.Errorf("a=%q b=%q", dataA, dataB)
	}
	if sha1 == sha2 {
		t.Error("expected distinct commit SHAs")
	}
}
