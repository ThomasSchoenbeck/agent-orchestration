package workflow

import (
	"context"
	"os"
	"testing"
	"time"

	"agent-orchestrator/db"
)

// openTestDB opens a fresh in-memory SQLite database for each test.
func openTestDB(t *testing.T) *db.Database {
	t.Helper()
	f, err := os.CreateTemp("", "workflow_test_*.db")
	if err != nil {
		t.Fatalf("create temp db file: %v", err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	f.Close()

	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// createProject inserts a test project and returns its ID.
func createProject(t *testing.T, d *db.Database, name string) string {
	t.Helper()
	p := &db.Project{Name: name, Status: db.TaskStatusBacklog}
	if err := d.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p.ID
}

// --- RetryDelay ---

func TestRetryDelay(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, time.Minute},
		{1, time.Minute},
		{2, 2 * time.Minute},
		{3, 4 * time.Minute},
		{4, 8 * time.Minute},
	}
	for _, c := range cases {
		got := RetryDelay(c.attempt)
		if got != c.want {
			t.Errorf("RetryDelay(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}

// --- HandleReviewResult ---

func TestScheduler_HandleReviewResult_Approved(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "TestProject2")
	s := NewScheduler(d, 300, 3, time.Minute)

	implTask := &db.Task{
		ProjectID: projectID,
		Role:      "worker",
		Status:    db.TaskStatusCompleted,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(context.Background(), implTask); err != nil {
		t.Fatalf("create impl task: %v", err)
	}

	reviewTask := &db.Task{
		ProjectID: projectID,
		Role:      "reviewer",
		Status:    db.TaskStatusReviewing,
		Payload:   map[string]interface{}{"parent_task_id": implTask.ID},
	}
	if err := d.CreateTask(context.Background(), reviewTask); err != nil {
		t.Fatalf("create review task: %v", err)
	}

	if err := s.HandleReviewResult(context.Background(), reviewTask, "approved"); err != nil {
		t.Fatalf("HandleReviewResult: %v", err)
	}

	updated, err := d.GetTask(context.Background(), reviewTask.ID)
	if err != nil {
		t.Fatalf("get review task: %v", err)
	}
	if updated.Status != db.TaskStatusAwaitingMerge {
		t.Errorf("expected review task status %q, got %q", db.TaskStatusAwaitingMerge, updated.Status)
	}
}

func TestScheduler_HandleReviewResult_Changes(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "TestProject3")
	s := NewScheduler(d, 300, 3, time.Minute)

	implTask := &db.Task{
		ProjectID: projectID,
		Role:      "worker",
		Status:    db.TaskStatusDeveloping,
		Attempts:  1,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(context.Background(), implTask); err != nil {
		t.Fatalf("create impl task: %v", err)
	}

	reviewTask := &db.Task{
		ProjectID: projectID,
		Role:      "reviewer",
		Status:    db.TaskStatusReviewing,
		Payload:   map[string]interface{}{"parent_task_id": implTask.ID},
	}
	if err := d.CreateTask(context.Background(), reviewTask); err != nil {
		t.Fatalf("create review task: %v", err)
	}

	if err := s.HandleReviewResult(context.Background(), reviewTask, "changes"); err != nil {
		t.Fatalf("HandleReviewResult: %v", err)
	}

	updated, err := d.GetTask(context.Background(), reviewTask.ID)
	if err != nil {
		t.Fatalf("get review task: %v", err)
	}
	if updated.Status != db.TaskStatusAwaitingRevision {
		t.Errorf("expected review task status %q, got %q", db.TaskStatusAwaitingRevision, updated.Status)
	}
}

func TestScheduler_HandleReviewResult_UnknownOutcome(t *testing.T) {
	d := openTestDB(t)
	s := NewScheduler(d, 300, 3, time.Minute)
	reviewTask := &db.Task{ID: "fake", Status: db.TaskStatusReviewing, Payload: map[string]interface{}{}}
	err := s.HandleReviewResult(context.Background(), reviewTask, "unknown_outcome")
	if err == nil {
		t.Error("expected error for unknown review outcome")
	}
}

// --- Tick / retry ---

func TestScheduler_Tick_RetriesFailedTask(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "RetryProject")
	s := NewScheduler(d, 300, 3, time.Minute)

	task := &db.Task{
		ProjectID: projectID,
		Role:      "worker",
		Status:    db.TaskStatusFailed,
		Attempts:  1,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	// Push updated_at into the past so retry delay is satisfied.
	_, err := d.RawDB().ExecContext(context.Background(),
		`UPDATE tasks SET updated_at=datetime('now','-2 minutes') WHERE id=?`, task.ID)
	if err != nil {
		t.Fatalf("backdate task: %v", err)
	}

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	updated, err := d.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != db.TaskStatusBacklog {
		t.Errorf("expected task retried to %q, got %q", db.TaskStatusBacklog, updated.Status)
	}
}

func TestScheduler_Tick_DoesNotRetryMaxAttempts(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "MaxRetryProject")
	s := NewScheduler(d, 300, 3, time.Minute)

	task := &db.Task{
		ProjectID: projectID,
		Role:      "worker",
		Status:    db.TaskStatusFailed,
		Attempts:  3, // already at max
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, _ = d.RawDB().ExecContext(context.Background(),
		`UPDATE tasks SET updated_at=datetime('now','-10 minutes') WHERE id=?`, task.ID)

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	updated, _ := d.GetTask(context.Background(), task.ID)
	if updated.Status != db.TaskStatusFailed {
		t.Errorf("expected max-retry task to remain %q, got %q", db.TaskStatusFailed, updated.Status)
	}
}
