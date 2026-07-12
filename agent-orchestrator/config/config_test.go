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
      - name: gpt-4o-mini
        default: true
        roles: [worker]

role_definitions:
  - name: worker
    provider: test-provider
    model_override: gpt-4o-mini

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

func TestLoad_TaskTypes(t *testing.T) {
	yaml := minimalValidYAML + `
task_types:
  - key: normal
    label: Normal
    branch_template: "feature/{slug}"
    default: true
  - key: bug
    label: Bug
    branch_template: "bug/{slug}"
`
	cfg, err := config.Load(writeTemp(t, yaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.TaskTypes) != 2 {
		t.Fatalf("TaskTypes len = %d, want 2", len(cfg.TaskTypes))
	}
	if cfg.TaskTypes[0].Key != "normal" || !cfg.TaskTypes[0].Default ||
		cfg.TaskTypes[0].BranchTemplate != "feature/{slug}" {
		t.Errorf("TaskTypes[0] = %+v", cfg.TaskTypes[0])
	}
	if cfg.TaskTypes[1].Key != "bug" || cfg.TaskTypes[1].Default {
		t.Errorf("TaskTypes[1] = %+v", cfg.TaskTypes[1])
	}
}

func TestLoad_SkillsAndSubagentSkills(t *testing.T) {
	yaml := minimalValidYAML + `
skills:
  - name: backend
    label: Backend
    description: Server-side.
    prompt_fragment: "Focus on backend."
    context_include: ["server/**", "db/**"]
    allowed_tools: [run_tests]

subagent_skills:
  - name: investigate_codebase
    label: Investigate Codebase
    tool_allowlist: [read_file, list_files, search_files]
    max_rounds: 8
    prompt_template: "Investigate {{instructions}} then summarize."
`
	cfg, err := config.Load(writeTemp(t, yaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Skills) != 1 {
		t.Fatalf("Skills len = %d, want 1", len(cfg.Skills))
	}
	if cfg.Skills[0].Name != "backend" || cfg.Skills[0].PromptFragment != "Focus on backend." {
		t.Errorf("Skills[0] = %+v", cfg.Skills[0])
	}
	if len(cfg.Skills[0].ContextInclude) != 2 || len(cfg.Skills[0].AllowedTools) != 1 {
		t.Errorf("Skills[0] slices = %+v", cfg.Skills[0])
	}
	if len(cfg.SubagentSkills) != 1 {
		t.Fatalf("SubagentSkills len = %d, want 1", len(cfg.SubagentSkills))
	}
	sa := cfg.SubagentSkills[0]
	if sa.Name != "investigate_codebase" || sa.MaxRounds != 8 {
		t.Errorf("SubagentSkills[0] = %+v", sa)
	}
	if len(sa.ToolAllowlist) != 3 || sa.ToolAllowlist[2] != "search_files" {
		t.Errorf("SubagentSkills[0].ToolAllowlist = %v", sa.ToolAllowlist)
	}
	if !strings.Contains(sa.PromptTemplate, "{{instructions}}") {
		t.Errorf("prompt_template should retain the placeholder: %q", sa.PromptTemplate)
	}
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

func TestValidate_ProviderWithoutModels(t *testing.T) {
	cfg := &config.Config{
		Providers:       []config.ProviderConfig{{Name: "p", Type: "ollama"}}, // no models
		RoleDefinitions: []config.RoleDefinitionConfig{{Name: "worker"}},
		Server:          config.ServerConfig{Port: 8080},
		Database:        config.DatabaseConfig{Path: "./test.db"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for provider without models, got nil")
	}
}

// TestContextThresholdFraction_Default (T7.1): omitted → 0.80 default.
func TestContextThresholdFraction_Default(t *testing.T) {
	path := writeTemp(t, minimalValidYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Agents.ContextThresholdFraction != config.DefaultContextThresholdFraction {
		t.Errorf("context_threshold_fraction = %v, want default %v",
			cfg.Agents.ContextThresholdFraction, config.DefaultContextThresholdFraction)
	}
}

// TestContextThresholdFraction_Explicit (T7.1): an in-range value is kept; an
// out-of-range value falls back to the default.
func TestContextThresholdFraction_Explicit(t *testing.T) {
	ok := writeTemp(t, minimalValidYAML+"\nagents:\n  context_threshold_fraction: 0.5\n")
	cfg, err := config.Load(ok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Agents.ContextThresholdFraction != 0.5 {
		t.Errorf("context_threshold_fraction = %v, want 0.5", cfg.Agents.ContextThresholdFraction)
	}

	bad := writeTemp(t, minimalValidYAML+"\nagents:\n  context_threshold_fraction: 1.5\n")
	cfg2, err := config.Load(bad)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg2.Agents.ContextThresholdFraction != config.DefaultContextThresholdFraction {
		t.Errorf("out-of-range fraction = %v, want default %v",
			cfg2.Agents.ContextThresholdFraction, config.DefaultContextThresholdFraction)
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

// TestLoad_RoleDefinitionsAndTemplates parses the new config sections.
func TestLoad_RoleDefinitionsAndTemplates(t *testing.T) {
	yaml := `
providers:
  - name: p
    type: openai_compatible
    base_url: http://localhost:1234/v1
    models:
      - name: gpt
        default: true
        roles: [worker]

role_definitions:
  - name: reviewer
    label: Reviewer
    provider: p
    capabilities: [handles_review]
    allowed_tools: [read_file, task_comment]

agent_templates:
  - name: worker-pool
    roles: [worker]
    replicas: 3
    autostart: true

server:
  port: 8080
database:
  path: ./data/test.db
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.RoleDefinitions) != 1 || cfg.RoleDefinitions[0].Name != "reviewer" {
		t.Fatalf("role_definitions not parsed: %+v", cfg.RoleDefinitions)
	}
	if len(cfg.RoleDefinitions[0].Capabilities) != 1 || cfg.RoleDefinitions[0].Capabilities[0] != "handles_review" {
		t.Errorf("capabilities = %v, want [handles_review]", cfg.RoleDefinitions[0].Capabilities)
	}
	if len(cfg.AgentTemplates) != 1 || cfg.AgentTemplates[0].Replicas != 3 || !cfg.AgentTemplates[0].Autostart {
		t.Errorf("agent_templates not parsed: %+v", cfg.AgentTemplates)
	}
}

// TestValidate_RoleRoutability (Phase 5, T5.2): a role with neither a models list,
// a provider binding, nor a provider that declares it is unroutable and rejected.
func TestValidate_RoleRoutability(t *testing.T) {
	base := func(rd config.RoleDefinitionConfig, provRoles []string) *config.Config {
		return &config.Config{
			Providers: []config.ProviderConfig{{
				Name: "p", Type: "ollama", Roles: provRoles,
				Models: []config.ProviderModelConfig{{Name: "m"}},
			}},
			RoleDefinitions: []config.RoleDefinitionConfig{rd},
			Server:          config.ServerConfig{Port: 8080},
			Database:        config.DatabaseConfig{Path: "./t.db"},
		}
	}

	// Unroutable: no models, no provider, not claimed.
	if err := base(config.RoleDefinitionConfig{Name: "worker"}, nil).Validate(); err == nil {
		t.Error("expected error for unroutable role, got nil")
	} else if !strings.Contains(err.Error(), "unroutable") {
		t.Errorf("expected 'unroutable' in error, got: %v", err)
	}

	// Routable via provider binding.
	if err := base(config.RoleDefinitionConfig{Name: "worker", Provider: "p"}, nil).Validate(); err != nil {
		t.Errorf("provider-bound role should be valid, got: %v", err)
	}
	// Routable via provider-declared roles.
	if err := base(config.RoleDefinitionConfig{Name: "worker"}, []string{"worker"}).Validate(); err != nil {
		t.Errorf("provider-claimed role should be valid, got: %v", err)
	}
	// Routable via a models priority list.
	rd := config.RoleDefinitionConfig{Name: "worker", Models: []config.ModelRefConfig{{Provider: "p", Model: "m"}}}
	if err := base(rd, nil).Validate(); err != nil {
		t.Errorf("models-list role should be valid, got: %v", err)
	}
	// Malformed priority-list entry (missing model) is rejected.
	bad := config.RoleDefinitionConfig{Name: "worker", Models: []config.ModelRefConfig{{Provider: "p"}}}
	if err := base(bad, nil).Validate(); err == nil {
		t.Error("expected error for models entry missing model, got nil")
	}
}

// TestValidate_SubagentModelsEntry (T5.2/T8.2): a subagent skill may omit its
// models list (it inherits the spawn route), but any entry it does declare must
// name both a provider and a model.
func TestValidate_SubagentModelsEntry(t *testing.T) {
	base := func(models []config.ModelRefConfig) *config.Config {
		return &config.Config{
			Providers: []config.ProviderConfig{{
				Name: "p", Type: "ollama", Models: []config.ProviderModelConfig{{Name: "m"}},
			}},
			RoleDefinitions: []config.RoleDefinitionConfig{{Name: "worker", Provider: "p"}},
			SubagentSkills:  []config.SubagentSkillConfig{{Name: "code_subtask", Models: models}},
			Server:          config.ServerConfig{Port: 8080},
			Database:        config.DatabaseConfig{Path: "./t.db"},
		}
	}

	// Empty models → valid (inherits spawn route).
	if err := base(nil).Validate(); err != nil {
		t.Errorf("empty subagent models should be valid, got: %v", err)
	}
	// Complete entry → valid.
	if err := base([]config.ModelRefConfig{{Provider: "p", Model: "m"}}).Validate(); err != nil {
		t.Errorf("complete subagent models entry should be valid, got: %v", err)
	}
	// Missing model → rejected.
	if err := base([]config.ModelRefConfig{{Provider: "p"}}).Validate(); err == nil {
		t.Error("expected error for subagent models entry missing model")
	}
	// Missing provider → rejected.
	if err := base([]config.ModelRefConfig{{Model: "m"}}).Validate(); err == nil {
		t.Error("expected error for subagent models entry missing provider")
	}
}

// TestValidate_RoleDefinitionsSatisfyRoleRequirement: role_definitions alone
// (no `roles` map) satisfies the "at least one role" rule.
func TestValidate_RoleDefinitionsSatisfyRoleRequirement(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{
			Name: "p", Type: "ollama",
			Models: []config.ProviderModelConfig{{Name: "m"}},
		}},
		// Role carries a provider binding so it is routable (Phase 5, T5.2).
		RoleDefinitions: []config.RoleDefinitionConfig{{Name: "worker", Provider: "p"}},
		Server:          config.ServerConfig{Port: 8080},
		Database:        config.DatabaseConfig{Path: "./test.db"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config with role_definitions, got: %v", err)
	}
}

// TestLoad_ShippedConfigs guards that the real config.yaml and
// config.example.yaml load and validate.
func TestLoad_ShippedConfigs(t *testing.T) {
	for _, p := range []string{"../config.yaml", "../config.example.yaml"} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("%s not present: %v", p, err)
		}
		if _, err := config.Load(p); err != nil {
			t.Errorf("%s failed to load/validate: %v", p, err)
		}
	}
}

func TestProviderDefaultModel(t *testing.T) {
	path := writeTemp(t, minimalValidYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cfg.Providers[0].DefaultModel(); got != "gpt-4o-mini" {
		t.Errorf("DefaultModel() = %q, want gpt-4o-mini", got)
	}
}
