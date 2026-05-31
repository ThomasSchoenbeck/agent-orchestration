package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestMergeLocks_CreateAndList(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	proj := &db.Project{Name: "lock-test", Status: "active"}
	_ = d.CreateProject(ctx, proj)
	task := &db.Task{
		ProjectID: proj.ID, Role: "worker",
		Status: db.TaskStatusAwaitingMerge, Priority: 1, Payload: map[string]interface{}{},
	}
	_ = d.CreateTask(ctx, task)

	paths := []string{"pkg/foo.go", "pkg/bar.go"}
	if err := d.CreateMergeLock(ctx, task.ID, paths); err != nil {
		t.Fatalf("CreateMergeLock: %v", err)
	}

	locks, err := d.ListMergeLocks(ctx)
	if err != nil {
		t.Fatalf("ListMergeLocks: %v", err)
	}
	if len(locks) != 1 {
		t.Fatalf("expected 1 lock, got %d", len(locks))
	}
	if locks[0].TaskID != task.ID {
		t.Errorf("task_id = %q, want %q", locks[0].TaskID, task.ID)
	}
	if len(locks[0].Paths) != 2 {
		t.Errorf("paths len = %d, want 2", len(locks[0].Paths))
	}
}

func TestMergeLocks_Delete(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	proj := &db.Project{Name: "lock-del", Status: "active"}
	_ = d.CreateProject(ctx, proj)
	task := &db.Task{
		ProjectID: proj.ID, Role: "worker",
		Status: db.TaskStatusMerging, Priority: 1, Payload: map[string]interface{}{},
	}
	_ = d.CreateTask(ctx, task)

	_ = d.CreateMergeLock(ctx, task.ID, []string{"a.go"})
	if err := d.DeleteMergeLock(ctx, task.ID); err != nil {
		t.Fatalf("DeleteMergeLock: %v", err)
	}

	locks, err := d.ListMergeLocks(ctx)
	if err != nil {
		t.Fatalf("ListMergeLocks after delete: %v", err)
	}
	if len(locks) != 0 {
		t.Errorf("expected 0 locks after delete, got %d", len(locks))
	}
}

func TestMergeLocks_MultipleTasksOrdering(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	proj := &db.Project{Name: "lock-multi", Status: "active"}
	_ = d.CreateProject(ctx, proj)

	t1 := &db.Task{
		ProjectID: proj.ID, Role: "worker",
		Status: db.TaskStatusMerging, Priority: 1, Payload: map[string]interface{}{},
	}
	t2 := &db.Task{
		ProjectID: proj.ID, Role: "worker",
		Status: db.TaskStatusMerging, Priority: 2, Payload: map[string]interface{}{},
	}
	_ = d.CreateTask(ctx, t1)
	_ = d.CreateTask(ctx, t2)

	_ = d.CreateMergeLock(ctx, t1.ID, []string{"x.go"})
	_ = d.CreateMergeLock(ctx, t2.ID, []string{"y.go"})

	locks, err := d.ListMergeLocks(ctx)
	if err != nil {
		t.Fatalf("ListMergeLocks: %v", err)
	}
	if len(locks) != 2 {
		t.Fatalf("expected 2 locks, got %d", len(locks))
	}
	// First lock returned should be t1 (inserted earlier, ORDER BY created_at ASC).
	if locks[0].TaskID != t1.ID {
		t.Errorf("first lock task_id = %q, want %q", locks[0].TaskID, t1.ID)
	}
}

func TestMergeLocks_DeleteNonExistentIsNoOp(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Deleting a lock that doesn't exist should not error.
	if err := d.DeleteMergeLock(ctx, "nonexistent-task-id"); err != nil {
		t.Errorf("DeleteMergeLock nonexistent: %v", err)
	}
}
