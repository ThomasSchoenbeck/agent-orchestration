package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestRoleDefinitionListAndDelete(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	r := &db.RoleDefinition{Name: "custom", Label: "Custom", Enabled: true}
	if err := d.CreateRoleDefinition(ctx, r); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}
	if list, err := d.ListRoleDefinitions(ctx); err != nil || len(list) == 0 {
		t.Fatalf("ListRoleDefinitions = %d (err=%v)", len(list), err)
	}
	if err := d.DeleteRoleDefinition(ctx, r.ID); err != nil {
		t.Fatalf("DeleteRoleDefinition: %v", err)
	}
	if _, err := d.GetRoleDefinition(ctx, r.ID); err == nil {
		t.Error("role should be gone after delete")
	}
}

func TestSkillDefinitionListByNamesDelete(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	s := &db.SkillDefinition{Name: "backend", Label: "Backend", Enabled: true}
	if err := d.CreateSkillDefinition(ctx, s); err != nil {
		t.Fatalf("CreateSkillDefinition: %v", err)
	}
	if list, err := d.ListSkillDefinitions(ctx); err != nil || len(list) == 0 {
		t.Fatalf("ListSkillDefinitions = %d (err=%v)", len(list), err)
	}
	byNames, err := d.SkillsByNames(ctx, []string{"backend"})
	if err != nil || len(byNames) != 1 {
		t.Fatalf("SkillsByNames = %d (err=%v), want 1", len(byNames), err)
	}
	if err := d.DeleteSkillDefinition(ctx, s.ID); err != nil {
		t.Fatalf("DeleteSkillDefinition: %v", err)
	}
}

func TestSubagentSkillList(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if err := d.CreateSubagentSkill(ctx, &db.SubagentSkill{Name: "investigate", Label: "Investigate"}); err != nil {
		t.Fatalf("CreateSubagentSkill: %v", err)
	}
	if list, err := d.ListSubagentSkills(ctx); err != nil || len(list) == 0 {
		t.Errorf("ListSubagentSkills = %d (err=%v)", len(list), err)
	}
}

func TestListTaskCommentsHelper(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)
	taskID := newTaskInProject(t, d, pid)
	if err := d.CreateComment(ctx, &db.TaskComment{TaskID: taskID, AuthorType: "user", Body: "hi"}); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if list, err := d.ListTaskComments(ctx, taskID); err != nil || len(list) != 1 {
		t.Errorf("ListTaskComments = %d (err=%v), want 1", len(list), err)
	}
}

func TestAgentUpdateDeleteOffline(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Update + template id.
	a := &db.Agent{Name: "w1", Roles: []string{"worker"}, Status: "online"}
	if err := d.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	a.Status = "idle"
	if err := d.UpdateAgent(ctx, a); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if got, _ := d.GetAgent(ctx, a.ID); got.Status != "idle" {
		t.Errorf("UpdateAgent status = %q, want idle", got.Status)
	}
	if err := d.SetAgentTemplateID(ctx, a.ID, "tmpl-1"); err != nil {
		t.Fatalf("SetAgentTemplateID: %v", err)
	}

	// Offline + stale cleanup (timeout 0 ⇒ everything is stale).
	if _, err := d.MarkOfflineAgents(ctx, 0); err != nil {
		t.Fatalf("MarkOfflineAgents: %v", err)
	}
	if _, err := d.DeleteStaleOfflineAgents(ctx, 0); err != nil {
		t.Fatalf("DeleteStaleOfflineAgents: %v", err)
	}

	// Explicit delete on a separate agent.
	b := &db.Agent{Name: "w2", Roles: []string{"worker"}, Status: "online"}
	if err := d.CreateAgent(ctx, b); err != nil {
		t.Fatalf("CreateAgent b: %v", err)
	}
	if err := d.DeleteAgent(ctx, b.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
}

func TestProjectBySlug(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	p := &db.Project{Name: "Proj", Slug: "proj-slug"}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if got, err := d.GetProjectBySlug(ctx, "proj-slug"); err != nil || got.ID != p.ID {
		t.Errorf("GetProjectBySlug: %v", err)
	}
	if got, err := d.GetProjectBySlugOrID(ctx, "proj-slug"); err != nil || got.ID != p.ID {
		t.Errorf("GetProjectBySlugOrID(slug): %v", err)
	}
	if got, err := d.GetProjectBySlugOrID(ctx, p.ID); err != nil || got.ID != p.ID {
		t.Errorf("GetProjectBySlugOrID(id): %v", err)
	}
	if _, err := d.GetProjectBySlug(ctx, "missing"); err == nil {
		t.Error("GetProjectBySlug(missing) should error")
	}
}

func TestLogsCreateAndList(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if err := d.CreateLog(ctx, &db.LogEntry{Level: "info", Message: "system up"}); err != nil {
		t.Fatalf("CreateLog: %v", err)
	}
	if list, err := d.ListLogs(ctx, db.LogFilters{Limit: 10}); err != nil || len(list) == 0 {
		t.Errorf("ListLogs = %d (err=%v)", len(list), err)
	}
	if _, err := d.ListLogs(ctx, db.LogFilters{Limit: 10, SystemOnly: true}); err != nil {
		t.Errorf("ListLogs SystemOnly: %v", err)
	}
}

func TestGetTaskCostEmpty(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)
	taskID := newTaskInProject(t, d, pid)
	if _, err := d.GetTaskCost(ctx, taskID); err != nil {
		t.Errorf("GetTaskCost: %v", err)
	}
}

func TestUpdateContextEmbedding(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)
	e := &db.ContextEntry{ProjectID: pid, Type: "note", Content: "x"}
	if err := d.CreateContextEntry(ctx, e); err != nil {
		t.Fatalf("CreateContextEntry: %v", err)
	}
	if err := d.UpdateContextEmbedding(ctx, e.ID, []float32{0.1, 0.2, 0.3}); err != nil {
		t.Errorf("UpdateContextEmbedding: %v", err)
	}
}

func TestSeedDefaultPlatformSettings(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if err := d.SeedDefaultPlatformSettings(ctx); err != nil {
		t.Fatalf("SeedDefaultPlatformSettings: %v", err)
	}
	if list, err := d.ListSettings(ctx); err != nil || len(list) == 0 {
		t.Errorf("expected default settings seeded, got %d (err=%v)", len(list), err)
	}
}

func TestDeleteTask(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)
	taskID := newTaskInProject(t, d, pid)
	if err := d.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := d.GetTask(ctx, taskID); err == nil {
		t.Error("GetTask after delete should error")
	}
}
