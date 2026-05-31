// Package config handles loading, parsing and validating the YAML configuration.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
type Config struct {
	Providers    []ProviderConfig          `yaml:"providers"`
	Models       []ModelConfig             `yaml:"models"`
	Roles        map[string]string         `yaml:"roles"`        // role → model name
	Routing      map[string]string         `yaml:"routing"`      // task type → role
	Prompts      map[string]string         `yaml:"prompts"`
	ContextRules map[string]ContextRule    `yaml:"context_rules"`
	Pricing      map[string]ModelPricing   `yaml:"pricing"`      // model → pricing (Phase 4)
	Server       ServerConfig              `yaml:"server"`
	Database     DatabaseConfig            `yaml:"database"`
	LogsDB       LogsDBConfig              `yaml:"logs_db"`
	Agents        AgentConfig               `yaml:"agents"`
	LogRetention  LogRetentionConfig         `yaml:"log_retention"`
	Storage       StorageConfig              `yaml:"storage"`
}

// ProviderModelConfig declares one model within a provider: its supported roles,
// token pricing, and behavioral flags that override the provider-level defaults.
type ProviderModelConfig struct {
	Name               string   `yaml:"name"`
	Roles              []string `yaml:"roles"`
	InputPerMillion    float64  `yaml:"input_per_million"`
	OutputPerMillion   float64  `yaml:"output_per_million"`
	TextToolCalls      bool     `yaml:"text_tool_calls"`
	FoldSystemIntoUser bool     `yaml:"fold_system_into_user"`
	SystemPrefix       string   `yaml:"system_prefix"`
	ToolAllowlist      []string `yaml:"tool_allowlist"`
}

// ProviderConfig defines a single LLM backend.
type ProviderConfig struct {
	Name       string                `yaml:"name"`
	Type       string                `yaml:"type"`       // openai_compatible | anthropic | ollama | azure
	BaseURL    string                `yaml:"base_url"`
	APIKey     string                `yaml:"api_key"`
	Model      string                `yaml:"model"`
	Deployment string                `yaml:"deployment"` // Azure OpenAI deployment name
	Models     []ProviderModelConfig `yaml:"models"`     // per-model role and pricing config
}

// ModelConfig names a provider+model combination and assigns it roles.
type ModelConfig struct {
	Name     string   `yaml:"name"`
	Provider string   `yaml:"provider"`
	Model    string   `yaml:"model"`
	Roles    []string `yaml:"roles"`
}

// ContextRule specifies which context types to include/exclude for a role.
type ContextRule struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port         int    `yaml:"port"`
	Host         string `yaml:"host"`
	ExternalHost string `yaml:"external_host"` // publicly-reachable address for agents (defaults to Host)
	TLSEnabled   bool   `yaml:"tls_enabled"`
}

// PublicHost returns the host that agents should use when connecting to the
// server's git endpoint. Uses ExternalHost when set; falls back to Host,
// substituting "localhost" for the bind-all address "0.0.0.0".
func (s ServerConfig) PublicHost() string {
	if s.ExternalHost != "" {
		return s.ExternalHost
	}
	if s.Host == "" || s.Host == "0.0.0.0" {
		return "localhost"
	}
	return s.Host
}

// DatabaseConfig holds database settings.
type DatabaseConfig struct {
	Type string `yaml:"type"` // sqlite
	Path string `yaml:"path"`
}

// LogsDBConfig optionally points log tables (agent_logs, task_logs, system logs)
// at a separate SQLite file. When Path is empty the server falls back to the
// default location (logs.db next to the main database).
type LogsDBConfig struct {
	Type string `yaml:"type"` // sqlite (only supported value)
	Path string `yaml:"path"` // empty → use default path
}

// AgentDefinition describes a single agent instance to launch.
// Used under agents.definitions in config.yaml.
type AgentDefinition struct {
	Name    string   `yaml:"name"`
	Roles   []string `yaml:"roles"`
	Mode    string   `yaml:"mode"`    // "colocated" or "remote"; default "colocated"
	Workdir string   `yaml:"workdir"` // overrides agents.workdir when set
	Config  string   `yaml:"config"`  // overrides the main config path for this agent's LLM setup
}

