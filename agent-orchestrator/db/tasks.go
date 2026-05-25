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
		t.Status = TaskStatusBacklog
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO tasks
		 (id, project_id, type, role, status, priority, assigned_agent_id, payload, attempts, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.ProjectID, t.Type, t.Role, t.Status, t.Priority,
		nullableStr(t.AssignedAgentID), marshalJSON(t.Payload), t.Attempts,
		t.CreatedAt, t.UpdatedAt,
	)
	if err == nil {
		d.logTaskEvent(ctx, t.ID, t.ProjectID, "", "task_created", "", t.Status, "Task created")
	}
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
	if f.RequirementID != "" {
		where = append(where,
			"id IN (SELECT task_id FROM task_project_links WHERE kind='requirement' AND target_id=?)")
		args = append(args, f.RequirementID)
	}
	if f.FeatureID != "" {
		where = append(where,
			"id IN (SELECT task_id FROM task_project_links WHERE kind='feature' AND target_id=?)")
		args = append(args, f.FeatureID)
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
	// Read the current status before overwriting so the log shows the real transition.
	var oldStatus string
	_ = d.db.QueryRowContext(ctx, `SELECT status FROM tasks WHERE id=?`, t.ID).Scan(&oldStatus)

	t.UpdatedAt = time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE tasks SET status=?, priority=?, assigned_agent_id=?, payload=?,
		 result=?, attempts=?, branch_head_sha=?, worktree_path=?, assigned_port=?,
		 updated_at=?, started_at=?, completed_at=?
		 WHERE id=?`,
		t.Status, t.Priority, nullableStr(t.AssignedAgentID),
		marshalJSON(t.Payload), nullableJSON(t.Result), t.Attempts,
		t.BranchHeadSHA, t.WorktreePath, nullableInt(t.AssignedPort),
		t.UpdatedAt, nullableTime(t.StartedAt), nullableTime(t.CompletedAt),
		t.ID,
	)
	if err == nil {
		var msg string
		if oldStatus != "" && oldStatus != t.Status {
			msg = fmt.Sprintf("Status: %s → %s", oldStatus, t.Status)
		} else {
			msg = "Fields updated"
		}
		d.logTaskEvent(ctx, t.ID, t.ProjectID, t.AssignedAgentID, "task_updated", oldStatus, t.Status, msg)
	}
	return err
}

// ClaimTask atomically claims a task for an agent using an IMMEDIATE transaction
// so concurrent agents cannot double-claim the same task.
// Returns an error if the task is already in_progress, completed, or failed.
func (d *Database) ClaimTask(ctx context.Context, taskID, agentID string) error {
	if err := d.withImmediateTx(ctx, func(tx *sql.Tx) error {
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
		if IsExecutionState(status) {
			return fmt.Errorf("task %q is already claimed by agent %q", taskID, assignedID)
		}
		if status == TaskStatusCompleted || status == TaskStatusFailed {
			return fmt.Errorf("task %q cannot be claimed: status is %q", taskID, status)
		}
		if !IsQueueState(status) {
			return fmt.Errorf("task %q cannot be claimed from state %q", taskID, status)
		}
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx,
			`UPDATE tasks SET status=?, assigned_agent_id=?, started_at=?,
			 updated_at=?, attempts=attempts+1 WHERE id=?`,
			claimTargetState(status), agentID, now, now, taskID,
		)
		return err
	}); err != nil {
		return err
	}
	d.logTaskEvent(ctx, taskID, "", agentID, "task_claimed", TaskStatusBacklog, TaskStatusDeveloping, "Task claimed by agent")
	// Emit a soft warning if any dependencies are not yet completed.
	if unfinished, err := d.uncompletedDeps(ctx, taskID); err == nil && len(unfinished) > 0 {
		msg := fmt.Sprintf("claimed with %d uncompleted dependenc", len(unfinished))
		if len(unfinished) == 1 {
			msg += "y"
		} else {
			msg += "ies"
		}
		d.logTaskEvent(ctx, taskID, "", agentID, EventTaskDependencyWarning, "", TaskStatusDeveloping, msg)
	}
	return nil
}

// SubmitTaskResult stores the result and updates task status.
func (d *Database) SubmitTaskResult(ctx context.Context, taskID string, result map[string]interface{}, status string) error {
	now := time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE tasks SET result=?, status=?, completed_at=?, updated_at=? WHERE id=?`,
		marshalJSON(result), status, now, now, taskID,
	)
	if err == nil {
		eventType := "task_completed"
		if status == "failed" {
			eventType = "task_failed"
		}
		d.logTaskEvent(ctx, taskID, "", "", eventType, TaskStatusDeveloping, status, "Task result submitted")
	}
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
		fmt.Sprintf(` WHERE (status='BACKLOG' OR status='AWAITING_REVISION' OR status='AWAITING_REVIEW' OR status='AWAITING_MERGE')
		 AND (assigned_agent_id IS NULL OR assigned_agent_id='')
		 AND role IN (%s) ORDER BY priority DESC, created_at ASC LIMIT 1`, placeholders)

	row := d.db.QueryRowContext(ctx, query, args...)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, nil // no tasks available
	}
	return t, err
}

