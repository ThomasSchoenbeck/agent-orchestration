// Package agent implements the autonomous agent process that registers with a
// server, polls for tasks, executes them, and reports results.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// ServerClient is a typed HTTP client for talking to the orchestrator server.
type ServerClient struct {
	serverURL string
	agentID   string
	client    *http.Client
}

// NewServerClient creates a client pointed at serverURL.
func NewServerClient(serverURL string) *ServerClient {
	return &ServerClient{
		serverURL: serverURL,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Register registers this agent and stores the returned agent ID.
// mode should be "colocated" or "remote" (defaults to "remote" if empty).
func (c *ServerClient) Register(ctx context.Context, name string, roles, skills []string, mode string, caps map[string]interface{}) (string, error) {
	body := api.RegisterAgentRequest{Name: name, Roles: roles, Skills: skills, Mode: mode, Capabilities: caps}
	var resp api.RegisterAgentResponse
	if err := c.post(ctx, "/api/agents/register", body, &resp); err != nil {
		return "", fmt.Errorf("register: %w", err)
	}
	c.agentID = resp.AgentID
	return resp.AgentID, nil
}

// ListSkills fetches all skill definitions from the server (Feature 6).
func (c *ServerClient) ListSkills(ctx context.Context) ([]*db.SkillDefinition, error) {
	var skills []*db.SkillDefinition
	if err := c.get(ctx, "/api/skills", &skills); err != nil {
		return nil, err
	}
	return skills, nil
}

// Heartbeat sends a heartbeat for the given agent ID and returns the server's
// control response (desired state + live roles/skills). Feature 7.
func (c *ServerClient) Heartbeat(ctx context.Context, agentID string) (*api.HeartbeatResponse, error) {
	var resp api.HeartbeatResponse
	if err := c.post(ctx, fmt.Sprintf("/api/agents/%s/heartbeat", agentID), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetNextTask fetches the next available task for the agent's roles.
func (c *ServerClient) GetNextTask(ctx context.Context, agentID string, roles []string) (*db.Task, error) {
	rolesParam := ""
	for i, r := range roles {
		if i > 0 {
			rolesParam += ","
		}
		rolesParam += r
	}
	url := fmt.Sprintf("/api/agents/%s/tasks/next?roles=%s", agentID, rolesParam)
	// The server returns ClaimTaskResponse with repo_url + branch so the agent
	// can clone the project repo and begin work.
	var resp struct {
		Task         *db.Task `json:"task"`
		RepoURL      string   `json:"repo_url"`
		Branch       string   `json:"branch"`
		AssignedPort int      `json:"assigned_port"`
	}
	if err := c.get(ctx, url, &resp); err != nil {
		return nil, err
	}
	if resp.Task == nil || resp.Task.ID == "" {
		return nil, nil
	}
	// Propagate workspace fields onto the task so executeTask can use them.
	if resp.RepoURL != "" {
		resp.Task.RepoURL = resp.RepoURL
	}
	if resp.Branch != "" {
		resp.Task.Branch = resp.Branch
	}
	if resp.AssignedPort != 0 {
		resp.Task.AssignedPort = resp.AssignedPort
	}
	return resp.Task, nil
}

// ClaimTask claims a task for this agent.
// The server returns api.ClaimTaskResponse; workspace fields are applied to the task.
func (c *ServerClient) ClaimTask(ctx context.Context, taskID, agentID string) (*db.Task, error) {
	body := map[string]string{"agent_id": agentID}
	var resp struct {
		Task         *db.Task `json:"task"`
		WorktreePath string   `json:"worktree_path"`
		RepoURL      string   `json:"repo_url"`
		Branch       string   `json:"branch"`
	}
	if err := c.post(ctx, fmt.Sprintf("/api/tasks/%s/claim", taskID), body, &resp); err != nil {
		return nil, err
	}
	if resp.Task == nil {
		return nil, fmt.Errorf("claim: server returned nil task")
	}
	// Propagate workspace fields onto the task.
	if resp.RepoURL != "" {
		resp.Task.RepoURL = resp.RepoURL
	}
	if resp.Branch != "" {
		resp.Task.Branch = resp.Branch
	}
	return resp.Task, nil
}

// SubmitTaskResult submits a completed or failed task result.
func (c *ServerClient) SubmitTaskResult(ctx context.Context, taskID string, result map[string]interface{}, status string, metrics *api.TaskMetrics) error {
	body := api.SubmitTaskResultRequest{Result: result, Status: status, Metrics: metrics}
	return c.post(ctx, fmt.Sprintf("/api/tasks/%s/result", taskID), body, nil)
}

// SubmitForReview notifies the server that the agent has pushed its branch and
// the task should transition to AWAITING_REVIEW. metrics may be nil.
func (c *ServerClient) SubmitForReview(ctx context.Context, taskID string, metrics *api.TaskMetrics) error {
	body := struct {
		Metrics *api.TaskMetrics `json:"metrics,omitempty"`
	}{Metrics: metrics}
	return c.post(ctx, fmt.Sprintf("/api/tasks/%s/submit-for-review", taskID), body, nil)
}

// PostReview posts a code review on a task. status should be one of:
// "approved", "changes_requested", "revision_requested".
func (c *ServerClient) PostReview(ctx context.Context, taskID, status, body, branchHeadSHA, agentID string) error {
	req := map[string]string{
		"author_type":     "agent",
		"author_role":     "reviewer",
		"author_id":       agentID,
		"status":          status,
		"body":            body,
		"branch_head_sha": branchHeadSHA,
	}
	return c.post(ctx, fmt.Sprintf("/api/tasks/%s/reviews", taskID), req, nil)
}

// ListPRs returns the pull requests opened for a task.
func (c *ServerClient) ListPRs(ctx context.Context, taskID string) ([]*db.PullRequest, error) {
	var prs []*db.PullRequest
	if err := c.get(ctx, fmt.Sprintf("/api/tasks/%s/pull-requests", taskID), &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

// SubmitPRDecision submits a deployer/merge-review decision on a pull request.
// verdict must be "approve" (triggers the merge) or "reject" (returns the task
// to revision). body carries the decision notes.
func (c *ServerClient) SubmitPRDecision(ctx context.Context, taskID, prID, verdict, body, agentID string) error {
	if verdict != "approve" && verdict != "reject" {
		return fmt.Errorf("SubmitPRDecision: invalid verdict %q", verdict)
	}
	req := map[string]string{
		"decider_id": agentID,
		"body":       body,
	}
	return c.post(ctx, fmt.Sprintf("/api/tasks/%s/pull-requests/%s/%s", taskID, prID, verdict), req, nil)
}

// PostLog ships a log entry to the server.
func (c *ServerClient) PostLog(ctx context.Context, entry db.LogEntry) error {
	return c.post(ctx, "/api/logs", entry, nil)
}

// SetOffline notifies the server that this agent is going offline gracefully.
// The server marks the agent status as "offline" immediately, avoiding the
// stale-heartbeat window that would otherwise leave old records visible.
func (c *ServerClient) SetOffline(ctx context.Context, agentID string) error {
	return c.post(ctx, fmt.Sprintf("/api/agents/%s/offline", agentID), struct{}{}, nil)
}

// PostComment posts a completion comment on a task from this agent.
func (c *ServerClient) PostComment(ctx context.Context, taskID, body, authorID string) error {
	req := map[string]interface{}{
		"body":        body,
		"author_type": "agent",
		"author_id":   authorID,
	}
	return c.post(ctx, fmt.Sprintf("/api/tasks/%s/comments", taskID), req, nil)
}

// --- HTTP helpers ---

func (c *ServerClient) get(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, body)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *ServerClient) post(ctx context.Context, path string, in, out interface{}) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, b)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
