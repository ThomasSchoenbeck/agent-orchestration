package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func hasCapability(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

func TestSeedRoles_NewTaxonomy(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	n, err := d.SeedRoleDefinitions(ctx, db.DefaultRoleDefinitions())
	if err != nil {
		t.Fatalf("SeedRoleDefinitions: %v", err)
	}
	if n != 7 {
		t.Fatalf("seeded %d roles, want 7", n)
	}

	want := map[string][]string{
		"worker":     {},
		"reviewer":   {"handles_review", "handles_merge"}, // reviewer owns the merge gate
		"planner":    {"creates_tasks"},
		"researcher": {"creates_tasks"},
		"security":   {},
		"deployer":   {"handles_deploy"}, // deployment only — no longer merges
		"designer":   {"creates_tasks"},
	}
	for name, caps := range want {
		got, err := d.GetRoleDefinitionByName(ctx, name)
		if err != nil {
			t.Errorf("role %q not seeded: %v", name, err)
			continue
		}
		for _, c := range caps {
			if !hasCapability(got.Capabilities, c) {
				t.Errorf("role %q missing capability %q (got %v)", name, c, got.Capabilities)
			}
		}
	}
}

func TestSeedRoles_Idempotent(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if _, err := d.SeedRoleDefinitions(ctx, db.DefaultRoleDefinitions()); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	n, err := d.SeedRoleDefinitions(ctx, db.DefaultRoleDefinitions())
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if n != 0 {
		t.Errorf("second seed inserted %d roles, want 0", n)
	}
	count, err := d.CountRoleDefinitions(ctx)
	if err != nil {
		t.Fatalf("CountRoleDefinitions: %v", err)
	}
	if count != 7 {
		t.Errorf("role count = %d, want 7", count)
	}
}