// AgentConfig holds agent timing defaults and the list of agent instances.
type AgentConfig struct {
	HeartbeatIntervalSec         int               `yaml:"heartbeat_interval_sec"`
	TaskPollIntervalSec          int               `yaml:"task_poll_interval_sec"`
	TaskTimeoutSec               int               `yaml:"task_timeout_sec"`
	MaxRetries                   int               `yaml:"max_retries"`               // max LLM call retries (Phase 4)
	FallbackProvider             string            `yaml:"fallback_provider"`         // provider name to fall back to (Phase 4)
	CircuitBreakerThreshold      int               `yaml:"circuit_breaker_threshold"` // failures before opening circuit (Phase 4)
	PortPoolStart                int               `yaml:"port_pool_start"`           // first port in agent test-port pool
	PortPoolSize                 int               `yaml:"port_pool_size"`            // number of ports in pool
	MergeSupervisorIntervalSec   int               `yaml:"merge_supervisor_interval_sec"`
	Workdir                      string            `yaml:"workdir"`     // default workdir root; per-agent gets {workdir}/{name}
	ServerURL                    string            `yaml:"server_url"`  // orchestrator URL for agent-mode connections
	Definitions                  []AgentDefinition `yaml:"definitions"` // agent instances to launch via "agents" subcommand

	// Startup connection retry settings (used when the server may not be ready yet).
	// Retry uses exponential backoff: delay doubles each attempt from InitialDelay up to MaxDelay.
	ConnectInitialDelayMs int `yaml:"connect_initial_delay_ms"` // initial backoff after first failure (ms)
	ConnectMaxDelayMs     int `yaml:"connect_max_delay_ms"`     // backoff ceiling (ms)
	ConnectMaxRetries     int `yaml:"connect_max_retries"`      // 0 = retry indefinitely
}

// StorageConfig holds paths for server-managed git repos and worktrees.
type StorageConfig struct {
	Root                         string `yaml:"root"`
	ReposDir                     string `yaml:"repos_dir"`      // subdirectory under Root for bare repos (default: "repos")
	WorktreesDir                 string `yaml:"worktrees_dir"`  // subdirectory under Root for agent worktrees (default: "worktrees")
	WorktreeRetentionFailedHours int    `yaml:"worktree_retention_failed_hours"`
}

// ModelPricing holds per-model cost config (USD per 1M tokens).
type ModelPricing struct {
	InputPerMillion  float64 `yaml:"input_per_million"`
	OutputPerMillion float64 `yaml:"output_per_million"`
}


// LogRetentionConfig holds default and per-type retention settings.
// These values seed the platform_settings table at startup (DB wins on conflict).
type LogRetentionConfig struct {
	AgentDefaultDays    int            `yaml:"agent_default_days"`
	TaskDefaultDays     int            `yaml:"task_default_days"`
	SystemDefaultDays   int            `yaml:"system_default_days"`
	CleanupIntervalMins int            `yaml:"cleanup_interval_minutes"`
	Overrides           map[string]int `yaml:"overrides"`
}

// Load reads a YAML config file at path, expands environment variables, and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	// Substitute ${ENV_VAR} patterns.
	content := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Apply defaults before validation.
	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Validate checks that the configuration is self-consistent.
func (c *Config) Validate() error {
	var errs []string

	if len(c.Providers) == 0 {
		errs = append(errs, "at least one provider is required")
	}

	for i, p := range c.Providers {
		if p.Name == "" {
			errs = append(errs, fmt.Sprintf("provider[%d]: name is required", i))
		}
		if p.Type == "" {
			errs = append(errs, fmt.Sprintf("provider[%d] %q: type is required", i, p.Name))
		}
	}

	if len(c.Models) == 0 {
		errs = append(errs, "at least one model is required")
	}

	for i, m := range c.Models {
		if m.Name == "" {
			errs = append(errs, fmt.Sprintf("model[%d]: name is required", i))
		}
		if m.Provider == "" {
			errs = append(errs, fmt.Sprintf("model[%d] %q: provider is required", i, m.Name))
		}
	}

	if len(c.Roles) == 0 {
		errs = append(errs, "at least one role mapping is required")
	}

	if c.Server.Port == 0 {
		errs = append(errs, "server.port must be set")
	}

	if c.Database.Path == "" {
		errs = append(errs, "database.path must be set")
	}

	if len(errs) > 0 {
		combined := ""
		for _, e := range errs {
			combined += "\n  - " + e
		}
		return errors.New("config validation errors:" + combined)
	}

	return nil
}

