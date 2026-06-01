package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
	"agent-orchestrator/git"
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

	// Agent logs
	s.mux.HandleFunc("/api/agent-logs", s.handleAgentLogs)

	// Task Logs
	s.mux.HandleFunc("/api/task-logs", s.handleTaskLogs)

	// LLM chat
	s.mux.HandleFunc("/api/llm/chat", s.handleLLMChat)

	// Conversations
	s.mux.HandleFunc("/api/conversations", s.handleConversations)
	s.mux.HandleFunc("/api/conversations/", s.handleConversationDetail)
	s.mux.HandleFunc("/api/chat-log", s.handleChatLog)

	// Meta (enumerations)
	s.mux.HandleFunc("/api/meta/task-roles", s.handleMetaTaskRoles)

	// Metrics
	s.mux.HandleFunc("/api/metrics", s.handleMetrics)
	s.mux.HandleFunc("/api/metrics/tokens", s.handleMetricsTokens)
	s.mux.HandleFunc("/api/metrics/costs", s.handleMetricsCosts)

	// WebSocket chat
	s.mux.HandleFunc("/ws/chat", s.handleWSChat)

	// Checklist templates
	s.mux.HandleFunc("/api/checklist-templates", s.handleChecklistTemplates)
	s.mux.HandleFunc("/api/checklist-templates/", s.handleChecklistTemplateDetail)

	// Settings
	s.mux.HandleFunc("/api/settings", s.handleSettings)
	s.mux.HandleFunc("/api/settings/", s.handleSettingDetail)

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
			Name:                 req.Name,
			Description:          req.Description,
			RepoPath:             req.RepoPath,
			GitURL:               req.GitURL,
			Slug:                 req.Slug,
			RemoteURL:            req.RemoteURL,
			RemoteCredentialsRef: req.RemoteCredentialsRef,
			CodingRules:          req.CodingRules,
			Config:               req.Config,
		}
		if err := s.db.CreateProject(r.Context(), p); err != nil {
			s.internalError(w, err)
			return
		}

		// Auto-generate slug from name when the caller did not supply one.
		// We do this after CreateProject so p.ID is available for uniqueness.
		if p.Slug == "" {
			base := slugify(p.Name)
			if base == "" {
				base = p.ID[:8]
			}
			slug := base
			if existing, _ := s.db.GetProjectBySlug(r.Context(), base); existing != nil {
				slug = base + "-" + p.ID[:8]
			}
			p.Slug = slug
		}

		// Initialise a bare git repo for this project.
		repoPath := s.storage.RepoPath(p.ID)
		if err := git.InitBare(repoPath); err != nil {
			log.Printf("server: InitBare project %q: %v", p.ID, err)
		} else {
			now := time.Now().UTC()
			p.ServerRepoInitialisedAt = &now

			// Wire optional upstream remote.
			if p.RemoteURL != "" {
				if err := git.AddRemote(repoPath, "upstream", p.RemoteURL); err != nil {
					log.Printf("server: AddRemote project %q: %v", p.ID, err)
				} else if req.InitialPull {
					token := s.resolveCredential(p.RemoteCredentialsRef)
					if err := git.FetchRemote(repoPath, "upstream", token); err != nil {
						log.Printf("server: FetchRemote project %q: %v", p.ID, err)
					} else if err := git.ResetBranchToRemote(repoPath, "upstream", "main"); err != nil {
						log.Printf("server: ResetBranchToRemote project %q: %v", p.ID, err)
					}
				}
			}

			// Seed main with a real commit so the HTTP server can serve
			// it via upload-pack. CommitFile uses the high-level API and
			// produces objects that are always packable and servable.
			if _, err := git.CommitFile(repoPath, "main", ".gitkeep", nil,
				"chore: initialise repository", "Agent Orchestrator", "noreply@agent-orchestrator"); err != nil {
				log.Printf("server: CommitFile (init) project %q: %v", p.ID, err)
			}

			if err := s.db.UpdateProject(r.Context(), p); err != nil {
				log.Printf("server: UpdateProject after InitBare %q: %v", p.ID, err)
			}
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

	// Handle sub-resources
	sub := pathSegment(r.URL.Path, "/api/projects/", 1)

	if sub == "tasks" {
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

	if sub == "chat" {
		s.handleProjectChat(w, r, id)
		return
	}

	if sub == "requirements" {
		rid := pathSegment(r.URL.Path, "/api/projects/", 2)
		if rid == "" {
			s.handleProjectRequirements(w, r, id)
		} else {
			s.handleProjectRequirementDetail(w, r, id, rid)
		}
		return
	}

	if sub == "features" {
		fid := pathSegment(r.URL.Path, "/api/projects/", 2)
		if fid == "" {
			s.handleProjectFeatures(w, r, id)
		} else {
			s.handleProjectFeatureDetail(w, r, id, fid)
		}
		return
	}

	if sub == "branches" {
		s.handleProjectBranches(w, r, id)
		return
	}

	if sub == "tree" {
		s.handleProjectTree(w, r, id)
		return
	}

	if sub == "file" {
		s.handleProjectFile(w, r, id)
		return
	}

	if sub == "diff" {
		s.handleProjectDiff(w, r, id)
		return
	}

	if sub == "commits" {
		s.handleProjectCommits(w, r, id)
		return
	}

	if sub == "files" {
		s.handleProjectFiles(w, r, id)
		return
	}

	if sub == "init-repo" {
		s.handleProjectInitRepo(w, r, id)
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
		if req.Slug != nil {
			p.Slug = *req.Slug
		}
		if req.RemoteURL != nil {
			p.RemoteURL = *req.RemoteURL
		}
		if req.RemoteCredentialsRef != nil {
			p.RemoteCredentialsRef = *req.RemoteCredentialsRef
		}
		if req.CodingRules != nil {
			p.CodingRules = *req.CodingRules
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
// Project git file/tree/diff routes
// =========================================================================

func (s *Server) projectRepoPath(w http.ResponseWriter, r *http.Request, projectID string) (string, bool) {
	p, err := s.db.GetProject(r.Context(), projectID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "project not found")
		return "", false
	}
	return s.storage.RepoPath(p.ID), true
}

func (s *Server) handleProjectInitRepo(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	repoPath, ok := s.projectRepoPath(w, r, projectID)
	if !ok {
		return
	}
	if err := git.InitBare(repoPath); err != nil {
		s.internalError(w, err)
		return
	}
	// CommitFile creates a real commit via the high-level API, ensuring all git
	// objects are stored in a format the HTTP server can pack and serve.
	// This also repairs existing projects whose initial commit was created with
	// the old low-level SetSize(0) approach (which produced objects that could
	// not be served via upload-pack).
	if _, err := git.CommitFile(repoPath, "main", ".gitkeep", nil,
		"chore: initialise repository", "Agent Orchestrator", "noreply@agent-orchestrator"); err != nil {
		s.internalError(w, err)
		return
	}
	log.Printf("project %s: bare repo initialised at %s", projectID, repoPath)
	api.WriteJSON(w, http.StatusOK, map[string]string{"repo_path": repoPath, "status": "ok"})
}

func (s *Server) handleProjectBranches(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	repoPath, ok := s.projectRepoPath(w, r, projectID)
	if !ok {
		return
	}
	branches, err := git.ListBranches(repoPath)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if branches == nil {
		branches = []string{}
	}
	api.WriteJSON(w, http.StatusOK, branches)
}

func (s *Server) handleProjectTree(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	repoPath, ok := s.projectRepoPath(w, r, projectID)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = "main"
	}
	subpath := r.URL.Query().Get("path")
	nodes, err := git.ReadTree(repoPath, ref, subpath)
	if err != nil {
		// A missing ref means the branch hasn't been pushed yet — treat it as
		// an empty tree rather than a 404 so the UI degrades gracefully.
		if strings.Contains(err.Error(), "not found") {
			api.WriteJSON(w, http.StatusOK, []git.TreeNode{})
			return
		}
		s.internalError(w, err)
		return
	}
	if nodes == nil {
		nodes = []git.TreeNode{}
	}
	api.WriteJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleProjectCommits(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	repoPath, ok := s.projectRepoPath(w, r, projectID)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = "main"
	}
	commits, err := git.ListCommits(repoPath, ref, 50)
	if err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, commits)
}

func (s *Server) handleProjectFile(w http.ResponseWriter, r *http.Request, projectID string) {
	repoPath, ok := s.projectRepoPath(w, r, projectID)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		ref := r.URL.Query().Get("ref")
		if ref == "" {
			ref = "main"
		}
		filePath := r.URL.Query().Get("path")
		if filePath == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "path is required")
			return
		}
		content, err := git.ReadFile(repoPath, ref, filePath)
		if err == git.ErrBinaryFile {
			api.WriteJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{"binary": true})
			return
		}
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
				return
			}
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"content":  string(content),
			"encoding": "utf8",
		})

	case http.MethodPut:
		var req struct {
			Path        string `json:"path"`
			Content     string `json:"content"`
			Branch      string `json:"branch"`
			Message     string `json:"message"`
			AuthorName  string `json:"author_name"`
			AuthorEmail string `json:"author_email"`
		}
		if !s.decodeJSON(w, r, &req) {
			return
		}
		if req.Path == "" || req.Branch == "" || req.Message == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "path, branch, and message are required")
			return
		}
		sha, err := git.CommitFile(repoPath, req.Branch, req.Path, []byte(req.Content), req.Message, req.AuthorName, req.AuthorEmail)
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]string{"sha": sha, "path": req.Path, "branch": req.Branch})

	default:
		methodNotAllowed(w)
	}
}

