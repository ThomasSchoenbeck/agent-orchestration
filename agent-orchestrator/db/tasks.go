package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CreateTask inserts a new task.
func (d *Database) CreateTask(ctx context.Context, t *Task) error {
	if t.ID == "" {
		t.ID = newID()
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = "planned"
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO tasks
		 (id, project_id, type, role, status, priority, assigned_agent_id, payload, attempts, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.ProjectID, t.Type, t.Role, t.Status, t.Priority,
		nullableStr(t.AssignedAgentID), marshalJSON(t.Payload), t.Attempts,
		t.CreatedAt, t.UpdatedAt,
	)
	return err
}

// GetTask retrieves a task by ID.
func (d *Database) GetTask(ctx context.Context, id string) (*Task, error) {
	row := d.db.QueryRowContext(ctx, taskSelectSQL+` WHERE id=?`, id)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task %q not found", id)
	}
	return t, err
}

// ListTasks returns tasks matching the given filters.
func (d *Database) ListTasks(ctx context.Context, f TaskFilters) ([]*Task, error) {
	query := taskSelectSQL
	var args []interface{}
	var where []string

	if f.ProjectID != "" {
		where = append(where, "project_id=?")
		args = append(args, f.ProjectID)
	}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	if f.Role != "" {
		where = append(where, "role=?")
		args = append(args, f.Role)
	}
	if f.AgentID != "" {
		where = append(where, "assigned_agent_id=?")
		args = append(args, f.AgentID)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY priority DESC, created_at ASC"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// UpdateTask updates a task's mutable fields.
func (d *Database) UpdateTask(ctx context.Context, t *Task) error {
	t.UpdatedAt = time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE tasks SET status=?, priority=?, assigned_agent_id=?, payload=?,
		 result=?, attempts=?, updated_at=?, started_at=?, completed_at=?
		 WHERE id=?`,
		t.Status, t.Priority, nullableStr(t.AssignedAgentID),
		marshalJSON(t.Payload), nullableJSON(t.Result), t.Attempts,
		t.UpdatedAt, nullableTime(t.StartedAt), nullableTime(t.CompletedAt),
		t.ID,
	)
	return err
}

// ClaimTask atomically claims a task for an agent.
// Returns an error if the task is already in_progress.
func (d *Database) ClaimTask(ctx context.Context, taskID, agentID string) error {
	return d.withTx(ctx, func(tx *sql.Tx) error {
		var status, assignedID string
		err := tx.QueryRowContext(ctx,
			`SELECT status, COALESCE(assigned_agent_id,'') FROM tasks WHERE id=?`, taskID,
		).Scan(&status, &assignedID)
		if err == sql.ErrNoRows {
			return fmt.Errorf("task %q not found", taskID)
		}
		if err != nil {
			return err
		}
		if status == "in_progress" {
			return fmt.Errorf("task %q is already claimed by agent %q", taskID, assignedID)
		}
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx,
			`UPDATE tasks SET status='in_progress', assigned_agent_id=?, started_at=?,
			 updated_at=?, attempts=attempts+1 WHERE id=?`,
			agentID, now, now, taskID,
		)
		return err
	})
}

// SubmitTaskResult stores the result and updates task status.
func (d *Database) SubmitTaskResult(ctx context.Context, taskID string, result map[string]interface{}, status string) error {
	now := time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE tasks SET result=?, status=?, completed_at=?, updated_at=? WHERE id=?`,
		marshalJSON(result), status, now, now, taskID,
	)
	return err
}

// GetNextTask returns the highest-priority unassigned task matching any of the given roles.
func (d *Database) GetNextTask(ctx context.Context, roles []string) (*Task, error) {
	if len(roles) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(roles))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]interface{}, len(roles))
	for i, r := range roles {
		args[i] = r
	}

	query := taskSelectSQL +
		fmt.Sprintf(` WHERE status='planned' AND (assigned_agent_id IS NULL OR assigned_agent_id='')
		 AND role IN (%s) ORDER BY priority DESC, created_at ASC LIMIT 1`, placeholders)

	row := d.db.QueryRowContext(ctx, query, args...)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, nil // no tasks available
	}
	return t, err
}

// RequeueTimedOutTasks re-queues in_progress tasks that have not been updated for timeoutSec seconds.
func (d *Database) RequeueTimedOutTasks(ctx context.Context, timeoutSec int) (int64, error) {
	res, err := d.db.ExecContext(ctx,
		`UPDATE tasks SET status='planned', assigned_agent_id=NULL, updated_at=CURRENT_TIMESTAMP
		 WHERE status='in_progress'
		 AND CAST((julianday('now') - julianday(updated_at)) * 86400 AS INTEGER) > ?`,
		timeoutSec,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- SQL fragment and scan helpers ---

const taskSelectSQL = `SELECT id, project_id, type, role, status, priority,
    COALESCE(assigned_agent_id,''), COALESCE(payload,'{}'), COALESCE(result,'{}'),
    attempts, created_at, updated_at,
    COALESCE(started_at,''), COALESCE(completed_at,'')
    FROM tasks`

func scanTask(row *sql.Row) (*Task, error) {
	var t Task
	var payloadJSON, resultJSON, assignedID string
	var createdAt, updatedAt, startedAt, completedAt string

	err := row.Scan(
		&t.ID, &t.ProjectID, &t.Type, &t.Role, &t.Status, &t.Priority,
		&assignedID, &payloadJSON, &resultJSON,
		&t.Attempts, &createdAt, &updatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}
	t.AssignedAgentID = assignedID
	t.Payload = unmarshalJSONMap(payloadJSON)
	if resultJSON != "{}" && resultJSON != "" {
		t.Result = unmarshalJSONMap(resultJSON)
	}
	t.CreatedAt = parseTime(createdAt)
	t.UpdatedAt = parseTime(updatedAt)
	if startedAt != "" {
		if ts := parseTime(startedAt); !ts.IsZero() {
			t.StartedAt = &ts
		}
	}
	if completedAt != "" {
		if ts := parseTime(completedAt); !ts.IsZero() {
			t.CompletedAt = &ts
		}
	}
	return &t, nil
}

func scanTasks(rows *sql.Rows) ([]*Task, error) {
	var tasks []*Task
	for rows.Next() {
		var t Task
		var payloadJSON, resultJSON, assignedID string
		var createdAt, updatedAt, startedAt, completedAt string

		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.Type, &t.Role, &t.Status, &t.Priority,
			&assignedID, &payloadJSON, &resultJSON,
			&t.Attempts, &createdAt, &updatedAt, &startedAt, &completedAt,
		); err != nil {
			return nil, err
		}
		t.AssignedAgentID = assignedID
		t.Payload = unmarshalJSONMap(payloadJSON)
		if resultJSON != "{}" && resultJSON != "" {
			t.Result = unmarshalJSONMap(resultJSON)
		}
		t.CreatedAt = parseTime(createdAt)
		t.UpdatedAt = parseTime(updatedAt)
		if startedAt != "" {
			if ts := parseTime(startedAt); !ts.IsZero() {
				t.StartedAt = &ts
			}
		}
		if completedAt != "" {
			if ts := parseTime(completedAt); !ts.IsZero() {
				t.CompletedAt = &ts
			}
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

// --- nullable helpers ---

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

func nullableJSON(m map[string]interface{}) interface{} {
	if m == nil {
		return nil
	}
	return marshalJSON(m)
}
