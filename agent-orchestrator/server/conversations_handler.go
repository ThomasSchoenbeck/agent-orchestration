package server

import (
	"net/http"
	"strconv"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// handleConversations handles GET (list) and POST (create) for conversations.
func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if v, err := strconv.Atoi(o); err == nil && v >= 0 {
				offset = v
			}
		}

		conversations, err := s.db.ListConversations(r.Context(), limit, offset)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if conversations == nil {
			conversations = []*db.Conversation{}
		}
		api.WriteJSON(w, http.StatusOK, conversations)

	case http.MethodPost:
		var req struct {
			Title      string `json:"title"`
			ProviderID string `json:"provider_id,omitempty"`
		}
		if !s.decodeJSON(w, r, &req) {
			return
		}

		c := &db.Conversation{
			Title:      req.Title,
			ProviderID: req.ProviderID,
		}
		if err := s.db.CreateConversation(r.Context(), c); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, c)

	default:
		methodNotAllowed(w)
	}
}

// handleConversationDetail handles GET, PUT, DELETE for a specific conversation,
// and POST to /conversations/{id}/messages.
func (s *Server) handleConversationDetail(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path, "/api/conversations/")
	if len(parts) == 0 {
		api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, "conversation id required")
		return
	}

	id := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch sub {
	case "messages":
		s.handleConversationMessages(w, r, id)

	default:
		// GET /conversations/{id}, PUT /conversations/{id}, DELETE /conversations/{id}
		switch r.Method {
		case http.MethodGet:
			// Get conversation with recent messages (default: last 50)
			messageLimit := 50
			if m := r.URL.Query().Get("message_limit"); m != "" {
				if v, err := strconv.Atoi(m); err == nil && v > 0 {
					messageLimit = v
				}
			}

			c, messages, err := s.db.GetConversationWithMessages(r.Context(), id, messageLimit)
			if err != nil {
				api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
				return
			}

			// Return both conversation and messages
			api.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"conversation": c,
				"messages":     messages,
			})

		case http.MethodPut:
			c, err := s.db.GetConversation(r.Context(), id)
			if err != nil {
				api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
				return
			}

			var req struct {
				Title      *string `json:"title"`
				ProviderID *string `json:"provider_id"`
			}
			if !s.decodeJSON(w, r, &req) {
				return
			}

			if req.Title != nil {
				c.Title = *req.Title
			}
			if req.ProviderID != nil {
				c.ProviderID = *req.ProviderID
			}

			if err := s.db.UpdateConversation(r.Context(), c); err != nil {
				s.internalError(w, err)
				return
			}
			api.WriteJSON(w, http.StatusOK, c)

		case http.MethodDelete:
			if err := s.db.DeleteConversation(r.Context(), id); err != nil {
				s.internalError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			methodNotAllowed(w)
		}
	}
}

// handleConversationMessages handles GET (list messages) and POST (add message).
func (s *Server) handleConversationMessages(w http.ResponseWriter, r *http.Request, conversationID string) {
	switch r.Method {
	case http.MethodGet:
		limit := 50
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if v, err := strconv.Atoi(o); err == nil && v >= 0 {
				offset = v
			}
		}

		messages, err := s.db.ListMessages(r.Context(), conversationID, limit, offset)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if messages == nil {
			messages = []*db.Message{}
		}
		api.WriteJSON(w, http.StatusOK, messages)

	case http.MethodPost:
		var req struct {
			Role   string `json:"role"`
			Content string `json:"content"`
			TokensUsed int `json:"tokens_used,omitempty"`
		}
		if !s.decodeJSON(w, r, &req) {
			return
		}

		if req.Role == "" || req.Content == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "role and content are required")
			return
		}

		m := &db.Message{
			ConversationID: conversationID,
			Role:           req.Role,
			Content:        req.Content,
			TokensUsed:     req.TokensUsed,
		}
		if err := s.db.AddMessage(r.Context(), m); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, m)

	default:
		methodNotAllowed(w)
	}
}