// handleProjectFiles handles POST /api/projects/:id/files — multi-file commit.
func (s *Server) handleProjectFiles(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	repoPath, ok := s.projectRepoPath(w, r, projectID)
	if !ok {
		return
	}
	var req struct {
		Branch      string `json:"branch"`
		Message     string `json:"message"`
		AuthorName  string `json:"author_name"`
		AuthorEmail string `json:"author_email"`
		Files       []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"files"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Branch == "" || req.Message == "" || len(req.Files) == 0 {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "branch, message, and files are required")
		return
	}
	changes := make([]git.FileChange, len(req.Files))
	for i, f := range req.Files {
		changes[i] = git.FileChange{Path: f.Path, Content: []byte(f.Content)}
	}
	sha, err := git.CommitFiles(repoPath, req.Branch, changes, req.Message, req.AuthorName, req.AuthorEmail)
	if err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"sha":    sha,
		"branch": req.Branch,
		"count":  len(req.Files),
	})
}

func (s *Server) handleProjectDiff(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	repoPath, ok := s.projectRepoPath(w, r, projectID)
	if !ok {
		return
	}
	base := r.URL.Query().Get("base")
	head := r.URL.Query().Get("head")
	if base == "" || head == "" {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "base and head are required")
		return
	}
	filePath := r.URL.Query().Get("path")
	if filePath != "" {
		diff, err := git.FileDiff(repoPath, base, head, filePath)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
				return
			}
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]string{"diff": diff})
		return
	}
	patches, err := git.BranchDiff(repoPath, base, head)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		s.internalError(w, err)
		return
	}
	if patches == nil {
		patches = []git.FilePatch{}
	}
	api.WriteJSON(w, http.StatusOK, patches)
}

