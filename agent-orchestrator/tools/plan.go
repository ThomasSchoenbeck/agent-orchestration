package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RegisterPlanTools registers the planning tools. They validate and parse the
// LLM's arguments, then delegate the actual work to the server over HTTP via the
// ToolBackend (the server owns the database).
func RegisterPlanTools(reg *Registry, backend ToolBackend) error {
	defs := []Definition{
		planProjectTool(backend),
		createWorkPackageTool(backend),
		bootstrapProjectTool(backend),
		syncScopeTool(backend),
		completeProjectTool(backend),
	}
	for _, d := range defs {
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	return nil
}

// decodeWorkPackages parses a JSON array (or single object) of work packages
// into the backend input type.
func decodeWorkPackages(s string) ([]WorkPackageInput, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("work_packages is empty")
	}
	if strings.HasPrefix(s, "[") {
		var out []WorkPackageInput
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return nil, fmt.Errorf("work_packages JSON array: %w", err)
		}
		return out, nil
	}
	var single WorkPackageInput
	if err := json.Unmarshal([]byte(s), &single); err != nil {
		return nil, fmt.Errorf("work_packages JSON object: %w", err)
	}
	return []WorkPackageInput{single}, nil
}

// decodeScopeItems parses a JSON array (or single object) of scope items into
// the backend input type. An empty string yields no items (the field is optional).
func decodeScopeItems(s string) ([]ScopeItemInput, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if strings.HasPrefix(s, "[") {
		var out []ScopeItemInput
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return nil, fmt.Errorf("scope items JSON array: %w", err)
		}
		return out, nil
	}
	var single ScopeItemInput
	if err := json.Unmarshal([]byte(s), &single); err != nil {
		return nil, fmt.Errorf("scope item JSON object: %w", err)
	}
	return []ScopeItemInput{single}, nil
}

// planProjectTool creates tasks for every work-package in a project plan.
func planProjectTool(backend ToolBackend) Definition {
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
			packages, err := decodeWorkPackages(wpJSON)
			if err != nil {
				return nil, fmt.Errorf("plan_project: %w", err)
			}
			if len(packages) == 0 {
				return nil, fmt.Errorf("plan_project: work_packages must not be empty")
			}
			res, err := backend.PlanProject(ctx, projectID, architecture, packages)
			if err != nil {
				return nil, fmt.Errorf("plan_project: %w", err)
			}
			return res, nil
		},
	}
}

// bootstrapProjectTool defines a project's scope by creating requirements and
// features from the project description.
func bootstrapProjectTool(backend ToolBackend) Definition {
	return Definition{
		Name: "bootstrap_project",
		Description: "Define a project's scope: create requirements and features derived from the " +
			"project description. Use this before planning work packages when a project has only a description.",
		Parameters: map[string]Param{
			"project_id":   {Type: "string", Description: "ID of the project to bootstrap"},
			"requirements": {Type: "string", Description: `JSON array of requirement objects with fields: title (string), body (string)`},
			"features":     {Type: "string", Description: `JSON array of feature objects with fields: title (string), body (string)`},
		},
		Required: []string{"project_id"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			projectID, err := strArg(args, "project_id")
			if err != nil {
				return nil, err
			}
			reqs, err := decodeScopeItems(strArgOpt(args, "requirements"))
			if err != nil {
				return nil, fmt.Errorf("bootstrap_project: %w", err)
			}
			feats, err := decodeScopeItems(strArgOpt(args, "features"))
			if err != nil {
				return nil, fmt.Errorf("bootstrap_project: %w", err)
			}
			res, err := backend.BootstrapProject(ctx, projectID, reqs, feats)
			if err != nil {
				return nil, fmt.Errorf("bootstrap_project: %w", err)
			}
			return res, nil
		},
	}
}

// syncScopeTool reconciles a project's requirements/features against a planner-
// supplied desired set derived from the current description.
func syncScopeTool(backend ToolBackend) Definition {
	return Definition{
		Name: "sync_scope",
		Description: "Reconcile a project's requirements and features with the current description. " +
			"Non-destructive: creates new items, leaves matched items untouched, and flags items no longer " +
			"covered by the description as needs_review (never deletes). Use after the description changes.",
		Parameters: map[string]Param{
			"project_id":   {Type: "string", Description: "ID of the project to reconcile"},
			"requirements": {Type: "string", Description: `JSON array of the desired requirement objects: title (string), body (string)`},
			"features":     {Type: "string", Description: `JSON array of the desired feature objects: title (string), body (string)`},
		},
		Required: []string{"project_id"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			projectID, err := strArg(args, "project_id")
			if err != nil {
				return nil, err
			}
			reqs, err := decodeScopeItems(strArgOpt(args, "requirements"))
			if err != nil {
				return nil, fmt.Errorf("sync_scope: %w", err)
			}
			feats, err := decodeScopeItems(strArgOpt(args, "features"))
			if err != nil {
				return nil, fmt.Errorf("sync_scope: %w", err)
			}
			res, err := backend.SyncScope(ctx, projectID, reqs, feats)
			if err != nil {
				return nil, fmt.Errorf("sync_scope: %w", err)
			}
			return res, nil
		},
	}
}

// completeProjectTool marks a project complete. The creates_tasks capability
// gate stays on the tool side; the server trusts the gated agent namespace.
func completeProjectTool(backend ToolBackend) Definition {
	return Definition{
		Name: "complete_project",
		Description: "Mark a project complete and stop auto-queue. Succeeds only when every feature is done, " +
			"every requirement is satisfied, and no tasks remain in a non-terminal state. " +
			"Provide a short summary of what was accomplished.",
		Parameters: map[string]Param{
			"project_id": {Type: "string", Description: "ID of the project to complete"},
			"summary":    {Type: "string", Description: "Short summary of what the project accomplished"},
		},
		Required: []string{"project_id"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if !contextHasCapability(ctx, "creates_tasks") {
				return nil, fmt.Errorf("complete_project: requires the creates_tasks capability")
			}
			projectID, err := strArg(args, "project_id")
			if err != nil {
				return nil, err
			}
			res, err := backend.CompleteProject(ctx, projectID, strArgOpt(args, "summary"))
			if err != nil {
				return nil, fmt.Errorf("complete_project: %w", err)
			}
			return res, nil
		},
	}
}

// createWorkPackageTool creates a single task in an existing project.
func createWorkPackageTool(backend ToolBackend) Definition {
	return Definition{
		Name:        "create_work_package",
		Description: "Create a single work-package task inside a project. Returns the new task's ID.",
		Parameters: map[string]Param{
			"project_id":  {Type: "string", Description: "ID of the project"},
			"title":       {Type: "string", Description: "Short title for the work package"},
			"description": {Type: "string", Description: "Detailed description of what needs to be done"},
			"role":        {Type: "string", Description: "Agent role required: worker|reviewer|planner|… (default: worker)"},
			"priority":    {Type: "number", Description: "Priority 1-10 (default: 5)"},
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
			wp := WorkPackageInput{
				Title:       title,
				Description: description,
				Role:        strArgOpt(args, "role"),
				Priority:    intArgOpt(args, "priority", 0),
			}
			res, err := backend.CreateWorkPackage(ctx, projectID, wp)
			if err != nil {
				return nil, fmt.Errorf("create_work_package: %w", err)
			}
			return res, nil
		},
	}
}
