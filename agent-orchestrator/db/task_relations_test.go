package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func newTaskInProject(t *testing.T, d *db.Database, pid string) string {
	t.Helper()
	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusBacklog}
	if err := d.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task.ID
}

func TestTaskComments(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)
	taskID := newTaskInProject(t, d, pid)

	c := &db.TaskComment{TaskID: taskID, AuthorType: "user", AuthorRole: "reviewer", Body: "looks good"}
	if err := d.CreateComment(ctx, c); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if c.ID == "" {
		t.Fatal("CreateComment did not assign an id")
	}

	// ListComments with empty reviewID returns comments not tied to a review.
	comments, err := d.ListComments(ctx, taskID, "")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 1 || comments[0].Body != "looks good" {
		t.Errorf("ListComments = %+v, want one 'looks good'", comments)
	}

	if err := d.DeleteComment(ctx, taskID, c.ID); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if again, _ := d.ListComments(ctx, taskID, ""); len(again) != 0 {
		t.Errorf("after delete ListComments = %d, want 0", len(again))
	}
}

func TestTaskDependencies(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)
	a := newTaskInProject(t, d, pid)
	b := newTaskInProject(t, d, pid)

	if _, err := d.AddDependency(ctx, a, b); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	// Self-dependency is rejected.
	if _, err := d.AddDependency(ctx, a, a); err == nil {
		t.Error("AddDependency on self should error")
	}

	deps, err := d.ListDependencies(ctx, a)
	if err != nil {
		t.Fatalf("ListDependencies: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("ListDependencies = %d, want 1", len(deps))
	}

	if err := d.RemoveDependency(ctx, a, b); err != nil {
		t.Fatalf("RemoveDependency: %v", err)
	}
	if deps, _ := d.ListDependencies(ctx, a); len(deps) != 0 {
		t.Errorf("after remove ListDependencies = %d, want 0", len(deps))
	}
}

func TestTaskProjectLinks(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)
	taskID := newTaskInProject(t, d, pid)

	req := &db.ProjectRequirement{ProjectID: pid, Title: "R"}
	if err := d.CreateRequirement(ctx, req); err != nil {
		t.Fatalf("CreateRequirement: %v", err)
	}

	if _, err := d.AddTaskLink(ctx, taskID, "requirement", req.ID); err != nil {
		t.Fatalf("AddTaskLink: %v", err)
	}
	// Invalid kind is rejected.
	if _, err := d.AddTaskLink(ctx, taskID, "bogus", req.ID); err == nil {
		t.Error("AddTaskLink with invalid kind should error")
	}

	links, err := d.ListTaskLinks(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskLinks: %v", err)
	}
	if len(links) != 1 || links[0].Kind != "requirement" || links[0].TargetID != req.ID {
		t.Errorf("ListTaskLinks = %+v", links)
	}

	if err := d.RemoveTaskLink(ctx, taskID, "requirement", req.ID); err != nil {
		t.Fatalf("RemoveTaskLink: %v", err)
	}
	if links, _ := d.ListTaskLinks(ctx, taskID); len(links) != 0 {
		t.Errorf("after remove ListTaskLinks = %d, want 0", len(links))
	}
}
