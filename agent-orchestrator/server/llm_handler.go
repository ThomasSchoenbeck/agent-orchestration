package server

import (
	"encoding/json"
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/llm"
	"agent-orchestrator/logging"
)

// handleLLMChat handles POST /api/llm/chat
//
// Request body:
//
//	{
//	  "role":        "worker",     // optional — route by role
//	  "task_type":   "implement",  // optional — route by task type (takes priority)
//	  "messages":    [...],        // required
//	  "tools":       [...],        // optional tool definitions
//	  "max_tokens":  4096,         // optional, default 4096
//	  "temperature": 0.7           // optional
//	}
func (s *Server) handleLLMChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, api.ErrCodeInvalidInput, "method not allowed")
		return
	}

	var req struct {
		Role        string        `json:"role"`
		Messages    []llm.Message `json:"messages"`
		Tools       []llm.ToolDef `json:"tools"`
		MaxTokens   int           `json:"max_tokens"`
		Temperature float32       `json:"temperature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "invalid JSON: "+err.Error())
		return
	}
	if len(req.Messages) == 0 {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "messages is required")
		return
	}

	if s.router == nil {
		api.WriteError(w, http.StatusServiceUnavailable, api.ErrCodeUnavailable, "router not configured")
		return
	}

	// Resolve the routing target by role.
	var (
		provider llm.LLMProvider
		model    string
		role     string
	)
	if req.Role != "" {
		result, err := s.router.RouteByRole(req.Role)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, err.Error())
			return
		}
		provider, model, role = result.Provider, result.Model, result.Role
	} else {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "role is required")
		return
	}

	if provider == nil {
		api.WriteError(w, http.StatusServiceUnavailable, api.ErrCodeUnavailable, "no provider available for role: "+role)
		return
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	chatReq := llm.ChatRequest{
		Model:       model,
		Messages:    req.Messages,
		Tools:       req.Tools,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
	}

	resp, err := provider.Chat(r.Context(), chatReq)
	if err != nil {
		s.log.Error(r.Context(), "llm chat error", map[string]interface{}{
			"role":  role,
			"model": model,
			"error": err.Error(),
		})
		api.WriteError(w, http.StatusBadGateway, api.ErrCodeInternal, "LLM provider error: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"role":     role,
		"model":    model,
		"provider": provider.Name(),
		"response": resp,
	})
}

// handleMetrics handles GET /api/metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, api.ErrCodeInvalidInput, "method not allowed")
		return
	}

	collector := logging.NewCollector(s.db)
	summary, err := collector.Summary(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.ErrCodeInternal, "failed to collect metrics: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, summary)
}

// handleMetricsTokens handles GET /api/metrics/tokens
func (s *Server) handleMetricsTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, api.ErrCodeInvalidInput, "method not allowed")
		return
	}
	collector := logging.NewCollector(s.db)
	tokens, err := collector.TokenMetrics(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.ErrCodeInternal, "failed to collect token metrics: "+err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, tokens)
}

// handleMetricsCosts handles GET /api/metrics/costs
func (s *Server) handleMetricsCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, api.ErrCodeInvalidInput, "method not allowed")
		return
	}
	collector := logging.NewCollector(s.db)
	costs, err := collector.CostMetrics(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.ErrCodeInternal, "failed to collect cost metrics: "+err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, costs)
}
