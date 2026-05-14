package db_test

import (
	"context"
	"testing"
)

func TestGetSetting_NotFound(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetSetting(context.Background(), "nonexistent.key")
	if err == nil {
		t.Error("expected error for missing key, got nil")
	}
}

func TestSetAndGetSetting(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.SetSetting(ctx, "foo.bar", "42", "A test setting"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	s, err := d.GetSetting(ctx, "foo.bar")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if s.Value != "42" {
		t.Errorf("expected value 42, got %q", s.Value)
	}
	if s.Description != "A test setting" {
		t.Errorf("expected description, got %q", s.Description)
	}
}

func TestSetSetting_UpdatesExisting(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	_ = d.SetSetting(ctx, "key1", "first", "desc")
	_ = d.SetSetting(ctx, "key1", "second", "desc")

	s, err := d.GetSetting(ctx, "key1")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if s.Value != "second" {
		t.Errorf("expected updated value %q, got %q", "second", s.Value)
	}
}

func TestSeedSetting_DoesNotOverwrite(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Set a value via SetSetting (simulating a UI update).
	_ = d.SetSetting(ctx, "retention.days", "99", "user-set")

	// SeedSetting must not clobber the existing value.
	_ = d.SeedSetting(ctx, "retention.days", "7", "config default")

	s, _ := d.GetSetting(ctx, "retention.days")
	if s.Value != "99" {
		t.Errorf("SeedSetting overwrote user value: expected 99, got %s", s.Value)
	}
}

func TestSeedSetting_InsertsWhenAbsent(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.SeedSetting(ctx, "new.key", "default", "a default"); err != nil {
		t.Fatalf("SeedSetting: %v", err)
	}
	s, err := d.GetSetting(ctx, "new.key")
	if err != nil {
		t.Fatalf("GetSetting after SeedSetting: %v", err)
	}
	if s.Value != "default" {
		t.Errorf("expected value default, got %s", s.Value)
	}
}

func TestListSettings(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	_ = d.SetSetting(ctx, "a", "1", "")
	_ = d.SetSetting(ctx, "b", "2", "")
	_ = d.SetSetting(ctx, "c", "3", "")

	settings, err := d.ListSettings(ctx)
	if err != nil {
		t.Fatalf("ListSettings: %v", err)
	}
	if len(settings) < 3 {
		t.Errorf("expected at least 3 settings, got %d", len(settings))
	}
}

func TestSeedDefaultRetentionSettings(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.SeedDefaultRetentionSettings(ctx); err != nil {
		t.Fatalf("SeedDefaultRetentionSettings: %v", err)
	}

	expectedKeys := []string{
		"log.retention.agent.default_days",
		"log.retention.task.default_days",
		"log.retention.system.default_days",
	}
	for _, key := range expectedKeys {
		s, err := d.GetSetting(ctx, key)
		if err != nil {
			t.Errorf("GetSetting(%q): %v", key, err)
			continue
		}
		if s.Value == "" {
			t.Errorf("expected non-empty value for %q", key)
		}
	}
}

func TestSeedDefaultRetentionSettings_Idempotent(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Seed twice — should not error or duplicate.
	_ = d.SeedDefaultRetentionSettings(ctx)
	if err := d.SeedDefaultRetentionSettings(ctx); err != nil {
		t.Fatalf("second SeedDefaultRetentionSettings: %v", err)
	}

	settings, err := d.ListSettings(ctx)
	if err != nil {
		t.Fatalf("ListSettings: %v", err)
	}
	seen := map[string]int{}
	for _, s := range settings {
		seen[s.Key]++
		if seen[s.Key] > 1 {
			t.Errorf("duplicate setting key: %q", s.Key)
		}
	}
}

func TestSeedRetentionFromConfig(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	overrides := map[string]int{
		"agent_registered": 3,
		"agent_offline":    1,
	}
	if err := d.SeedRetentionFromConfig(ctx, 14, 30, 7, overrides); err != nil {
		t.Fatalf("SeedRetentionFromConfig: %v", err)
	}

	// Default keys must exist.
	s, err := d.GetSetting(ctx, "log.retention.agent.default_days")
	if err != nil {
		t.Fatalf("GetSetting agent: %v", err)
	}
	if s.Value != "14" {
		t.Errorf("expected agent default 14, got %s", s.Value)
	}

	s2, err := d.GetSetting(ctx, "log.retention.task.default_days")
	if err != nil {
		t.Fatalf("GetSetting task: %v", err)
	}
	if s2.Value != "30" {
		t.Errorf("expected task default 30, got %s", s2.Value)
	}
}

func TestSeedRetentionFromConfig_DoesNotOverwriteUIValues(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Simulate user updating a value via the UI.
	_ = d.SetSetting(ctx, "log.retention.agent.default_days", "999", "user-set")

	// Config seeding must not overwrite.
	_ = d.SeedRetentionFromConfig(ctx, 14, 30, 7, nil)

	s, _ := d.GetSetting(ctx, "log.retention.agent.default_days")
	if s.Value != "999" {
		t.Errorf("SeedRetentionFromConfig overwrote UI value: got %s", s.Value)
	}
}
