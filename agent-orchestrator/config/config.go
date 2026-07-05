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
	ContextRules map[string]ContextRule    `yaml:"context_rules"`
	Pricing      map[string]ModelPricing   `yaml:"pricing"`      // model → pricing fallback
	Server       ServerConfig              `yaml:"server"`
	Database     DatabaseConfig            `yaml:"database"`
	LogsDB       LogsDBConfig              `yaml:"logs_db"`
	Agents        AgentConfig               `yaml:"agents"`
	LogRetention  LogRetentionConfig         `yaml:"log_retention"`
	Storage       StorageConfig              `yaml:"storage"`
	Merge         MergeConfig                `yaml:"merge"`
	// RoleDefinitions seed agent role definitions (capabilities, prompt, tools)
	// into the DB on first run.
	RoleDefinitions []RoleDefinitionConfig    `yaml:"role_definitions"`
	// AgentTemplates seed server-managed agent templates into the DB on first run.
	AgentTemplates  []AgentTemplateConfig     `yaml:"agent_templates"`
	// TaskTypes seed configurable task types (branch-name templates) into the DB
	// on first run. The Settings page is the source of truth afterwards.
	TaskTypes       []TaskTypeConfig           `yaml:"task_types"`
	// Skills seed persona skill definitions (specializations agents compose on top
	// of roles) into the DB on first run. Empty → the built-in starter set.
	Skills          []SkillConfig              `yaml:"skills"`
	// SubagentSkills seed spawnable subagent skills (run_subagent units of work)
	// into the DB on first run. Empty → the built-in starter set.
	SubagentSkills  []SubagentSkillConfig      `yaml:"subagent_skills"`
}

// TaskTypeConfig defines one task type seeded on first run: a unique key, a
// display label, the per-type branch-name template, and whether it is the
// default applied when a task has no explicit type.
type TaskTypeConfig struct {
	Key            string `yaml:"key"`
	Label          string `yaml:"label"`
	BranchTemplate string `yaml:"branch_template"`
	Default        bool   `yaml:"default"`
}

// ProviderModelConfig declares one model within a provider: its supported roles,
// token pricing, and behavioral flags that override the provider-level defaults.
type ProviderModelConfig struct {
	Name               string   `yaml:"name"`
	Default            bool     `yaml:"default"` // this provider's default model
	InputPerMillion    float64  `yaml:"input_per_million"`
	OutputPerMillion   float64  `yaml:"output_per_million"`
	ContextWindow      int      `yaml:"context_window"` // max context tokens (for context-size display)
	TextToolCalls      bool     `yaml:"text_tool_calls"`
	FoldSystemIntoUser bool     `yaml:"fold_system_into_user"`
	SystemPrefix       string   `yaml:"system_prefix"`
	ToolAllowlist      []string `yaml:"tool_allowlist"`
	SystemPrompt       string   `yaml:"system_prompt"` // model-level prompt layer (Phase 5, T5.4)
}

// ProviderConfig defines a single LLM backend. Its models are declared in the
// Models list; the one flagged `default: true` (or the first) is the default.
type ProviderConfig struct {
	Name       string                `yaml:"name"`
	Type       string                `yaml:"type"`       // openai_compatible | anthropic | ollama | azure
	BaseURL    string                `yaml:"base_url"`
	APIKey     string                `yaml:"api_key"`
	Deployment string                `yaml:"deployment"` // Azure OpenAI deployment name
	SystemPrompt string              `yaml:"system_prompt"` // provider-level prompt layer (Phase 5, T5.4)
	Roles      []string              `yaml:"roles"`      // roles this provider can serve (coarse fallback; Phase 5, T5.1)
	Models     []ProviderModelConfig `yaml:"models"`     // per-model pricing + behavioral config
	// RequestTimeoutSec overrides the HTTP request timeout (seconds). 0 → per-type
	// default (300s for local ollama/openai_compatible servers, 120s for cloud).
	RequestTimeoutSec int `yaml:"request_timeout_sec"`
}

