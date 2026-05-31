package db

// DefaultRoleDefinitions returns the built-in seven-role taxonomy (Feature 3).
// These are seeded on a fresh database that has no role definitions and no
// config-defined roles. Capabilities are the routing/authorization flags acted
// on by the server; AllowedTools is a suggested starting tool set per role.
func DefaultRoleDefinitions() []*RoleDefinition {
	mk := func(name, label, desc string, caps, tools []string) *RoleDefinition {
		return &RoleDefinition{
			Name:         name,
			Label:        label,
			Description:  desc,
			Capabilities: caps,
			AllowedTools: tools,
			Temperature:  0.7,
			MaxTokens:    4096,
			Enabled:      true,
		}
	}
	return []*RoleDefinition{
		mk("worker", "Worker",
			"General implementation: code, tests, docs, debug, refactor",
			nil,
			[]string{"read", "write", "diff", "tests", "comment"}),
		mk("reviewer", "Reviewer",
			"Reviews work; makes suggestions only, no file writes",
			[]string{"handles_review"},
			[]string{"read", "list", "comment"}),
		mk("planner", "Planner",
			"Planning, architecture, orchestration, self-improvement analysis",
			[]string{"creates_tasks"},
			[]string{"bootstrap_project", "create_work_package", "list_tasks", "read", "comment", "web_search"}),
		mk("researcher", "Researcher",
			"Web research, technology exploration, improvement proposals",
			[]string{"creates_tasks"},
			[]string{"web_search", "read", "comment", "create_work_package"}),
		mk("security", "Security",
			"Autonomous security analysis; claims role:security tasks from BACKLOG",
			nil,
			[]string{"read", "list", "run_command", "web_search", "comment"}),
		mk("deployer", "Deployer",
			"CI/CD, infrastructure, releases; reviews PRs and merges on approval",
			[]string{"handles_merge", "handles_deploy"},
			[]string{"read", "run_command", "comment", "diff"}),
		mk("designer", "Designer",
			"UI/UX design; can create implementation tasks for workers",
			[]string{"creates_tasks"},
			[]string{"read", "write", "comment", "create_work_package"}),
	}
}
