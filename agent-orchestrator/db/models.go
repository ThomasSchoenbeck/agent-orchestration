package db

import (
	"encoding/json"
	"time"
)

// --- Project ---

type Project struct {
	ID                       string                 `json:"id"`
	Name                     string                 `json:"name"`
	Description              string                 `json:"description"`
	RepoPath                 string                 `json:"repo_path"`                  // legacy local path hint (optional)
	GitURL                   string                 `json:"git_url"`                    // git remote URL (legacy)
	Slug                     string                 `json:"slug"`                       // URL-safe name used by git HTTP server
	RemoteURL                string                 `json:"remote_url,omitempty"`       // upstream remote URL for mirroring
	RemoteCredentialsRef     string                 `json:"remote_credentials_ref,omitempty"` // env-var / settings key for upstream auth
	CodingRules              string                 `json:"coding_rules,omitempty"`     // freeform coding rules written to .agent_context/
	Status                   string                 `json:"status"`                     // active | complete (auto-queue) — legacy values still tolerated
	ScopeDirty               bool                   `json:"scope_dirty"`                // description changed; requirements/features may be stale (Feature 5)
	AutoQueue                bool                   `json:"auto_queue"`                 // armed: backlog auto-replenishes until complete (Feature 4)
	MaxOpenTasks             int                    `json:"max_open_tasks"`             // 0 = unlimited; safety cap on open work
	PlanRounds               int                    `json:"plan_rounds"`                // planning rounds used this activation (safety counter)
	Config                   map[string]interface{} `json:"config"`
	ServerRepoInitialisedAt  *time.Time             `json:"server_repo_initialised_at,omitempty"`
	CreatedAt                time.Time              `json:"created_at"`
	UpdatedAt                time.Time              `json:"updated_at"`
}

// Task state constants (mirrors workflow.TaskStatus — duplicated here to avoid
// the db package importing workflow).
const (
	TaskStatusBacklog          = "BACKLOG"
	TaskStatusUnqueued         = "UNQUEUED"       // parked: not claimable until re-queued to BACKLOG
	TaskStatusAwaitingInput    = "AWAITING_INPUT" // agent asked for help; parked until a human/orchestrator answers
	TaskStatusDeveloping       = "DEVELOPING"
	TaskStatusAwaitingReview   = "AWAITING_REVIEW"
	TaskStatusReviewing        = "REVIEWING"
	TaskStatusAwaitingRevision = "AWAITING_REVISION"
	TaskStatusAwaitingMerge    = "AWAITING_MERGE"
	TaskStatusMerging          = "MERGING"
	TaskStatusCompleted        = "COMPLETED"
	TaskStatusFailed           = "FAILED"
)

// IsQueueState returns true for states where no agent holds the task.
func IsQueueState(s string) bool {
	switch s {
	case TaskStatusBacklog, TaskStatusAwaitingReview,
		TaskStatusAwaitingRevision, TaskStatusAwaitingMerge:
		return true
	}
	return false
}

// IsExecutionState returns true for states where an agent is actively working.
func IsExecutionState(s string) bool {
	switch s {
	case TaskStatusDeveloping, TaskStatusReviewing, TaskStatusMerging:
		return true
	}
	return false
}

// --- Task ---

