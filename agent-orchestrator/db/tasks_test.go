package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

// TestUpdateTask_StatusChangeLogged verifies that UpdateTask records
// the correct old_status and new_status in the task log entry.
func TestUpdateTask_StatusChangeLogged(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()

	// Create a project and task.
	p := &db.Project{Name: "p1", RepoPath: "/tmp/r"}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	task := &db.Task{
		ProjectID: p.ID,
		Role:      "worker",
		Status:    db.TaskStatusBacklog,
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Update task — change status from BACKLOG to DEVELOPING.
	task.Status = db.TaskStatusDeveloping
	if err := d.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	// Retrieve task logs and find the task_updated entry.
	logs, err := d.ListTaskLogs(ctx, db.TaskLogFilters{TaskID: task.ID, Limit: 50})
	if err != nil {
		t.Fatalf("ListTaskLogs: %v", err)
	}

	var found bool
	for _, l := range logs {
		if l.EventType == "task_updated" {
			found = true
			if l.OldStatus != db.TaskStatusBacklog {
				t.Errorf("task_updated: OldStatus = %q, want %q", l.OldStatus, db.TaskStatusBacklog)
			}
			if l.NewStatus != db.TaskStatusDeveloping {
				t.Errorf("task_updated: NewStatus = %q, want %q", l.NewStatus, db.TaskStatusDeveloping)
			}
		}
	}
	if !found {
		t.Errorf("no task_updated log entry found; got %d entries", len(logs))
	}
}

// TestUpdateTask_NoStatusChange verifies that UpdateTask still logs
// when only non-status fields change, using identical old/new status.
func TestUpdateTask_NoStatusChange(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()

	p := &db.Project{Name: "p2", RepoPath: "/tmp/r2"}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	task := &db.Task{
		ProjectID: p.ID,
		Role:      "worker",
		Priority:  3,
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Update only priority, keep status the same.
	task.Priority = 7
	if err := d.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	logs, err := d.ListTaskLogs(ctx, db.TaskLogFilters{TaskID: task.ID, Limit: 50})
	if err != nil {
		t.Fatalf("ListTaskLogs: %v", err)
	}

	var found bool
	for _, l := range logs {
		if l.EventType == "task_updated" {
			found = true
			// OldStatus and NewStatus should both be BACKLOG (unchanged).
			if l.OldStatus != db.TaskStatusBacklog {
				t.Errorf("OldStatus = %q, want %q", l.OldStatus, db.TaskStatusBacklog)
			}
			if l.NewStatus != db.TaskStatusBacklog {
				t.Errorf("NewStatus = %q, want %q", l.NewStatus, db.TaskStatusBacklog)
			}
		}
	}
	if !found {
		t.Errorf("no task_updated log entry found")
	}
}
