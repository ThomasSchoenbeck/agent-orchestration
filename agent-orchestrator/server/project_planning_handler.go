package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// Agent-only project planning endpoints (T5). These move the bodies of the
// planning tools (formerly in package tools, operating on *db.Database) onto the
// server, which owns the database. They are mounted under /api/agent/projects/*
// via handleAgentProjectDetail and reachable only behind the agent API key.
//
// The logic mirrors the original tools; tools become thin HTTP callers (T6).

type workPackageBody struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Role        string `json:"role"`
	Priority    int    `json:"priority"`
}

type scopeItemBody struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// resolveTaskRole maps an LLM-supplied role name to its stored id, matching the
// UI task-creation and agent-registration paths (which store role refs as ids).
// Without this, work packages created with a role name like "worker" never match
// an agent whose roles are stored as ids, so they are never claimed. Empty roles
// default to "worker"; unknown names are returned unchanged.
func (s *Server) resolveTaskRole(ctx context.Context, role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "worker"
	}
	if resolved, err := s.db.ResolveRoleRefs(ctx, []string{role}); err == nil {
		return resolved[0]
	}
	return role
}

// handleAgentProjectDetail intercepts the agent-only planning POST actions and
// delegates everything else to the shared handleProjectDetail.
func (s *Server) handleAgentProjectDetail(w http.ResponseWriter, r *http.Request) {
	id := pathSegment(r.URL.Path, "/api/projects/", 0)
	sub := pathSegment(r.URL.Path, "/api/projects/", 1)
	if r.Method == http.MethodPost && id != "" {
		switch sub {
		case "plan":
			s.handlePlanProject(w, r, id)
			return
		case "work-packages":
			s.handleCreateWorkPackage(w, r, id)
			return
		case "bootstrap":
			s.handleBootstrapProject(w, r, id)
			return
		case "sync-scope":
			s.handleSyncScope(w, r, id)
			return
		case "complete":
			s.handleCompleteProject(w, r, id)
			return
		}
	}
	s.handleProjectDetail(w, r)
}

// handleProjectResync enqueues an orchestrator task that reconciles a project's
// scope against its description. The task description comes from the orchestrator
// role definition's resync_prompt (seeded from config), falling back to the
// built-in DefaultResyncPrompt when unset.
//
// POST /api/projects/{id}/resync
func (s *Server) handleProjectResync(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	ctx := r.Context()
	if _, err := s.db.GetProject(ctx, projectID); err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "project not found")
		return
	}

	// The re-sync task description now lives on the orchestrator role definition;
	// fall back to the built-in default when unset or the role doesn't exist. The
	// same lookup yields the role id used for the task assignment.
	role := "orchestrator"
	description := db.DefaultResyncPrompt
	if rd, err := s.db.GetRoleDefinitionByName(ctx, role); err == nil {
		role = rd.ID
		if rd.ResyncPrompt != "" {
			description = rd.ResyncPrompt
		}
	}

	task := &db.Task{
		ProjectID: projectID,
		Role:      role,
		Priority:  8,
		Payload: map[string]interface{}{
			"mode":        "sync",
			"title":       "Re-sync project scope",
			"description": description,
		},
	}
	if err := s.db.CreateTask(ctx, task); err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, task)
}

