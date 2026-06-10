package server

import (
	"context"
	"path/filepath"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// TestRoleNames_MapsIDsToNames backs B3: the poll/pull log description shows
// human-readable role names. roleNames maps role ids to names and falls back to
// the ref when the role is unknown.
func TestRoleNames_MapsIDsToNames(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	role := &db.RoleDefinition{Name: "worker", Label: "Worker", Enabled: true}
	if err := database.CreateRoleDefinition(ctx, role); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
		Database: config.DatabaseConfig{Path: dbPath},
		Storage:  config.StorageConfig{Root: t.TempDir()},
	}
	s := New(cfg, database, llm.NewRegistry())

	got := s.roleNames(ctx, []string{role.ID, "nonexistent"})
	if len(got) != 2 || got[0] != "worker" || got[1] != "nonexistent" {
		t.Fatalf("roleNames = %v, want [worker nonexistent]", got)
	}
}
