package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateConversation inserts a new conversation.
func (d *Database) CreateConversation(ctx context.Context, c *Conversation) error {
	if c.ID == "" {
		c.ID = newID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	c.UpdatedAt = time.Now().UTC()

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO conversations (id, title, provider_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		c.ID, c.Title, c.ProviderID, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

// GetConversation retrieves a conversation by ID.
func (d *Database) GetConversation(ctx context.Context, id string) (*Conversation, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT id, title, provider_id, created_at, updated_at
		 FROM conversations WHERE id = ?`,
		id,
	)

	var c Conversation
	var createdAt, updatedAt string
	err := row.Scan(&c.ID, &c.Title, &c.ProviderID, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("conversation not found")
	}
	if err != nil {
		return nil, err
	}

	c.CreatedAt = parseTime(createdAt)
	c.UpdatedAt = parseTime(updatedAt)
	return &c, nil
}

// ListConversations retrieves all conversations, optionally limited and offset.
func (d *Database) ListConversations(ctx context.Context, limit, offset int) ([]*Conversation, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, title, provider_id, created_at, updated_at
		 FROM conversations
		 ORDER BY updated_at DESC
		 LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []*Conversation
	for rows.Next() {
		var c Conversation
		var createdAt, updatedAt string
		if err := rows.Scan(&c.ID, &c.Title, &c.ProviderID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(createdAt)
		c.UpdatedAt = parseTime(updatedAt)
		conversations = append(conversations, &c)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return conversations, nil
}

// UpdateConversation updates a conversation's title and/or provider_id.
func (d *Database) UpdateConversation(ctx context.Context, c *Conversation) error {
	c.UpdatedAt = time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE conversations
		 SET title = ?, provider_id = ?, updated_at = ?
		 WHERE id = ?`,
		c.Title, c.ProviderID, c.UpdatedAt, c.ID,
	)
	return err
}

// DeleteConversation deletes a conversation and all its messages.
func (d *Database) DeleteConversation(ctx context.Context, id string) error {
	// Delete messages first
	_, err := d.db.ExecContext(ctx,
		`DELETE FROM messages WHERE conversation_id = ?`, id,
	)
	if err != nil {
		return err
	}
	// Delete conversation
	_, err = d.db.ExecContext(ctx,
		`DELETE FROM conversations WHERE id = ?`, id,
	)
	return err
}

// AddMessage inserts a new message into a conversation.
func (d *Database) AddMessage(ctx context.Context, m *Message) error {
	if m.ID == "" {
		m.ID = newID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO messages
		 (id, conversation_id, role, content, tokens_used, input_tokens, output_tokens, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ConversationID, m.Role, m.Content,
		m.TokensUsed, m.InputTokens, m.OutputTokens, m.DurationMs,
		m.CreatedAt,
	)
	return err
}

// ListMessages retrieves all messages for a conversation, in chronological order.
func (d *Database) ListMessages(ctx context.Context, conversationID string, limit, offset int) ([]*Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, tokens_used, input_tokens, output_tokens, duration_ms, created_at
		 FROM messages
		 WHERE conversation_id = ?
		 ORDER BY created_at ASC
		 LIMIT ? OFFSET ?`,
		conversationID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var m Message
		var createdAt string
		if err := rows.Scan(
			&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.TokensUsed, &m.InputTokens, &m.OutputTokens, &m.DurationMs,
			&createdAt,
		); err != nil {
			return nil, err
		}
		m.CreatedAt = parseTime(createdAt)
		messages = append(messages, &m)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

// ListChatLog returns a lightweight view of recent messages across all conversations,
// joined with the conversation's provider name. Used by the Logs page.
func (d *Database) ListChatLog(ctx context.Context, limit int) ([]*ChatLogEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT m.created_at,
		       COALESCE(p.name, '') AS provider_name,
		       m.role,
		       substr(m.content, 1, 20) AS preview
		FROM messages m
		JOIN conversations c ON m.conversation_id = c.id
		LEFT JOIN providers p ON c.provider_id = p.id
		ORDER BY m.created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*ChatLogEntry
	for rows.Next() {
		var e ChatLogEntry
		var createdAt, role string
		if err := rows.Scan(&createdAt, &e.ProviderName, &role, &e.Preview); err != nil {
			return nil, err
		}
		e.Timestamp = parseTime(createdAt)
		if role == "user" {
			e.Direction = "user_to_llm"
		} else {
			e.Direction = "llm_to_user"
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// GetConversationWithMessages retrieves a conversation and its recent messages.
func (d *Database) GetConversationWithMessages(ctx context.Context, id string, messageLimit int) (*Conversation, []*Message, error) {
	c, err := d.GetConversation(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	messages, err := d.ListMessages(ctx, id, messageLimit, 0)
	if err != nil {
		return nil, nil, err
	}
	return c, messages, nil
}
