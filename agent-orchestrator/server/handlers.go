package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// registerHandlers wires all routes onto the ServeMux.
func (s *Server) registerHandlers() {
	// Projects
	s.mux.HandleFunc("/api/projects", s.handleProjects)
	s.mux.HandleFunc("/api/projects/", s.handleProjectDetail)

	// Tasks
	s.mux.HandleFunc("/api/tasks", s.handleTasks)
	s.mux.HandleFunc("/api/tasks/", s.handleTaskDetail)

	// Agents
	s.mux.HandleFunc("/api/agents", s.handleAgents)
	s.mux.HandleFunc("/api/agents/register", s.handleAgentRegister)
	s.mux.HandleFunc("/api/agents/", s.handleAgentDetail)

	// Providers
	s.mux.HandleFunc("/api/providers", s.handleProviders)
	s.mux.HandleFunc("/api/providers/", s.handleProviderDetail)

	// Roles
	s.mux.HandleFunc("/api/roles", s.handleRoles)
	s.mux.HandleFunc("/api/roles/", s.handleRoleDetail)

	// Context
	s.mux.HandleFunc("/api/context/save", s.handleContextSave)
	s.mux.HandleFunc("/api/context/query", s.handleContextQuery)

	// Logs
	s.mux.HandleFunc("/api/logs", s.handleLogs)

	// LLM chat
	s.mux.HandleFunc("/api/llm/chat", s.handleLLMChat)

	// Meta (enumerations)
	s.mux.HandleFunc("/api/meta/task-types", s.handleMetaTaskTypes)
	s.mux.HandleFunc("/api/meta/task-roles", s.handleMetaTaskRoles)

	// Metrics
	s.mux.HandleFunc("/api/metrics", s.handleMetrics)
	s.mux.HandleFunc("/api/metrics/tokens", s.handleMetricsTokens)
	s.mux.HandleFunc("/api/metrics/costs", s.handleMetricsCosts)

	// WebSocket chat
	s.mux.HandleFunc("/ws/chat", s.handleWSChat)

	// Health
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

// =========================================================================
// Projects
// =========================================================================

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := s.db.ListProjects(r.Context())
		if err != nil {
			s.internalError(w, err)
			return
		}
		if projects == nil {
			projects = []*db.Project{}
		}
		api.WriteJSON(w, http.StatusOK, projects)

	case http.MethodPost:
		var req api.CreateProjectRequest
		if !s.decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "name is required")
			return
		}
		p := &db.Project{
			Name:        req.Name,
			Description: req.Description,
			RepoPath:    req.RepoPath,
			GitURL:      req.GitURL,
			Config:      req.Config,
		}
		if err := s.db.CreateProject(r.Context(), p); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, p)

	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleProjectDetail(w http.ResponseWriter, r *http.Request) {
	id := pathSegment(r.URL.Path, "/api/projects/", 0)
	if id == "" {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "project id required")
		return
	}

	// Handle sub-resource: /api/projects/{id}/tasks
	if sub := pathSegment(r.URL.Path, "/api/projects/", 1); sub == "tasks" {
		tasks, err := s.db.ListTasks(r.Context(), db.TaskFilters{ProjectID: id})
		if err != nil {
			s.internalError(w, err)
			return
		}
		if tasks == nil {
			tasks = []*db.Task{}
		}
		api.WriteJSON(w, http.StatusOK, tasks)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := s.db.GetProject(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, p)

	case http.MethodPut:
		p, err := s.db.GetProject(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		var req api.UpdateProjectRequest
		if !s.decodeJSON(w, r, &req) {
			return
		}
		if req.Name != nil {
			p.Name = *req.Name
		}
		if req.Description != nil {
			p.Description = *req.Description
		}
		if req.RepoPath != nil {
			p.RepoPath = *req.RepoPath
		}
		if req.GitURL != nil {
			p.GitURL = *req.GitURL
		}
		if req.Status != nil {
			p.Status = *req.Status
		}
		if req.Config != nil {
			p.Config = req.Config
		}
		if err := s.db.UpdateProject(r.Context(), p); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, p)

	case http.MethodDelete:
		if err := s.db.DeleteProject(r.Context(), id); err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

// =========================================================================
// Tasks
// =========================================================================

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		f := db.TaskFilters{
			ProjectID: r.URL.Query().Get("project_id"),
			Status:    r.URL.Query().Get("status"),
			Role:      r.URL.Query().Get("role"),
			AgentID:   r.URL.Query().Get("agent_id"),
		}
		tasks, err := s.db.ListTasks(r.Context(), f)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if tasks == nil {
			tasks = []*db.Task{}
		}
		api.WriteJSON(w, http.StatusOK, tasks)

	case http.MethodPost:
		var req api.CreateTaskRequest
		if !s.decodeJSON(w, r, &req) {
			return
		}
		if req.ProjectID == "" || req.Type == "" || req.Role == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput,
				"project_id, type, and role are required")
			return
		}
		t := &db.Task{
			ProjectID: req.ProjectID,
			Type:      req.Type,
			Role:      req.Role,
			Priority:  req.Priority,
			Payload:   req.Payload,
		}
		if err := s.db.CreateTask(r.Context(), t); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, t)

	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleTaskDetail(w http.ResponseWriter, r *http.Request) {
	// Path: /api/tasks/{id}  OR  /api/tasks/{id}/claim  OR  /api/tasks/{id}/result
	parts := splitPath(r.URL.Path, "/api/tasks/")
	if len(parts) == 0 {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "task id required")
		return
	}
	id := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch sub {
	case "claim":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var body struct {
			AgentID string `json:"agent_id"`
		}
		if !s.decodeJSON(w, r, &body) {
			return
		}
		if body.AgentID == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "agent_id is required")
			return
		}
		if err := s.db.ClaimTask(r.Context(), id, body.AgentID); err != nil {
			api.WriteError(w, http.StatusConflict, api.ErrCodeConflict, err.Error())
			return
		}
		t, _ := s.db.GetTask(r.Context(), id)
		api.WriteJSON(w, http.StatusOK, t)

	case "result":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req api.SubmitTaskResultRequest
		if !s.decodeJSON(w, r, &req) {
			return
		}
		if req.Status == "" {
			req.Status = "completed"
		}
		if err := s.db.SubmitTaskResult(r.Context(), id, req.Result, req.Status); err != nil {
			s.internalError(w, err)
			return
		}
		// Optionally record metrics.
		if req.Metrics != nil {
			_ = s.db.CreateMetric(r.Context(), &db.Metric{
				TaskID:     id,
				TokensUsed: req.Metrics.TokensUsed,
				Cost:       req.Metrics.Cost,
				DurationMs: req.Metrics.DurationMs,
				Success:    req.Status == "completed",
			})
		}
		t, _ := s.db.GetTask(r.Context(), id)
		api.WriteJSON(w, http.StatusOK, t)

	default:
		switch r.Method {
		case http.MethodGet:
			t, err := s.db.GetTask(r.Context(), id)
			if err != nil {
				api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
				return
			}
			api.WriteJSON(w, http.StatusOK, t)

		case http.MethodPut:
			t, err := s.db.GetTask(r.Context(), id)
			if err != nil {
				api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
				return
			}
			var req api.UpdateTaskRequest
			if !s.decodeJSON(w, r, &req) {
				return
			}
			if req.Status != nil {
				t.Status = *req.Status
			}
			if req.Priority != nil {
				t.Priority = *req.Priority
			}
			if req.Payload != nil {
				t.Payload = req.Payload
			}
			if err := s.db.UpdateTask(r.Context(), t); err != nil {
				s.internalError(w, err)
				return
			}
			api.WriteJSON(w, http.StatusOK, t)

		default:
			methodNotAllowed(w)
		}
	}
}

