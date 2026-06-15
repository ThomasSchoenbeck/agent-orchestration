package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-orchestrator/db"
)

func TestRoleAllowsContextType(t *testing.T) {
	if !roleAllowsContextType(nil, "project_features") {
		t.Error("nil role should allow any context type")
	}
	inc := &db.RoleDefinition{ContextInclude: []string{"project_features"}}
	if roleAllowsContextType(inc, "project_requirements") {
		t.Error("include-list that omits the type should deny it")
	}
	if !roleAllowsContextType(inc, "project_features") {
		t.Error("include-list containing the type should allow it")
	}
	exc := &db.RoleDefinition{ContextExclude: []string{"project_features"}}
	if roleAllowsContextType(exc, "project_features") {
		t.Error("exclude-list containing the type should deny it")
	}
	if !roleAllowsContextType(exc, "project_requirements") {
		t.Error("type not in exclude-list should be allowed")
	}
}

func TestContainsString(t *testing.T) {
	if !containsString([]string{"a", "b"}, "b") {
		t.Error("should find present element")
	}
	if containsString([]string{"a"}, "z") {
		t.Error("should not find absent element")
	}
}

func TestWriteAgentContext_WritesAllFiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	proj := &db.Project{Name: "P", Description: "a project", CodingRules: "use tabs"}
	if err := database.CreateProject(ctx, proj); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	// Role "analyst" is not defined → unknown role → scope is allowed (scope.md written).
	// AWAITING_REVISION status triggers the review-context branch.
	task := &db.Task{
		ProjectID: proj.ID,
		Role:      "analyst",
		Status:    db.TaskStatusAwaitingRevision,
		Payload:   map[string]interface{}{"title": "Build X", "description": "do the thing"},
	}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	review := &db.TaskReview{TaskID: task.ID, AuthorType: "agent", AuthorRole: "reviewer", Status: "revision_requested", Body: "please fix"}
	if err := database.CreateTaskReview(ctx, review); err != nil {
		t.Fatalf("CreateTaskReview: %v", err)
	}
	if err := database.CreateComment(ctx, &db.TaskComment{
		TaskID: task.ID, ReviewID: review.ID, AuthorType: "agent", AuthorRole: "reviewer", Body: "see line 5",
	}); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	wt := t.TempDir()
	if err := writeAgentContext(ctx, database, task, wt); err != nil {
		t.Fatalf("writeAgentContext: %v", err)
	}

	ctxDir := filepath.Join(wt, ".agent_context")
	for _, f := range []string{"task.md", "project_rules.md", "scope.md", "last_review.md", "review_thread.md"} {
		if _, err := os.Stat(filepath.Join(ctxDir, f)); err != nil {
			t.Errorf("expected %s to be written: %v", f, err)
		}
	}

	// task.md carries the title; project_rules.md carries the coding rules.
	taskMD, _ := os.ReadFile(filepath.Join(ctxDir, "task.md"))
	if !strings.Contains(string(taskMD), "Build X") {
		t.Error("task.md missing the task title")
	}
	rulesMD, _ := os.ReadFile(filepath.Join(ctxDir, "project_rules.md"))
	if !strings.Contains(string(rulesMD), "use tabs") {
		t.Error("project_rules.md missing the coding rules")
	}

	// .agent_context/ must be gitignored so provisioning files aren't committed.
	gi, _ := os.ReadFile(filepath.Join(wt, ".gitignore"))
	if !strings.Contains(string(gi), ".agent_context/") {
		t.Error(".gitignore not updated to ignore .agent_context/")
	}
}