type Task struct {
	ID              string                 `json:"id"`
	ProjectID       string                 `json:"project_id"`
	Role            string                 `json:"role"`   // worker | reviewer | planner | ...
	ReviewRole      string                 `json:"review_role,omitempty"` // role that should handle this task's review step
	TaskTypeID      string                 `json:"task_type_id,omitempty"` // references task_types.id; drives the branch name
	Focus           []string               `json:"focus,omitempty"`       // optional required skills; empty = any (Feature 6)
	Status          string                 `json:"status"` // BACKLOG | DEVELOPING | AWAITING_REVIEW | REVIEWING | AWAITING_REVISION | AWAITING_MERGE | MERGING | COMPLETED | FAILED
	Priority        int                    `json:"priority"`
	AssignedAgentID string                 `json:"assigned_agent_id,omitempty"`
	Payload         map[string]interface{} `json:"payload"`
	Result          map[string]interface{} `json:"result,omitempty"`
	Attempts        int                    `json:"attempts"`
	BranchHeadSHA   string                 `json:"branch_head_sha,omitempty"`
	LastPushAt      *time.Time             `json:"last_push_at,omitempty"`
	WorktreePath    string                 `json:"worktree_path,omitempty"`
	AssignedPort    int                    `json:"assigned_port,omitempty"`
	// Branch is the human-readable branch the agent works on (e.g. "feature/<slug>").
	// Resolved from the task type's template and persisted at claim time, then immutable.
	Branch          string                 `json:"branch,omitempty"`
	// Agent-side only: populated from the claim response, never persisted to DB.
	RepoURL         string                 `json:"repo_url,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
}

// TaskFilters defines optional filters for ListTasks.
type TaskFilters struct {
	ProjectID     string
	Status        string
	Role          string
	AgentID       string
	RequirementID string // filter by linked requirement
	FeatureID     string // filter by linked feature
	Limit         int
	Offset        int
}

// TaskType is a configurable task category that drives the branch-name template
// for its tasks. Seeded from config on first run; editable in Settings.
type TaskType struct {
	ID             string    `json:"id"`
	Key            string    `json:"key"`             // unique slug: normal | bug | hotfix | release | ...
	Label          string    `json:"label"`           // display name
	BranchTemplate string    `json:"branch_template"` // e.g. "feature/{slug}"
	IsDefault      bool      `json:"is_default"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// --- Project Requirements / Features ---

// ProjectRequirement is one requirement entry on a project.
type ProjectRequirement struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`      // markdown
	Status    string    `json:"status"`    // proposed | accepted | satisfied | needs_review (Feature 5)
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProjectFeature is one feature entry on a project.
type ProjectFeature struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`      // markdown
	Status    string    `json:"status"`    // planned | in_progress | done | needs_review (Feature 5)
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// --- Pull Request ---

// PullRequest represents the merge gate for a task: the reviewer opens it on
// approval, and a deployer (handles_merge) or a human decides it. Merge only
// follows an explicit approval decision — PRs are never auto-approved.
type PullRequest struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"task_id"`
	ProjectID    string    `json:"project_id"`
	Branch       string    `json:"branch"`        // source: task/<id>
	Base         string    `json:"base"`          // always "main"
	Title        string    `json:"title"`
	Body         string    `json:"body"`          // reviewer's approval summary
	Status       string    `json:"status"`        // open | approved | rejected | merged
	AuthorID     string    `json:"author_id"`     // reviewer agent that opened it
	AuthorName   string    `json:"author_name"`
	DeciderID    string    `json:"decider_id"`    // deployer agent or human that decided
	DecisionBody string    `json:"decision_body"` // the merge reviewer's verdict notes
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TaskProjectLink links a task to a requirement or feature.
type TaskProjectLink struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Kind      string    `json:"kind"`      // requirement | feature
	TargetID  string    `json:"target_id"` // FK to requirement or feature
	CreatedAt time.Time `json:"created_at"`
}

// --- Agent ---

type Agent struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Roles         []string               `json:"roles"`
	Status        string                 `json:"status"` // online | offline | idle | busy
	Mode          string                 `json:"mode"`   // colocated | remote
	CurrentTaskID string                 `json:"current_task_id,omitempty"`
	Skills        []string               `json:"skills"` // live specializations (Feature 6/7)
	// Feature 7 lifecycle: durable start params (captured at each registration)
	// vs live values (mutable at runtime via the UI); control via DesiredState.
	StartRoles    []string               `json:"start_roles"`
	StartSkills   []string               `json:"start_skills"`
	DesiredState  string                 `json:"desired_state"`         // run | stop (default: run)
	TemplateID    string                 `json:"template_id,omitempty"` // set when spawned from an AgentTemplate (Feature 8)
	Capabilities  map[string]interface{} `json:"capabilities"`
	RegisteredAt  time.Time              `json:"registered_at"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
}

// --- Agent Template (Feature 8) ---

// AgentTemplate is a reusable definition the server spawns co-located agent
// instances from. Instances are ordinary agent rows linked back via TemplateID.
type AgentTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`      // base name; instances get "-1", "-2" suffixes
	Roles     []string  `json:"roles"`     // start roles for spawned agents
	Skills    []string  `json:"skills"`    // start skills
	Replicas  int       `json:"replicas"`  // desired number of running instances
	Autostart bool      `json:"autostart"` // relaunch desired replicas when the server boots
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// --- Provider ---

// ProviderModel describes one model offered by a provider: its supported roles,
// token pricing, and behavioral flags that override the provider-level defaults.
// Boolean fields use omitempty so that the zero value (false) is not stored —
// model-level overrides only apply when the field is explicitly set to true.
type ProviderModel struct {
	Name               string   `json:"name"`
	Roles              []string `json:"roles"`               // roles this model serves
	InputPerMillion    float64  `json:"input_per_million"`   // USD per 1M input tokens
	OutputPerMillion   float64  `json:"output_per_million"`  // USD per 1M output tokens
	ContextWindow      int      `json:"context_window,omitempty"` // max context tokens (0 = unknown)
	// Behavioral flags — override the provider-level Config when set.
	TextToolCalls      bool     `json:"text_tool_calls,omitempty"`
	FoldSystemIntoUser bool     `json:"fold_system_into_user,omitempty"`
	SystemPrefix       string   `json:"system_prefix,omitempty"`
	ToolAllowlist      []string `json:"tool_allowlist,omitempty"`
}

type Provider struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	BaseURL      string                 `json:"base_url"`
	ModelName    string                 `json:"model_name"`
	APIKey       string                 `json:"api_key,omitempty"`
	Enabled      bool                   `json:"enabled"`
	Roles        []string               `json:"roles"`        // roles this provider can serve (coarse fallback)
	Capabilities []string               `json:"capabilities"`
	Config       map[string]interface{} `json:"config"`
	Models       []ProviderModel        `json:"models"`       // per-model role and pricing config
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// --- Agent Role Definition ---

type RoleDefinition struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`           // slug, e.g. "worker"
	Label          string    `json:"label"`          // display name
	Description    string    `json:"description"`
	ProviderID     string    `json:"provider_id,omitempty"`
	ModelOverride  string    `json:"model_override"` // empty = use provider default
	SystemPrompt   string    `json:"system_prompt"`
	ContextInclude []string  `json:"context_include"`
	ContextExclude []string  `json:"context_exclude"`
	Capabilities   []string  `json:"capabilities"`     // lifecycle capabilities, e.g. handles_review, creates_tasks, handles_merge
	AllowedTools   []string  `json:"allowed_tools"`    // if non-empty, only these tools are sent to the LLM
	Temperature    float64   `json:"temperature"`
	MaxTokens      int       `json:"max_tokens"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// --- Skill Definition (Feature 6) ---

// SkillDefinition is a configurable specialization (stack/technology/persona)
// orthogonal to roles. It carries a prompt fragment ("soul"), context rules,
// and optional tools that an agent composes on top of its role(s). Skills carry
// no capabilities — lifecycle authority lives only in roles.
type SkillDefinition struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`  // slug: "backend", "react"
	Label          string    `json:"label"`
	Description    string    `json:"description"`
	PromptFragment string    `json:"prompt_fragment"` // injected into the system prompt
	ContextInclude []string  `json:"context_include"`
	ContextExclude []string  `json:"context_exclude"`
	AllowedTools   []string  `json:"allowed_tools"` // optional tools added to the role's set
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// --- Context ---