// =========================================================================
// Tasks
// =========================================================================

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		f := db.TaskFilters{
			ProjectID:     r.URL.Query().Get("project_id"),
			Status:        r.URL.Query().Get("status"),
			Role:          r.URL.Query().Get("role"),
			AgentID:       r.URL.Query().Get("agent_id"),
			RequirementID: r.URL.Query().Get("requirement_id"),
			FeatureID:     r.URL.Query().Get("feature_id"),
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
		if req.ProjectID == "" || req.Role == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput,
				"project_id and role are required")
			return
		}
		t := &db.Task{
			ProjectID:  req.ProjectID,
			Role:       req.Role,
			ReviewRole: req.ReviewRole,
			Priority:   req.Priority,
			Payload:    req.Payload,
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
		claimResp := s.prepareClaimResponse(r.Context(), t, body.AgentID)
		api.WriteJSON(w, http.StatusOK, claimResp)

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
			s.recordMetric(r.Context(), id, "", req.Metrics, req.Status == db.TaskStatusCompleted)
		}
		t, _ := s.db.GetTask(r.Context(), id)
		if t != nil {
			s.releaseTaskResources(t)
		}
		api.WriteJSON(w, http.StatusOK, t)

	case "queue":
		// POST /api/tasks/{id}/queue
		// Re-queues any non-COMPLETED task by resetting it to BACKLOG and
		// clearing the agent assignment so it can be picked up again.
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		t, err := s.db.GetTask(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if t.Status == db.TaskStatusCompleted {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput,
				"cannot queue a completed task")
			return
		}
		s.releaseTaskResources(t)
		t.Status = db.TaskStatusBacklog
		t.AssignedAgentID = ""
		if err := s.db.UpdateTask(r.Context(), t); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, t)

	case "unqueue":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		t, err := s.db.GetTask(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if t.Status == db.TaskStatusCompleted {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput,
				"cannot unqueue a completed task")
			return
		}
		s.releaseTaskResources(t)
		t.Status = db.TaskStatusBacklog
		t.AssignedAgentID = ""
		if err := s.db.UpdateTask(r.Context(), t); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, t)

	case "submit-for-review":
		// POST /api/tasks/{id}/submit-for-review
		// The agent calls this after pushing its branch. State should already be
		// AWAITING_REVIEW (driven by the git post-receive hook); this endpoint is
		// a metadata confirmation that lets the agent cleanly release its claim.
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		t, err := s.db.GetTask(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		// Optionally record metrics submitted by the agent.
		var reviewReq struct {
			Metrics *api.TaskMetrics `json:"metrics,omitempty"`
		}
		_ = s.decodeJSONOptional(r, &reviewReq)
		if reviewReq.Metrics != nil {
			s.recordMetric(r.Context(), id, t.AssignedAgentID, reviewReq.Metrics, true)
		}
		// If the post-receive hook hasn't fired yet, transition manually.
		if t.Status == db.TaskStatusDeveloping {
			_ = s.db.TransitionTaskState(r.Context(), id,
				db.TaskStatusDeveloping, db.TaskStatusAwaitingReview,
				t.AssignedAgentID, "submitted for review by agent")
			t, _ = s.db.GetTask(r.Context(), id)
		}
		if t != nil {
			s.releaseTaskResources(t)
		}
		api.WriteJSON(w, http.StatusOK, t)

	case "cost":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		cost, err := s.db.GetTaskCost(r.Context(), id)
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, cost)

	case "logs":
		switch r.Method {
		case http.MethodGet:
			logs, err := s.db.ListTaskLogs(r.Context(), db.TaskLogFilters{TaskID: id, Limit: 200})
			if err != nil {
				s.internalError(w, err)
				return
			}
			if logs == nil {
				logs = []*db.TaskLog{}
			}
			api.WriteJSON(w, http.StatusOK, logs)
		case http.MethodDelete:
			n1, err := s.db.DeleteTaskLogsByTask(r.Context(), id)
			if err != nil {
				s.internalError(w, err)
				return
			}
			n2, err := s.db.DeleteLogsByTask(r.Context(), id)
			if err != nil {
				s.internalError(w, err)
				return
			}
			api.WriteJSON(w, http.StatusOK, map[string]int64{"deleted": n1 + n2})
		default:
			methodNotAllowed(w)
		}

	case "links":
		s.handleTaskLinks(w, r, id)

	case "dependencies":
		s.handleTaskDependencies(w, r, id)

	case "checklist":
		sub2 := ""
		if len(parts) > 2 {
			sub2 = parts[2]
		}
		switch sub2 {
		case "":
			s.handleTaskChecklist(w, r, id)
		case "iterations":
			s.handleTaskChecklistIterations(w, r, id)
		default:
			s.handleTaskChecklistItem(w, r, id, sub2)
		}

	case "comments":
		s.handleTaskComments(w, r, id, parts)

	case "reviews":
		s.handleTaskReviews(w, r, id, parts)

	case "pull-requests":
		s.handleTaskPullRequests(w, r, id, parts)

	case "transitions":
		// GET /api/tasks/{id}/transitions — state-transition history
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		transitions, err := s.db.ListStateTransitions(r.Context(), id)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if transitions == nil {
			transitions = []*db.StateTransition{}
		}
		api.WriteJSON(w, http.StatusOK, transitions)

	case "chat":
		s.handleTaskChat(w, r, id)

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
			oldStatus := t.Status
			oldDesc, _ := t.Payload["description"].(string)
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
			// Log what changed.
			if req.Status != nil && *req.Status != oldStatus {
				log.Printf("task %s: status %s → %s", id, oldStatus, *req.Status)
			}
			if req.Payload != nil {
				newDesc, _ := req.Payload["description"].(string)
				if newDesc != oldDesc {
					log.Printf("task %s: description updated", id)
				}
			}
			api.WriteJSON(w, http.StatusOK, t)

		case http.MethodDelete:
			if err := s.db.DeleteTask(r.Context(), id); err != nil {
				api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)

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

	// Normalise mode.
	mode := req.Mode
	if mode != "colocated" && mode != "remote" {
		mode = "remote"
	}

	// Check if agent with this name already exists; if so, update it.
	existing, _ := s.db.GetAgentByName(r.Context(), req.Name)
	if existing != nil {
		existing.Roles = req.Roles
		existing.Mode = mode
		existing.Capabilities = req.Capabilities
		existing.Status = "online"
		if err := s.db.UpdateAgent(r.Context(), existing); err != nil {
			s.internalError(w, err)
			return
		}
		_ = s.db.UpdateHeartbeat(r.Context(), existing.ID)
		api.WriteJSON(w, http.StatusOK, api.RegisterAgentResponse{AgentID: existing.ID})
		_ = s.db.CreateAgentLog(r.Context(), &db.AgentLog{
			AgentID:     existing.ID,
			AgentName:   existing.Name,
			EventType:   "agent_registered",
			Description: "Agent re-registered",
		})
		return
	}

	a := &db.Agent{
		Name:         req.Name,
		Roles:        req.Roles,
		Mode:         mode,
		Capabilities: req.Capabilities,
		Status:       "online",
	}
	if err := s.db.CreateAgent(r.Context(), a); err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, api.RegisterAgentResponse{AgentID: a.ID})
	_ = s.db.CreateAgentLog(r.Context(), &db.AgentLog{
		AgentID:     a.ID,
		AgentName:   a.Name,
		EventType:   "agent_registered",
		Description: "Agent registered",
	})
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

	// /api/agents/{id}/offline — graceful shutdown notification
	if len(parts) == 2 && parts[1] == "offline" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		a, err := s.db.GetAgent(r.Context(), agentID)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		a.Status = "offline"
		if err := s.db.UpdateAgent(r.Context(), a); err != nil {
			s.internalError(w, err)
			return
		}
		_ = s.db.CreateAgentLog(r.Context(), &db.AgentLog{
			AgentID:     agentID,
			AgentName:   a.Name,
			EventType:   "agent_offline",
			Description: "Agent deregistered (clean shutdown)",
		})
		log.Printf("agent %q (%s): deregistered (clean shutdown)", a.Name, agentID)
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "offline"})
		return
	}

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
		if s.isDebugMode(r.Context()) {
			_ = s.db.CreateAgentLog(r.Context(), &db.AgentLog{
				AgentID:   agentID,
				EventType: db.EventAgentHeartbeat,
			})
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
		taskFoundID := ""
		if task != nil {
			taskFoundID = task.ID
		}
		s.recordPoll(agentID, roles, taskFoundID)
		if s.isDebugMode(r.Context()) {
			eventType := db.EventAgentPollQuery
			if task == nil {
				eventType = db.EventAgentPollNoTask
			}
			_ = s.db.CreateAgentLog(r.Context(), &db.AgentLog{
				AgentID:   agentID,
				EventType: eventType,
			})
		}
		if task == nil {
			api.WriteJSON(w, http.StatusOK, nil)
			return
		}

		// Atomically claim the task before enriching the response.
		// Without ClaimTask here, every poll returns the same BACKLOG task and
		// calls UpdateTask, flooding the log with "Task updated" entries.
		if err := s.db.ClaimTask(r.Context(), task.ID, agentID); err != nil {
			// Race: another agent claimed it between GetNextTask and here.
			// Return nil so the caller retries on its next poll cycle.
			api.WriteJSON(w, http.StatusOK, nil)
			return
		}
		task, _ = s.db.GetTask(r.Context(), task.ID)
		claimResp := s.prepareClaimResponse(r.Context(), task, agentID)
		api.WriteJSON(w, http.StatusOK, claimResp)
		return
	}

	// /api/agents/{id}/poll-status
	if len(parts) == 2 && parts[1] == "poll-status" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		ps := s.getPollStatus(agentID)
		api.WriteJSON(w, http.StatusOK, ps)
		return
	}

	// /api/agents/{id}/stats
	if len(parts) == 2 && parts[1] == "stats" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		a, err := s.db.GetAgent(r.Context(), agentID)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		// Fetch all tasks this agent has worked on.
		tasks, err := s.db.ListTasks(r.Context(), db.TaskFilters{AgentID: agentID, Limit: 2000})
		if err != nil {
			s.internalError(w, err)
			return
		}
		var (
			totalTasks     int
			completedTasks int
			failedTasks    int
			totalTokens    int
			totalDurationMs int64
		)
		for _, t := range tasks {
			totalTasks++
			switch t.Status {
			case db.TaskStatusCompleted:
				completedTasks++
			case db.TaskStatusFailed:
				failedTasks++
			}
			if t.Result != nil {
				if tok, ok := t.Result["tokens_used"].(float64); ok {
					totalTokens += int(tok)
				}
			}
			if t.StartedAt != nil && t.CompletedAt != nil {
				totalDurationMs += t.CompletedAt.Sub(*t.StartedAt).Milliseconds()
			}
		}
		var uptimeMs int64
		if !a.RegisteredAt.IsZero() {
			uptimeMs = time.Since(a.RegisteredAt).Milliseconds()
		}
		api.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"uptime_ms":          uptimeMs,
			"registered_at":      a.RegisteredAt,
			"last_heartbeat":     a.LastHeartbeat,
			"total_tasks":        totalTasks,
			"completed_tasks":    completedTasks,
			"failed_tasks":       failedTasks,
			"total_tokens":       totalTokens,
			"avg_task_ms":        func() int64 {
				if completedTasks > 0 { return totalDurationMs / int64(completedTasks) }
				return 0
			}(),
		})
		return
	}

	// /api/agents/{id}/logs
	if len(parts) == 2 && parts[1] == "logs" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		logs, err := s.db.ListAgentLogs(r.Context(), db.AgentLogFilters{AgentID: agentID, Limit: 200})
		if err != nil {
			s.internalError(w, err)
			return
		}
		if logs == nil {
			logs = []*db.AgentLog{}
		}
		api.WriteJSON(w, http.StatusOK, logs)
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
				if len(p.Roles) > 0 {
					s.llmReg.SetRoles(p.Name, p.ModelName, p.Roles)
				}
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
				s.llmReg.SetRoles(p.Name, p.ModelName, p.Roles)
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
		limit := 200
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if n, err := strconv.Atoi(lStr); err == nil && n > 0 && n <= 2000 {
				limit = n
			}
		}
		f := db.LogFilters{
			AgentID:    r.URL.Query().Get("agent_id"),
			TaskID:     r.URL.Query().Get("task_id"),
			ProjectID:  r.URL.Query().Get("project_id"),
			Level:      r.URL.Query().Get("level"),
			Limit:      limit,
			SystemOnly: r.URL.Query().Get("system_only") == "true",
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

	case http.MethodDelete:
		cutoff := time.Now().UTC().Add(24 * time.Hour) // default: delete all
		if b := r.URL.Query().Get("before"); b != "" {
			if t, err := time.Parse(time.RFC3339, b); err == nil {
				cutoff = t
			}
		}
		n, err := s.db.DeleteOldLogs(r.Context(), cutoff)
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]int64{"deleted": n})

	default:
		methodNotAllowed(w)
	}
}

