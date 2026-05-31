package server

import (
	"net/http"

	"agent-orchestrator/api"
)

// metaItem describes a task type or agent role.
type metaItem struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

var taskRoles = []metaItem{
	{"worker", "Worker", "General-purpose implementation agent"},
	{"reviewer", "Reviewer", "Reviews and approves work produced by a worker"},
	{"tester", "Tester", "Writes and runs tests; validates correctness"},
	{"orchestrator", "Orchestrator", "High-level planning and task coordination"},
	{"architect", "Architect", "System design and architectural decisions"},
	{"analyst", "Analyst", "Research, requirements analysis, and reporting"},
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
			items = append(items, metaItem{Value: d.Name, Label: label, Description: d.Description})
		}
		if len(items) > 0 {
			api.WriteJSON(w, http.StatusOK, items)
			return
		}
	}
	// Fallback to the built-in list when no role definitions exist yet.
	api.WriteJSON(w, http.StatusOK, taskRoles)
}