// RequeueTimedOutTasks re-queues execution-state tasks that have not been updated for timeoutSec seconds.
func (d *Database) RequeueTimedOutTasks(ctx context.Context, timeoutSec int) (int64, error) {
	res, err := d.db.ExecContext(ctx,
		`UPDATE tasks SET status='BACKLOG', assigned_agent_id=NULL, updated_at=CURRENT_TIMESTAMP
		 WHERE (status='DEVELOPING' OR status='REVIEWING' OR status='MERGING')
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
    attempts,
    COALESCE(branch_head_sha,''), COALESCE(last_push_at,''),
    COALESCE(worktree_path,''), COALESCE(assigned_port,0),
    created_at, updated_at,
    COALESCE(started_at,''), COALESCE(completed_at,'')
    FROM tasks`

func scanTask(row *sql.Row) (*Task, error) {
	var t Task
	var payloadJSON, resultJSON, assignedID string
	var branchSHA, lastPushAt, worktreePath string
	var assignedPort int
	var createdAt, updatedAt, startedAt, completedAt string

	err := row.Scan(
		&t.ID, &t.ProjectID, &t.Type, &t.Role, &t.Status, &t.Priority,
		&assignedID, &payloadJSON, &resultJSON,
		&t.Attempts,
		&branchSHA, &lastPushAt,
		&worktreePath, &assignedPort,
		&createdAt, &updatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}
	t.AssignedAgentID = assignedID
	t.Payload = unmarshalJSONMap(payloadJSON)
	if resultJSON != "{}" && resultJSON != "" {
		t.Result = unmarshalJSONMap(resultJSON)
	}
	t.BranchHeadSHA = branchSHA
	t.WorktreePath = worktreePath
	t.AssignedPort = assignedPort
	if lastPushAt != "" {
		if ts := parseTime(lastPushAt); !ts.IsZero() {
			t.LastPushAt = &ts
		}
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
		var branchSHA, lastPushAt, worktreePath string
		var assignedPort int
		var createdAt, updatedAt, startedAt, completedAt string

		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.Type, &t.Role, &t.Status, &t.Priority,
			&assignedID, &payloadJSON, &resultJSON,
			&t.Attempts,
			&branchSHA, &lastPushAt,
			&worktreePath, &assignedPort,
			&createdAt, &updatedAt, &startedAt, &completedAt,
		); err != nil {
			return nil, err
		}
		t.AssignedAgentID = assignedID
		t.Payload = unmarshalJSONMap(payloadJSON)
		if resultJSON != "{}" && resultJSON != "" {
			t.Result = unmarshalJSONMap(resultJSON)
		}
		t.BranchHeadSHA = branchSHA
		t.WorktreePath = worktreePath
		t.AssignedPort = assignedPort
		if lastPushAt != "" {
			if ts := parseTime(lastPushAt); !ts.IsZero() {
				t.LastPushAt = &ts
			}
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

func nullableInt(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

// claimTargetState maps a queue state to its execution counterpart.
// BACKLOG/AWAITING_REVISION → DEVELOPING
// AWAITING_REVIEW → REVIEWING
// AWAITING_MERGE  → MERGING
func claimTargetState(queueState string) string {
	switch queueState {
	case TaskStatusAwaitingReview:
		return TaskStatusReviewing
	case TaskStatusAwaitingMerge:
		return TaskStatusMerging
	default: // BACKLOG, AWAITING_REVISION
		return TaskStatusDeveloping
	}
}

// TransitionTaskState atomically moves a task to a new state, recording the
// transition in task_state_transitions.
func (d *Database) TransitionTaskState(ctx context.Context, taskID, fromState, toState, actorAgentID, reason string) error {
	return d.withImmediateTx(ctx, func(tx *sql.Tx) error {
		var current string
		if err := tx.QueryRowContext(ctx, "SELECT status FROM tasks WHERE id=?", taskID).Scan(&current); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("task %q not found", taskID)
			}
			return err
		}
		if current != fromState {
			return fmt.Errorf("task %q: expected state %s, got %s", taskID, fromState, current)
		}
		now := time.Now().UTC()
		if _, err := tx.ExecContext(ctx,
			"UPDATE tasks SET status=?, updated_at=? WHERE id=?", toState, now, taskID,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO task_state_transitions (id, task_id, from_state, to_state, actor_agent_id, reason)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			newID(), taskID, fromState, toState, actorAgentID, reason,
		)
		return err
	})
}

// DeleteTask removes a task by ID. Returns an error if not found.
func (d *Database) DeleteTask(ctx context.Context, id string) error {
	res, err := d.db.ExecContext(ctx, `DELETE FROM tasks WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %q not found", id)
	}
	return nil
}
