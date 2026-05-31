package db_test

import (
	"context"
	"testing"
	"time"

	"agent-orchestrator/db"
)

func TestTaskReviews_CreateAndList(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Create a project and task first.
	proj := &db.Project{Name: "rev-test", Status: "active"}
	if err := d.CreateProject(ctx, proj); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task := &db.Task{
		ProjectID: proj.ID,
		Role:      "worker",
		Status:    db.TaskStatusAwaitingReview,
		Priority:  5,
		Payload:   map[string]interface{}{"title": "Review task"},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rev := &db.TaskReview{
		TaskID:        task.ID,
		AuthorType:    "agent",
		AuthorRole:    "reviewer",
		AuthorID:      "agent-1",
		Status:        "changes_requested",
		Body:          "Please fix the error handling.",
		BranchHeadSHA: "abc123",
	}
	if err := d.CreateTaskReview(ctx, rev); err != nil {
		t.Fatalf("CreateTaskReview: %v", err)
	}
	if rev.ID == "" {
		t.Fatal("expected non-empty review ID")
	}
	if rev.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}

	reviews, err := d.ListTaskReviews(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].Status != "changes_requested" {
		t.Errorf("status = %q, want %q", reviews[0].Status, "changes_requested")
	}
	if reviews[0].Body != "Please fix the error handling." {
		t.Errorf("body mismatch")
	}
}

func TestTaskReviews_GetByID(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	proj := &db.Project{Name: "rev-get-test", Status: "active"}
	_ = d.CreateProject(ctx, proj)
	task := &db.Task{
		ProjectID: proj.ID, Role: "worker",
		Status: db.TaskStatusReviewing, Priority: 1,
		Payload: map[string]interface{}{},
	}
	_ = d.CreateTask(ctx, task)

	rev := &db.TaskReview{
		TaskID:     task.ID,
		AuthorType: "system",
		AuthorRole: "merge_supervisor",
		Status:     "revision_requested",
		Body:       "Merge conflict detected.",
	}
	_ = d.CreateTaskReview(ctx, rev)

	got, err := d.GetTaskReview(ctx, rev.ID)
	if err != nil {
		t.Fatalf("GetTaskReview: %v", err)
	}
	if got.Body != rev.Body {
		t.Errorf("body = %q, want %q", got.Body, rev.Body)
	}
	_ = time.Now() // suppress import
}

func TestTaskReviews_ListEmpty(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	proj := &db.Project{Name: "rev-empty", Status: "active"}
	_ = d.CreateProject(ctx, proj)
	task := &db.Task{
		ProjectID: proj.ID, Role: "worker",
		Status: db.TaskStatusBacklog, Priority: 1, Payload: map[string]interface{}{},
	}
	_ = d.CreateTask(ctx, task)

	reviews, err := d.ListTaskReviews(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskReviews: %v", err)
	}
	if len(reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(reviews))
	}
}
