package server

import (
	"context"
	"path/filepath"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// TestResolveTaskBranch verifies T5: the claim path generates a branch from the
// task type's template (slugified title), and appends a short id when another
// task in the project already owns that branch.
func TestResolveTaskBranch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	bug := &db.TaskType{Key: "bug", Label: "Bug", BranchTemplate: "bug/{slug}", IsDefault: true}
	if err := database.CreateTaskType(ctx, bug); err != nil {
		t.Fatalf("CreateTaskType: %v", err)
	}

	s := New(&config.Config{Storage: config.StorageConfig{Root: t.TempDir()}}, database, llm.NewRegistry())

	task := &db.Task{
		ID: "t1", ProjectID: "p1", TaskTypeID: bug.ID,
		Payload: map[string]interface{}{"title": "Add login button"},
	}
	if got := s.resolveTaskBranch(ctx, task); got != "bug/add-login-button" {
		t.Errorf("branch = %q, want bug/add-login-button", got)
	}

	// A different task already owns that branch → collision suffix (short id).
	owner := &db.Task{ID: "owner", ProjectID: "p1", Branch: "bug/add-login-button", Status: db.TaskStatusDeveloping}
	if err := database.CreateTask(ctx, owner); err != nil {
		t.Fatalf("CreateTask owner: %v", err)
	}
	task2 := &db.Task{
		ID: "abcdefgh1234", ProjectID: "p1", TaskTypeID: bug.ID,
		Payload: map[string]interface{}{"title": "Add login button"},
	}
	if got := s.resolveTaskBranch(ctx, task2); got != "bug/add-login-button-abcdefgh" {
		t.Errorf("collision branch = %q, want bug/add-login-button-abcdefgh", got)
	}
}
