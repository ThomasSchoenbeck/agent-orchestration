package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

// seedPRTask creates a project + task and returns them for PR tests.
func seedPRTask(t *testing.T, d *db.Database, ctx context.Context) (*db.Project, *db.Task) {
	t.Helper()
	proj := &db.Project{Name: "pr-test", Status: "active"}
	if err := d.CreateProject(ctx, proj); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task := &db.Task{
		ProjectID: proj.ID,
		Role:      "worker",
		Status:    db.TaskStatusAwaitingMerge,
		Priority:  5,
		Payload:   map[string]interface{}{"title": "PR task"},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return proj, task
}

func TestPullRequests_CreateGetList(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	proj, task := seedPRTask(t, d, ctx)

	pr := &db.PullRequest{
		TaskID:     task.ID,
		ProjectID:  proj.ID,
		Branch:     "task/" + task.ID,
		Title:      "Implement feature",
		Body:       "LGTM, ready to merge",
		AuthorID:   "reviewer-1",
		AuthorName: "Reviewer One",
	}
	if err := d.CreatePR(ctx, pr); err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if pr.ID == "" {
		t.Fatal("expected non-empty PR ID")
	}
	if pr.Base != "main" {
		t.Errorf("Base defaulted to %q, want main", pr.Base)
	}
	if pr.Status != "open" {
		t.Errorf("Status defaulted to %q, want open", pr.Status)
	}
	if pr.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	got, err := d.GetPR(ctx, pr.ID)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if got.Title != "Implement feature" || got.AuthorName != "Reviewer One" {
		t.Errorf("GetPR mismatch: %+v", got)
	}

	list, err := d.ListPRsForTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListPRsForTask: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(list))
	}
}

func TestPullRequests_UpdateStatusAndDecision(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	proj, task := seedPRTask(t, d, ctx)

	pr := &db.PullRequest{TaskID: task.ID, ProjectID: proj.ID, Branch: "task/" + task.ID, Status: "open"}
	if err := d.CreatePR(ctx, pr); err != nil {
		t.Fatalf("CreatePR: %v", err)
	}

	if err := d.UpdatePRStatus(ctx, pr.ID, "approved"); err != nil {
		t.Fatalf("UpdatePRStatus: %v", err)
	}
	got, _ := d.GetPR(ctx, pr.ID)
	if got.Status != "approved" {
		t.Errorf("status = %q, want approved", got.Status)
	}

	if err := d.SetPRDecision(ctx, pr.ID, "merged", "deployer-1", "merged cleanly"); err != nil {
		t.Fatalf("SetPRDecision: %v", err)
	}
	got, _ = d.GetPR(ctx, pr.ID)
	if got.Status != "merged" {
		t.Errorf("status = %q, want merged", got.Status)
	}
	if got.DeciderID != "deployer-1" {
		t.Errorf("decider = %q, want deployer-1", got.DeciderID)
	}
	if got.DecisionBody != "merged cleanly" {
		t.Errorf("decision body = %q", got.DecisionBody)
	}
}

func TestPullRequests_GetMissing(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.GetPR(ctx, "does-not-exist"); err == nil {
		t.Fatal("expected error for missing PR")
	}
}
