package workflow

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

// helpers: openTestDB and createProject are defined in scheduler_test.go

func createTaskForMerge(t *testing.T, d *db.Database, projectID string) *db.Task {
	t.Helper()
	task := &db.Task{
		ProjectID: projectID,
		Role:      "worker",
		Status:    db.TaskStatusAwaitingMerge,
		Priority:  1,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func newSupervisor(d *db.Database) *MergeSupervisor {
	return &MergeSupervisor{database: d, paths: nil, intervalSec: 10}
}

// --- checkFileLocks ---

func TestCheckFileLocks_NoLocks(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	sup := newSupervisor(d)

	projID := createProject(t, d, "no-locks")
	task := createTaskForMerge(t, d, projID)

	conflict, other := sup.checkFileLocks(ctx, task.ID, []string{"a.go"})
	if conflict {
		t.Errorf("expected no conflict, got conflict with %q", other)
	}
}

func TestCheckFileLocks_OtherTaskLock_NoOverlap(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	sup := newSupervisor(d)

	projID := createProject(t, d, "no-overlap")
	t1 := createTaskForMerge(t, d, projID)
	t2 := createTaskForMerge(t, d, projID)

	// t1 holds a lock on x.go; t2 wants to merge y.go — no overlap.
	_ = d.CreateMergeLock(ctx, t1.ID, []string{"x.go"})

	conflict, _ := sup.checkFileLocks(ctx, t2.ID, []string{"y.go"})
	if conflict {
		t.Error("expected no conflict for non-overlapping files")
	}
}

func TestCheckFileLocks_OtherTaskLock_Overlap(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	sup := newSupervisor(d)

	projID := createProject(t, d, "overlap")
	t1 := createTaskForMerge(t, d, projID)
	t2 := createTaskForMerge(t, d, projID)

	// t1 holds a lock that overlaps with t2's changed files.
	_ = d.CreateMergeLock(ctx, t1.ID, []string{"shared.go", "util.go"})

	conflict, other := sup.checkFileLocks(ctx, t2.ID, []string{"shared.go"})
	if !conflict {
		t.Error("expected conflict, got none")
	}
	if other != t1.ID {
		t.Errorf("conflicting task = %q, want %q", other, t1.ID)
	}
}

func TestCheckFileLocks_OwnLockIgnored(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	sup := newSupervisor(d)

	projID := createProject(t, d, "own-lock")
	task := createTaskForMerge(t, d, projID)

	// Task holds its own merge lock (idempotent re-check scenario).
	_ = d.CreateMergeLock(ctx, task.ID, []string{"a.go"})

	// Checking against itself should not report a conflict.
	conflict, _ := sup.checkFileLocks(ctx, task.ID, []string{"a.go"})
	if conflict {
		t.Error("task should not conflict with its own lock")
	}
}

func TestCheckFileLocks_MultipleOtherLocks(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	sup := newSupervisor(d)

	projID := createProject(t, d, "multi-locks")
	t1 := createTaskForMerge(t, d, projID)
	t2 := createTaskForMerge(t, d, projID)
	t3 := createTaskForMerge(t, d, projID)

	_ = d.CreateMergeLock(ctx, t1.ID, []string{"a.go"})
	_ = d.CreateMergeLock(ctx, t2.ID, []string{"b.go", "c.go"})

	// t3 overlaps only with t2.
	conflict, other := sup.checkFileLocks(ctx, t3.ID, []string{"b.go"})
	if !conflict {
		t.Error("expected conflict")
	}
	if other != t2.ID {
		t.Errorf("conflicting task = %q, want %q", other, t2.ID)
	}
}

// --- acquireMergeLock / releaseMergeLock ---

func TestAcquireAndReleaseMergeLock(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	sup := newSupervisor(d)

	projID := createProject(t, d, "acquire-release")
	task := createTaskForMerge(t, d, projID)

	if err := sup.acquireMergeLock(ctx, task.ID, []string{"foo.go"}); err != nil {
		t.Fatalf("acquireMergeLock: %v", err)
	}

	locks, _ := d.ListMergeLocks(ctx)
	if len(locks) != 1 {
		t.Fatalf("expected 1 lock, got %d", len(locks))
	}

	if err := sup.releaseMergeLock(ctx, task.ID); err != nil {
		t.Fatalf("releaseMergeLock: %v", err)
	}

	locks, _ = d.ListMergeLocks(ctx)
	if len(locks) != 0 {
		t.Errorf("expected 0 locks after release, got %d", len(locks))
	}
}
