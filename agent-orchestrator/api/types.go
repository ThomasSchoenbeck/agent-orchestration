package api

// --- Project request/response types ---

type CreateProjectRequest struct {
	Name                 string                 `json:"name"`
	Description          string                 `json:"description"`
	RepoPath             string                 `json:"repo_path"` // local filesystem path
	GitURL               string                 `json:"git_url"`   // git remote URL (legacy)
	Slug                 string                 `json:"slug"`
	RemoteURL            string                 `json:"remote_url,omitempty"`
	RemoteCredentialsRef string                 `json:"remote_credentials_ref,omitempty"`
	CodingRules          string                 `json:"coding_rules,omitempty"`
	InitialPull          bool                   `json:"initial_pull,omitempty"` // fetch upstream and reset main on create
	Config               map[string]interface{} `json:"config"`
}

type UpdateProjectRequest struct {
	Name                 *string                `json:"name,omitempty"`
	Description          *string                `json:"description,omitempty"`
	RepoPath             *string                `json:"repo_path,omitempty"`
	GitURL               *string                `json:"git_url,omitempty"`
	Slug                 *string                `json:"slug,omitempty"`
	RemoteURL            *string                `json:"remote_url,omitempty"`
	RemoteCredentialsRef *string                `json:"remote_credentials_ref,omitempty"`
	CodingRules          *string                `json:"coding_rules,omitempty"`
	Status               *string                `json:"status,omitempty"`
	AutoQueue            *bool                  `json:"auto_queue,omitempty"`
	MaxOpenTasks         *int                   `json:"max_open_tasks,omitempty"`
	PlanRounds           *int                   `json:"plan_rounds,omitempty"`
	Config               map[string]interface{} `json:"config,omitempty"`
}

// --- Task request/response types ---

type CreateTaskRequest struct {
	ProjectID  string                 `json:"project_id"`
	Role       string                 `json:"role"`
	ReviewRole string                 `json:"review_role,omitempty"`
	Priority   int                    `json:"priority"`
	Payload    map[string]interface{} `json:"payload"`
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
	TokensUsed   int     `json:"tokens_used"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	Cost         float64 `json:"cost,omitempty"`
	DurationMs   int     `json:"duration_ms"`
	Model        string  `json:"model,omitempty"`
}

// --- Agent request/response types ---

type RegisterAgentRequest struct {
	Name         string                 `json:"name"`
	Roles        []string               `json:"roles"`
	Mode         string                 `json:"mode,omitempty"` // colocated | remote (default: remote)
	Capabilities map[string]interface{} `json:"capabilities"`
}

type RegisterAgentResponse struct {
	AgentID string `json:"agent_id"`
}

// ClaimTaskResponse is returned by the task-claim endpoint and carries
// workspace details that differ between colocated and remote agents.
type ClaimTaskResponse struct {
	Task         interface{} `json:"task"`
	// Colocated agents receive a local path; remote agents receive a URL+branch.
	WorktreePath string `json:"worktree_path,omitempty"` // colocated only
	RepoURL      string `json:"repo_url,omitempty"`      // remote only
	Branch       string `json:"branch,omitempty"`        // remote only
	AssignedPort int    `json:"assigned_port,omitempty"`
}

// --- Requirements / Features / Task Links ---

type CreateRequirementRequest struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Status   string `json:"status"`
	Position int    `json:"position"`
}

type UpdateRequirementRequest struct {
	Title    *string `json:"title,omitempty"`
	Body     *string `json:"body,omitempty"`
	Status   *string `json:"status,omitempty"`
	Position *int    `json:"position,omitempty"`
}

type CreateFeatureRequest struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Status   string `json:"status"`
	Position int    `json:"position"`
}

type UpdateFeatureRequest struct {
	Title    *string `json:"title,omitempty"`
	Body     *string `json:"body,omitempty"`
	Status   *string `json:"status,omitempty"`
	Position *int    `json:"position,omitempty"`
}

// --- Comments ---

type CreateCommentRequest struct {
	Body       string `json:"body"`
	ReviewID   string `json:"review_id,omitempty"`
	AuthorType string `json:"author_type,omitempty"` // defaults to "user"
	AuthorRole string `json:"author_role,omitempty"`
	AuthorID   string `json:"author_id,omitempty"`
}

// --- Checklist ---

type CreateChecklistItemRequest struct {
	GroupLabel string `json:"group_label"`
	Position   int    `json:"position"`
	Label      string `json:"label"`
	Status     string `json:"status"`
}

type UpdateChecklistItemRequest struct {
	GroupLabel *string `json:"group_label,omitempty"`
	Position   *int    `json:"position,omitempty"`
	Label      *string `json:"label,omitempty"`
	Status     *string `json:"status,omitempty"`
}

type CreateChecklistTemplateRequest struct {
	Name      string `json:"name"`
	ItemsJSON string `json:"items_json"`
}

type UpdateChecklistTemplateRequest struct {
	Name      *string `json:"name,omitempty"`
	ItemsJSON *string `json:"items_json,omitempty"`
}

// --- Task dependencies ---

type AddDependencyRequest struct {
	DependsOnID string `json:"depends_on_id"`
}

type RemoveDependencyRequest struct {
	DependsOnID string `json:"depends_on_id"`
}

type AddTaskLinkRequest struct {
	Kind     string `json:"kind"`      // requirement | feature
	TargetID string `json:"target_id"`
}

type RemoveTaskLinkRequest struct {
	Kind     string `json:"kind"`
	TargetID string `json:"target_id"`
}

// --- Generic paginated list response ---

type ListResponse struct {
	Items  interface{} `json:"items"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit,omitempty"`
	Offset int         `json:"offset,omitempty"`
}
