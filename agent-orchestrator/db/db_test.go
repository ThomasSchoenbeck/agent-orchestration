package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"agent-orchestrator/db"
)

func openTestDB(t *testing.T) *db.Database {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestOpenAndMigrate(t *testing.T) {
	d := openTestDB(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

// --- Project CRUD ---

func TestProjectCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Create
	p := &db.Project{
		Name:        "Test Project",
		Description: "A test project",
		RepoPath:    "/tmp/repo",
	}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected non-empty ID after create")
	}

	// Get
	got, err := d.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Name != p.Name {
		t.Errorf("expected name %q, got %q", p.Name, got.Name)
	}

	// Update
	got.Status = "in_progress"
	if err := d.UpdateProject(ctx, got); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	updated, err := d.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProject after update: %v", err)
	}
	if updated.Status != "in_progress" {
		t.Errorf("expected status in_progress, got %s", updated.Status)
	}

	// List
	list, err := d.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 project, got %d", len(list))
	}

	// Delete
	if err := d.DeleteProject(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	list, _ = d.ListProjects(ctx)
	if len(list) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(list))
	}
}

// --- Task CRUD ---

func TestTaskCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Need a project first
	p := &db.Project{Name: "P"}
	_ = d.CreateProject(ctx, p)

	// Create task
	task := &db.Task{
		ProjectID: p.ID,
		Type:      "implement",
		Role:      "worker",
		Priority:  5,
		Payload:   map[string]interface{}{"desc": "do something"},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Get
	got, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Role != "worker" {
		t.Errorf("expected role worker, got %s", got.Role)
	}

	// List with filter
	tasks, err := d.ListTasks(ctx, db.TaskFilters{ProjectID: p.ID})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}

	// Claim
	agent := &db.Agent{Name: "worker-1", Roles: []string{"worker"}}
	_ = d.CreateAgent(ctx, agent)
	if err := d.ClaimTask(ctx, task.ID, agent.ID); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	// Double-claim should fail
	if err := d.ClaimTask(ctx, task.ID, agent.ID); err == nil {
		t.Fatal("expected error on double-claim, got nil")
	}

	// Submit result
	if err := d.SubmitTaskResult(ctx, task.ID, map[string]interface{}{"output": "done"}, "completed"); err != nil {
		t.Fatalf("SubmitTaskResult: %v", err)
	}
	final, _ := d.GetTask(ctx, task.ID)
	if final.Status != "completed" {
		t.Errorf("expected status completed, got %s", final.Status)
	}
}

// --- Agent CRUD ---

func TestAgentCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	a := &db.Agent{
		Name:  "test-agent",
		Roles: []string{"worker", "reviewer"},
	}
	if err := d.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	got, err := d.GetAgent(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if len(got.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(got.Roles))
	}

	// Heartbeat
	if err := d.UpdateHeartbeat(ctx, a.ID); err != nil {
		t.Fatalf("UpdateHeartbeat: %v", err)
	}

	// List
	agents, err := d.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

// --- GetNextTask ---

func TestGetNextTask(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	p := &db.Project{Name: "P"}
	_ = d.CreateProject(ctx, p)

	// Create two tasks with different priorities
	t1 := &db.Task{ProjectID: p.ID, Type: "implement", Role: "worker", Priority: 1}
	t2 := &db.Task{ProjectID: p.ID, Type: "implement", Role: "worker", Priority: 10}
	_ = d.CreateTask(ctx, t1)
	_ = d.CreateTask(ctx, t2)

	// Should get highest priority first
	next, err := d.GetNextTask(ctx, []string{"worker"})
	if err != nil {
		t.Fatalf("GetNextTask: %v", err)
	}
	if next == nil {
		t.Fatal("expected task, got nil")
	}
	if next.ID != t2.ID {
		t.Errorf("expected high-priority task %s, got %s", t2.ID, next.ID)
	}

	// No matching role
	none, err := d.GetNextTask(ctx, []string{"reviewer"})
	if err != nil {
		t.Fatalf("GetNextTask: %v", err)
	}
	if none != nil {
		t.Error("expected nil for unmatched role")
	}
}