// =========================================================================
// Agents
// =========================================================================

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	agents, err := s.db.ListAgents(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	if agents == nil {
		agents = []*db.Agent{}
	}
	api.WriteJSON(w, http.StatusOK, agents)
}

func (s *Server) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req api.RegisterAgentRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "name is required")
		return
	}
	if len(req.Roles) == 0 {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "at least one role is required")
		return
	}

	// Check if agent with this name already exists; if so, update it.
	existing, _ := s.db.GetAgentByName(r.Context(), req.Name)
	if existing != nil {
		existing.Roles = req.Roles
		existing.Capabilities = req.Capabilities
		existing.Status = "online"
		if err := s.db.UpdateAgent(r.Context(), existing); err != nil {
			s.internalError(w, err)
			return
		}
		_ = s.db.UpdateHeartbeat(r.Context(), existing.ID)
		api.WriteJSON(w, http.StatusOK, api.RegisterAgentResponse{AgentID: existing.ID})
		return
	}

	a := &db.Agent{
		Name:         req.Name,
		Roles:        req.Roles,
		Capabilities: req.Capabilities,
		Status:       "online",
	}
	if err := s.db.CreateAgent(r.Context(), a); err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, api.RegisterAgentResponse{AgentID: a.ID})
}

func (s *Server) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	// Routes under /api/agents/:
	//   GET  /api/agents/{id}
	//   POST /api/agents/{id}/heartbeat
	//   GET  /api/agents/{id}/tasks/next
	//   (register is caught before this handler)

	parts := splitPath(r.URL.Path, "/api/agents/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	agentID := parts[0]

	// /api/agents/{id}/heartbeat
	if len(parts) == 2 && parts[1] == "heartbeat" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if err := s.db.UpdateHeartbeat(r.Context(), agentID); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// /api/agents/{id}/tasks/next
	if len(parts) >= 3 && parts[1] == "tasks" && parts[2] == "next" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		rolesParam := r.URL.Query().Get("roles")
		var roles []string
		if rolesParam != "" {
			for _, role := range strings.Split(rolesParam, ",") {
				role = strings.TrimSpace(role)
				if role != "" {
					roles = append(roles, role)
				}
			}
		} else {
			// Use the agent's own roles.
			a, err := s.db.GetAgent(r.Context(), agentID)
			if err != nil {
				api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
				return
			}
			roles = a.Roles
		}
		task, err := s.db.GetNextTask(r.Context(), roles)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if task == nil {
			api.WriteJSON(w, http.StatusOK, nil)
			return
		}
		api.WriteJSON(w, http.StatusOK, task)
		return
	}

	// /api/agents/{id}
	switch r.Method {
	case http.MethodGet:
		a, err := s.db.GetAgent(r.Context(), agentID)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, a)

	case http.MethodDelete:
		if err := s.db.DeleteAgent(r.Context(), agentID); err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

