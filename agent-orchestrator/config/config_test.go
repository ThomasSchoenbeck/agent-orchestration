package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-orchestrator/config"
)

const minimalValidYAML = `
providers:
  - name: test-provider
    type: openai_compatible
    base_url: http://localhost:1234/v1
    api_key: test-key

models:
  - name: test-model
    provider: test-provider
    model: gpt-4o-mini
    roles: [worker]

roles:
  worker: test-model

server:
  port: 8080
  host: "0.0.0.0"

database:
  path: ./data/test.db
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeTemp(t, minimalValidYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "test-provider" {
		t.Errorf("unexpected provider name: %s", cfg.Providers[0].Name)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTemp(t, "{ not: valid: yaml: !!!")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_EnvSubstitution(t *testing.T) {
	t.Setenv("TEST_API_KEY", "super-secret")
	yaml := strings.Replace(minimalValidYAML, "test-key", "${TEST_API_KEY}", 1)
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Providers[0].APIKey != "super-secret" {
		t.Errorf("env substitution failed: got %q", cfg.Providers[0].APIKey)
	}
}

func TestValidate_NoProviders(t *testing.T) {
	cfg := &config.Config{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "provider") {
		t.Errorf("expected 'provider' in error, got: %v", err)
	}
}

func TestValidate_NoModels(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "p", Type: "ollama"}},
		Roles:     map[string]string{"worker": "m"},
		Server:    config.ServerConfig{Port: 8080},
		Database:  config.DatabaseConfig{Path: "./test.db"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for missing models, got nil")
	}
}

func TestDefaults_Applied(t *testing.T) {
	path := writeTemp(t, minimalValidYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Agents.HeartbeatIntervalSec != config.DefaultHeartbeatIntervalSec {
		t.Errorf("expected default heartbeat interval %d, got %d",
			config.DefaultHeartbeatIntervalSec, cfg.Agents.HeartbeatIntervalSec)
	}
	if cfg.Agents.TaskPollIntervalSec != config.DefaultTaskPollIntervalSec {
		t.Errorf("expected default poll interval %d, got %d",
			config.DefaultTaskPollIntervalSec, cfg.Agents.TaskPollIntervalSec)
	}
}

func TestProviderByName(t *testing.T) {
	path := writeTemp(t, minimalValidYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p, err := cfg.ProviderByName("test-provider")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Type != "openai_compatible" {
		t.Errorf("unexpected provider type: %s", p.Type)
	}
	_, err = cfg.ProviderByName("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing provider, got nil")
	}
}

func TestLoad_LogsDB_Explicit(t *testing.T) {
	yaml := minimalValidYAML + `
logs_db:
  path: ./data/logs.db
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogsDB.Path != "./data/logs.db" {
		t.Errorf("expected logs_db.path './data/logs.db', got %q", cfg.LogsDB.Path)
	}
}

func TestLoad_LogsDB_DefaultsEmpty(t *testing.T) {
	path := writeTemp(t, minimalValidYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogsDB.Path != "" {
		t.Errorf("expected empty logs_db.path by default, got %q", cfg.LogsDB.Path)
	}
}

func TestProviderForRole(t *testing.T) {
	path := writeTemp(t, minimalValidYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	provider, model, err := cfg.ProviderForRole("worker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.Name != "test-provider" {
		t.Errorf("unexpected provider name: %s", provider.Name)
	}
	if model.Name != "test-model" {
		t.Errorf("unexpected model name: %s", model.Name)
	}
}
