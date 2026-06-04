package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func sliceHas(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// TestRenameRoleReferences_CascadesAcrossTables verifies that renaming a role
// rewrites every stored reference (providers + per-model roles, agents +
// start_roles, templates, tasks) and leaves unrelated roles untouched.
func TestRenameRoleReferences_CascadesAcrossTables(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	prov := &db.Provider{
		Name: "p", Type: "ollama", BaseURL: "http://x", Enabled: true,
		Roles: []string{"worker", "reviewer"},
		Models: []db.ProviderModel{
			{Name: "m1", Roles: []string{"worker"}},
			{Name: "m2", Roles: []string{"reviewer"}},
		},
	}
	if err := d.CreateProvider(ctx, prov); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	ag := &db.Agent{Name: "a1", Roles: []string{"worker"}, StartRoles: []string{"worker"}, Mode: "colocated", Status: "online"}
	if err := d.CreateAgent(ctx, ag); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	tpl := &db.AgentTemplate{Name: "t1", Roles: []string{"worker"}, Replicas: 1, Enabled: true}
	if err := d.CreateAgentTemplate(ctx, tpl); err != nil {
		t.Fatalf("CreateAgentTemplate: %v", err)
	}

	task := &db.Task{ProjectID: "proj1", Role: "worker", ReviewRole: "worker", Status: db.TaskStatusBacklog}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := d.RenameRoleReferences(ctx, "worker", "builder"); err != nil {
		t.Fatalf("RenameRoleReferences: %v", err)
	}

	// Provider: provider-level + model-level roles renamed; reviewer untouched.
	gotProv, err := d.GetProvider(ctx, prov.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if !sliceHas(gotProv.Roles, "builder") || sliceHas(gotProv.Roles, "worker") {
		t.Errorf("provider roles = %v, want builder (not worker)", gotProv.Roles)
	}
	if !sliceHas(gotProv.Roles, "reviewer") {
		t.Errorf("provider roles dropped reviewer: %v", gotProv.Roles)
	}
	var m1 []string
	for _, m := range gotProv.Models {
		if m.Name == "m1" {
			m1 = m.Roles
		}
	}
	if !sliceHas(m1, "builder") || sliceHas(m1, "worker") {
		t.Errorf("model m1 roles = %v, want builder", m1)
	}

	// Agent: roles + start_roles renamed.
	gotAg, err := d.GetAgentByName(ctx, "a1")
	if err != nil {
		t.Fatalf("GetAgentByName: %v", err)
	}
	if !sliceHas(gotAg.Roles, "builder") || !sliceHas(gotAg.StartRoles, "builder") {
		t.Errorf("agent roles=%v start_roles=%v, want builder", gotAg.Roles, gotAg.StartRoles)
	}

	// Template.
	gotTpl, err := d.GetAgentTemplate(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("GetAgentTemplate: %v", err)
	}
	if !sliceHas(gotTpl.Roles, "builder") {
		t.Errorf("template roles = %v, want builder", gotTpl.Roles)
	}

	// Task.
	gotTask, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if gotTask.Role != "builder" || gotTask.ReviewRole != "builder" {
		t.Errorf("task role=%q review_role=%q, want builder", gotTask.Role, gotTask.ReviewRole)
	}
}

func TestRoleIDByName_RoundTrips(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if err := d.CreateRoleDefinition(ctx, &db.RoleDefinition{Name: "worker", Label: "Worker", Enabled: true}); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}
	id, err := d.RoleIDByName(ctx, "worker")
	if err != nil || id == "" {
		t.Fatalf("RoleIDByName: id=%q err=%v", id, err)
	}
	name, err := d.RoleNameByID(ctx, id)
	if err != nil || name != "worker" {
		t.Fatalf("RoleNameByID: name=%q err=%v", name, err)
	}
	if missing, _ := d.RoleIDByName(ctx, "nope"); missing != "" {
		t.Errorf("expected empty id for unknown role, got %q", missing)
	}
}
