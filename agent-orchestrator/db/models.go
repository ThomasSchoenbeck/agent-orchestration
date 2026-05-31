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
	Status                   string                 `json:"status"`
	Config                   map[string]interface{} `json:"config"`
	ServerRepoInitialisedAt  *time.Time             `json:"server_repo_initialised_at,omitempty"`
	CreatedAt                time.Time              `json:"created_at"`
	UpdatedAt                time.Time              `json:"updated_at"`
}

// Task state constants (mirrors workflow.TaskStatus — duplicated here to avoid
// the db package importing workflow).
const (
	TaskStatusBacklog          = "BACKLOG"
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
	Type            string                 `json:"type"`   // plan | implement | review | test | ...
	Role            string                 `json:"role"`   // orchestrator | worker | reviewer | ...
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
	// Agent-side only: populated from the claim response, never persisted to DB.
	RepoURL         string                 `json:"repo_url,omitempty"`
	Branch          string                 `json:"branch,omitempty"`
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

// --- Project Requirements / Features ---

// ProjectRequirement is one requirement entry on a project.
type ProjectRequirement struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`      // markdown
	Status    string    `json:"status"`    // proposed | accepted | implemented | obsolete
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
	Status    string    `json:"status"`    // planned | in_progress | done | dropped
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	Capabilities  map[string]interface{} `json:"capabilities"`
	RegisteredAt  time.Time              `json:"registered_at"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
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
	TaskTypes      []string  `json:"task_types"`       // task types routed to this role
	AllowedTools   []string  `json:"allowed_tools"`    // if non-empty, only these tools are sent to the LLM
	Temperature    float64   `json:"temperature"`
	MaxTokens      int       `json:"max_tokens"`
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
	DurationMs   int       `json:"duration_ms"`
	Success      bool      `json:"success"`
	CreatedAt    time.Time `json:"created_at"`
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
