package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestRoleDefinition_ModelsRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	role := &db.RoleDefinition{
		Name:    "worker",
		Label:   "Worker",
		Enabled: true,
		Models: []db.ModelRef{
			{Provider: "anthropic", Model: "claude-x"},
			{Provider: "ollama", Model: "qwen"},
		},
	}
	if err := d.CreateRoleDefinition(ctx, role); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	got, err := d.GetRoleDefinitionByName(ctx, "worker")
	if err != nil {
		t.Fatalf("GetRoleDefinitionByName: %v", err)
	}
	if len(got.Models) != 2 {
		t.Fatalf("Models len = %d, want 2 (%+v)", len(got.Models), got.Models)
	}
	if got.Models[0] != (db.ModelRef{Provider: "anthropic", Model: "claude-x"}) {
		t.Errorf("Models[0] = %+v", got.Models[0])
	}
	if got.Models[1].Provider != "ollama" || got.Models[1].Model != "qwen" {
		t.Errorf("Models[1] = %+v", got.Models[1])
	}

	// Update replaces the ordered list.
	got.Models = []db.ModelRef{{Provider: "openai", Model: "gpt"}}
	if err := d.UpdateRoleDefinition(ctx, got); err != nil {
		t.Fatalf("UpdateRoleDefinition: %v", err)
	}
	again, _ := d.GetRoleDefinition(ctx, got.ID)
	if len(again.Models) != 1 || again.Models[0].Provider != "openai" {
		t.Errorf("updated Models = %+v", again.Models)
	}
}

func TestRoleDefinition_ModelsDefaultsEmpty(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// No Models set → round-trips as an empty (non-nil) list, not "null".
	if err := d.CreateRoleDefinition(ctx, &db.RoleDefinition{Name: "reviewer", Enabled: true}); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}
	got, _ := d.GetRoleDefinitionByName(ctx, "reviewer")
	if got.Models == nil {
		t.Error("expected non-nil empty Models slice")
	}
	if len(got.Models) != 0 {
		t.Errorf("expected empty Models, got %+v", got.Models)
	}
}

func TestSubagentSkill_ModelsRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	sk := &db.SubagentSkill{
		Name:    "code_subtask",
		Enabled: true,
		Models: []db.ModelRef{
			{Provider: "anthropic", Model: "claude-code"},
			{Provider: "ollama", Model: "qwen-coder"},
		},
	}
	if err := d.CreateSubagentSkill(ctx, sk); err != nil {
		t.Fatalf("CreateSubagentSkill: %v", err)
	}
	got, err := d.GetSubagentSkillByName(ctx, "code_subtask")
	if err != nil {
		t.Fatalf("GetSubagentSkillByName: %v", err)
	}
	if len(got.Models) != 2 || got.Models[0].Model != "claude-code" || got.Models[1].Provider != "ollama" {
		t.Errorf("Models = %+v", got.Models)
	}
}
