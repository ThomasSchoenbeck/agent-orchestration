package api

// --- Project request/response types ---

type CreateProjectRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	RepoPath    string                 `json:"repo_path"` // local filesystem path
	GitURL      string                 `json:"git_url"`   // git remote URL
	Config      map[string]interface{} `json:"config"`
}

type UpdateProjectRequest struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	RepoPath    *string                `json:"repo_path,omitempty"`
	GitURL      *string                `json:"git_url,omitempty"`
	Status      *string                `json:"status,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// --- Task request/response types ---

type CreateTaskRequest struct {
	ProjectID string                 `json:"project_id"`
	Type      string                 `json:"type"`
	Role      string                 `json:"role"`
	Priority  int                    `json:"priority"`
	Payload   map[string]interface{} `json:"payload"`
}

type UpdateTaskRequest struct {
	Status   *string                `json:"status,omitempty"`
	Priority *int                   `json:"priority,omitempty"`
	Payload  map[string]interface{} `json:"payload,omitempty"`
}

type SubmitTaskResultRequest struct {
	Result map[string]interface{} `json:"result"`
	Status string                 `json:"status"`
	Metrics *TaskMetrics           `json:"metrics,omitempty"`
}

type TaskMetrics struct {
	TokensUsed int     `json:"tokens_used"`
	Cost       float64 `json:"cost"`
	DurationMs int     `json:"duration_ms"`
}

// --- Agent request/response types ---

type RegisterAgentRequest struct {
	Name         string                 `json:"name"`
	Roles        []string               `json:"roles"`
	Capabilities map[string]interface{} `json:"capabilities"`
}

type RegisterAgentResponse struct {
	AgentID string `json:"agent_id"`
}

// --- Generic paginated list response ---

type ListResponse struct {
	Items  interface{} `json:"items"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit,omitempty"`
	Offset int         `json:"offset,omitempty"`
}
