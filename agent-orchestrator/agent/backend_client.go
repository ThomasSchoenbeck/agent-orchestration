package agent

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"agent-orchestrator/db"
	"agent-orchestrator/tools"
)

// This file implements tools.ToolBackend on *ServerClient: the agent's tools
// reach the server entirely over HTTP (no DB access). SubmitTaskResult is
// already defined in client.go and satisfies the interface as-is.

// ListTasks lists tasks for a project with optional filters.
func (c *ServerClient) ListTasks(ctx context.Context, f db.TaskFilters) ([]*db.Task, error) {
	q := url.Values{}
	if f.ProjectID != "" {
		q.Set("project_id", f.ProjectID)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.Role != "" {
		q.Set("role", f.Role)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	var tasks []*db.Task
	if err := c.get(ctx, "/api/tasks?"+q.Encode(), &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// PeekNextTask returns the next queued task matching roles, without claiming it.
func (c *ServerClient) PeekNextTask(ctx context.Context, roles []string) (*db.Task, error) {
	q := url.Values{}
	q.Set("roles", strings.Join(roles, ","))
	var resp struct {
		Task *db.Task `json:"task"`
	}
	if err := c.get(ctx, "/api/tasks/next?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	return resp.Task, nil
}

// SaveContext persists a context entry and returns the stored entry (with ID).
func (c *ServerClient) SaveContext(ctx context.Context, entry *db.ContextEntry) (*db.ContextEntry, error) {
	var saved db.ContextEntry
	if err := c.post(ctx, "/api/context/save", entry, &saved); err != nil {
		return nil, err
	}
	return &saved, nil
}

// QueryContext searches stored context for a project by keyword.
func (c *ServerClient) QueryContext(ctx context.Context, projectID, query string, limit int) ([]*db.ContextEntry, error) {
	q := url.Values{}
	q.Set("project_id", projectID)
	q.Set("query", query)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var entries []*db.ContextEntry
	if err := c.get(ctx, "/api/context/query?"+q.Encode(), &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// PostTaskComment posts an agent comment on a task and returns the comment ID.
func (c *ServerClient) PostTaskComment(ctx context.Context, taskID, body, role, reviewID string) (string, error) {
	req := map[string]interface{}{
		"body":        body,
		"author_type": "agent",
		"author_role": role,
		"review_id":   reviewID,
	}
	var created db.TaskComment
	if err := c.post(ctx, "/api/tasks/"+taskID+"/comments", req, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// PlanProject persists architecture context and creates a task per work package.
func (c *ServerClient) PlanProject(ctx context.Context, projectID, architecture string, packages []tools.WorkPackageInput) (map[string]interface{}, error) {
	body := map[string]interface{}{"architecture": architecture, "work_packages": packages}
	var resp map[string]interface{}
	if err := c.post(ctx, "/api/projects/"+projectID+"/plan", body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateWorkPackage creates a single task in a project.
func (c *ServerClient) CreateWorkPackage(ctx context.Context, projectID string, wp tools.WorkPackageInput) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.post(ctx, "/api/projects/"+projectID+"/work-packages", wp, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// BootstrapProject defines a project's scope (requirements + features).
func (c *ServerClient) BootstrapProject(ctx context.Context, projectID string, requirements, features []tools.ScopeItemInput) (map[string]interface{}, error) {
	body := map[string]interface{}{"requirements": requirements, "features": features}
	var resp map[string]interface{}
	if err := c.post(ctx, "/api/projects/"+projectID+"/bootstrap", body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// SyncScope reconciles a project's requirements/features with a desired set.
func (c *ServerClient) SyncScope(ctx context.Context, projectID string, requirements, features []tools.ScopeItemInput) (map[string]interface{}, error) {
	body := map[string]interface{}{"requirements": requirements, "features": features}
	var resp map[string]interface{}
	if err := c.post(ctx, "/api/projects/"+projectID+"/sync-scope", body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetProject fetches a project (including its description) over HTTP.
func (c *ServerClient) GetProject(ctx context.Context, projectID string) (*db.Project, error) {
	var p db.Project
	if err := c.get(ctx, "/api/projects/"+projectID, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListRequirements fetches a project's requirements over HTTP.
func (c *ServerClient) ListRequirements(ctx context.Context, projectID string) ([]*db.ProjectRequirement, error) {
	var reqs []*db.ProjectRequirement
	if err := c.get(ctx, "/api/projects/"+projectID+"/requirements", &reqs); err != nil {
		return nil, err
	}
	return reqs, nil
}

// ListFeatures fetches a project's features over HTTP.
func (c *ServerClient) ListFeatures(ctx context.Context, projectID string) ([]*db.ProjectFeature, error) {
	var feats []*db.ProjectFeature
	if err := c.get(ctx, "/api/projects/"+projectID+"/features", &feats); err != nil {
		return nil, err
	}
	return feats, nil
}

// CompleteProject marks a project complete.
func (c *ServerClient) CompleteProject(ctx context.Context, projectID, summary string) (map[string]interface{}, error) {
	body := map[string]interface{}{"summary": summary}
	var resp map[string]interface{}
	if err := c.post(ctx, "/api/projects/"+projectID+"/complete", body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
