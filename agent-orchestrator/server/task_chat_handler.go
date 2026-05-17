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

// handleTaskChat handles POST /api/tasks/:id/chat
func (s *Server) handleTaskChat(w http.ResponseWriter, r *http.Request, taskID string) {
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

	task, err := s.db.GetTask(r.Context(), taskID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "task not found")
		return
	}

	// Create or reuse conversation.
	var convID string
	if req.ConversationID != "" {
		convID = req.ConversationID
	} else {
		title := ""
		if v, ok := task.Payload["title"].(string); ok {
			title = v
		}
		if title == "" {
			title = task.Type
		}
		conv := &db.Conversation{
			Title:      fmt.Sprintf("Task: %s", title),
			ProviderID: req.ProviderID,
		}
		if err := s.db.CreateConversation(r.Context(), conv); err != nil {
			s.internalError(w, err)
			return
		}
		convID = conv.ID
	}

	messages, err := s.db.ListMessages(r.Context(), convID, 50, 0)
	if err != nil {
		s.internalError(w, err)
		return
	}

	systemPrompt := s.buildTaskSystemPrompt(r.Context(), task)

	var llmMessages []llm.Message
	llmMessages = append(llmMessages, llm.Message{Role: "system", Content: systemPrompt})
	for _, msg := range messages {
		llmMessages = append(llmMessages, llm.Message{Role: msg.Role, Content: msg.Content})
	}
	llmMessages = append(llmMessages, llm.Message{Role: "user", Content: req.Message})

	if s.router == nil {
		api.WriteError(w, http.StatusServiceUnavailable, api.ErrCodeUnavailable, "router not configured")
		return
	}
	result, err := s.router.RouteByRole("orchestrator")
	if err != nil {
		api.WriteError(w, http.StatusServiceUnavailable, api.ErrCodeUnavailable, "no provider available")
		return
	}

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

	_ = s.db.AddMessage(r.Context(), &db.Message{ConversationID: convID, Role: "user", Content: req.Message})
	_ = s.db.AddMessage(r.Context(), &db.Message{ConversationID: convID, Role: "assistant", Content: resp.Content})

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"reply":           resp.Content,
		"conversation_id": convID,
		"tokens_used":     resp.TokensUsed,
	})
}

// buildTaskSystemPrompt constructs the system prompt for a task-scoped chat.
func (s *Server) buildTaskSystemPrompt(ctx context.Context, task *db.Task) string {
	title := ""
	if v, ok := task.Payload["title"].(string); ok {
		title = v
	}
	desc := ""
	if v, ok := task.Payload["description"].(string); ok {
		desc = v
	}

	var parts []string
	parts = append(parts, fmt.Sprintf(
		"You are an assistant helping with a task.\nTask: %s\nType: %s | Role: %s | Status: %s",
		title, task.Type, task.Role, task.Status,
	))
	if desc != "" {
		parts = append(parts, "Description:\n"+desc)
	}

	// Recent task logs (last 10).
	logs, err := s.db.ListTaskLogs(ctx, db.TaskLogFilters{TaskID: task.ID, Limit: 10})
	if err == nil && len(logs) > 0 {
		lines := []string{"Recent events:"}
		for _, l := range logs {
			lines = append(lines, fmt.Sprintf("- [%s] %s", l.EventType, l.Description))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}

	// Linked requirements and features.
	links, err := s.db.ListTaskLinks(ctx, task.ID)
	if err == nil && len(links) > 0 {
		var reqs, feats []string
		for _, l := range links {
			if l.Kind == "requirement" {
				reqs = append(reqs, l.TargetID)
			} else {
				feats = append(feats, l.TargetID)
			}
		}
		if len(reqs) > 0 {
			parts = append(parts, "Linked requirements: "+strings.Join(reqs, ", "))
		}
		if len(feats) > 0 {
			parts = append(parts, "Linked features: "+strings.Join(feats, ", "))
		}
	}

	return strings.Join(parts, "\n\n")
}
