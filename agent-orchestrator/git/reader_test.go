package git_test

import (
	"path/filepath"
	"testing"

	"agent-orchestrator/git"
)

// initRepoWithFile creates a bare repo with one committed file at filePath and returns the repo path.
func initRepoWithFile(t *testing.T, filePath string, content []byte) string {
	t.Helper()
	repoPath := filepath.Join(tempDir(t), "test.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "main", filePath, content, "initial commit", "test", "test@example.com"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	return repoPath
}

func TestListBranches_EmptyRepo(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "empty.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	branches, err := git.ListBranches(repoPath)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 0 {
		t.Errorf("expected 0 branches in empty repo, got %d", len(branches))
	}
}

func TestListBranches_ReturnsBranch(t *testing.T) {
	repoPath := initRepoWithFile(t, "README.md", []byte("hello"))

	branches, err := git.ListBranches(repoPath)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 || branches[0] != "main" {
		t.Errorf("expected [main], got %v", branches)
	}
}

func TestReadTree_Root(t *testing.T) {
	repoPath := initRepoWithFile(t, "hello.txt", []byte("world"))

	nodes, err := git.ReadTree(repoPath, "main", "")
	if err != nil {
		t.Fatalf("ReadTree: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d: %+v", len(nodes), nodes)
	}
	if nodes[0].Name != "hello.txt" {
		t.Errorf("Name = %q, want hello.txt", nodes[0].Name)
	}
	if nodes[0].Type != "blob" {
		t.Errorf("Type = %q, want blob", nodes[0].Type)
	}
	if nodes[0].Size != 5 {
		t.Errorf("Size = %d, want 5", nodes[0].Size)
	}
}

func TestReadTree_Subdir(t *testing.T) {
	repoPath := initRepoWithFile(t, "src/main.go", []byte("package main"))

	// Root should show "src" as a tree.
	nodes, err := git.ReadTree(repoPath, "main", "")
	if err != nil {
		t.Fatalf("ReadTree root: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "src" || nodes[0].Type != "tree" {
		t.Fatalf("expected root to contain src (tree), got %+v", nodes)
	}

	// Expanding src should show main.go.
	sub, err := git.ReadTree(repoPath, "main", "src")
	if err != nil {
		t.Fatalf("ReadTree src: %v", err)
	}
	if len(sub) != 1 || sub[0].Name != "main.go" {
		t.Fatalf("expected [main.go], got %+v", sub)
	}
	if sub[0].Path != "src/main.go" {
		t.Errorf("Path = %q, want src/main.go", sub[0].Path)
	}
}

func TestReadFile_Success(t *testing.T) {
	repoPath := initRepoWithFile(t, "data.txt", []byte("contents here"))

	data, err := git.ReadFile(repoPath, "main", "data.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "contents here" {
		t.Errorf("content = %q, want %q", string(data), "contents here")
	}
}

func TestReadFile_NotFound(t *testing.T) {
	repoPath := initRepoWithFile(t, "exists.txt", []byte("x"))

	_, err := git.ReadFile(repoPath, "main", "missing.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadFile_BinaryDetected(t *testing.T) {
	// File with a null byte should return ErrBinaryFile.
	repoPath := initRepoWithFile(t, "bin.dat", []byte{0x00, 0x01, 0x02})

	_, err := git.ReadFile(repoPath, "main", "bin.dat")
	if err != git.ErrBinaryFile {
		t.Errorf("expected ErrBinaryFile, got %v", err)
	}
}

func TestReadTree_EmptyRepo(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "empty.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	nodes, err := git.ReadTree(repoPath, "main", "")
	if err != nil {
		t.Fatalf("ReadTree on empty repo: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected empty slice for empty repo, got %d nodes", len(nodes))
	}
}

func TestReadTree_NonexistentSubpath(t *testing.T) {
	repoPath := initRepoWithFile(t, "a.txt", []byte("x"))

	_, err := git.ReadTree(repoPath, "main", "nosuchdir")
	if err == nil {
		t.Error("expected error for nonexistent subpath, got nil")
	}
}
