package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestGetNextTask_DependencyGated(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	p := &db.Project{Name: "dep-gate", Status: "active"}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	a := &db.Task{ProjectID: p.ID, Role: "worker", Status: db.TaskStatusBacklog, Priority: 1}
	if err := d.CreateTask(ctx, a); err != nil {
		t.Fatalf("CreateTask A: %v", err)
	}
	// B has higher priority, so it would be picked first if it were not gated.
	b := &db.Task{ProjectID: p.ID, Role: "worker", Status: db.TaskStatusBacklog, Priority: 9}
	if err := d.CreateTask(ctx, b); err != nil {
		t.Fatalf("CreateTask B: %v", err)
	}
	if _, err := d.AddDependency(ctx, b.ID, a.ID); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	// B is gated by A → the only claimable task is A.
	got, err := d.GetNextTask(ctx, []string{"worker"})
	if err != nil {
		t.Fatalf("GetNextTask: %v", err)
	}
	if got == nil || got.ID != a.ID {
		t.Fatalf("expected task A while B is gated, got %+v", got)
	}

	// Complete A → B becomes claimable.
	a.Status = db.TaskStatusCompleted
	if err := d.UpdateTask(ctx, a); err != nil {
		t.Fatalf("UpdateTask A complete: %v", err)
	}
	got, err = d.GetNextTask(ctx, []string{"worker"})
	if err != nil {
		t.Fatalf("GetNextTask after A complete: %v", err)
	}
	if got == nil || got.ID != b.ID {
		t.Fatalf("expected task B after dependency completed, got %+v", got)
	}
}

func TestAutoQueueHelpers(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	armed := &db.Project{Name: "armed", Status: "active", AutoQueue: true}
	if err := d.CreateProject(ctx, armed); err != nil {
		t.Fatalf("CreateProject armed: %v", err)
	}
	off := &db.Project{Name: "off", Status: "active", AutoQueue: false}
	if err := d.CreateProject(ctx, off); err != nil {
		t.Fatalf("CreateProject off: %v", err)
	}

	list, err := d.ListAutoQueueProjects(ctx)
	if err != nil {
		t.Fatalf("ListAutoQueueProjects: %v", err)
	}
	if len(list) != 1 || list[0].ID != armed.ID {
		t.Fatalf("expected only the armed project, got %d", len(list))
	}

	// Open task count.
	if err := d.CreateTask(ctx, &db.Task{ProjectID: armed.ID, Role: "worker", Status: db.TaskStatusBacklog}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := d.CreateTask(ctx, &db.Task{ProjectID: armed.ID, Role: "worker", Status: db.TaskStatusCompleted}); err != nil {
		t.Fatalf("CreateTask completed: %v", err)
	}
	open, err := d.CountOpenTasks(ctx, armed.ID)
	if err != nil {
		t.Fatalf("CountOpenTasks: %v", err)
	}
	if open != 1 {
		t.Errorf("CountOpenTasks = %d, want 1 (completed excluded)", open)
	}

	// Plan rounds increment.
	if err := d.IncrementPlanRounds(ctx, armed.ID); err != nil {
		t.Fatalf("IncrementPlanRounds: %v", err)
	}
	got, _ := d.GetProject(ctx, armed.ID)
	if got.PlanRounds != 1 {
		t.Errorf("PlanRounds = %d, want 1", got.PlanRounds)
	}
}
