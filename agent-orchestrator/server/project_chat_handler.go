package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// handleProjectChat handles POST /api/projects/:id/chat
// Provides project-scoped chat with context about the project.
func (s *Server) handleProjectChat(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req struct {
		Message        string `json:"message"`
		ProviderID     string `json:"provider_id,omitempty"`
		ConversationID string `json:"conversation_id,omitempty"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Message == "" {
		api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "message is required")
		return
	}

	// Load project
	project, err := s.db.GetProject(r.Context(), projectID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "project not found")
		return
	}

	// Create or get conversation
	var convID string
	if req.ConversationID != "" {
		convID = req.ConversationID
	} else {
		// Create new conversation for this project
		conv := &db.Conversation{
			Title:      fmt.Sprintf("Project: %s", project.Name),
			ProviderID: req.ProviderID,
		}
		if err := s.db.CreateConversation(r.Context(), conv); err != nil {
			s.internalError(w, err)
			return
		}
		convID = conv.ID
	}

	// Load conversation history
	messages, err := s.db.ListMessages(r.Context(), convID, 50, 0)
	if err != nil {
		s.internalError(w, err)
		return
	}

	// Build context for the system prompt
	context := s.buildProjectContext(r.Context(), project)

	// Build message history for LLM
	var llmMessages []llm.Message

	// Add system prompt with project context
	systemPrompt := fmt.Sprintf(
		"You are an assistant helping with the '%s' project. %s\n\n%s",
		project.Name, project.Description, context,
	)
	llmMessages = append(llmMessages, llm.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	// Add conversation history
	for _, msg := range messages {
		llmMessages = append(llmMessages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add user message
	llmMessages = append(llmMessages, llm.Message{
		Role:    "user",
		Content: req.Message,
	})

	// Determine provider using router
	if s.router == nil {
		api.WriteError(w, http.StatusServiceUnavailable, api.ErrCodeUnavailable, "router not configured")
		return
	}

	result, err := s.router.RouteByRole("orchestrator")
	if err != nil {
		api.WriteError(w, http.StatusServiceUnavailable, api.ErrCodeUnavailable, "no provider available")
		return
	}

	// Call LLM
	resp, err := result.Provider.Chat(r.Context(), llm.ChatRequest{
		Model:       result.Model,
		Messages:    llmMessages,
		MaxTokens:   2048,
		Temperature: 0.7,
	})
	if err != nil {
		s.internalError(w, err)
		return
	}

	// Save messages to conversation
	_ = s.db.AddMessage(r.Context(), &db.Message{
		ConversationID: convID,
		Role:           "user",
		Content:        req.Message,
	})
	_ = s.db.AddMessage(r.Context(), &db.Message{
		ConversationID: convID,
		Role:           "assistant",
		Content:        resp.Content,
	})

	// Return response
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"reply":             resp.Content,
		"conversation_id":   convID,
		"tokens_used":       resp.TokensUsed,
	})
}

// buildProjectContext assembles context information about the project for the LLM.
func (s *Server) buildProjectContext(ctx context.Context, project *db.Project) string {
	var parts []string

	// Task summary
	tasks, err := s.db.ListTasks(ctx, db.TaskFilters{ProjectID: project.ID})
	if err == nil && len(tasks) > 0 {
		counts := make(map[string]int)
		for _, t := range tasks {
			counts[t.Status]++
		}
		statuses := []string{}
		for status, count := range counts {
			statuses = append(statuses, fmt.Sprintf("%s: %d", status, count))
		}
		parts = append(parts, fmt.Sprintf("Project has %d tasks: %s", len(tasks), strings.Join(statuses, ", ")))
	}

	// Recent context entries (top 5)
	contexts, err := s.db.QueryContext(ctx, project.ID, "", 5)
	if err == nil && len(contexts) > 0 {
		contextItems := []string{}
		for _, ce := range contexts {
			if len(contextItems) >= 5 {
				break
			}
			contextItems = append(contextItems, fmt.Sprintf("- [%s] %s", ce.Type, ce.Content))
		}
		if len(contextItems) > 0 {
			parts = append(parts, fmt.Sprintf("Recent context:\n%s", strings.Join(contextItems, "\n")))
		}
	}

	return strings.Join(parts, "\n\n")
}
