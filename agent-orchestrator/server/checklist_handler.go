package server

import (
	"database/sql"
	"log"
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// handleTaskComments handles:
//
//	GET    /api/tasks/{id}/comments[?review_id=…]
//	POST   /api/tasks/{id}/comments
//	DELETE /api/tasks/{id}/comments/{commentID}
func (s *Server) handleTaskComments(w http.ResponseWriter, r *http.Request, taskID string, parts []string) {
	// parts[2] is the optional comment ID
	commentID := ""
	if len(parts) > 2 {
		commentID = parts[2]
	}

	switch r.Method {
	case http.MethodGet:
		reviewID := r.URL.Query().Get("review_id")
		comments, err := s.db.ListComments(r.Context(), taskID, reviewID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, comments)

	case http.MethodPost:
		if commentID != "" {
			methodNotAllowed(w)
			return
		}
		var body api.CreateCommentRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.Body == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "body is required")
			return
		}
		authorType := body.AuthorType
		if authorType == "" {
			authorType = "user"
		}
		c := &db.TaskComment{
			TaskID:     taskID,
			ReviewID:   body.ReviewID,
			AuthorType: authorType,
			AuthorRole: body.AuthorRole,
			AuthorID:   body.AuthorID,
			Body:       body.Body,
		}
		if err := s.db.CreateComment(r.Context(), c); err != nil {
			s.internalError(w, err)
			return
		}
		log.Printf("task %s: comment added (author_type=%s author_id=%s)", taskID, c.AuthorType, c.AuthorID)
		api.WriteJSON(w, http.StatusCreated, c)

	case http.MethodDelete:
		if commentID == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "comment id required")
			return
		}
		err := s.db.DeleteComment(r.Context(), taskID, commentID)
		if err == sql.ErrNoRows {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "comment not found")
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

// handleTaskChecklist handles:
//
//	GET  /api/tasks/{id}/checklist
//	POST /api/tasks/{id}/checklist
func (s *Server) handleTaskChecklist(w http.ResponseWriter, r *http.Request, taskID string) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.db.ListChecklistItems(r.Context(), taskID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, items)

	case http.MethodPost:
		var body api.CreateChecklistItemRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.Label == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "label is required")
			return
		}
		item := &db.ChecklistItem{
			TaskID:     taskID,
			GroupLabel: body.GroupLabel,
			Position:   body.Position,
			Label:      body.Label,
			Status:     body.Status,
		}
		if err := s.db.CreateChecklistItem(r.Context(), item); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, item)

	default:
		methodNotAllowed(w)
	}
}

// handleTaskChecklistItem handles:
//
//	PATCH  /api/tasks/{id}/checklist/{itemID}
//	DELETE /api/tasks/{id}/checklist/{itemID}
func (s *Server) handleTaskChecklistItem(w http.ResponseWriter, r *http.Request, taskID, itemID string) {
	switch r.Method {
	case http.MethodPatch:
		var body api.UpdateChecklistItemRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		// Fetch existing to apply patch.
		items, err := s.db.ListChecklistItems(r.Context(), taskID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		var item *db.ChecklistItem
		for _, it := range items {
			if it.ID == itemID {
				item = it
				break
			}
		}
		if item == nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "checklist item not found")
			return
		}
		if body.GroupLabel != nil {
			item.GroupLabel = *body.GroupLabel
		}
		if body.Position != nil {
			item.Position = *body.Position
		}
		if body.Label != nil {
			item.Label = *body.Label
		}
		if body.Status != nil {
			item.Status = *body.Status
		}
		if err := s.db.UpdateChecklistItem(r.Context(), item); err == sql.ErrNoRows {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "checklist item not found")
			return
		} else if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, item)

	case http.MethodDelete:
		if err := s.db.DeleteChecklistItem(r.Context(), taskID, itemID); err == sql.ErrNoRows {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "checklist item not found")
			return
		} else if err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

// handleTaskChecklistIterations handles:
//
//	POST /api/tasks/{id}/checklist/iterations  — clone latest group with reset status
func (s *Server) handleTaskChecklistIterations(w http.ResponseWriter, r *http.Request, taskID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	newGroup, err := s.db.CloneChecklistIteration(r.Context(), taskID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"group_label": newGroup})
}

// ── Checklist templates ───────────────────────────────────────────────────────

// handleChecklistTemplates handles:
//
//	GET  /api/checklist-templates
//	POST /api/checklist-templates
func (s *Server) handleChecklistTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		templates, err := s.db.ListChecklistTemplates(r.Context())
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, templates)

	case http.MethodPost:
		var body api.CreateChecklistTemplateRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.Name == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "name is required")
			return
		}
		t := &db.ChecklistTemplate{
			Name:      body.Name,
			ItemsJSON: body.ItemsJSON,
		}
		if err := s.db.CreateChecklistTemplate(r.Context(), t); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, t)

	default:
		methodNotAllowed(w)
	}
}

// handleChecklistTemplateDetail handles:
//
//	PUT    /api/checklist-templates/{id}
//	DELETE /api/checklist-templates/{id}
func (s *Server) handleChecklistTemplateDetail(w http.ResponseWriter, r *http.Request) {
	id := pathSegment(r.URL.Path, "/api/checklist-templates/", 0)
	if id == "" {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "template id required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		existing, err := s.db.GetChecklistTemplate(r.Context(), id)
		if err == sql.ErrNoRows {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "template not found")
			return
		}
		if err != nil {
			s.internalError(w, err)
			return
		}
		var body api.UpdateChecklistTemplateRequest
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.Name != nil {
			existing.Name = *body.Name
		}
		if body.ItemsJSON != nil {
			existing.ItemsJSON = *body.ItemsJSON
		}
		if err := s.db.UpdateChecklistTemplate(r.Context(), existing); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, existing)

	case http.MethodDelete:
		if err := s.db.DeleteChecklistTemplate(r.Context(), id); err == sql.ErrNoRows {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "template not found")
			return
		} else if err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}