// DefaultModel returns the provider's default model name: the one flagged
// `default: true`, else the first model, else "".
func (p *ProviderConfig) DefaultModel() string {
	for _, m := range p.Models {
		if m.Default {
			return m.Name
		}
	}
	if len(p.Models) > 0 {
		return p.Models[0].Name
	}
	return ""
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
	TLSEnabled   bool   `yaml:"tls_enabled"`   // legacy/unused — see Insecure (kept for back-compat)

	// Insecure serves plain HTTP instead of HTTPS. The server runs HTTPS by
	// default; set this (or pass --insecure) for local/dev use.
	Insecure bool `yaml:"insecure"`
	// TLSCertFile/TLSKeyFile point at a provided certificate + key. When both are
	// set they are used; otherwise the server auto-generates a self-signed cert
	// under {storage.root}/tls and reuses it on restart.
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`
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

// MergeConfig controls how approved pull requests are merged. Both options
// default to true when unset (hence *bool: nil means "use the default").
type MergeConfig struct {
	Squash       *bool `yaml:"squash"`        // squash the branch into one commit (default true)
	DeleteBranch *bool `yaml:"delete_branch"` // delete the source branch after merge (default true)
}

// ShouldSquash reports whether merges should squash (default true).
func (m MergeConfig) ShouldSquash() bool { return m.Squash == nil || *m.Squash }

// ShouldDeleteBranch reports whether the source branch is deleted post-merge (default true).
func (m MergeConfig) ShouldDeleteBranch() bool { return m.DeleteBranch == nil || *m.DeleteBranch }

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
	Skills  []string `yaml:"skills"`  // specializations this agent provides (Feature 6)
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
	MaxManagedAgents             int               `yaml:"max_managed_agents"` // cap on server-spawned agents (0 = unlimited)
	APIKey                       string            `yaml:"api_key"`            // bearer token agents must present on /api/agent/*; empty = open
	ServerCA                     string            `yaml:"server_ca"`          // PEM CA file the agent trusts for the server's HTTPS cert
	TLSInsecure                  bool              `yaml:"tls_insecure"`       // agent skips TLS verification (dev only)
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

// ModelRefConfig is one entry in an ordered provider>model priority list (Phase 5,
// T5.2), used by roles and subagent skills. The router resolves the first
// available (provider, model) pair, failing over to the next on error.
type ModelRefConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// RoleDefinitionConfig defines an agent role seeded into the DB on first run:
// its capabilities, persona, tool allowlist, and which provider/model serves it.
type RoleDefinitionConfig struct {
	Name           string   `yaml:"name"`
	Label          string   `yaml:"label"`
	Description    string   `yaml:"description"`
	Provider       string   `yaml:"provider"`       // provider name this role routes to
	ModelOverride  string   `yaml:"model_override"` // specific model string (optional)
	// Models is the ordered provider>model priority list (Phase 5). When set the
	// router resolves it with failover; Provider/ModelOverride remain the pre-Phase-5
	// single-binding fallback until T5.5 switches routing over.
	Models         []ModelRefConfig `yaml:"models"`
	Capabilities   []string `yaml:"capabilities"`   // handles_review | handles_merge | creates_tasks | ...
	AllowedTools   []string `yaml:"allowed_tools"`  // empty → all tools
	ContextInclude []string `yaml:"context_include"`
	ContextExclude []string `yaml:"context_exclude"`
	SystemPrompt   string   `yaml:"system_prompt"`
	Temperature    float64  `yaml:"temperature"`
	MaxTokens      int      `yaml:"max_tokens"`
	// ResyncPrompt is the task description handed to this role on project scope
	// re-sync (used by the orchestrator/creates_tasks role; empty → built-in default).
	ResyncPrompt   string   `yaml:"resync_prompt"`
}

// AgentTemplateConfig defines a server-managed agent template seeded on first run.
type AgentTemplateConfig struct {
	Name      string   `yaml:"name"`
	Roles     []string `yaml:"roles"`
	Skills    []string `yaml:"skills"`
	Replicas  int      `yaml:"replicas"`
	Autostart bool     `yaml:"autostart"`
}


// SkillConfig defines one persona skill definition seeded on first run: a
// specialization (prompt fragment + context scoping + optional tools) an agent
// composes on top of its role(s).
type SkillConfig struct {
	Name           string   `yaml:"name"`
	Label          string   `yaml:"label"`
	Description    string   `yaml:"description"`
	PromptFragment string   `yaml:"prompt_fragment"`
	ContextInclude []string `yaml:"context_include"`
	ContextExclude []string `yaml:"context_exclude"`
	AllowedTools   []string `yaml:"allowed_tools"`
}

// SubagentSkillConfig defines one spawnable subagent skill seeded on first run.
// A subagent runs its own focused tool loop (via run_subagent) and returns only a
// summary, keeping the main agent's context small.
type SubagentSkillConfig struct {
	Name           string   `yaml:"name"`
	Label          string   `yaml:"label"`
	Description    string   `yaml:"description"`
	PromptTemplate string   `yaml:"prompt_template"` // {{instructions}} is replaced with the main agent's ask
	ToolAllowlist  []string `yaml:"tool_allowlist"`
	ContextInclude []string `yaml:"context_include"`
	ContextExclude []string `yaml:"context_exclude"`
	// Models is the subagent's ordered provider>model priority list (Phase 5).
	Models         []ModelRefConfig `yaml:"models"`
	MaxRounds      int      `yaml:"max_rounds"`
	MaxTokens      int      `yaml:"max_tokens"`
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
		if len(p.Models) == 0 {
			errs = append(errs, fmt.Sprintf("provider[%d] %q: at least one model is required", i, p.Name))
		}
	}

	if len(c.RoleDefinitions) == 0 {
		errs = append(errs, "at least one role_definitions entry is required")
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

