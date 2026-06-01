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
		bootstrapProjectTool(database),
		syncScopeTool(database),
		completeProjectTool(database),
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
					Role:      role,
					Status:    db.TaskStatusBacklog,
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

// scopeItemJSON is the shape expected for each requirement/feature passed to
// bootstrap_project.
type scopeItemJSON struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// decodeScopeItems parses a JSON array (or single object) of scope items.
// An empty string yields no items (the field is optional).
func decodeScopeItems(s string) ([]scopeItemJSON, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if strings.HasPrefix(s, "[") {
		var out []scopeItemJSON
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return nil, fmt.Errorf("scope items JSON array: %w", err)
		}
		return out, nil
	}
	var single scopeItemJSON
	if err := json.Unmarshal([]byte(s), &single); err != nil {
		return nil, fmt.Errorf("scope item JSON object: %w", err)
	}
	return []scopeItemJSON{single}, nil
}

// bootstrapProjectTool defines a project's scope by creating requirements and
// features from the project description. Intended for a planner agent that has
// read the description and derived the structured scope.
func bootstrapProjectTool(database *db.Database) Definition {
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
			if _, err := database.GetProject(ctx, projectID); err != nil {
				return nil, fmt.Errorf("bootstrap_project: project not found: %w", err)
			}

			// First-time only: if scope already exists, do not duplicate it —
			// the planner should use sync_scope to reconcile instead.
			existingReqs, _ := database.ListRequirements(ctx, projectID)
			existingFeats, _ := database.ListFeatures(ctx, projectID)
			if len(existingReqs) > 0 || len(existingFeats) > 0 {
				return map[string]interface{}{
					"success": true,
					"skipped": true,
					"reason":  "project already has requirements/features; use sync_scope to reconcile after description changes",
				}, nil
			}

			reqs, err := decodeScopeItems(strArgOpt(args, "requirements"))
			if err != nil {
				return nil, fmt.Errorf("bootstrap_project: %w", err)
			}
			feats, err := decodeScopeItems(strArgOpt(args, "features"))
			if err != nil {
				return nil, fmt.Errorf("bootstrap_project: %w", err)
			}
			if len(reqs) == 0 && len(feats) == 0 {
				return nil, fmt.Errorf("bootstrap_project: provide at least one requirement or feature")
			}

			reqIDs := make([]string, 0, len(reqs))
			for i, r := range reqs {
				if strings.TrimSpace(r.Title) == "" {
					continue
				}
				rec := &db.ProjectRequirement{ProjectID: projectID, Title: r.Title, Body: r.Body, Position: i}
				if err := database.CreateRequirement(ctx, rec); err != nil {
					return nil, fmt.Errorf("bootstrap_project: create requirement %q: %w", r.Title, err)
				}
				reqIDs = append(reqIDs, rec.ID)
			}
			featIDs := make([]string, 0, len(feats))
			for i, f := range feats {
				if strings.TrimSpace(f.Title) == "" {
					continue
				}
				rec := &db.ProjectFeature{ProjectID: projectID, Title: f.Title, Body: f.Body, Position: i}
				if err := database.CreateFeature(ctx, rec); err != nil {
					return nil, fmt.Errorf("bootstrap_project: create feature %q: %w", f.Title, err)
				}
				featIDs = append(featIDs, rec.ID)
			}

			return map[string]interface{}{
				"success":           true,
				"requirement_ids":   reqIDs,
				"feature_ids":       featIDs,
				"requirement_count": len(reqIDs),
				"feature_count":     len(featIDs),
			}, nil
		},
	}
}