// =========================================================================
// Task Logs
// =========================================================================

func (s *Server) handleTaskLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
	f := db.TaskLogFilters{
		TaskID:    r.URL.Query().Get("task_id"),
		ProjectID: r.URL.Query().Get("project_id"),
		AgentID:   r.URL.Query().Get("agent_id"),
		EventType: r.URL.Query().Get("event_type"),
		Limit:     500,
	}
	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			f.Since = t
		}
	}
	if until := r.URL.Query().Get("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			f.Until = t
		}
	}
	logs, err := s.db.ListTaskLogs(r.Context(), f)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if logs == nil {
		logs = []*db.TaskLog{}
	}
	api.WriteJSON(w, http.StatusOK, logs)

	case http.MethodDelete:
		var before time.Time
		if b := r.URL.Query().Get("before"); b != "" {
			if t, err := time.Parse(time.RFC3339, b); err == nil {
				before = t
			}
		}
		n, err := s.db.DeleteTaskLogs(r.Context(), before)
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]int64{"deleted": n})

	default:
		methodNotAllowed(w)
	}
}

// =========================================================================
// Agent Logs
// =========================================================================

func (s *Server) handleAgentLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		f := db.AgentLogFilters{
			AgentID:     r.URL.Query().Get("agent_id"),
			EventType:   r.URL.Query().Get("event_type"),
			TaskID:      r.URL.Query().Get("task_id"),
			ExecutionID: r.URL.Query().Get("execution_id"),
			Search:      r.URL.Query().Get("search"),
			Limit:       500,
		}
		if since := r.URL.Query().Get("since"); since != "" {
			if t, err := time.Parse(time.RFC3339, since); err == nil {
				f.Since = t
			}
		}
		if until := r.URL.Query().Get("until"); until != "" {
			if t, err := time.Parse(time.RFC3339, until); err == nil {
				f.Until = t
			}
		}
		logs, err := s.db.ListAgentLogs(r.Context(), f)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if logs == nil {
			logs = []*db.AgentLog{}
		}
		api.WriteJSON(w, http.StatusOK, logs)

	case http.MethodDelete:
		var before time.Time
		if b := r.URL.Query().Get("before"); b != "" {
			if t, err := time.Parse(time.RFC3339, b); err == nil {
				before = t
			}
		}
		n, err := s.db.DeleteAgentLogs(r.Context(), before)
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]int64{"deleted": n})

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

