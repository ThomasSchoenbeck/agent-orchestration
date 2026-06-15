package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

func newCtxTestServer(t *testing.T) (*Server, *db.Database) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
		Database: config.DatabaseConfig{Path: dbPath},
		Storage:  config.StorageConfig{Root: t.TempDir()},
	}
	return New(cfg, database, llm.NewRegistry()), database
}

func TestBuildProjectContext(t *testing.T) {
	s, database := newCtxTestServer(t)
	ctx := context.Background()

	proj := &db.Project{Name: "P"}
	if err := database.CreateProject(ctx, proj); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	for _, st := range []string{db.TaskStatusBacklog, db.TaskStatusDeveloping} {
		if err := database.CreateTask(ctx, &db.Task{ProjectID: proj.ID, Role: "worker", Status: st}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
	}
	_ = database.CreateContextEntry(ctx, &db.ContextEntry{ProjectID: proj.ID, Type: "note", Content: "design decision"})

	out := s.buildProjectContext(ctx, proj)
	if !strings.Contains(out, "Project has 2 tasks") {
		t.Errorf("context missing task summary; got: %q", out)
	}
}

func TestBuildTaskSystemPrompt(t *testing.T) {
	s, database := newCtxTestServer(t)
	ctx := context.Background()

	proj := &db.Project{Name: "P"}
	if err := database.CreateProject(ctx, proj); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task := &db.Task{
		ProjectID: proj.ID, Role: "worker", Status: db.TaskStatusDeveloping,
		Payload: map[string]interface{}{"title": "Build login", "description": "OAuth flow"},
	}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	req := &db.ProjectRequirement{ProjectID: proj.ID, Title: "Auth"}
	if err := database.CreateRequirement(ctx, req); err != nil {
		t.Fatalf("CreateRequirement: %v", err)
	}
	if _, err := database.AddTaskLink(ctx, task.ID, "requirement", req.ID); err != nil {
		t.Fatalf("AddTaskLink: %v", err)
	}

	out := s.buildTaskSystemPrompt(ctx, task)
	for _, want := range []string{"Build login", "Description:", "OAuth flow", "Linked requirements"} {
		if !strings.Contains(out, want) {
			t.Errorf("system prompt missing %q; got:\n%s", want, out)
		}
	}
}
