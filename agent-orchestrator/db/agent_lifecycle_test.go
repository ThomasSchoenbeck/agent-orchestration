package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestCreateAgent_DefaultsStartAndDesired(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	a := &db.Agent{Name: "w1", Roles: []string{"worker"}, Skills: []string{"backend"}}
	if err := d.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	got, _ := d.GetAgent(ctx, a.ID)
	if len(got.StartRoles) != 1 || got.StartRoles[0] != "worker" {
		t.Errorf("start_roles = %v, want [worker]", got.StartRoles)
	}
	if len(got.StartSkills) != 1 || got.StartSkills[0] != "backend" {
		t.Errorf("start_skills = %v, want [backend]", got.StartSkills)
	}
	if got.DesiredState != "run" {
		t.Errorf("desired_state = %q, want run", got.DesiredState)
	}
}

func TestUpdateAgentLiveConfig_LeavesStart(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	a := &db.Agent{Name: "w2", Roles: []string{"worker"}, Skills: []string{"backend"}}
	if err := d.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	if err := d.UpdateAgentLiveConfig(ctx, a.ID, []string{"worker", "reviewer"}, []string{"frontend"}); err != nil {
		t.Fatalf("UpdateAgentLiveConfig: %v", err)
	}
	got, _ := d.GetAgent(ctx, a.ID)
	if len(got.Roles) != 2 {
		t.Errorf("live roles = %v, want 2", got.Roles)
	}
	// Start params must be untouched.
	if len(got.StartRoles) != 1 || got.StartRoles[0] != "worker" {
		t.Errorf("start_roles changed to %v", got.StartRoles)
	}
	if len(got.StartSkills) != 1 || got.StartSkills[0] != "backend" {
		t.Errorf("start_skills changed to %v", got.StartSkills)
	}
}

func TestResetAgentToStart(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	a := &db.Agent{Name: "w3", Roles: []string{"worker"}, Skills: []string{"backend"}}
	if err := d.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	_ = d.UpdateAgentLiveConfig(ctx, a.ID, []string{"worker", "reviewer"}, []string{"frontend"})

	if err := d.ResetAgentToStart(ctx, a.ID); err != nil {
		t.Fatalf("ResetAgentToStart: %v", err)
	}
	got, _ := d.GetAgent(ctx, a.ID)
	if len(got.Roles) != 1 || got.Roles[0] != "worker" {
		t.Errorf("after reset live roles = %v, want [worker]", got.Roles)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "backend" {
		t.Errorf("after reset live skills = %v, want [backend]", got.Skills)
	}
}

func TestAgentSystemPrompt_RoundTripAndSetter(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Round-trips through CreateAgent.
	a := &db.Agent{Name: "w5", Roles: []string{"worker"}, SystemPrompt: "you are careful"}
	if err := d.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	got, _ := d.GetAgent(ctx, a.ID)
	if got.SystemPrompt != "you are careful" {
		t.Errorf("system_prompt = %q, want %q", got.SystemPrompt, "you are careful")
	}

	// Targeted setter (PATCH path) updates it.
	if err := d.SetAgentSystemPrompt(ctx, a.ID, "reviewer persona"); err != nil {
		t.Fatalf("SetAgentSystemPrompt: %v", err)
	}
	got, _ = d.GetAgent(ctx, a.ID)
	if got.SystemPrompt != "reviewer persona" {
		t.Errorf("after set, system_prompt = %q, want %q", got.SystemPrompt, "reviewer persona")
	}

	// Re-registration (UpdateAgent) preserves it when the record already carries it.
	got.Status = "online"
	if err := d.UpdateAgent(ctx, got); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	got, _ = d.GetAgent(ctx, a.ID)
	if got.SystemPrompt != "reviewer persona" {
		t.Errorf("after UpdateAgent, system_prompt = %q, want %q", got.SystemPrompt, "reviewer persona")
	}
}

func TestSetAgentDesiredState(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	a := &db.Agent{Name: "w4", Roles: []string{"worker"}}
	if err := d.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if err := d.SetAgentDesiredState(ctx, a.ID, "stop"); err != nil {
		t.Fatalf("SetAgentDesiredState: %v", err)
	}
	got, _ := d.GetAgent(ctx, a.ID)
	if got.DesiredState != "stop" {
		t.Errorf("desired_state = %q, want stop", got.DesiredState)
	}
}
