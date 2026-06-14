package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"agent-orchestrator/tools"
)

func writeSearchFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"main.go":        "package main\n\nfunc main() { handleRequest() }\n",
		"server.go":      "package main\n\nfunc handleRequest() {}\n",
		"sub/util.go":    "package sub\n\n// TODO: refactor\nfunc Helper() {}\n",
		"README.md":      "# Project\nhandleRequest is the entrypoint.\n",
		"sub/notes.txt":  "nothing relevant here\n",
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// A directory that must be skipped.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("handleRequest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runSearch(t *testing.T, reg *tools.Registry, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	res, err := reg.Execute(context.Background(), "search_files", args)
	if err != nil {
		t.Fatalf("search_files: %v", err)
	}
	return res.(map[string]interface{})
}

func TestSearchFiles_ContentMatches(t *testing.T) {
	dir := writeSearchFixture(t)
	reg := setupCodeTools(t)

	m := runSearch(t, reg, map[string]interface{}{
		"repo_path": dir, "query": "handleRequest",
	})
	matches := m["matches"].([]map[string]interface{})
	if len(matches) == 0 {
		t.Fatal("expected content matches for handleRequest")
	}
	// Every match carries a file + line number, and .git was skipped.
	for _, mm := range matches {
		if mm["file"] == ".git/config" {
			t.Error("search must skip the .git directory")
		}
		if _, ok := mm["line"]; !ok {
			t.Errorf("content match missing line number: %v", mm)
		}
	}
}

func TestSearchFiles_NameMode(t *testing.T) {
	dir := writeSearchFixture(t)
	reg := setupCodeTools(t)

	m := runSearch(t, reg, map[string]interface{}{
		"repo_path": dir, "query": "util", "mode": "name",
	})
	matches := m["matches"].([]map[string]interface{})
	if len(matches) != 1 || matches[0]["file"] != "sub/util.go" {
		t.Fatalf("name search = %v, want [sub/util.go]", matches)
	}
}

func TestSearchFiles_GlobFilter(t *testing.T) {
	dir := writeSearchFixture(t)
	reg := setupCodeTools(t)

	m := runSearch(t, reg, map[string]interface{}{
		"repo_path": dir, "query": "handleRequest", "file_glob": "*.md",
	})
	matches := m["matches"].([]map[string]interface{})
	if len(matches) != 1 || matches[0]["file"] != "README.md" {
		t.Fatalf("glob-filtered search = %v, want only README.md", matches)
	}
}

func TestSearchFiles_IgnoreCase(t *testing.T) {
	dir := writeSearchFixture(t)
	reg := setupCodeTools(t)

	sensitive := runSearch(t, reg, map[string]interface{}{"repo_path": dir, "query": "HANDLEREQUEST"})
	if len(sensitive["matches"].([]map[string]interface{})) != 0 {
		t.Error("case-sensitive search should not match HANDLEREQUEST")
	}
	insensitive := runSearch(t, reg, map[string]interface{}{
		"repo_path": dir, "query": "HANDLEREQUEST", "ignore_case": true,
	})
	if len(insensitive["matches"].([]map[string]interface{})) == 0 {
		t.Error("case-insensitive search should match")
	}
}

func TestSearchFiles_MaxResultsTruncates(t *testing.T) {
	dir := writeSearchFixture(t)
	reg := setupCodeTools(t)

	m := runSearch(t, reg, map[string]interface{}{
		"repo_path": dir, "query": "handleRequest", "max_results": 1,
	})
	if len(m["matches"].([]map[string]interface{})) != 1 {
		t.Errorf("expected exactly 1 match with max_results=1")
	}
	if m["truncated"] != true {
		t.Error("expected truncated=true when results are capped")
	}
}

func TestSearchFiles_InvalidRegex(t *testing.T) {
	dir := writeSearchFixture(t)
	reg := setupCodeTools(t)
	if _, err := reg.Execute(context.Background(), "search_files", map[string]interface{}{
		"repo_path": dir, "query": "([unclosed",
	}); err == nil {
		t.Error("expected an error for an invalid regex")
	}
}
