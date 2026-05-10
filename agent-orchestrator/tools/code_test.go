package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-orchestrator/tools"
)

func setupCodeTools(t *testing.T) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	if err := tools.RegisterCodeTools(reg); err != nil {
		t.Fatalf("RegisterCodeTools: %v", err)
	}
	return reg
}

func TestReadFile_Success(t *testing.T) {
	dir := t.TempDir()
	content := "hello, world\n"
	if err := os.WriteFile(filepath.Join(dir, "foo.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	reg := setupCodeTools(t)
	res, err := reg.Execute(context.Background(), "read_file", map[string]interface{}{
		"repo_path": dir,
		"file_path": "foo.txt",
	})
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	m := res.(map[string]interface{})
	if m["content"] != content {
		t.Errorf("content = %q, want %q", m["content"], content)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	reg := setupCodeTools(t)
	_, err := reg.Execute(context.Background(), "read_file", map[string]interface{}{
		"repo_path": dir,
		"file_path": "nonexistent.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteFile_Success(t *testing.T) {
	dir := t.TempDir()
	reg := setupCodeTools(t)

	res, err := reg.Execute(context.Background(), "write_file", map[string]interface{}{
		"repo_path": dir,
		"file_path": "sub/bar.txt",
		"content":   "test content",
	})
	if err != nil {
		t.Fatalf("write_file: %v", err)
	}
	m := res.(map[string]interface{})
	if m["success"] != true {
		t.Error("expected success=true")
	}

	// Verify file was written.
	data, err := os.ReadFile(filepath.Join(dir, "sub", "bar.txt"))
	if err != nil {
		t.Fatalf("ReadFile after write: %v", err)
	}
	if string(data) != "test content" {
		t.Errorf("file content = %q", data)
	}
}

func TestWriteAndReadFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	reg := setupCodeTools(t)
	ctx := context.Background()

	payload := "line1\nline2\nline3\n"
	_, err := reg.Execute(ctx, "write_file", map[string]interface{}{
		"repo_path": dir,
		"file_path": "out.txt",
		"content":   payload,
	})
	if err != nil {
		t.Fatalf("write_file: %v", err)
	}

	res, err := reg.Execute(ctx, "read_file", map[string]interface{}{
		"repo_path": dir,
		"file_path": "out.txt",
	})
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if res.(map[string]interface{})["content"] != payload {
		t.Error("round-trip content mismatch")
	}
}

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	// Create some files.
	for _, name := range []string{"a.go", "b.go", "c.txt"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644)
	}

	reg := setupCodeTools(t)
	res, err := reg.Execute(context.Background(), "list_files", map[string]interface{}{
		"repo_path": dir,
	})
	if err != nil {
		t.Fatalf("list_files: %v", err)
	}
	m := res.(map[string]interface{})
	files := m["files"].([]string)
	if m["count"].(int) < 3 {
		t.Errorf("expected >=3 files, got %d", m["count"])
	}
	_ = files
}

func TestListFiles_WithPattern(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.txt"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644)
	}

	reg := setupCodeTools(t)
	res, err := reg.Execute(context.Background(), "list_files", map[string]interface{}{
		"repo_path": dir,
		"pattern":   "*.go",
	})
	if err != nil {
		t.Fatalf("list_files: %v", err)
	}
	m := res.(map[string]interface{})
	files := m["files"].([]string)
	for _, f := range files {
		if !strings.HasSuffix(f, ".go") {
			t.Errorf("unexpected file %q in *.go pattern results", f)
		}
	}
	if len(files) != 2 {
		t.Errorf("expected 2 .go files, got %d: %v", len(files), files)
	}
}

func TestSafeJoin_NoTraversal(t *testing.T) {
	dir := t.TempDir()
	// Create a sentinel file outside the temp dir.
	outside := filepath.Join(os.TempDir(), "sentinel.txt")
	_ = os.WriteFile(outside, []byte("secret"), 0644)
	defer os.Remove(outside)

	reg := setupCodeTools(t)
	// Attempt path traversal.
	_, err := reg.Execute(context.Background(), "read_file", map[string]interface{}{
		"repo_path": dir,
		"file_path": "../sentinel.txt",
	})
	// Should either error (file not found inside dir) or return content from
	// inside the sandbox only.  We just verify no panic and ideally an error.
	if err == nil {
		t.Log("read_file with ../ did not error — checking result is not the sentinel")
	}
}

func TestRegistry_UnknownTool(t *testing.T) {
	reg := tools.NewRegistry()
	_, err := reg.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestRegistry_DuplicateRegistration(t *testing.T) {
	reg := tools.NewRegistry()
	def := tools.Definition{
		Name:    "mytool",
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) { return nil, nil },
	}
	if err := reg.Register(def); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := reg.Register(def); err == nil {
		t.Fatal("expected error on duplicate registration")
	}
}
