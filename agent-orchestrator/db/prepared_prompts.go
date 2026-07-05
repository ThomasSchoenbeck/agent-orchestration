package db

import (
	"context"
	"time"
)

// CreatePreparedPrompt appends a synthesized prompt row (Phase 4, prompt_prep).
func (d *Database) CreatePreparedPrompt(ctx context.Context, p *PreparedPrompt) error {
	if p.ID == "" {
		p.ID = newID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO prepared_prompts (id, task_id, session_id, round, prompt, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.TaskID, p.SessionID, p.Round, p.Prompt, p.CreatedAt,
	)
	return err
}

// ListPreparedPrompts returns a task's synthesized prompts oldest-first.
func (d *Database) ListPreparedPrompts(ctx context.Context, taskID string) ([]*PreparedPrompt, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, task_id, session_id, round, prompt, created_at
		 FROM prepared_prompts WHERE task_id=? ORDER BY created_at, round`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*PreparedPrompt
	for rows.Next() {
		var p PreparedPrompt
		var createdAt string
		if err := rows.Scan(&p.ID, &p.TaskID, &p.SessionID, &p.Round, &p.Prompt, &createdAt); err != nil {
			return nil, err
		}
		p.CreatedAt = parseTime(createdAt)
		out = append(out, &p)
	}
	return out, rows.Err()
}
