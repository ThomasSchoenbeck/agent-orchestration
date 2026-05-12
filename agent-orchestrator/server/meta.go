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

var taskTypes = []metaItem{
	{"implement", "Implement", "Write or modify code to fulfil a specification"},
	{"review", "Code Review", "Review code for correctness, style, and completeness"},
	{"test", "Write Tests", "Create unit or integration tests for existing code"},
	{"design", "Design", "Architecture decisions, diagrams, or specification documents"},
	{"research", "Research", "Investigate options and write up findings"},
	{"debug", "Debug", "Diagnose and fix a failing test or runtime error"},
	{"document", "Document", "Write or update docs, READMEs, or inline comments"},
	{"refactor", "Refactor", "Improve code quality without changing observable behaviour"},
	{"deploy", "Deploy", "CI/CD pipeline, infrastructure, or release steps"},
	{"plan", "Plan", "Break down a goal into concrete work packages"},
}

var taskRoles = []metaItem{
	{"worker", "Worker", "General-purpose implementation agent"},
	{"reviewer", "Reviewer", "Reviews and approves work produced by a worker"},
	{"tester", "Tester", "Writes and runs tests; validates correctness"},
	{"orchestrator", "Orchestrator", "High-level planning and task coordination"},
	{"architect", "Architect", "System design and architectural decisions"},
	{"analyst", "Analyst", "Research, requirements analysis, and reporting"},
}

func (s *Server) handleMetaTaskTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	api.WriteJSON(w, http.StatusOK, taskTypes)
}

func (s *Server) handleMetaTaskRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	api.WriteJSON(w, http.StatusOK, taskRoles)
}
