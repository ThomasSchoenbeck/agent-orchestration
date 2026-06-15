package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/tools"
)

// session + subagent tools are intercepted by the executor; their registry
// handlers are defensive no-ops that return an error.
func TestSessionAndSubagentAreExecutorHandled(t *testing.T) {
	reg := tools.NewRegistry()
	if err := tools.RegisterSessionTool(reg); err != nil {
		t.Fatalf("RegisterSessionTool: %v", err)
	}
	if err := tools.RegisterSubagentTool(reg); err != nil {
		t.Fatalf("RegisterSubagentTool: %v", err)
	}
	if _, err := reg.Execute(context.Background(), "checkpoint_session", map[string]interface{}{}); err == nil {
		t.Error("checkpoint_session handler should return an error (executor-handled)")
	}
	if _, err := reg.Execute(context.Background(), "run_subagent", map[string]interface{}{"skill": "x", "instructions": "y"}); err == nil {
		t.Error("run_subagent handler should return an error (executor-handled)")
	}
}

func TestRequestInputTool(t *testing.T) {
	reg := tools.NewRegistry()
	if err := tools.RegisterTaskTools(reg, nil); err != nil { // request_input handler doesn't touch the backend
		t.Fatalf("RegisterTaskTools: %v", err)
	}
	res, err := reg.Execute(context.Background(), "request_input", map[string]interface{}{"question": "which db?"})
	if err != nil {
		t.Fatalf("request_input: %v", err)
	}
	m, _ := res.(map[string]interface{})
	if m["status"] != "awaiting_input" || m["question"] != "which db?" {
		t.Errorf("request_input result = %v", res)
	}
}

func TestWriteAndReadFileTools(t *testing.T) {
	dir := t.TempDir()
	reg := tools.NewRegistry()
	if err := tools.RegisterCodeTools(reg); err != nil {
		t.Fatalf("RegisterCodeTools: %v", err)
	}
	ctx := context.Background()

	// write_file creates parent dirs and writes content.
	if _, err := reg.Execute(ctx, "write_file", map[string]interface{}{
		"repo_path": dir, "file_path": "sub/a.txt", "content": "hello",
	}); err != nil {
		t.Fatalf("write_file: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "sub", "a.txt")); string(b) != "hello" {
		t.Errorf("file content = %q, want hello", b)
	}

	// Missing content → error.
	if _, err := reg.Execute(ctx, "write_file", map[string]interface{}{"repo_path": dir, "file_path": "b.txt"}); err == nil {
		t.Error("write_file without content should error")
	}

	// read_file on a missing file → error.
	if _, err := reg.Execute(ctx, "read_file", map[string]interface{}{"repo_path": dir, "file_path": "nope.txt"}); err == nil {
		t.Error("read_file on missing file should error")
	}
}

func TestTaskCommentTool(t *testing.T) {
	d, _, projectID := openPlanDB(t)
	ctx := context.Background()

	task := &db.Task{ProjectID: projectID, Role: "worker", Status: db.TaskStatusBacklog}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	reg := tools.NewRegistry()
	if err := tools.RegisterCommentTools(reg, toolBackend(t, d)); err != nil {
		t.Fatalf("RegisterCommentTools: %v", err)
	}

	res, err := reg.Execute(ctx, "task_comment", map[string]interface{}{"task_id": task.ID, "body": "looks good"})
	if err != nil {
		t.Fatalf("task_comment: %v", err)
	}
	if m, _ := res.(map[string]string); m["status"] != "posted" {
		t.Errorf("task_comment result = %v", res)
	}

	// Missing body → error (validated before the backend call).
	if _, err := reg.Execute(ctx, "task_comment", map[string]interface{}{"task_id": task.ID}); err == nil {
		t.Error("task_comment without body should error")
	}
}
