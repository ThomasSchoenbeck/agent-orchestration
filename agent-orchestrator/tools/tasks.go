package tools

import (
	"context"
	"fmt"

	"agent-orchestrator/db"
)

// RegisterTaskTools registers all task-management tools into reg.
// These tools allow an agent to list tasks, submit results, and fetch the
// next available task — all through the database directly.
func RegisterTaskTools(reg *Registry, database *db.Database) error {
	tools := []Definition{
		listTasksTool(database),
		submitTaskResultTool(database),
		getNextTaskTool(database),
	}
	for _, t := range tools {
		if err := reg.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// listTasksTool returns a Definition for the list_tasks tool.
func listTasksTool(database *db.Database) Definition {
	return Definition{
		Name:        "list_tasks",
		Description: "List tasks for a project. Optionally filter by status or role.",
		Parameters: map[string]Param{
			"project_id": {Type: "string", Description: "Project ID to list tasks for (required)"},
			"status":     {Type: "string", Description: "Filter by status: planned|in_progress|completed|failed (optional)"},
			"role":       {Type: "string", Description: "Filter by role: worker|orchestrator|reviewer (optional)"},
			"limit":      {Type: "number", Description: "Maximum number of tasks to return (default 50)"},
		},
		Required: []string{"project_id"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			projectID, err := strArg(args, "project_id")
			if err != nil {
				return nil, err
			}
			filters := db.TaskFilters{
				ProjectID: projectID,
				Status:    strArgOpt(args, "status"),
				Role:      strArgOpt(args, "role"),
				Limit:     intArgOpt(args, "limit", 50),
			}
			tasks, err := database.ListTasks(ctx, filters)
			if err != nil {
				return nil, fmt.Errorf("list_tasks: %w", err)
			}
			return map[string]interface{}{
				"tasks": tasks,
				"count": len(tasks),
			}, nil
		},
	}
}

// submitTaskResultTool returns a Definition for the submit_task_result tool.
func submitTaskResultTool(database *db.Database) Definition {
	return Definition{
		Name:        "submit_task_result",
		Description: "Submit the result of a completed or failed task. Updates the task status and stores the result.",
		Parameters: map[string]Param{
			"task_id": {Type: "string", Description: "ID of the task being completed"},
			"status":  {Type: "string", Description: "Final status: completed|failed|needs_review"},
			"output":  {Type: "string", Description: "Text output or summary of the work done"},
			"error":   {Type: "string", Description: "Error message if status is failed (optional)"},
		},
		Required: []string{"task_id", "status", "output"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			taskID, err := strArg(args, "task_id")
			if err != nil {
				return nil, err
			}
			status, err := strArg(args, "status")
			if err != nil {
				return nil, err
			}
			output, err := strArg(args, "output")
			if err != nil {
				return nil, err
			}

			result := map[string]interface{}{
				"output": output,
			}
			if errMsg := strArgOpt(args, "error"); errMsg != "" {
				result["error"] = errMsg
			}

			if err := database.SubmitTaskResult(ctx, taskID, result, status); err != nil {
				return nil, fmt.Errorf("submit_task_result: %w", err)
			}
			return map[string]interface{}{
				"success": true,
				"task_id": taskID,
				"status":  status,
			}, nil
		},
	}
}

// getNextTaskTool returns a Definition for the get_next_task tool.
func getNextTaskTool(database *db.Database) Definition {
	return Definition{
		Name:        "get_next_task",
		Description: "Fetch the next available planned task matching the given roles. Returns null if no tasks are available.",
		Parameters: map[string]Param{
			"roles": {Type: "string", Description: "Comma-separated roles the agent can handle (e.g. 'worker,reviewer')"},
		},
		Required: []string{"roles"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			rolesStr, err := strArg(args, "roles")
			if err != nil {
				return nil, err
			}
			var roles []string
			start := 0
			for i := 0; i <= len(rolesStr); i++ {
				if i == len(rolesStr) || rolesStr[i] == ',' {
					if tok := rolesStr[start:i]; tok != "" {
						roles = append(roles, tok)
					}
					start = i + 1
				}
			}

			task, err := database.GetNextTask(ctx, roles)
			if err != nil {
				return nil, fmt.Errorf("get_next_task: %w", err)
			}
			if task == nil {
				return map[string]interface{}{"task": nil}, nil
			}
			return map[string]interface{}{"task": task}, nil
		},
	}
}
