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
	p := &db.Project{Name: name, Status: "planned"}
	if err := d.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p.ID
}

// --- State machine tests ---

func TestIsValidTransition(t *testing.T) {
	valid := []struct{ from, to TaskStatus }{
		{StatusPlanned, StatusInProgress},
		{StatusInProgress, StatusCompleted},
		{StatusInProgress, StatusFailed},
		{StatusInProgress, StatusNeedsReview},
		{StatusNeedsReview, StatusApproved},
		{StatusNeedsReview, StatusInProgress},
		{StatusApproved, StatusCompleted},
		{StatusFailed, StatusPlanned},
	}
	for _, c := range valid {
		if !IsValidTransition(c.from, c.to) {
			t.Errorf("expected valid transition %s→%s", c.from, c.to)
		}
	}
	// Invalid transitions.
	invalid := []struct{ from, to TaskStatus }{
		{StatusCompleted, StatusPlanned},
		{StatusPlanned, StatusCompleted},
		{StatusApproved, StatusPlanned},
		{StatusFailed, StatusCompleted},
	}
	for _, c := range invalid {
		if IsValidTransition(c.from, c.to) {
			t.Errorf("expected invalid transition %s→%s", c.from, c.to)
		}
	}
}

func TestValidateTransition_Error(t *testing.T) {
	err := ValidateTransition(StatusCompleted, StatusPlanned)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}

func TestValidateTransition_OK(t *testing.T) {
	if err := ValidateTransition(StatusPlanned, StatusInProgress); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- FollowOnType tests ---

func TestFollowOnType(t *testing.T) {
	cases := []struct {
		src     TaskType
		outcome string
		want    TaskType
		wantOK  bool
	}{
		{TypeImplement, "completed", TypeReview, true},
		{TypeReview, "approved", TypeTest, true},
		{TypeReview, "changes", "", false},
		{TypeTest, "completed", "", false},
		{TypePlan, "completed", "", false},
	}
	for _, c := range cases {
		got, ok := FollowOnType(c.src, c.outcome)
		if ok != c.wantOK || got != c.want {
			t.Errorf("FollowOnType(%q,%q) = (%q,%v), want (%q,%v)",
				c.src, c.outcome, got, ok, c.want, c.wantOK)
		}
	}
}

// --- RoleForType ---

func TestRoleForType(t *testing.T) {
	cases := []struct{ typ TaskType; want string }{
		{TypePlan, "orchestrator"},
		{TypeImplement, "worker"},
		{TypeReview, "reviewer"},
		{TypeTest, "worker"},
		{"unknown", "worker"},
	}
	for _, c := range cases {
		if got := RoleForType(c.typ); got != c.want {
			t.Errorf("RoleForType(%q) = %q, want %q", c.typ, got, c.want)
		}
	}
}

// --- RetryDelay ---

func TestRetryDelay(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, time.Minute},  // edge case
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

// --- CreateFollowOnTask ---

func TestScheduler_CreateFollowOnTask(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "TestProject")
	s := NewScheduler(d, 300, 3, time.Minute)

	parent := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    "completed",
		Priority:  5,
		Payload:   map[string]interface{}{"description": "build feature"},
	}
	if err := d.CreateTask(context.Background(), parent); err != nil {
		t.Fatalf("create parent task: %v", err)
	}

	if err := s.CreateFollowOnTask(context.Background(), parent, TypeReview); err != nil {
		t.Fatalf("CreateFollowOnTask: %v", err)
	}

	tasks, err := d.ListTasks(context.Background(), db.TaskFilters{ProjectID: projectID, Status: "planned"})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 follow-on task, got %d", len(tasks))
	}
	followOn := tasks[0]
	if followOn.Type != "review" {
		t.Errorf("expected type %q, got %q", "review", followOn.Type)
	}
	if followOn.Role != "reviewer" {
		t.Errorf("expected role %q, got %q", "reviewer", followOn.Role)
	}
	if followOn.Priority != 5 {
		t.Errorf("expected inherited priority 5, got %d", followOn.Priority)
	}
	if followOn.Payload["parent_task_id"] != parent.ID {
		t.Errorf("expected parent_task_id %q in payload, got %v", parent.ID, followOn.Payload["parent_task_id"])
	}
}

