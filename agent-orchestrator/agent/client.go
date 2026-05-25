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
func (c *ServerClient) Register(ctx context.Context, name string, roles []string, mode string, caps map[string]interface{}) (string, error) {
	body := api.RegisterAgentRequest{Name: name, Roles: roles, Mode: mode, Capabilities: caps}
	var resp api.RegisterAgentResponse
	if err := c.post(ctx, "/api/agents/register", body, &resp); err != nil {
		return "", fmt.Errorf("register: %w", err)
	}
	c.agentID = resp.AgentID
	return resp.AgentID, nil
}

// Heartbeat sends a heartbeat for the given agent ID.
func (c *ServerClient) Heartbeat(ctx context.Context, agentID string) error {
	return c.post(ctx, fmt.Sprintf("/api/agents/%s/heartbeat", agentID), nil, nil)
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
	var task db.Task
	if err := c.get(ctx, url, &task); err != nil {
		return nil, err
	}
	if task.ID == "" {
		return nil, nil
	}
	return &task, nil
}

// ClaimTask claims a task for this agent.
// The server returns api.ClaimTaskResponse; we extract just the task.
func (c *ServerClient) ClaimTask(ctx context.Context, taskID, agentID string) (*db.Task, error) {
	body := map[string]string{"agent_id": agentID}
	var resp struct {
		Task *db.Task `json:"task"`
	}
	if err := c.post(ctx, fmt.Sprintf("/api/tasks/%s/claim", taskID), body, &resp); err != nil {
		return nil, err
	}
	if resp.Task == nil {
		return nil, fmt.Errorf("claim: server returned nil task")
	}
	return resp.Task, nil
}

// SubmitTaskResult submits a completed or failed task result.
func (c *ServerClient) SubmitTaskResult(ctx context.Context, taskID string, result map[string]interface{}, status string, metrics *api.TaskMetrics) error {
	body := api.SubmitTaskResultRequest{Result: result, Status: status, Metrics: metrics}
	return c.post(ctx, fmt.Sprintf("/api/tasks/%s/result", taskID), body, nil)
}

// SubmitForReview notifies the server that the agent has pushed its branch and
// the task should transition to AWAITING_REVIEW.
func (c *ServerClient) SubmitForReview(ctx context.Context, taskID string) error {
	return c.post(ctx, fmt.Sprintf("/api/tasks/%s/submit-for-review", taskID), struct{}{}, nil)
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

// PostLog ships a log entry to the server.
func (c *ServerClient) PostLog(ctx context.Context, entry db.LogEntry) error {
	return c.post(ctx, "/api/logs", entry, nil)
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
