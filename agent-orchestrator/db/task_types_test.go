package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestTaskType_CRUDAndDefaultInvariant(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	a := &db.TaskType{Key: "normal", Label: "Normal", BranchTemplate: "feature/{slug}", IsDefault: true, SortOrder: 0}
	b := &db.TaskType{Key: "bug", Label: "Bug", BranchTemplate: "bug/{slug}", SortOrder: 1}
	if err := d.CreateTaskType(ctx, a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := d.CreateTaskType(ctx, b); err != nil {
		t.Fatalf("create b: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected a generated id")
	}

	got, err := d.GetTaskType(ctx, a.ID)
	if err != nil || got.Key != "normal" || got.BranchTemplate != "feature/{slug}" || !got.IsDefault {
		t.Fatalf("GetTaskType = %+v err=%v", got, err)
	}
	byKey, err := d.GetTaskTypeByKey(ctx, "bug")
	if err != nil || byKey.ID != b.ID {
		t.Fatalf("GetTaskTypeByKey = %+v err=%v", byKey, err)
	}

	list, err := d.ListTaskTypes(ctx)
	if err != nil || len(list) != 2 || list[0].Key != "normal" || list[1].Key != "bug" {
		t.Fatalf("ListTaskTypes = %+v err=%v (want normal, bug)", list, err)
	}

	// Single-default invariant: promoting b clears a.
	b.IsDefault = true
	if err := d.UpdateTaskType(ctx, b); err != nil {
		t.Fatalf("update b: %v", err)
	}
	def, err := d.GetDefaultTaskType(ctx)
	if err != nil || def.ID != b.ID {
		t.Fatalf("GetDefaultTaskType = %+v err=%v, want b", def, err)
	}
	a2, _ := d.GetTaskType(ctx, a.ID)
	if a2.IsDefault {
		t.Errorf("a should no longer be default after b promoted")
	}

	if err := d.DeleteTaskType(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := d.GetTaskType(ctx, a.ID); err == nil {
		t.Errorf("expected not-found after delete")
	}
}

func TestSeedTaskTypes_DefaultsAndIdempotent(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	n, err := d.SeedTaskTypes(ctx, db.DefaultTaskTypes())
	if err != nil || n != 4 {
		t.Fatalf("SeedTaskTypes = %d err=%v, want 4", n, err)
	}
	if cnt, _ := d.CountTaskTypes(ctx); cnt != 4 {
		t.Errorf("CountTaskTypes = %d, want 4", cnt)
	}
	def, err := d.GetDefaultTaskType(ctx)
	if err != nil || def.Key != "normal" {
		t.Errorf("default = %+v err=%v, want normal", def, err)
	}
	// Re-seeding inserts nothing (idempotent by key).
	n2, err := d.SeedTaskTypes(ctx, db.DefaultTaskTypes())
	if err != nil || n2 != 0 {
		t.Errorf("re-seed = %d err=%v, want 0", n2, err)
	}
}

func TestTask_PersistsTypeAndBranch(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	tt := &db.TaskType{Key: "bug", Label: "Bug", BranchTemplate: "bug/{slug}", IsDefault: true}
	if err := d.CreateTaskType(ctx, tt); err != nil {
		t.Fatalf("create type: %v", err)
	}

	task := &db.Task{
		ProjectID: "p1", Role: "worker", Status: db.TaskStatusDeveloping,
		TaskTypeID: tt.ID, Branch: "bug/add-login-button",
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	got, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.TaskTypeID != tt.ID {
		t.Errorf("TaskTypeID = %q, want %q", got.TaskTypeID, tt.ID)
	}
	if got.Branch != "bug/add-login-button" {
		t.Errorf("Branch = %q, want bug/add-login-button", got.Branch)
	}

	// Git-hook resolution path.
	byBranch, err := d.GetTaskByBranch(ctx, "p1", "bug/add-login-button")
	if err != nil || byBranch.ID != task.ID {
		t.Fatalf("GetTaskByBranch = %+v err=%v", byBranch, err)
	}

	n, err := d.CountTasksUsingType(ctx, tt.ID)
	if err != nil || n != 1 {
		t.Errorf("CountTasksUsingType = %d err=%v, want 1", n, err)
	}

	if _, err := d.GetTaskByBranch(ctx, "p1", "no-such-branch"); err == nil {
		t.Errorf("expected not-found for unknown branch")
	}
}
