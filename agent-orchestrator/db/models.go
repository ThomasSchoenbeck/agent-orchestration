package db

import (
	"encoding/json"
	"time"
)

// --- Project ---

type Project struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	RepoPath    string                 `json:"repo_path"`
	Status      string                 `json:"status"` // planned | in_progress | completed | failed
	Config      map[string]interface{} `json:"config"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// --- Task ---

type Task struct {
	ID              string                 `json:"id"`
	ProjectID       string                 `json:"project_id"`
	Type            string                 `json:"type"`   // plan | implement | review | test | ...
	Role            string                 `json:"role"`   // orchestrator | worker | reviewer | ...
	Status          string                 `json:"status"` // planned | in_progress | needs_review | approved | completed | failed
	Priority        int                    `json:"priority"`
	AssignedAgentID string                 `json:"assigned_agent_id,omitempty"`
	Payload         map[string]interface{} `json:"payload"`
	Result          map[string]interface{} `json:"result,omitempty"`
	Attempts        int                    `json:"attempts"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
}

// TaskFilters defines optional filters for ListTasks.
type TaskFilters struct {
	ProjectID string
	Status    string
	Role      string
	AgentID   string
	Limit     int
	Offset    int
}

// --- Agent ---

type Agent struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Roles         []string               `json:"roles"`
	Status        string                 `json:"status"` // online | offline | idle | busy
	CurrentTaskID string                 `json:"current_task_id,omitempty"`
	Capabilities  map[string]interface{} `json:"capabilities"`
	RegisteredAt  time.Time              `json:"registered_at"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
}

// --- Provider ---

type Provider struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	BaseURL      string                 `json:"base_url"`
	ModelName    string                 `json:"model_name"`
	APIKey       string                 `json:"api_key,omitempty"`
	Capabilities []string               `json:"capabilities"`
	Config       map[string]interface{} `json:"config"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
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
	AgentID   string
	TaskID    string
	ProjectID string
	Level     string
	Limit     int
	Offset    int
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
