package workflow

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func newQueueSupervisor(d *db.Database) *QueueSupervisor {
	return &QueueSupervisor{database: d, intervalSec: 10, planRoundCeiling: defaultPlanRoundCeiling}
}

// armedProject creates an auto-queue-armed, active project.
func armedProject(t *testing.T, d *db.Database, p *db.Project) string {
	t.Helper()
	p.AutoQueue = true
	if p.Status == "" {
		p.Status = "active"
	}
	if p.Name == "" {
		p.Name = "armed"
	}
	if err := d.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p.ID
}

func plannerTaskCount(t *testing.T, d *db.Database, projectID string) int {
	t.Helper()
	tasks, err := d.ListTasks(context.Background(), db.TaskFilters{ProjectID: projectID, Role: "planner"})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	return len(tasks)
}

func TestQueueSupervisor_ReplenishesOnDrain(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := armedProject(t, d, &db.Project{Name: "drain"})

	newQueueSupervisor(d).TickOnce(ctx)

	if n := plannerTaskCount(t, d, pid); n != 1 {
		t.Fatalf("expected exactly 1 planner task on drain, got %d", n)
	}
	// plan_rounds incremented.
	got, _ := d.GetProject(ctx, pid)
	if got.PlanRounds != 1 {
		t.Errorf("plan_rounds = %d, want 1", got.PlanRounds)
	}
}

func TestQueueSupervisor_NoReplenishWhileWorkOpen(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := armedProject(t, d, &db.Project{Name: "busy"})

	// One open (BACKLOG) worker task → work is flowing.
	if err := d.CreateTask(ctx, &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusBacklog}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	newQueueSupervisor(d).TickOnce(ctx)

	if n := plannerTaskCount(t, d, pid); n != 0 {
		t.Errorf("expected no planner task while work is open, got %d", n)
	}
}

func TestQueueSupervisor_RespectsMaxOpenTasks(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := armedProject(t, d, &db.Project{Name: "capped", MaxOpenTasks: 2})

	// Two open tasks → at the cap.
	for i := 0; i < 2; i++ {
		if err := d.CreateTask(ctx, &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusBacklog}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
	}

	newQueueSupervisor(d).TickOnce(ctx)

	if n := plannerTaskCount(t, d, pid); n != 0 {
		t.Errorf("expected no planner task at/over max_open_tasks, got %d", n)
	}
}

func TestQueueSupervisor_RespectsPlanRoundCeiling(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := armedProject(t, d, &db.Project{Name: "ceiling", PlanRounds: 3})

	qs := newQueueSupervisor(d)
	qs.planRoundCeiling = 3 // already reached

	qs.TickOnce(ctx)

	if n := plannerTaskCount(t, d, pid); n != 0 {
		t.Errorf("expected no planner task once ceiling reached, got %d", n)
	}
}

func TestReArm_TriggersImprovementMode(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := armedProject(t, d, &db.Project{Name: "rearm"})

	// Scope exists and is fully satisfied (a done feature, no open tasks) →
	// a re-armed project should plan in improvement mode.
	if err := d.CreateFeature(ctx, &db.ProjectFeature{ProjectID: pid, Title: "Shipped", Status: db.FeatStatusDone}); err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	newQueueSupervisor(d).TickOnce(ctx)

	tasks, _ := d.ListTasks(ctx, db.TaskFilters{ProjectID: pid, Role: "planner"})
	if len(tasks) != 1 {
		t.Fatalf("expected 1 planner task, got %d", len(tasks))
	}
	if mode, _ := tasks[0].Payload["mode"].(string); mode != "improvement" {
		t.Errorf("planner mode = %q, want improvement", mode)
	}
}

func TestQueueSupervisor_InitialModeForFreshProject(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := armedProject(t, d, &db.Project{Name: "fresh"})

	newQueueSupervisor(d).TickOnce(ctx)

	tasks, _ := d.ListTasks(ctx, db.TaskFilters{ProjectID: pid, Role: "planner"})
	if len(tasks) != 1 {
		t.Fatalf("expected 1 planner task, got %d", len(tasks))
	}
	if mode, _ := tasks[0].Payload["mode"].(string); mode != "initial" {
		t.Errorf("planner mode = %q, want initial", mode)
	}
}

// TestAutoQueue_Terminates drives a full bounded cycle: drain → planner enqueued
// → the planner declares completion (status=complete, auto_queue off) → the
// supervisor no longer enqueues. The loop provably ends.
func TestAutoQueue_Terminates(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := armedProject(t, d, &db.Project{Name: "terminates"})

	qs := newQueueSupervisor(d)

	// Round 1: drain → one planner task.
	qs.TickOnce(ctx)
	if n := plannerTaskCount(t, d, pid); n != 1 {
		t.Fatalf("round 1: expected 1 planner task, got %d", n)
	}

	// Simulate the planner finishing and declaring the project complete.
	planner, _ := d.ListTasks(ctx, db.TaskFilters{ProjectID: pid, Role: "planner"})
	planner[0].Status = db.TaskStatusCompleted
	if err := d.UpdateTask(ctx, planner[0]); err != nil {
		t.Fatalf("complete planner task: %v", err)
	}
	p, _ := d.GetProject(ctx, pid)
	p.Status = "complete"
	p.AutoQueue = false
	if err := d.UpdateProject(ctx, p); err != nil {
		t.Fatalf("complete project: %v", err)
	}

	// Further ticks must not enqueue anything — the project is disarmed.
	qs.TickOnce(ctx)
	qs.TickOnce(ctx)
	if n := plannerTaskCount(t, d, pid); n != 1 {
		t.Errorf("expected no further planner tasks after completion, got %d", n)
	}
}
