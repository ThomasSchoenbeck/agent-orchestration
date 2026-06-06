package tools

import (
	"context"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// ToolBackend is the server-facing data interface the agent tools use to do
// their work. It is implemented by the agent's HTTP ServerClient (no direct DB
// access).
//
// It is declared in package tools — not package agent — so that tools never
// imports agent. Since agent already imports tools (to register the tools), a
// tools→agent import would form a cycle. Go's implicit interfaces let
// *agent.ServerClient satisfy this without either package referencing the
// other's concrete types.
type ToolBackend interface {
	ListTasks(ctx context.Context, f db.TaskFilters) ([]*db.Task, error)
	PeekNextTask(ctx context.Context, roles []string) (*db.Task, error)
	SubmitTaskResult(ctx context.Context, taskID string, result map[string]interface{}, status string, metrics *api.TaskMetrics) error
	SaveContext(ctx context.Context, entry *db.ContextEntry) (*db.ContextEntry, error)
	QueryContext(ctx context.Context, projectID, query string, limit int) ([]*db.ContextEntry, error)
	PostTaskComment(ctx context.Context, taskID, body, role, reviewID string) (string, error)
	PlanProject(ctx context.Context, projectID, architecture string, packages []WorkPackageInput) (map[string]interface{}, error)
	CreateWorkPackage(ctx context.Context, projectID string, wp WorkPackageInput) (map[string]interface{}, error)
	BootstrapProject(ctx context.Context, projectID string, requirements, features []ScopeItemInput) (map[string]interface{}, error)
	SyncScope(ctx context.Context, projectID string, requirements, features []ScopeItemInput) (map[string]interface{}, error)
	CompleteProject(ctx context.Context, projectID, summary string) (map[string]interface{}, error)
}

// WorkPackageInput is one work package for plan_project / create_work_package.
// JSON tags match the server's request body shape.
type WorkPackageInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Role        string `json:"role"`
	Priority    int    `json:"priority"`
}

// ScopeItemInput is a requirement or feature for bootstrap_project / sync_scope.
type ScopeItemInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}
