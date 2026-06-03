package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestSkillDefinition_CRUDRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	sk := &db.SkillDefinition{
		Name:           "backend",
		Label:          "Backend",
		Description:    "Server side",
		PromptFragment: "You focus on backend.",
		ContextInclude: []string{"server/**"},
		AllowedTools:   []string{"run_tests"},
		Enabled:        true,
	}
	if err := d.CreateSkillDefinition(ctx, sk); err != nil {
		t.Fatalf("CreateSkillDefinition: %v", err)
	}
	if sk.ID == "" {
		t.Fatal("expected generated ID")
	}

	got, err := d.GetSkillDefinitionByName(ctx, "backend")
	if err != nil {
		t.Fatalf("GetSkillDefinitionByName: %v", err)
	}
	if got.PromptFragment != "You focus on backend." {
		t.Errorf("prompt fragment = %q", got.PromptFragment)
	}
	if len(got.ContextInclude) != 1 || got.ContextInclude[0] != "server/**" {
		t.Errorf("context include = %v", got.ContextInclude)
	}

	got.PromptFragment = "Updated soul."
	got.Enabled = false
	if err := d.UpdateSkillDefinition(ctx, got); err != nil {
		t.Fatalf("UpdateSkillDefinition: %v", err)
	}
	again, _ := d.GetSkillDefinition(ctx, got.ID)
	if again.PromptFragment != "Updated soul." || again.Enabled {
		t.Errorf("update not persisted: %+v", again)
	}
}

func TestSeedSkills_Idempotent(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	n1, err := d.SeedSkillDefinitions(ctx, db.DefaultSkillDefinitions())
	if err != nil {
		t.Fatalf("seed 1: %v", err)
	}
	if n1 == 0 {
		t.Fatal("expected to seed starter skills")
	}
	n2, err := d.SeedSkillDefinitions(ctx, db.DefaultSkillDefinitions())
	if err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second seed inserted %d, want 0", n2)
	}
}

func mkFocusTask(t *testing.T, d *db.Database, ctx context.Context, projectID string, focus []string) *db.Task {
	t.Helper()
	task := &db.Task{ProjectID: projectID, Role: "worker", Status: db.TaskStatusBacklog, Priority: 5, Focus: focus}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func TestGetNextTask_FocusSubsetMatch(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := &db.Project{Name: "focus", Status: "active"}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task := mkFocusTask(t, d, ctx, p.ID, []string{"frontend"})

	// Frontend-skilled agent claims it.
	got, err := d.GetNextTaskWithSkills(ctx, []string{"worker"}, []string{"frontend", "react"})
	if err != nil {
		t.Fatalf("GetNextTaskWithSkills: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("expected frontend agent to claim the task, got %+v", got)
	}

	// Backend agent does not.
	none, err := d.GetNextTaskWithSkills(ctx, []string{"worker"}, []string{"backend"})
	if err != nil {
		t.Fatalf("GetNextTaskWithSkills backend: %v", err)
	}
	if none != nil {
		t.Errorf("backend agent must not claim frontend-focused task, got %+v", none)
	}
}

func TestGetNextTask_EmptyFocusUnrestricted(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := &db.Project{Name: "nofocus", Status: "active"}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task := mkFocusTask(t, d, ctx, p.ID, nil)

	// Any role-matching agent claims it regardless of skills.
	got, err := d.GetNextTaskWithSkills(ctx, []string{"worker"}, nil)
	if err != nil {
		t.Fatalf("GetNextTaskWithSkills: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("expected unfocused task to be claimable, got %+v", got)
	}
}

func TestGetNextTask_FocusRequiresAllTags(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := &db.Project{Name: "alltags", Status: "active"}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	mkFocusTask(t, d, ctx, p.ID, []string{"frontend", "react"})

	// Agent has only one of the two required tags → no claim.
	none, err := d.GetNextTaskWithSkills(ctx, []string{"worker"}, []string{"frontend"})
	if err != nil {
		t.Fatalf("GetNextTaskWithSkills: %v", err)
	}
	if none != nil {
		t.Errorf("agent missing a required focus tag must not claim, got %+v", none)
	}

	// Agent with both tags claims it.
	got, _ := d.GetNextTaskWithSkills(ctx, []string{"worker"}, []string{"frontend", "react"})
	if got == nil {
		t.Error("agent with all focus tags should claim the task")
	}
}