// =========================================================================
// Providers
// =========================================================================

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		providers, err := s.db.ListProviders(r.Context())
		if err != nil {
			s.internalError(w, err)
			return
		}
		if providers == nil {
			providers = []*db.Provider{}
		}
		// Strip API keys from list response.
		for _, p := range providers {
			p.APIKey = ""
		}
		api.WriteJSON(w, http.StatusOK, providers)

	case http.MethodPost:
		p := db.Provider{Enabled: true} // default to enabled
		if !s.decodeJSON(w, r, &p) {
			return
		}
		if p.Name == "" || p.Type == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "name and type are required")
			return
		}
		if err := s.db.CreateProvider(r.Context(), &p); err != nil {
			s.internalError(w, err)
			return
		}
		// Sync new provider into in-memory registry.
		if p.Enabled {
			if prov, err := llm.NewFromSpec(p.Name, p.Type, p.BaseURL, p.APIKey, p.ModelName, p.Config); err == nil {
				s.llmReg.Set(p.Name, prov)
			}
		}
		p.APIKey = "" // don't echo back
		api.WriteJSON(w, http.StatusCreated, p)

	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleProviderDetail(w http.ResponseWriter, r *http.Request) {
	id := pathSegment(r.URL.Path, "/api/providers/", 0)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	sub := pathSegment(r.URL.Path, "/api/providers/", 1)

	// POST /api/providers/seed
	if id == "seed" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleProviderSeed(w, r)
		return
	}

	// POST /api/providers/:id/test
	if sub == "test" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleProviderTest(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := s.db.GetProvider(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		p.APIKey = "" // don't echo back
		api.WriteJSON(w, http.StatusOK, p)

	case http.MethodPut:
		p, err := s.db.GetProvider(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		savedKey := p.APIKey
		if !s.decodeJSON(w, r, p) {
			return
		}
		p.ID = id
		if p.APIKey == "" {
			p.APIKey = savedKey // preserve existing key if not supplied
		}
		if err := s.db.UpdateProvider(r.Context(), p); err != nil {
			s.internalError(w, err)
			return
		}
		// Sync registry: replace if enabled, remove if disabled.
		if p.Enabled {
			if prov, err := llm.NewFromSpec(p.Name, p.Type, p.BaseURL, p.APIKey, p.ModelName, p.Config); err == nil {
				s.llmReg.Set(p.Name, prov)
			}
		} else {
			s.llmReg.Remove(p.Name)
		}
		p.APIKey = ""
		api.WriteJSON(w, http.StatusOK, p)

	case http.MethodDelete:
		p, err := s.db.GetProvider(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if err := s.db.DeleteProvider(r.Context(), id); err != nil {
			s.internalError(w, err)
			return
		}
		s.llmReg.Remove(p.Name)
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

// handleProviderSeed imports providers from the loaded config into the DB
// (idempotent — skips any provider whose name already exists).
func (s *Server) handleProviderSeed(w http.ResponseWriter, r *http.Request) {
	existing, err := s.db.ListProviders(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	existingNames := make(map[string]struct{}, len(existing))
	for _, p := range existing {
		existingNames[p.Name] = struct{}{}
	}

	var toSeed []*db.Provider
	for _, pcfg := range s.cfg.Providers {
		if _, ok := existingNames[pcfg.Name]; ok {
			continue
		}
		toSeed = append(toSeed, &db.Provider{
			Name:      pcfg.Name,
			Type:      pcfg.Type,
			BaseURL:   pcfg.BaseURL,
			APIKey:    pcfg.APIKey,
			ModelName: pcfg.Model,
			Enabled:   true,
		})
	}

	n, err := s.db.SeedProviders(r.Context(), toSeed)
	if err != nil {
		s.internalError(w, err)
		return
	}

	// Load any newly seeded providers into the in-memory registry.
	if n > 0 {
		if all, err := s.db.ListProviders(r.Context()); err == nil {
			for _, p := range all {
				if p.Enabled {
					if prov, err := llm.NewFromSpec(p.Name, p.Type, p.BaseURL, p.APIKey, p.ModelName, p.Config); err == nil {
						s.llmReg.Set(p.Name, prov)
					}
				}
			}
		}
	}

	log.Printf("provider seed: inserted %d new provider(s)", n)
	api.WriteJSON(w, http.StatusOK, map[string]int{"seeded": n})
}

// handleProviderTest instantiates a provider on-the-fly and makes a minimal
// "Say hi" chat request to verify the connection.
func (s *Server) handleProviderTest(w http.ResponseWriter, r *http.Request, id string) {
	p, err := s.db.GetProvider(r.Context(), id)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
		return
	}

	prov, err := llm.NewFromSpec(p.Name, p.Type, p.BaseURL, p.APIKey, p.ModelName, p.Config)
	if err != nil {
		api.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"ok": false, "latency_ms": 0, "error": err.Error(),
		})
		return
	}
	defer prov.Close()

	start := time.Now()
	resp, chatErr := prov.Chat(r.Context(), llm.ChatRequest{
		Model:     p.ModelName,
		Messages:  []llm.Message{{Role: "user", Content: "Say hi"}},
		MaxTokens: 10,
	})
	latencyMs := time.Since(start).Milliseconds()

	if chatErr != nil {
		api.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"ok": false, "latency_ms": latencyMs, "error": chatErr.Error(),
		})
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "latency_ms": latencyMs, "reply": resp.Content,
	})
}

