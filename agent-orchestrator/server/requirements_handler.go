package server

import (
	"database/sql"
	"net/http"
	"strings"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// handleTaskDependencies handles:
//
//	GET    /api/tasks/{id}/dependencies
//	POST   /api/tasks/{id}/dependencies        body: {depends_on_id}
//	DELETE /api/tasks/{id}/dependencies        body: {depends_on_id}
func (s *Server) handleTaskDependencies(w http.ResponseWriter, r *http.Request, taskID string) {
	switch r.Method {
	case http.MethodGet:
		deps, err := s.db.ListDependencies(r.Context(), taskID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, deps)

	case http.MethodPost:
		var body api.AddDependencyRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.DependsOnID == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "depends_on_id is required")
			return
		}
		dep, err := s.db.AddDependency(r.Context(), taskID, body.DependsOnID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "itself") {
				api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, err.Error())
				return
			}
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, dep)

	case http.MethodDelete:
		var body api.RemoveDependencyRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.DependsOnID == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "depends_on_id is required")
			return
		}
		err := s.db.RemoveDependency(r.Context(), taskID, body.DependsOnID)
		if err == sql.ErrNoRows {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "dependency not found")
			return
		}
		if err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

// handleProjectRequirements handles:
//
//	GET  /api/projects/{id}/requirements
//	POST /api/projects/{id}/requirements
func (s *Server) handleProjectRequirements(w http.ResponseWriter, r *http.Request, projectID string) {
	switch r.Method {
	case http.MethodGet:
		reqs, err := s.db.ListRequirements(r.Context(), projectID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		// Attach linked-task counts.
		counts, _ := s.db.CountLinkedTasksForRequirements(r.Context(), projectID)
		type reqWithCount struct {
			*db.ProjectRequirement
			LinkedTasks int `json:"linked_tasks"`
		}
		out := make([]reqWithCount, len(reqs))
		for i, req := range reqs {
			out[i] = reqWithCount{req, counts[req.ID]}
		}
		api.WriteJSON(w, http.StatusOK, out)

	case http.MethodPost:
		var body api.CreateRequirementRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.Title == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "title is required")
			return
		}
		req := &db.ProjectRequirement{
			ProjectID: projectID,
			Title:     body.Title,
			Body:      body.Body,
			Status:    body.Status,
			Position:  body.Position,
		}
		if err := s.db.CreateRequirement(r.Context(), req); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, req)

	default:
		methodNotAllowed(w)
	}
}

// handleProjectRequirementDetail handles:
//
//	PATCH  /api/projects/{id}/requirements/{rid}
//	DELETE /api/projects/{id}/requirements/{rid}
func (s *Server) handleProjectRequirementDetail(w http.ResponseWriter, r *http.Request, projectID, reqID string) {
	switch r.Method {
	case http.MethodPatch:
		existing, err := s.db.GetRequirement(r.Context(), reqID)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if existing.ProjectID != projectID {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "requirement not found in project")
			return
		}
		var body api.UpdateRequirementRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.Title != nil {
			existing.Title = *body.Title
		}
		if body.Body != nil {
			existing.Body = *body.Body
		}
		if body.Status != nil {
			existing.Status = *body.Status
		}
		if body.Position != nil {
			existing.Position = *body.Position
		}
		if err := s.db.UpdateRequirement(r.Context(), existing); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, existing)

	case http.MethodDelete:
		existing, err := s.db.GetRequirement(r.Context(), reqID)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if existing.ProjectID != projectID {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "requirement not found in project")
			return
		}
		if err := s.db.DeleteRequirement(r.Context(), reqID); err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

// handleProjectFeatures handles:
//
//	GET  /api/projects/{id}/features
//	POST /api/projects/{id}/features
func (s *Server) handleProjectFeatures(w http.ResponseWriter, r *http.Request, projectID string) {
	switch r.Method {
	case http.MethodGet:
		feats, err := s.db.ListFeatures(r.Context(), projectID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		counts, _ := s.db.CountLinkedTasksForFeatures(r.Context(), projectID)
		type featWithCount struct {
			*db.ProjectFeature
			LinkedTasks int `json:"linked_tasks"`
		}
		out := make([]featWithCount, len(feats))
		for i, f := range feats {
			out[i] = featWithCount{f, counts[f.ID]}
		}
		api.WriteJSON(w, http.StatusOK, out)

	case http.MethodPost:
		var body api.CreateFeatureRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.Title == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "title is required")
			return
		}
		feat := &db.ProjectFeature{
			ProjectID: projectID,
			Title:     body.Title,
			Body:      body.Body,
			Status:    body.Status,
			Position:  body.Position,
		}
		if err := s.db.CreateFeature(r.Context(), feat); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, feat)

	default:
		methodNotAllowed(w)
	}
}

// handleProjectFeatureDetail handles:
//
//	PATCH  /api/projects/{id}/features/{fid}
//	DELETE /api/projects/{id}/features/{fid}
func (s *Server) handleProjectFeatureDetail(w http.ResponseWriter, r *http.Request, projectID, featID string) {
	switch r.Method {
	case http.MethodPatch:
		existing, err := s.db.GetFeature(r.Context(), featID)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if existing.ProjectID != projectID {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "feature not found in project")
			return
		}
		var body api.UpdateFeatureRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.Title != nil {
			existing.Title = *body.Title
		}
		if body.Body != nil {
			existing.Body = *body.Body
		}
		if body.Status != nil {
			existing.Status = *body.Status
		}
		if body.Position != nil {
			existing.Position = *body.Position
		}
		if err := s.db.UpdateFeature(r.Context(), existing); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, existing)

	case http.MethodDelete:
		existing, err := s.db.GetFeature(r.Context(), featID)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if existing.ProjectID != projectID {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "feature not found in project")
			return
		}
		if err := s.db.DeleteFeature(r.Context(), featID); err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

// handleTaskLinks handles:
//
//	GET    /api/tasks/{id}/links
//	POST   /api/tasks/{id}/links
//	DELETE /api/tasks/{id}/links  (body: {kind, target_id})
func (s *Server) handleTaskLinks(w http.ResponseWriter, r *http.Request, taskID string) {
	switch r.Method {
	case http.MethodGet:
		links, err := s.db.ListTaskLinks(r.Context(), taskID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if links == nil {
			links = []*db.TaskProjectLink{}
		}
		api.WriteJSON(w, http.StatusOK, links)

	case http.MethodPost:
		var body api.AddTaskLinkRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		link, err := s.db.AddTaskLink(r.Context(), taskID, body.Kind, body.TargetID)
		if err != nil {
			// cross-project or not-found errors are 400
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not match") || strings.Contains(err.Error(), "invalid kind") {
				api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, err.Error())
				return
			}
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, link)

	case http.MethodDelete:
		var body api.RemoveTaskLinkRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if err := s.db.RemoveTaskLink(r.Context(), taskID, body.Kind, body.TargetID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
				return
			}
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}