// resolveCredential resolves a credential reference to a token string.
// The ref is treated as an environment variable name; falls back to empty string.
func (s *Server) resolveCredential(ref string) string {
	if ref == "" {
		return ""
	}
	// Try environment variable first.
	if val := os.Getenv(ref); val != "" {
		return val
	}
	// Try platform_settings key.
	setting, err := s.db.GetSetting(context.Background(), ref)
	if err == nil {
		return setting.Value
	}
	return ""
}

func methodNotAllowed(w http.ResponseWriter) {
	api.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

// pathSegment extracts path components after a prefix.
// e.g. pathSegment("/api/projects/abc/tasks", "/api/projects/", 0) → "abc"
//
//	pathSegment("/api/projects/abc/tasks", "/api/projects/", 1) → "tasks"
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

// slugify converts a human-readable name into a URL-safe slug:
// lowercase, alphanumeric and hyphens only, no leading/trailing/consecutive hyphens.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevHyphen := true // suppress leading hyphens
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case (r == ' ' || r == '-' || r == '_') && !prevHyphen:
			b.WriteRune('-')
			prevHyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// decodeJSONOptional tries to decode the request body into v. Ignores EOF
// (empty body) and decode errors — the caller should check populated fields.
func (s *Server) decodeJSONOptional(r *http.Request, v interface{}) error {
	if r.ContentLength == 0 {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(v)
}

// recordMetric creates a metric row for a task, calculating cost from the
// provider model list when a model name is present in the metrics.
func (s *Server) recordMetric(ctx context.Context, taskID, agentID string, m *api.TaskMetrics, success bool) {
	if m == nil {
		return
	}
	cost := m.Cost
	// If the agent submitted a model name, re-derive cost from provider pricing
	// when it wasn't already calculated (or to verify it).
	if m.Model != "" && m.Cost == 0 && (m.InputTokens > 0 || m.OutputTokens > 0) {
		cost = s.costFromModel(ctx, m.Model, m.InputTokens, m.OutputTokens)
	}
	_ = s.db.CreateMetric(ctx, &db.Metric{
		TaskID:       taskID,
		AgentID:      agentID,
		Model:        m.Model,
		TokensUsed:   m.TokensUsed,
		InputTokens:  m.InputTokens,
		OutputTokens: m.OutputTokens,
		Cost:         cost,
		DurationMs:   m.DurationMs,
		Success:      success,
	})
}

// costFromModel looks up the provider that owns modelName and computes cost
// using its per-model pricing. Returns 0 when no pricing is found.
func (s *Server) costFromModel(ctx context.Context, modelName string, inputTokens, outputTokens int) float64 {
	providers, err := s.db.ListProviders(ctx)
	if err != nil {
		return 0
	}
	for _, p := range providers {
		for _, m := range p.Models {
			if m.Name == modelName {
				in := float64(inputTokens) / 1_000_000 * m.InputPerMillion
				out := float64(outputTokens) / 1_000_000 * m.OutputPerMillion
				return in + out
			}
		}
	}
	return 0
}