type ContextEntry struct {
	ID        string                 `json:"id"`
	ProjectID string                 `json:"project_id,omitempty"`
	TaskID    string                 `json:"task_id,omitempty"`
	Type      string                 `json:"type"` // summary | embedding | snippet | note
	Content   string                 `json:"content"`
	Embedding []float32              `json:"embedding,omitempty"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
}

// --- Log ---

type LogEntry struct {
	ID        string                 `json:"id"`
	AgentID   string                 `json:"agent_id,omitempty"`
	TaskID    string                 `json:"task_id,omitempty"`
	ProjectID string                 `json:"project_id,omitempty"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata"`
	Timestamp time.Time              `json:"timestamp"`
}

// LogFilters defines filters for querying logs.
type LogFilters struct {
	AgentID    string
	TaskID     string
	ProjectID  string
	Level      string
	Limit      int
	Offset     int
	SystemOnly bool // when true, only return entries with no agent_id and no task_id
}

// --- Conversation ---

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	ProviderID string   `json:"provider_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// --- Message ---

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"` // "user" or "assistant"
	Content        string    `json:"content"`
	TokensUsed     int       `json:"tokens_used,omitempty"`
	InputTokens    int       `json:"input_tokens,omitempty"`
	OutputTokens   int       `json:"output_tokens,omitempty"`
	DurationMs     int       `json:"duration_ms,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// --- Chat log ---

// ChatLogEntry is a lightweight view of a message used by the Logs page.
type ChatLogEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	ProviderName string    `json:"provider_name"`
	Direction    string    `json:"direction"` // user_to_llm | llm_to_user
	Preview      string    `json:"preview"`   // first 20 chars of content
}

// --- Metrics ---

type Metric struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"task_id,omitempty"`
	AgentID      string    `json:"agent_id,omitempty"`
	Model        string    `json:"model,omitempty"`
	TokensUsed   int       `json:"tokens_used"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	Cost         float64   `json:"cost"`
	InputCost    float64   `json:"input_cost"`
	OutputCost   float64   `json:"output_cost"`
	DurationMs   int       `json:"duration_ms"`
	Success      bool      `json:"success"`
	CreatedAt    time.Time `json:"created_at"`
	// F6 attribution: who/what incurred the cost.
	Source         string `json:"source,omitempty"`          // "agent" | "chat"
	ProviderID     string `json:"provider_id,omitempty"`
	AgentRole      string `json:"agent_role,omitempty"`      // role/type for agent source
	ConversationID string `json:"conversation_id,omitempty"` // chat source
	ProjectID      string `json:"project_id,omitempty"`
}

// --- JSON marshal helpers ---

func marshalJSON(v interface{}) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func marshalJSONArray(v interface{}) string {
	if v == nil {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalJSONMap(s string) map[string]interface{} {
	if s == "" {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	_ = json.Unmarshal([]byte(s), &m)
	if m == nil {
		m = map[string]interface{}{}
	}
	return m
}

// parseTime parses a datetime string in either SQLite default format or RFC3339.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// SQLite CURRENT_TIMESTAMP format
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t
	}
	// RFC3339 / ISO8601 with timezone
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	// RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

func unmarshalJSONStringSlice(s string) []string {
	if s == "" {
		return []string{}
	}
	var sl []string
	_ = json.Unmarshal([]byte(s), &sl)
	if sl == nil {
		sl = []string{}
	}
	return sl
}