// normalizeTitle lowercases and trims a scope item title for matching.
func normalizeTitle(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// syncScopeTool reconciles a project's requirements/features against a planner-
// supplied desired set (derived from the current description). It is
// non-destructive: new intent is created, matched items are left untouched
// (preserving IDs, status, and task links), and existing items no longer in the
// desired set are flagged needs_review (never deleted). Clears scope_dirty.
func syncScopeTool(database *db.Database) Definition {
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
			if _, err := database.GetProject(ctx, projectID); err != nil {
				return nil, fmt.Errorf("sync_scope: project not found: %w", err)
			}
			desiredReqs, err := decodeScopeItems(strArgOpt(args, "requirements"))
			if err != nil {
				return nil, fmt.Errorf("sync_scope: %w", err)
			}
			desiredFeats, err := decodeScopeItems(strArgOpt(args, "features"))
			if err != nil {
				return nil, fmt.Errorf("sync_scope: %w", err)
			}

			var added, flagged []string
			unchanged := 0

			// --- requirements ---
			existingReqs, _ := database.ListRequirements(ctx, projectID)
			existingReqTitles := map[string]bool{}
			for _, r := range existingReqs {
				existingReqTitles[normalizeTitle(r.Title)] = true
			}
			desiredReqTitles := map[string]bool{}
			for _, r := range desiredReqs {
				desiredReqTitles[normalizeTitle(r.Title)] = true
			}
			for _, r := range existingReqs {
				if desiredReqTitles[normalizeTitle(r.Title)] {
					unchanged++
					continue
				}
				if r.Status != db.ScopeStatusNeedsReview {
					r.Status = db.ScopeStatusNeedsReview
					_ = database.UpdateRequirement(ctx, r)
				}
				flagged = append(flagged, "requirement: "+r.Title)
			}
			for i, r := range desiredReqs {
				if strings.TrimSpace(r.Title) == "" || existingReqTitles[normalizeTitle(r.Title)] {
					continue
				}
				rec := &db.ProjectRequirement{ProjectID: projectID, Title: r.Title, Body: r.Body, Position: len(existingReqs) + i}
				if err := database.CreateRequirement(ctx, rec); err != nil {
					return nil, fmt.Errorf("sync_scope: create requirement %q: %w", r.Title, err)
				}
				added = append(added, "requirement: "+r.Title)
			}

			// --- features ---
			existingFeats, _ := database.ListFeatures(ctx, projectID)
			existingFeatTitles := map[string]bool{}
			for _, f := range existingFeats {
				existingFeatTitles[normalizeTitle(f.Title)] = true
			}
			desiredFeatTitles := map[string]bool{}
			for _, f := range desiredFeats {
				desiredFeatTitles[normalizeTitle(f.Title)] = true
			}
			for _, f := range existingFeats {
				if desiredFeatTitles[normalizeTitle(f.Title)] {
					unchanged++
					continue
				}
				if f.Status != db.ScopeStatusNeedsReview {
					f.Status = db.ScopeStatusNeedsReview
					_ = database.UpdateFeature(ctx, f)
				}
				flagged = append(flagged, "feature: "+f.Title)
			}
			for i, f := range desiredFeats {
				if strings.TrimSpace(f.Title) == "" || existingFeatTitles[normalizeTitle(f.Title)] {
					continue
				}
				rec := &db.ProjectFeature{ProjectID: projectID, Title: f.Title, Body: f.Body, Position: len(existingFeats) + i}
				if err := database.CreateFeature(ctx, rec); err != nil {
					return nil, fmt.Errorf("sync_scope: create feature %q: %w", f.Title, err)
				}
				added = append(added, "feature: "+f.Title)
			}

			// Reconcile done: scope is fresh again.
			_ = database.SetScopeDirty(ctx, projectID, false)

			// Post the diff as a project-visible log for human review.
			_ = database.CreateLog(ctx, &db.LogEntry{
				ProjectID: projectID,
				Level:     "info",
				Message: fmt.Sprintf("Scope sync: %d added, %d unchanged, %d flagged for review",
					len(added), unchanged, len(flagged)),
				Metadata: map[string]interface{}{
					"event":   "scope_synced",
					"added":   added,
					"flagged": flagged,
				},
			})

			return map[string]interface{}{
				"success":   true,
				"added":     added,
				"unchanged": unchanged,
				"flagged":   flagged,
			}, nil
		},
	}
}

// completeProjectTool marks a project complete and disarms auto-queue, but only
// when its declared scope is satisfied: every feature done, every requirement
// satisfied, and no tasks in a non-terminal state. Requires creates_tasks.
func completeProjectTool(database *db.Database) Definition {
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
			p, err := database.GetProject(ctx, projectID)
			if err != nil {
				return nil, fmt.Errorf("complete_project: project not found: %w", err)
			}
			ok, reason, err := database.ProjectScopeSatisfied(ctx, projectID)
			if err != nil {
				return nil, fmt.Errorf("complete_project: %w", err)
			}
			if !ok {
				return nil, fmt.Errorf("complete_project: scope not satisfied — %s", reason)
			}

			summary := strArgOpt(args, "summary")
			p.Status = "complete"
			p.AutoQueue = false // disarm: halts the auto-queue replenishment loop
			if err := database.UpdateProject(ctx, p); err != nil {
				return nil, fmt.Errorf("complete_project: %w", err)
			}

			if summary != "" {
				_ = database.CreateContextEntry(ctx, &db.ContextEntry{
					ProjectID: projectID,
					Type:      "summary",
					Content:   "Project completion: " + summary,
				})
			}
			_ = database.CreateLog(ctx, &db.LogEntry{
				ProjectID: projectID,
				Level:     "info",
				Message:   "Project marked complete; auto-queue disarmed",
				Metadata:  map[string]interface{}{"event": "project_completed", "summary": summary},
			})

			return map[string]interface{}{"success": true, "status": "complete", "auto_queue": false}, nil
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

			role := strArgOpt(args, "role")
			if role == "" {
				role = "worker"
			}
			priority := intArgOpt(args, "priority", 5)

			task := &db.Task{
				ProjectID: projectID,
				Role:      role,
				Status:    db.TaskStatusBacklog,
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
				"priority":  priority,
			}, nil
		},
	}
}
