package tools

import (
	"context"
	"fmt"

	"agent-orchestrator/db"
)

// progressBackend is the narrow data dependency of get_task_progress: reading a
// task's persisted checkpoint sessions. Satisfied by *agent.ServerClient. Kept
// separate from ToolBackend so adding progress reads doesn't ripple through every
// ToolBackend implementation.
type progressBackend interface {
	ListAgentSessions(ctx context.Context, taskID string) ([]*db.AgentSession, error)
}

// RegisterProgressTool registers get_task_progress. backend may be nil (e.g. the
// server's metadata registry, which never invokes handlers).
func RegisterProgressTool(reg *Registry, backend progressBackend) error {
	return reg.Register(getTaskProgressTool(backend))
}

func getTaskProgressTool(backend progressBackend) Definition {
	return Definition{
		Name: "get_task_progress",
		Description: "Reconstruct what has happened on this task so far from its durable record: the " +
			"ordered checkpoint summaries of prior work sessions (each with its round, kind, and status). " +
			"Use this when resuming a task in a new session — or on another agent that never had the " +
			"worktree — to catch up on progress before continuing.",
		Parameters: map[string]Param{
			"task_id": {Type: "string", Description: "The task to summarize progress for"},
		},
		Required: []string{"task_id"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			taskID, err := strArg(args, "task_id")
			if err != nil {
				return nil, err
			}
			if backend == nil {
				return nil, fmt.Errorf("get_task_progress: no backend configured")
			}
			sessions, err := backend.ListAgentSessions(ctx, taskID)
			if err != nil {
				return nil, fmt.Errorf("get_task_progress: %w", err)
			}
			checkpoints := make([]map[string]interface{}, 0, len(sessions))
			for _, s := range sessions {
				checkpoints = append(checkpoints, map[string]interface{}{
					"round":   s.Round,
					"kind":    s.Kind,
					"status":  s.Status,
					"summary": s.Summary,
				})
			}
			return map[string]interface{}{
				"task_id":     taskID,
				"checkpoints": checkpoints,
				"count":       len(checkpoints),
			}, nil
		},
	}
}