// --- HandleReviewResult ---

func TestScheduler_HandleReviewResult_Approved(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "TestProject2")
	s := NewScheduler(d, 300, 3, time.Minute)

	// Create a completed implement task.
	implTask := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    "completed",
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(context.Background(), implTask); err != nil {
		t.Fatalf("create impl task: %v", err)
	}

	// Create a review task.
	reviewTask := &db.Task{
		ProjectID: projectID,
		Type:      "review",
		Role:      "reviewer",
		Status:    "in_progress",
		Payload:   map[string]interface{}{"parent_task_id": implTask.ID},
	}
	if err := d.CreateTask(context.Background(), reviewTask); err != nil {
		t.Fatalf("create review task: %v", err)
	}

	if err := s.HandleReviewResult(context.Background(), reviewTask, "approved"); err != nil {
		t.Fatalf("HandleReviewResult: %v", err)
	}

	// Review task should now be "approved".
	updated, err := d.GetTask(context.Background(), reviewTask.ID)
	if err != nil {
		t.Fatalf("get review task: %v", err)
	}
	if updated.Status != "approved" {
		t.Errorf("expected review task status %q, got %q", "approved", updated.Status)
	}

	// A test task should have been created.
	planned, err := d.ListTasks(context.Background(), db.TaskFilters{ProjectID: projectID, Status: "planned"})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(planned) != 1 || planned[0].Type != "test" {
		t.Errorf("expected 1 planned test task, got %d tasks", len(planned))
	}
}

func TestScheduler_HandleReviewResult_Changes(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "TestProject3")
	s := NewScheduler(d, 300, 3, time.Minute)

	// Create implement task (completed).
	implTask := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    "completed",
		Attempts:  1,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(context.Background(), implTask); err != nil {
		t.Fatalf("create impl task: %v", err)
	}

	reviewTask := &db.Task{
		ProjectID: projectID,
		Type:      "review",
		Role:      "reviewer",
		Status:    "in_progress",
		Payload:   map[string]interface{}{"parent_task_id": implTask.ID},
	}
	if err := d.CreateTask(context.Background(), reviewTask); err != nil {
		t.Fatalf("create review task: %v", err)
	}

	if err := s.HandleReviewResult(context.Background(), reviewTask, "changes"); err != nil {
		t.Fatalf("HandleReviewResult: %v", err)
	}

	// Review task should now be "needs_review".
	updated, err := d.GetTask(context.Background(), reviewTask.ID)
	if err != nil {
		t.Fatalf("get review task: %v", err)
	}
	if updated.Status != "needs_review" {
		t.Errorf("expected status %q, got %q", "needs_review", updated.Status)
	}

	// Implement task should be re-queued to "planned".
	impl, err := d.GetTask(context.Background(), implTask.ID)
	if err != nil {
		t.Fatalf("get impl task: %v", err)
	}
	if impl.Status != "planned" {
		t.Errorf("expected implement task re-queued to planned, got %q", impl.Status)
	}
}

func TestScheduler_HandleReviewResult_UnknownOutcome(t *testing.T) {
	d := openTestDB(t)
	s := NewScheduler(d, 300, 3, time.Minute)
	reviewTask := &db.Task{ID: "fake", Status: "in_progress", Payload: map[string]interface{}{}}
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

	// Create a failed task with 1 attempt that was updated >1 min ago.
	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    "failed",
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
	if updated.Status != "planned" {
		t.Errorf("expected task retried to %q, got %q", "planned", updated.Status)
	}
}

func TestScheduler_Tick_DoesNotRetryMaxAttempts(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "MaxRetryProject")
	s := NewScheduler(d, 300, 3, time.Minute)

	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    "failed",
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
	if updated.Status != "failed" {
		t.Errorf("expected max-retry task to remain %q, got %q", "failed", updated.Status)
	}
}
