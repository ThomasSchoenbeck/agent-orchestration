package db

import (
	"context"
	"encoding/json"
	"time"
)

// MergeLock represents a file-path lock held by a task that is currently merging.
type MergeLock struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Paths     []string  `json:"paths"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateMergeLock inserts a new merge lock for taskID covering paths.
func (d *Database) CreateMergeLock(ctx context.Context, taskID string, paths []string) error {
	pathsJSON, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx,
		`INSERT INTO merge_locks (id, task_id, paths_json, created_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		newID(), taskID, string(pathsJSON),
	)
	return err
}

// DeleteMergeLock removes the merge lock for the given task.
func (d *Database) DeleteMergeLock(ctx context.Context, taskID string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM merge_locks WHERE task_id=?`, taskID)
	return err
}

// ListMergeLocks returns all active merge locks.
func (d *Database) ListMergeLocks(ctx context.Context) ([]*MergeLock, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, task_id, paths_json, created_at FROM merge_locks ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locks []*MergeLock
	for rows.Next() {
		var l MergeLock
		var pathsJSON, createdAt string
		if err := rows.Scan(&l.ID, &l.TaskID, &pathsJSON, &createdAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(pathsJSON), &l.Paths)
		l.CreatedAt = parseTime(createdAt)
		locks = append(locks, &l)
	}
	return locks, rows.Err()
}