// =========================================================================
// Context
// =========================================================================

func (s *Server) handleContextSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var e db.ContextEntry
	if !s.decodeJSON(w, r, &e) {
		return
	}
	if e.Content == "" {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "content is required")
		return
	}
	if e.Type == "" {
		e.Type = "note"
	}
	if err := s.db.CreateContextEntry(r.Context(), &e); err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, e)
}

func (s *Server) handleContextQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	projectID := r.URL.Query().Get("project_id")
	query := r.URL.Query().Get("query")
	entries, err := s.db.QueryContext(r.Context(), projectID, query, 20)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if entries == nil {
		entries = []*db.ContextEntry{}
	}
	api.WriteJSON(w, http.StatusOK, entries)
}

// =========================================================================
// Logs
// =========================================================================

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		f := db.LogFilters{
			AgentID:   r.URL.Query().Get("agent_id"),
			TaskID:    r.URL.Query().Get("task_id"),
			ProjectID: r.URL.Query().Get("project_id"),
			Level:     r.URL.Query().Get("level"),
			Limit:     100,
		}
		logs, err := s.db.ListLogs(r.Context(), f)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if logs == nil {
			logs = []*db.LogEntry{}
		}
		api.WriteJSON(w, http.StatusOK, logs)

	case http.MethodPost:
		var l db.LogEntry
		if !s.decodeJSON(w, r, &l) {
			return
		}
		if l.Message == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "message is required")
			return
		}
		if l.Level == "" {
			l.Level = "info"
		}
		if err := s.db.CreateLog(r.Context(), &l); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, l)

	default:
		methodNotAllowed(w)
	}
}

// =========================================================================
// Helpers
// =========================================================================

func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	api.WriteError(w, http.StatusInternalServerError, api.ErrCodeInternal, "internal server error")
}

func methodNotAllowed(w http.ResponseWriter) {
	api.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

// pathSegment extracts path components after a prefix.
// e.g. pathSegment("/api/projects/abc/tasks", "/api/projects/", 0) → "abc"
//      pathSegment("/api/projects/abc/tasks", "/api/projects/", 1) → "tasks"
func pathSegment(path, prefix string, index int) string {
	trimmed := strings.TrimPrefix(path, prefix)
	parts := strings.Split(strings.TrimSuffix(trimmed, "/"), "/")
	if index < len(parts) {
		return parts[index]
	}
	return ""
}

// splitPath strips prefix and returns remaining path segments.
func splitPath(path, prefix string) []string {
	trimmed := strings.TrimPrefix(path, prefix)
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
