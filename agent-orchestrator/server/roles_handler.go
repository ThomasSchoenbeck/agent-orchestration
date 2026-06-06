package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"text/template"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// =========================================================================
// Roles (agent role definitions)
// =========================================================================

// defaultToolsForRole returns the suggested tool set for well-known role names.
// These are applied when a new role is created without an explicit allowed_tools
// list, so the UI immediately shows a sensible starting point for small models.
// Returning nil means "no restriction — send all tools".
func defaultToolsForRole(name string) []string {
	switch name {
	case "worker":
		return []string{"read_file", "write_file", "list_files", "apply_diff", "run_tests", "task_comment"}
	case "reviewer":
		return []string{"read_file", "list_files", "task_comment"}
	case "orchestrator":
		return []string{"list_tasks", "create_work_package", "plan_project", "bootstrap_project", "sync_scope", "complete_project", "query_context", "save_context", "task_comment"}
	default:
		return nil
	}
}

func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		roles, err := s.db.ListRoleDefinitions(r.Context())
		if err != nil {
			s.internalError(w, err)
			return
		}
		if roles == nil {
			roles = []*db.RoleDefinition{}
		}
		api.WriteJSON(w, http.StatusOK, roles)

	case http.MethodPost:
		var rd db.RoleDefinition
		if !s.decodeJSON(w, r, &rd) {
			return
		}
		if rd.Name == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "name is required")
			return
		}
		if rd.Temperature == 0 {
			rd.Temperature = 0.7
		}
		if rd.MaxTokens == 0 {
			rd.MaxTokens = 4096
		}
		rd.Enabled = true
		// Pre-populate allowed_tools with sensible defaults when not provided.
		// Users can edit these in the UI at any time.
		if len(rd.AllowedTools) == 0 {
			rd.AllowedTools = defaultToolsForRole(rd.Name)
		}
		if err := s.db.CreateRoleDefinition(r.Context(), &rd); err != nil {
			s.internalError(w, err)
			return
		}
		s.router.ReloadFromDB(s.db)
		api.WriteJSON(w, http.StatusCreated, rd)

	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleRoleDetail(w http.ResponseWriter, r *http.Request) {
	id := pathSegment(r.URL.Path, "/api/roles/", 0)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	sub := pathSegment(r.URL.Path, "/api/roles/", 1)

	// POST /api/roles/:id/preview-prompt
	if sub == "preview-prompt" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleRolePreviewPrompt(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rd, err := s.db.GetRoleDefinition(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, rd)

	case http.MethodPut:
		rd, err := s.db.GetRoleDefinition(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		oldName := rd.Name
		if !s.decodeJSON(w, r, rd) {
			return
		}
		rd.ID = id
		if rd.Name == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "name is required")
			return
		}
		if err := s.db.UpdateRoleDefinition(r.Context(), rd); err != nil {
			s.internalError(w, err)
			return
		}
		// Renaming a role: cascade the new name to every reference so providers,
		// agents, templates, and tasks keep pointing at this role.
		if rd.Name != oldName {
			if err := s.db.RenameRoleReferences(r.Context(), oldName, rd.Name); err != nil {
				s.internalError(w, err)
				return
			}
		}
		s.router.ReloadFromDB(s.db)
		api.WriteJSON(w, http.StatusOK, rd)

	case http.MethodDelete:
		if err := s.db.DeleteRoleDefinition(r.Context(), id); err != nil {
			s.internalError(w, err)
			return
		}
		s.router.ReloadFromDB(s.db)
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

// handleRolePreviewPrompt renders the system prompt template with provided variables.
func (s *Server) handleRolePreviewPrompt(w http.ResponseWriter, r *http.Request, id string) {
	rd, err := s.db.GetRoleDefinition(r.Context(), id)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
		return
	}

	// Variables are optional — decode if a body was sent.
	var vars map[string]interface{}
	if r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&vars)
	}

	rendered := rd.SystemPrompt
	if vars != nil && strings.Contains(rd.SystemPrompt, "{{") {
		tmpl, terr := template.New("sp").Parse(rd.SystemPrompt)
		if terr == nil {
			var buf bytes.Buffer
			if terr = tmpl.Execute(&buf, vars); terr == nil {
				rendered = buf.String()
			}
		}
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"rendered": rendered})
}