// POST /api/agent/projects/{id}/plan
func (s *Server) handlePlanProject(w http.ResponseWriter, r *http.Request, projectID string) {
	var req struct {
		Architecture string            `json:"architecture"`
		WorkPackages []workPackageBody `json:"work_packages"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	if _, err := s.db.GetProject(ctx, projectID); err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "project not found")
		return
	}
	if strings.TrimSpace(req.Architecture) == "" {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "architecture is required")
		return
	}
	if len(req.WorkPackages) == 0 {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "work_packages must not be empty")
		return
	}

	if err := s.db.CreateContextEntry(ctx, &db.ContextEntry{
		ProjectID: projectID, Type: "summary", Content: "Architecture: " + req.Architecture,
	}); err != nil {
		s.internalError(w, err)
		return
	}

	taskIDs := make([]string, 0, len(req.WorkPackages))
	for _, wp := range req.WorkPackages {
		priority := wp.Priority
		if priority == 0 {
			priority = 5
		}
		role := s.resolveTaskRole(ctx, wp.Role)
		task := &db.Task{
			ProjectID: projectID, Role: role, Status: db.TaskStatusBacklog, Priority: priority,
			Payload: map[string]interface{}{"title": wp.Title, "description": wp.Description},
		}
		if err := s.db.CreateTask(ctx, task); err != nil {
			s.internalError(w, err)
			return
		}
		taskIDs = append(taskIDs, task.ID)
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"task_ids":     taskIDs,
		"task_count":   len(taskIDs),
		"architecture": req.Architecture,
		"planned_at":   time.Now().UTC().Format(time.RFC3339),
	})
}

// POST /api/agent/projects/{id}/work-packages
func (s *Server) handleCreateWorkPackage(w http.ResponseWriter, r *http.Request, projectID string) {
	var wp workPackageBody
	if !s.decodeJSON(w, r, &wp) {
		return
	}
	if strings.TrimSpace(wp.Title) == "" || strings.TrimSpace(wp.Description) == "" {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "title and description are required")
		return
	}
	role := s.resolveTaskRole(r.Context(), wp.Role)
	priority := wp.Priority
	if priority == 0 {
		priority = 5
	}
	task := &db.Task{
		ProjectID: projectID, Role: role, Status: db.TaskStatusBacklog, Priority: priority,
		Payload: map[string]interface{}{"title": wp.Title, "description": wp.Description},
	}
	if err := s.db.CreateTask(r.Context(), task); err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true, "task_id": task.ID, "title": wp.Title, "role": role, "priority": priority,
	})
}

// POST /api/agent/projects/{id}/bootstrap
func (s *Server) handleBootstrapProject(w http.ResponseWriter, r *http.Request, projectID string) {
	var req struct {
		Requirements []scopeItemBody `json:"requirements"`
		Features     []scopeItemBody `json:"features"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	if _, err := s.db.GetProject(ctx, projectID); err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "project not found")
		return
	}

	// First-time only: if scope already exists, do not duplicate — reconcile via sync-scope.
	existingReqs, _ := s.db.ListRequirements(ctx, projectID)
	existingFeats, _ := s.db.ListFeatures(ctx, projectID)
	if len(existingReqs) > 0 || len(existingFeats) > 0 {
		api.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true, "skipped": true,
			"reason": "project already has requirements/features; use sync-scope to reconcile after description changes",
		})
		return
	}
	if len(req.Requirements) == 0 && len(req.Features) == 0 {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "provide at least one requirement or feature")
		return
	}

	reqIDs := make([]string, 0, len(req.Requirements))
	for i, rq := range req.Requirements {
		if strings.TrimSpace(rq.Title) == "" {
			continue
		}
		rec := &db.ProjectRequirement{ProjectID: projectID, Title: rq.Title, Body: rq.Body, Position: i}
		if err := s.db.CreateRequirement(ctx, rec); err != nil {
			s.internalError(w, err)
			return
		}
		reqIDs = append(reqIDs, rec.ID)
	}
	featIDs := make([]string, 0, len(req.Features))
	for i, f := range req.Features {
		if strings.TrimSpace(f.Title) == "" {
			continue
		}
		rec := &db.ProjectFeature{ProjectID: projectID, Title: f.Title, Body: f.Body, Position: i}
		if err := s.db.CreateFeature(ctx, rec); err != nil {
			s.internalError(w, err)
			return
		}
		featIDs = append(featIDs, rec.ID)
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":           true,
		"requirement_ids":   reqIDs,
		"feature_ids":       featIDs,
		"requirement_count": len(reqIDs),
		"feature_count":     len(featIDs),
	})
}

