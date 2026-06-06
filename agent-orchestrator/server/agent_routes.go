package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// registerAgentRoutes mounts the agent-facing API under /api/agent/*, gated by
// the optional agents.api_key bearer token. Handlers are shared with the UI
// routes (mounted under /api/* without auth); only the path prefix and the auth
// middleware differ. The provider-secrets endpoint
// (/api/agent/internal/providers) lives ONLY here, so API keys never leave the
// server via an unauthenticated route.
//
// agentRewrite maps /api/agent/<rest> to /api/<rest> before dispatch, so each
// inner handler sees the same path it does on the UI mux (e.g.
// /api/tasks/{id}/result) and reuses its existing fixed-prefix parsing unchanged.
func (s *Server) registerAgentRoutes() {
	am := http.NewServeMux()

	// Agent lifecycle.
	am.HandleFunc("/api/agents", s.handleAgents)
	am.HandleFunc("/api/agents/register", s.handleAgentRegister)
	am.HandleFunc("/api/agents/", s.handleAgentDetail)

	// Work surface (shared handlers; T5 adds the coarse planning sub-routes).
	am.HandleFunc("/api/tasks", s.handleTasks)
	am.HandleFunc("/api/tasks/", s.handleAgentTaskDetail)
	am.HandleFunc("/api/projects", s.handleProjects)
	am.HandleFunc("/api/projects/", s.handleAgentProjectDetail)
	am.HandleFunc("/api/context/save", s.handleContextSave)
	am.HandleFunc("/api/context/query", s.handleContextQuery)
	am.HandleFunc("/api/logs", s.handleLogs)
	am.HandleFunc("/api/skills", s.handleSkills)
	am.HandleFunc("/api/skills/", s.handleSkillDetail)
	am.HandleFunc("/api/roles", s.handleRoles)
	am.HandleFunc("/api/roles/", s.handleRoleDetail)

	// Provider config WITH secrets — agent-only, behind the gate.
	am.HandleFunc("/api/internal/providers", s.handleInternalProviders)

	s.mux.Handle("/api/agent/", s.agentAuth(agentRewrite(am)))
}

// agentRewrite rewrites /api/agent/<rest> to /api/<rest> so the shared UI
// handlers (which match fixed /api/... patterns and parse fixed prefixes) work
// unchanged under the agent namespace.
func agentRewrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api" + strings.TrimPrefix(r.URL.Path, "/api/agent")
		r.URL.RawPath = ""
		next.ServeHTTP(w, r)
	})
}

// agentAuth enforces the bearer token on the agent namespace when
// agents.api_key is configured. When the key is empty, the namespace is open
// (no auth), matching the documented default.
func (s *Server) agentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := s.cfg.Agents.APIKey
		if key != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				api.WriteError(w, http.StatusUnauthorized, api.ErrCodeUnauthorized,
					"valid agent API key required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// handleAgentTaskDetail intercepts the agent-only read-only "next task" peek and
// delegates everything else to the shared handleTaskDetail. Task IDs are UUIDs,
// so "next" never collides with a real task.
func (s *Server) handleAgentTaskDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet &&
		pathSegment(r.URL.Path, "/api/tasks/", 0) == "next" &&
		pathSegment(r.URL.Path, "/api/tasks/", 1) == "" {
		s.handleAgentNextTask(w, r)
		return
	}
	s.handleTaskDetail(w, r)
}

// handleAgentNextTask returns the next queued task matching the roles query,
// without claiming it (a read-only peek for the get_next_task tool).
func (s *Server) handleAgentNextTask(w http.ResponseWriter, r *http.Request) {
	var roles []string
	for _, p := range strings.Split(r.URL.Query().Get("roles"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			roles = append(roles, p)
		}
	}
	task, err := s.db.GetNextTask(r.Context(), roles)
	if err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{"task": task})
}

// handleInternalProviders returns providers WITH their API keys and models, for
// agent self-configuration. Reachable only via the gated /api/agent namespace —
// the UI's /api/providers endpoint strips keys.
func (s *Server) handleInternalProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	providers, err := s.db.ListProviders(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	if providers == nil {
		providers = []*db.Provider{}
	}
	api.WriteJSON(w, http.StatusOK, providers)
}
