package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent-orchestrator/db"
)

// RegisterPlanTools registers the planning tools (plan_project,
// create_work_package) into reg.
func RegisterPlanTools(reg *Registry, database *db.Database) error {
	defs := []Definition{
		planProjectTool(database),
		createWorkPackageTool(database),
	}
	for _, d := range defs {
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	return nil
}

// workPackageJSON is the shape expected inside the work_packages JSON array.
type workPackageJSON struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Role        string `json:"role"`
	Priority    int    `json:"priority"`
}

// decodeWorkPackages parses a JSON array (or single object) of work packages.
func decodeWorkPackages(s string) ([]workPackageJSON, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("work_packages is empty")
	}
	if strings.HasPrefix(s, "[") {
		var out []workPackageJSON
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return nil, fmt.Errorf("work_packages JSON array: %w", err)
		}
		return out, nil
	}
	// Treat as single object.
	var single workPackageJSON
	if err := json.Unmarshal([]byte(s), &single); err != nil {
		return nil, fmt.Errorf("work_packages JSON object: %w", err)
	}
	return []workPackageJSON{single}, nil
}

// planProjectTool creates tasks for every work-package in a project plan.
func planProjectTool(database *db.Database) Definition {
	return Definition{
		Name: "plan_project",
		Description: "Create a development plan for a project. " +
			"Persists architecture context and creates a task for each work package. " +
			"Returns the IDs of all created tasks.",
		Parameters: map[string]Param{
			"project_id":    {Type: "string", Description: "ID of the existing project to plan"},
			"architecture":  {Type: "string", Description: "High-level architecture summary"},
			"work_packages": {Type: "string", Description: `JSON array of work-package objects with fields: title (string), description (string), role (string), priority (number 1-10)`},
		},
		Required: []string{"project_id", "architecture", "work_packages"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			projectID, err := strArg(args, "project_id")
			if err != nil {
				return nil, err
			}
			architecture, err := strArg(args, "architecture")
			if err != nil {
				return nil, err
			}
			wpJSON, err := strArg(args, "work_packages")
			if err != nil {
				return nil, err
			}

			// Verify the project exists.
			if _, err := database.GetProject(ctx, projectID); err != nil {
				return nil, fmt.Errorf("plan_project: project not found: %w", err)
			}

			// Persist architecture summary as a context entry.
			entry := &db.ContextEntry{
				ProjectID: projectID,
				Type:      "summary",
				Content:   "Architecture: " + architecture,
			}
			if err := database.CreateContextEntry(ctx, entry); err != nil {
				return nil, fmt.Errorf("plan_project: save architecture context: %w", err)
			}

			// Parse and validate work packages.
			packages, err := decodeWorkPackages(wpJSON)
			if err != nil {
				return nil, fmt.Errorf("plan_project: %w", err)
			}
			if len(packages) == 0 {
				return nil, fmt.Errorf("plan_project: work_packages must not be empty")
			}

			// Create a task for each work package.
			taskIDs := make([]string, 0, len(packages))
			for _, wp := range packages {
				priority := wp.Priority
				if priority == 0 {
					priority = 5
				}
				role := wp.Role
				if role == "" {
					role = "worker"
				}
				task := &db.Task{
					ProjectID: projectID,
					Type:      "implement",
					Role:      role,
					Status:    "planned",
					Priority:  priority,
					Payload: map[string]interface{}{
						"title":       wp.Title,
						"description": wp.Description,
					},
				}
				if err := database.CreateTask(ctx, task); err != nil {
					return nil, fmt.Errorf("plan_project: create task %q: %w", wp.Title, err)
				}
				taskIDs = append(taskIDs, task.ID)
			}

			return map[string]interface{}{
				"success":      true,
				"task_ids":     taskIDs,
				"task_count":   len(taskIDs),
				"architecture": architecture,
				"planned_at":   time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
	}
}

// createWorkPackageTool creates a single task in an existing project.
func createWorkPackageTool(database *db.Database) Definition {
	return Definition{
		Name:        "create_work_package",
		Description: "Create a single work-package task inside a project. Returns the new task's ID.",
		Parameters: map[string]Param{
			"project_id":  {Type: "string", Description: "ID of the project"},
			"title":       {Type: "string", Description: "Short title for the work package"},
			"description": {Type: "string", Description: "Detailed description of what needs to be done"},
			"role":        {Type: "string", Description: "Agent role required: worker|orchestrator|reviewer (default: worker)"},
			"priority":    {Type: "number", Description: "Priority 1-10 (default: 5)"},
			"task_type":   {Type: "string", Description: "Task type: implement|review|plan (default: implement)"},
		},
		Required: []string{"project_id", "title", "description"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			projectID, err := strArg(args, "project_id")
			if err != nil {
				return nil, err
			}
			title, err := strArg(args, "title")
			if err != nil {
				return nil, err
			}
			description, err := strArg(args, "description")
			if err != nil {
				return nil, err
			}

			role := strArgOpt(args, "role")
			if role == "" {
				role = "worker"
			}
			taskType := strArgOpt(args, "task_type")
			if taskType == "" {
				taskType = "implement"
			}
			priority := intArgOpt(args, "priority", 5)

			task := &db.Task{
				ProjectID: projectID,
				Type:      taskType,
				Role:      role,
				Status:    "planned",
				Priority:  priority,
				Payload: map[string]interface{}{
					"title":       title,
					"description": description,
				},
			}
			if err := database.CreateTask(ctx, task); err != nil {
				return nil, fmt.Errorf("create_work_package: %w", err)
			}

			return map[string]interface{}{
				"success":   true,
				"task_id":   task.ID,
				"title":     title,
				"role":      role,
				"task_type": taskType,
				"priority":  priority,
			}, nil
		},
	}
}
