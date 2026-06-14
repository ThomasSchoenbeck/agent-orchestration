package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestRoleDefinition_ResyncPromptRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	r := &db.RoleDefinition{
		Name: "orchestrator", Label: "Orchestrator",
		Capabilities: []string{"creates_tasks"},
		ResyncPrompt: "reconcile scope then plan work",
		Enabled:      true,
	}
	if err := d.CreateRoleDefinition(ctx, r); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	got, err := d.GetRoleDefinitionByName(ctx, "orchestrator")
	if err != nil {
		t.Fatalf("GetRoleDefinitionByName: %v", err)
	}
	if got.ResyncPrompt != "reconcile scope then plan work" {
		t.Errorf("resync_prompt = %q", got.ResyncPrompt)
	}

	got.ResyncPrompt = "updated prompt"
	if err := d.UpdateRoleDefinition(ctx, got); err != nil {
		t.Fatalf("UpdateRoleDefinition: %v", err)
	}
	again, _ := d.GetRoleDefinition(ctx, got.ID)
	if again.ResyncPrompt != "updated prompt" {
		t.Errorf("resync_prompt after update = %q", again.ResyncPrompt)
	}
}
