package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// UpsertTaskMemory writes the task's memory, replacing any existing row for the
// same task_id (one memory row per task). The id of a pre-existing row is
// preserved; callers should read back via GetTaskMemory for the canonical row.
func (d *Database) UpsertTaskMemory(ctx context.Context, m *TaskMemory) error {
	if m.TaskID == "" {
		return fmt.Errorf("UpsertTaskMemory: task_id is required")
	}
	if m.ID == "" {
		m.ID = newID()
	}
	m.UpdatedAt = time.Now().UTC()
	contentJSON, err := json.Marshal(m.Content)
	if err != nil {
		return fmt.Errorf("marshal task memory: %w", err)
	}
	_, err = d.db.ExecContext(ctx,
		`INSERT INTO task_memory (id, task_id, content_json, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(task_id) DO UPDATE SET
		   content_json = excluded.content_json,
		   updated_at   = excluded.updated_at`,
		m.ID, m.TaskID, string(contentJSON), m.UpdatedAt,
	)
	return err
}

// GetTaskMemory returns the task's memory, or (nil, nil) when none exists yet —
// an absent memory is not an error (callers treat it as empty).
func (d *Database) GetTaskMemory(ctx context.Context, taskID string) (*TaskMemory, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT id, task_id, content_json, updated_at FROM task_memory WHERE task_id=?`, taskID)
	var m TaskMemory
	var contentJSON, updatedAt string
	if err := row.Scan(&m.ID, &m.TaskID, &contentJSON, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if contentJSON != "" {
		_ = json.Unmarshal([]byte(contentJSON), &m.Content)
	}
	m.UpdatedAt = parseTime(updatedAt)
	return &m, nil
}
