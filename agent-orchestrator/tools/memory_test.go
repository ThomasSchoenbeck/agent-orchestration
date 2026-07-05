package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/tools"
)

func setupMemoryTools(t *testing.T) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	if err := tools.RegisterMemoryTools(reg); err != nil {
		t.Fatalf("RegisterMemoryTools: %v", err)
	}
	return reg
}

func writeMem(t *testing.T, reg *tools.Registry, dir, section, content, mode string) {
	t.Helper()
	args := map[string]interface{}{"repo_path": dir, "section": section, "content": content}
	if mode != "" {
		args["mode"] = mode
	}
	if _, err := reg.Execute(context.Background(), "write_memory", args); err != nil {
		t.Fatalf("write_memory(%s): %v", section, err)
	}
}

func TestMemory_WriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	reg := setupMemoryTools(t)

	writeMem(t, reg, dir, "summary", "Build the widget", "")
	writeMem(t, reg, dir, "progress", "read the spec", "")
	writeMem(t, reg, dir, "progress", "wrote the parser", "")
	writeMem(t, reg, dir, "open_questions", "which model for review?", "")

	res, err := reg.Execute(context.Background(), "read_memory", map[string]interface{}{"repo_path": dir})
	if err != nil {
		t.Fatalf("read_memory: %v", err)
	}
	mem := res.(map[string]interface{})["memory"].(db.TaskMemoryContent)
	if mem.Summary != "Build the widget" {
		t.Errorf("summary = %q", mem.Summary)
	}
	if len(mem.Progress) != 2 || mem.Progress[1] != "wrote the parser" {
		t.Errorf("progress = %v", mem.Progress)
	}
	if len(mem.OpenQuestions) != 1 {
		t.Errorf("open_questions = %v", mem.OpenQuestions)
	}

	// Both worktree files are written under .agent_context/.
	for _, name := range []string{"memory.json", "memory.md"} {
		if _, err := os.Stat(filepath.Join(dir, ".agent_context", name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
}

func TestMemory_ReplaceMode(t *testing.T) {
	dir := t.TempDir()
	reg := setupMemoryTools(t)

	writeMem(t, reg, dir, "decisions", "first", "")
	writeMem(t, reg, dir, "decisions", "second", "")
	writeMem(t, reg, dir, "decisions", "final only", "replace")

	res, _ := reg.Execute(context.Background(), "read_memory", map[string]interface{}{"repo_path": dir, "section": "decisions"})
	got := res.(map[string]interface{})["value"].([]string)
	if len(got) != 1 || got[0] != "final only" {
		t.Errorf("replace should leave a single entry, got %v", got)
	}
}

func TestMemory_ReadEmptyIsNotError(t *testing.T) {
	dir := t.TempDir()
	reg := setupMemoryTools(t)

	res, err := reg.Execute(context.Background(), "read_memory", map[string]interface{}{"repo_path": dir})
	if err != nil {
		t.Fatalf("read on empty memory should not error: %v", err)
	}
	mem := res.(map[string]interface{})["memory"].(db.TaskMemoryContent)
	if mem.Summary != "" || len(mem.Progress) != 0 {
		t.Errorf("expected empty memory, got %+v", mem)
	}
}

func TestMemory_UnknownSectionErrors(t *testing.T) {
	dir := t.TempDir()
	reg := setupMemoryTools(t)

	_, err := reg.Execute(context.Background(), "write_memory", map[string]interface{}{
		"repo_path": dir, "section": "bogus", "content": "x",
	})
	if err == nil {
		t.Fatal("expected error for unknown section")
	}
}