// POST /api/agent/projects/{id}/sync-scope
func (s *Server) handleSyncScope(w http.ResponseWriter, r *http.Request, projectID string) {
	var req struct {
		Requirements []scopeItemBody `json:"requirements"`
		Features     []scopeItemBody `json:"features"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	if _, err := s.db.GetProject(ctx, projectID); err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "project not found")
		return
	}

	var added, flagged []string
	unchanged := 0

	// --- requirements ---
	existingReqs, _ := s.db.ListRequirements(ctx, projectID)
	existingReqTitles := map[string]bool{}
	for _, rq := range existingReqs {
		existingReqTitles[normalizeScopeTitle(rq.Title)] = true
	}
	desiredReqTitles := map[string]bool{}
	for _, rq := range req.Requirements {
		desiredReqTitles[normalizeScopeTitle(rq.Title)] = true
	}
	for _, rq := range existingReqs {
		if desiredReqTitles[normalizeScopeTitle(rq.Title)] {
			unchanged++
			continue
		}
		if rq.Status != db.ScopeStatusNeedsReview {
			rq.Status = db.ScopeStatusNeedsReview
			_ = s.db.UpdateRequirement(ctx, rq)
		}
		flagged = append(flagged, "requirement: "+rq.Title)
	}
	for i, rq := range req.Requirements {
		if strings.TrimSpace(rq.Title) == "" || existingReqTitles[normalizeScopeTitle(rq.Title)] {
			continue
		}
		rec := &db.ProjectRequirement{ProjectID: projectID, Title: rq.Title, Body: rq.Body, Position: len(existingReqs) + i}
		if err := s.db.CreateRequirement(ctx, rec); err != nil {
			s.internalError(w, err)
			return
		}
		added = append(added, "requirement: "+rq.Title)
	}

	// --- features ---
	existingFeats, _ := s.db.ListFeatures(ctx, projectID)
	existingFeatTitles := map[string]bool{}
	for _, f := range existingFeats {
		existingFeatTitles[normalizeScopeTitle(f.Title)] = true
	}
	desiredFeatTitles := map[string]bool{}
	for _, f := range req.Features {
		desiredFeatTitles[normalizeScopeTitle(f.Title)] = true
	}
	for _, f := range existingFeats {
		if desiredFeatTitles[normalizeScopeTitle(f.Title)] {
			unchanged++
			continue
		}
		if f.Status != db.ScopeStatusNeedsReview {
			f.Status = db.ScopeStatusNeedsReview
			_ = s.db.UpdateFeature(ctx, f)
		}
		flagged = append(flagged, "feature: "+f.Title)
	}
	for i, f := range req.Features {
		if strings.TrimSpace(f.Title) == "" || existingFeatTitles[normalizeScopeTitle(f.Title)] {
			continue
		}
		rec := &db.ProjectFeature{ProjectID: projectID, Title: f.Title, Body: f.Body, Position: len(existingFeats) + i}
		if err := s.db.CreateFeature(ctx, rec); err != nil {
			s.internalError(w, err)
			return
		}
		added = append(added, "feature: "+f.Title)
	}

	// Reconcile done: scope is fresh again.
	_ = s.db.SetScopeDirty(ctx, projectID, false)

	_ = s.db.CreateLog(ctx, &db.LogEntry{
		ProjectID: projectID,
		Level:     "info",
		Message: fmt.Sprintf("Scope sync: %d added, %d unchanged, %d flagged for review",
			len(added), unchanged, len(flagged)),
		Metadata: map[string]interface{}{"event": "scope_synced", "added": added, "flagged": flagged},
	})

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "added": added, "unchanged": unchanged, "flagged": flagged,
	})
}

// POST /api/agent/projects/{id}/complete
//
// The creates_tasks capability gate intentionally stays on the tool side (T6);
// the server trusts the gated agent namespace.
func (s *Server) handleCompleteProject(w http.ResponseWriter, r *http.Request, projectID string) {
	var req struct {
		Summary string `json:"summary"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	p, err := s.db.GetProject(ctx, projectID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "project not found")
		return
	}
	ok, reason, err := s.db.ProjectScopeSatisfied(ctx, projectID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !ok {
		api.WriteError(w, http.StatusConflict, api.ErrCodeConflict, "scope not satisfied — "+reason)
		return
	}

	p.Status = "complete"
	p.AutoQueue = false
	if err := s.db.UpdateProject(ctx, p); err != nil {
		s.internalError(w, err)
		return
	}
	if req.Summary != "" {
		_ = s.db.CreateContextEntry(ctx, &db.ContextEntry{
			ProjectID: projectID, Type: "summary", Content: "Project completion: " + req.Summary,
		})
	}
	_ = s.db.CreateLog(ctx, &db.LogEntry{
		ProjectID: projectID, Level: "info",
		Message:  "Project marked complete; auto-queue disarmed",
		Metadata: map[string]interface{}{"event": "project_completed", "summary": req.Summary},
	})

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "status": "complete", "auto_queue": false,
	})
}

// normalizeScopeTitle lowercases and trims a scope item title for matching.
func normalizeScopeTitle(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
