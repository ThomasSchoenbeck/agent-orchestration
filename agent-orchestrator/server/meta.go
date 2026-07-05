package server

import (
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/tools"
)

// metaItem describes a task type or agent role.
type metaItem struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
	ID          string `json:"id,omitempty"` // role-definition id (roles only) for id↔name display mapping
}

var taskRoles = []metaItem{
	{Value: "worker", Label: "Worker", Description: "General-purpose implementation agent"},
	{Value: "reviewer", Label: "Reviewer", Description: "Reviews and approves work produced by a worker"},
	{Value: "tester", Label: "Tester", Description: "Writes and runs tests; validates correctness"},
	{Value: "orchestrator", Label: "Orchestrator", Description: "High-level planning and task coordination"},
	{Value: "architect", Label: "Architect", Description: "System design and architectural decisions"},
	{Value: "analyst", Label: "Analyst", Description: "Research, requirements analysis, and reporting"},
}

func (s *Server) handleMetaTaskRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	// Feature 3: the task-role list is the live set of enabled role definitions
	// rather than a hardcoded list. The response shape (metaItem) is preserved
	// for backward compatibility; capability-aware dropdowns can read /api/roles.
	if defs, err := s.db.ListRoleDefinitions(r.Context()); err == nil {
		items := make([]metaItem, 0, len(defs))
		for _, d := range defs {
			if !d.Enabled {
				continue
			}
			label := d.Label
			if label == "" {
				label = d.Name
			}
			items = append(items, metaItem{Value: d.Name, Label: label, Description: d.Description, ID: d.ID})
		}
		if len(items) > 0 {
			api.WriteJSON(w, http.StatusOK, items)
			return
		}
	}
	// Fallback to the built-in list when no role definitions exist yet.
	api.WriteJSON(w, http.StatusOK, taskRoles)
}

// handleMetaSkills returns the live set of enabled skill definitions, used to
// populate the task focus multiselect (Feature 6).
func (s *Server) handleMetaSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	defs, err := s.db.ListSkillDefinitions(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	items := make([]metaItem, 0, len(defs))
	for _, d := range defs {
		if !d.Enabled {
			continue
		}
		label := d.Label
		if label == "" {
			label = d.Name
		}
		items = append(items, metaItem{Value: d.Name, Label: label, Description: d.Description})
	}
	api.WriteJSON(w, http.StatusOK, items)
}

// handleMetaTools returns the available code tool names so the UI can offer a
// searchable multiselect for role/skill allowed_tools instead of free text.
func (s *Server) handleMetaTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	// nil backend: this only enumerates tool definitions (names/descriptions) for
	// the UI; the handlers are never invoked here, so no ToolBackend is needed.
	reg := tools.NewRegistry()
	_ = tools.RegisterCodeTools(reg)
	_ = tools.RegisterTaskTools(reg, nil)
	_ = tools.RegisterPlanTools(reg, nil)
	_ = tools.RegisterContextTools(reg, nil)
	_ = tools.RegisterCommentTools(reg, nil)
	_ = tools.RegisterSubagentTool(reg)
	_ = tools.RegisterSessionTool(reg)
	_ = tools.RegisterMemoryTools(reg)
	_ = tools.RegisterProgressTool(reg, nil)

	defs := reg.List()
	items := make([]metaItem, 0, len(defs))
	for _, d := range defs {
		items = append(items, metaItem{Value: d.Name, Label: d.Name, Description: d.Description})
	}
	api.WriteJSON(w, http.StatusOK, items)
}
