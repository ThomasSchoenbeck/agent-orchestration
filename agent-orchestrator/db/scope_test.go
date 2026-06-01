package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func mkProjectForScope(t *testing.T, d *db.Database, ctx context.Context) *db.Project {
	t.Helper()
	p := &db.Project{Name: "scope-test", Status: "active"}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p
}

func mkTaskWithStatus(t *testing.T, d *db.Database, ctx context.Context, projectID, status string) *db.Task {
	t.Helper()
	task := &db.Task{ProjectID: projectID, Role: "worker", Status: status, Priority: 5}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func TestRecomputeScopeStatus_FeatureDoneWhenAllTasksComplete(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := mkProjectForScope(t, d, ctx)

	feat := &db.ProjectFeature{ProjectID: p.ID, Title: "Login"}
	if err := d.CreateFeature(ctx, feat); err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	task := mkTaskWithStatus(t, d, ctx, p.ID, db.TaskStatusCompleted)
	if _, err := d.AddTaskLink(ctx, task.ID, "feature", feat.ID); err != nil {
		t.Fatalf("AddTaskLink: %v", err)
	}

	if err := d.RecomputeLinkedScopeStatus(ctx, task.ID); err != nil {
		t.Fatalf("RecomputeLinkedScopeStatus: %v", err)
	}
	got, _ := d.GetFeature(ctx, feat.ID)
	if got.Status != db.FeatStatusDone {
		t.Errorf("feature status = %q, want %q", got.Status, db.FeatStatusDone)
	}
}

func TestRecomputeScopeStatus_InProgressWhenPartial(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := mkProjectForScope(t, d, ctx)

	feat := &db.ProjectFeature{ProjectID: p.ID, Title: "Login"}
	if err := d.CreateFeature(ctx, feat); err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	done := mkTaskWithStatus(t, d, ctx, p.ID, db.TaskStatusCompleted)
	open := mkTaskWithStatus(t, d, ctx, p.ID, db.TaskStatusDeveloping)
	if _, err := d.AddTaskLink(ctx, done.ID, "feature", feat.ID); err != nil {
		t.Fatalf("AddTaskLink done: %v", err)
	}
	if _, err := d.AddTaskLink(ctx, open.ID, "feature", feat.ID); err != nil {
		t.Fatalf("AddTaskLink open: %v", err)
	}

	if err := d.RecomputeLinkedScopeStatus(ctx, done.ID); err != nil {
		t.Fatalf("RecomputeLinkedScopeStatus: %v", err)
	}
	got, _ := d.GetFeature(ctx, feat.ID)
	if got.Status != db.FeatStatusInProgress {
		t.Errorf("feature status = %q, want %q", got.Status, db.FeatStatusInProgress)
	}
}

func TestRecomputeScopeStatus_NeedsReviewNotOverwritten(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := mkProjectForScope(t, d, ctx)

	feat := &db.ProjectFeature{ProjectID: p.ID, Title: "Login", Status: db.ScopeStatusNeedsReview}
	if err := d.CreateFeature(ctx, feat); err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	task := mkTaskWithStatus(t, d, ctx, p.ID, db.TaskStatusCompleted)
	if _, err := d.AddTaskLink(ctx, task.ID, "feature", feat.ID); err != nil {
		t.Fatalf("AddTaskLink: %v", err)
	}

	if err := d.RecomputeLinkedScopeStatus(ctx, task.ID); err != nil {
		t.Fatalf("RecomputeLinkedScopeStatus: %v", err)
	}
	got, _ := d.GetFeature(ctx, feat.ID)
	if got.Status != db.ScopeStatusNeedsReview {
		t.Errorf("feature status = %q, want needs_review (must not be overwritten)", got.Status)
	}
}

func TestRecomputeScopeStatus_RequirementSatisfied(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := mkProjectForScope(t, d, ctx)

	req := &db.ProjectRequirement{ProjectID: p.ID, Title: "Must auth"}
	if err := d.CreateRequirement(ctx, req); err != nil {
		t.Fatalf("CreateRequirement: %v", err)
	}
	task := mkTaskWithStatus(t, d, ctx, p.ID, db.TaskStatusCompleted)
	if _, err := d.AddTaskLink(ctx, task.ID, "requirement", req.ID); err != nil {
		t.Fatalf("AddTaskLink: %v", err)
	}

	if err := d.RecomputeLinkedScopeStatus(ctx, task.ID); err != nil {
		t.Fatalf("RecomputeLinkedScopeStatus: %v", err)
	}
	got, _ := d.GetRequirement(ctx, req.ID)
	if got.Status != db.ReqStatusSatisfied {
		t.Errorf("requirement status = %q, want %q", got.Status, db.ReqStatusSatisfied)
	}
}

func TestProjectScopeSatisfied(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := mkProjectForScope(t, d, ctx)

	feat := &db.ProjectFeature{ProjectID: p.ID, Title: "F", Status: db.FeatStatusInProgress}
	if err := d.CreateFeature(ctx, feat); err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	// Not satisfied: feature is in_progress.
	ok, reason, err := d.ProjectScopeSatisfied(ctx, p.ID)
	if err != nil {
		t.Fatalf("ProjectScopeSatisfied: %v", err)
	}
	if ok {
		t.Errorf("expected not satisfied while feature in_progress; reason=%q", reason)
	}

	// Mark feature done, no open tasks → satisfied.
	feat.Status = db.FeatStatusDone
	if err := d.UpdateFeature(ctx, feat); err != nil {
		t.Fatalf("UpdateFeature: %v", err)
	}
	ok, reason, err = d.ProjectScopeSatisfied(ctx, p.ID)
	if err != nil {
		t.Fatalf("ProjectScopeSatisfied: %v", err)
	}
	if !ok {
		t.Errorf("expected satisfied; got reason=%q", reason)
	}

	// An open task breaks satisfaction.
	mkTaskWithStatus(t, d, ctx, p.ID, db.TaskStatusDeveloping)
	ok, _, _ = d.ProjectScopeSatisfied(ctx, p.ID)
	if ok {
		t.Error("expected not satisfied with an open task")
	}
}

func TestSetScopeDirty_Roundtrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := mkProjectForScope(t, d, ctx)

	if p.ScopeDirty {
		t.Fatal("new project should not be scope_dirty")
	}
	if err := d.SetScopeDirty(ctx, p.ID, true); err != nil {
		t.Fatalf("SetScopeDirty true: %v", err)
	}
	got, _ := d.GetProject(ctx, p.ID)
	if !got.ScopeDirty {
		t.Error("expected scope_dirty true after SetScopeDirty")
	}
	if err := d.SetScopeDirty(ctx, p.ID, false); err != nil {
		t.Fatalf("SetScopeDirty false: %v", err)
	}
	got, _ = d.GetProject(ctx, p.ID)
	if got.ScopeDirty {
		t.Error("expected scope_dirty false after clear")
	}
}
