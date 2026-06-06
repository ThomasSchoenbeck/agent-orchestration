package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestSeedAgentTemplates_Idempotent(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	tpls := []*db.AgentTemplate{
		{Name: "worker", Roles: []string{"worker"}, Replicas: 2, Enabled: true},
		{Name: "reviewer", Roles: []string{"reviewer"}, Replicas: 1, Enabled: true},
	}
	n, err := d.SeedAgentTemplates(ctx, tpls)
	if err != nil {
		t.Fatalf("SeedAgentTemplates: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 seeded, got %d", n)
	}
	// Re-seeding (same names) inserts nothing.
	n2, err := d.SeedAgentTemplates(ctx, tpls)
	if err != nil {
		t.Fatalf("SeedAgentTemplates (2nd): %v", err)
	}
	if n2 != 0 {
		t.Errorf("expected 0 on re-seed, got %d", n2)
	}
	count, _ := d.CountAgentTemplates(ctx)
	if count != 2 {
		t.Errorf("expected 2 templates total, got %d", count)
	}
}

func TestAgentTemplate_CRUDRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	tpl := &db.AgentTemplate{
		Name:      "backend-worker",
		Roles:     []string{"worker"},
		Skills:    []string{"backend", "go"},
		Replicas:  3,
		Autostart: true,
		Enabled:   true,
	}
	if err := d.CreateAgentTemplate(ctx, tpl); err != nil {
		t.Fatalf("CreateAgentTemplate: %v", err)
	}
	if tpl.ID == "" {
		t.Fatal("expected generated ID")
	}

	got, err := d.GetAgentTemplate(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("GetAgentTemplate: %v", err)
	}
	if got.Name != "backend-worker" || got.Replicas != 3 || !got.Autostart {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if len(got.Skills) != 2 {
		t.Errorf("skills = %v, want 2", got.Skills)
	}

	got.Replicas = 5
	got.Autostart = false
	if err := d.UpdateAgentTemplate(ctx, got); err != nil {
		t.Fatalf("UpdateAgentTemplate: %v", err)
	}
	again, _ := d.GetAgentTemplate(ctx, tpl.ID)
	if again.Replicas != 5 || again.Autostart {
		t.Errorf("update not persisted: %+v", again)
	}

	if err := d.SetTemplateReplicas(ctx, tpl.ID, 2); err != nil {
		t.Fatalf("SetTemplateReplicas: %v", err)
	}
	again, _ = d.GetAgentTemplate(ctx, tpl.ID)
	if again.Replicas != 2 {
		t.Errorf("replicas = %d, want 2", again.Replicas)
	}

	list, _ := d.ListAgentTemplates(ctx)
	if len(list) != 1 {
		t.Fatalf("ListAgentTemplates = %d, want 1", len(list))
	}

	if err := d.DeleteAgentTemplate(ctx, tpl.ID); err != nil {
		t.Fatalf("DeleteAgentTemplate: %v", err)
	}
	if _, err := d.GetAgentTemplate(ctx, tpl.ID); err == nil {
		t.Error("expected error after delete")
	}
}

func TestListAgentsByTemplate(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	tplID := "tpl-123"
	if err := d.CreateAgent(ctx, &db.Agent{Name: "bw-1", Roles: []string{"worker"}, TemplateID: tplID}); err != nil {
		t.Fatalf("CreateAgent bw-1: %v", err)
	}
	if err := d.CreateAgent(ctx, &db.Agent{Name: "bw-2", Roles: []string{"worker"}, TemplateID: tplID}); err != nil {
		t.Fatalf("CreateAgent bw-2: %v", err)
	}
	// A remote agent with no template.
	if err := d.CreateAgent(ctx, &db.Agent{Name: "remote-1", Roles: []string{"worker"}}); err != nil {
		t.Fatalf("CreateAgent remote-1: %v", err)
	}

	instances, err := d.ListAgentsByTemplate(ctx, tplID)
	if err != nil {
		t.Fatalf("ListAgentsByTemplate: %v", err)
	}
	if len(instances) != 2 {
		t.Errorf("expected 2 template instances, got %d", len(instances))
	}
	for _, a := range instances {
		if a.TemplateID != tplID {
			t.Errorf("instance %q has template_id %q", a.Name, a.TemplateID)
		}
	}
}
