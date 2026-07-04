package server

import (
	"context"
	"strings"
	"testing"

	"agent-orchestrator/db"
)

func TestProjectMergeMutex(t *testing.T) {
	s, _ := newCtxTestServer(t)
	m1 := s.projectMergeMutex("p1")
	m2 := s.projectMergeMutex("p1")
	if m1 != m2 {
		t.Error("projectMergeMutex should return the same mutex for the same project")
	}
	if s.projectMergeMutex("p2") == m1 {
		t.Error("projectMergeMutex should return distinct mutexes per project")
	}
}

func TestSquashCommitInfo(t *testing.T) {
	s, database := newCtxTestServer(t)
	ctx := context.Background()
	pid := newScopeProjectS(t, database)
	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusAwaitingMerge,
		Payload: map[string]interface{}{"title": "Ship it"}}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	msg, name, email := s.squashCommitInfo(ctx, task, "decider-1")
	if !strings.Contains(msg, "task/"+task.ID) || !strings.Contains(msg, "Ship it") {
		t.Errorf("squash message = %q", msg)
	}
	if name != "decider-1" { // no agent record ⇒ falls back to the decider id
		t.Errorf("squash author name = %q, want decider-1", name)
	}
	if email == "" {
		t.Error("squash author email should not be empty")
	}
}

func TestReleaseStaleExecutionTasks(t *testing.T) {
	s, _ := newCtxTestServer(t)
	// No tasks in flight ⇒ no-op, but must not panic.
	s.releaseStaleExecutionTasks(context.Background())
}

func TestRunMaintenance_ReturnsOnCancel(t *testing.T) {
	s, _ := newCtxTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled ⇒ runMaintenance returns immediately
	s.runMaintenance(ctx)
}

// newScopeProjectS creates a project via the db handle for internal tests.
func newScopeProjectS(t *testing.T, d *db.Database) string {
	t.Helper()
	p := &db.Project{Name: "P"}
	if err := d.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p.ID
}
