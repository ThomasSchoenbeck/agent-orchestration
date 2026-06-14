package db

import (
	"context"
	"time"
)

// CreateAgentSession persists a checkpoint of a task's main loop.
func (d *Database) CreateAgentSession(ctx context.Context, s *AgentSession) error {
	if s.ID == "" {
		s.ID = newID()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	if s.Messages == "" {
		s.Messages = "[]"
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO agent_sessions (id, task_id, agent_id, summary, messages, round, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.TaskID, s.AgentID, s.Summary, s.Messages, s.Round, s.CreatedAt,
	)
	return err
}

// ListAgentSessionsByTask returns a task's checkpoints, oldest first.
func (d *Database) ListAgentSessionsByTask(ctx context.Context, taskID string) ([]*AgentSession, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, task_id, agent_id, summary, messages, round, created_at
		 FROM agent_sessions WHERE task_id=? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AgentSession
	for rows.Next() {
		var s AgentSession
		var createdAt string
		if err := rows.Scan(&s.ID, &s.TaskID, &s.AgentID, &s.Summary, &s.Messages, &s.Round, &createdAt); err != nil {
			return nil, err
		}
		s.CreatedAt = parseTime(createdAt)
		out = append(out, &s)
	}
	return out, rows.Err()
}
