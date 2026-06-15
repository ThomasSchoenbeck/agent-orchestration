package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestProviderCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	p := &db.Provider{Name: "openai", Type: "openai_compatible", BaseURL: "http://x/v1", APIKey: "sk-1", Enabled: true}
	if err := d.CreateProvider(ctx, p); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if p.ID == "" {
		t.Fatal("CreateProvider did not assign an id")
	}

	got, err := d.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.Name != "openai" || got.Type != "openai_compatible" || !got.Enabled {
		t.Errorf("GetProvider = %+v", got)
	}

	got.Name = "openai-2"
	got.Enabled = false
	if err := d.UpdateProvider(ctx, got); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if re, _ := d.GetProvider(ctx, p.ID); re.Name != "openai-2" || re.Enabled {
		t.Errorf("update not persisted: %+v", re)
	}

	list, err := d.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListProviders = %d, want 1", len(list))
	}

	n, err := d.CountProviders(ctx)
	if err != nil || n != 1 {
		t.Errorf("CountProviders = %d (err=%v), want 1", n, err)
	}

	if err := d.DeleteProvider(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if n, _ := d.CountProviders(ctx); n != 0 {
		t.Errorf("CountProviders after delete = %d, want 0", n)
	}
}

func TestSeedProviders(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	seeds := []*db.Provider{
		{Name: "a", Type: "openai_compatible", BaseURL: "http://a/v1"},
		{Name: "b", Type: "openai_compatible", BaseURL: "http://b/v1"},
	}
	n, err := d.SeedProviders(ctx, seeds)
	if err != nil {
		t.Fatalf("SeedProviders: %v", err)
	}
	if n != 2 {
		t.Errorf("seeded = %d, want 2", n)
	}
	if c, _ := d.CountProviders(ctx); c != 2 {
		t.Errorf("CountProviders after seed = %d, want 2", c)
	}
}