// applyDefaults fills in zero values with sensible defaults.
func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = DefaultServerPort
	}
	if c.Server.Host == "" {
		c.Server.Host = DefaultServerHost
	}
	if c.Database.Type == "" {
		c.Database.Type = DefaultDBType
	}
	if c.Database.Path == "" {
		c.Database.Path = DefaultDBPath
	}
	if c.Agents.HeartbeatIntervalSec == 0 {
		c.Agents.HeartbeatIntervalSec = DefaultHeartbeatIntervalSec
	}
	if c.Agents.TaskPollIntervalSec == 0 {
		c.Agents.TaskPollIntervalSec = DefaultTaskPollIntervalSec
	}
	if c.Agents.TaskTimeoutSec == 0 {
		c.Agents.TaskTimeoutSec = DefaultTaskTimeoutSec
	}
	if c.Agents.MaxRetries == 0 {
		c.Agents.MaxRetries = DefaultMaxRetries
	}
	if c.Agents.CircuitBreakerThreshold == 0 {
		c.Agents.CircuitBreakerThreshold = DefaultCircuitBreakerThreshold
	}
	if c.LogRetention.AgentDefaultDays == 0 {
		c.LogRetention.AgentDefaultDays = 14
	}
	if c.LogRetention.TaskDefaultDays == 0 {
		c.LogRetention.TaskDefaultDays = 30
	}
	if c.LogRetention.SystemDefaultDays == 0 {
		c.LogRetention.SystemDefaultDays = 7
	}
	if c.LogRetention.CleanupIntervalMins == 0 {
		c.LogRetention.CleanupIntervalMins = 60
	}
	if c.Storage.Root == "" {
		c.Storage.Root = DefaultStorageRoot
	}
	if c.Storage.ReposDir == "" {
		c.Storage.ReposDir = DefaultReposDir
	}
	if c.Storage.WorktreesDir == "" {
		c.Storage.WorktreesDir = DefaultWorktreesDir
	}
	if c.Storage.WorktreeRetentionFailedHours == 0 {
		c.Storage.WorktreeRetentionFailedHours = DefaultWorktreeRetentionFailedHours
	}
	if c.Agents.PortPoolStart == 0 {
		c.Agents.PortPoolStart = DefaultPortPoolStart
	}
	if c.Agents.PortPoolSize == 0 {
		c.Agents.PortPoolSize = DefaultPortPoolSize
	}
	if c.Agents.MergeSupervisorIntervalSec == 0 {
		c.Agents.MergeSupervisorIntervalSec = DefaultMergeSupervisorIntervalSec
	}
	if c.Agents.ConnectInitialDelayMs == 0 {
		c.Agents.ConnectInitialDelayMs = DefaultConnectInitialDelayMs
	}
	if c.Agents.ConnectMaxDelayMs == 0 {
		c.Agents.ConnectMaxDelayMs = DefaultConnectMaxDelayMs
	}
	if c.Agents.ConnectMaxRetries == 0 {
		c.Agents.ConnectMaxRetries = DefaultConnectMaxRetries
	}
}

// ProviderByName returns the ProviderConfig for the given name, or an error.
func (c *Config) ProviderByName(name string) (*ProviderConfig, error) {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return &c.Providers[i], nil
		}
	}
	return nil, fmt.Errorf("provider %q not found in config", name)
}

// ModelByName returns the ModelConfig for the given name, or an error.
func (c *Config) ModelByName(name string) (*ModelConfig, error) {
	for i := range c.Models {
		if c.Models[i].Name == name {
			return &c.Models[i], nil
		}
	}
	return nil, fmt.Errorf("model %q not found in config", name)
}

// ProviderForRole resolves role → model name → provider name → ProviderConfig.
func (c *Config) ProviderForRole(role string) (*ProviderConfig, *ModelConfig, error) {
	modelName, ok := c.Roles[role]
	if !ok {
		return nil, nil, fmt.Errorf("no model mapped to role %q", role)
	}
	model, err := c.ModelByName(modelName)
	if err != nil {
		return nil, nil, err
	}
	provider, err := c.ProviderByName(model.Provider)
	if err != nil {
		return nil, nil, err
	}
	return provider, model, nil
}
