package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TaskLog is one task lifecycle event row.
type TaskLog struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	ProjectID   string    `json:"project_id,omitempty"`
	AgentID     string    `json:"agent_id,omitempty"`
	EventType   string    `json:"event_type"`
	OldStatus   string    `json:"old_status,omitempty"`
	NewStatus   string    `json:"new_status,omitempty"`
	Description string    `json:"description,omitempty"`
	Metadata    string    `json:"metadata"`
	Timestamp   time.Time `json:"timestamp"`
}

// TaskLogFilters narrows ListTaskLogs results.
type TaskLogFilters struct {
	TaskID    string
	ProjectID string
	AgentID   string
	EventType string
	Since     time.Time
	Until     time.Time
	Limit     int
}

// CreateTaskLog writes one event to today's task_logs_* partition.
// Returns immediately if LogDB is nil (log DB not configured).
func (d *Database) CreateTaskLog(ctx context.Context, l *TaskLog) error {
	if d.LogDB == nil {
		return nil
	}
	if l.ID == "" {
		l.ID = newID()
	}
	if l.Timestamp.IsZero() {
		l.Timestamp = time.Now().UTC()
	}
	if l.Metadata == "" {
		l.Metadata = "{}"
	}
	tbl, err := ensureTaskLogPartition(ctx, d.LogDB.db, l.Timestamp)
	if err != nil {
		return err
	}
	_, err = d.LogDB.db.ExecContext(ctx,
		"INSERT INTO "+tbl+
			" (id, task_id, project_id, agent_id, event_type, old_status, new_status, description, metadata, timestamp)"+
			" VALUES (?,?,?,?,?,?,?,?,?,?)",
		l.ID, l.TaskID, l.ProjectID, l.AgentID,
		l.EventType, l.OldStatus, l.NewStatus, l.Description,
		l.Metadata, l.Timestamp.UTC(),
	)
	return err
}

// ListTaskLogs queries across partition tables that overlap [since, until].
// Returns events ordered by timestamp ASC.
func (d *Database) ListTaskLogs(ctx context.Context, f TaskLogFilters) ([]*TaskLog, error) {
	if d.LogDB == nil {
		return nil, nil
	}
	since := f.Since
	until := f.Until
	if since.IsZero() {
		since = time.Now().UTC().Add(-30 * 24 * time.Hour)
	}
	if until.IsZero() {
		until = time.Now().UTC()
	}
	limit := f.Limit
	if limit == 0 {
		limit = 500
	}

	candidates := partitionTablesForRange(taskLogPrefix, since, until)
	tables, err := existingTables(ctx, d.LogDB.db, candidates)
	if err != nil {
		return nil, fmt.Errorf("list task log partitions: %w", err)
	}
	if len(tables) == 0 {
		return nil, nil
	}

	// Build UNION ALL query across partitions.
	var parts []string
	var args []interface{}
	for _, tbl := range tables {
		var where []string
		var tblArgs []interface{}

		where = append(where, "timestamp BETWEEN ? AND ?")
		tblArgs = append(tblArgs, since.UTC(), until.UTC())
		if f.TaskID != "" {
			where = append(where, "task_id=?")
			tblArgs = append(tblArgs, f.TaskID)
		}
		if f.ProjectID != "" {
			where = append(where, "project_id=?")
			tblArgs = append(tblArgs, f.ProjectID)
		}
		if f.AgentID != "" {
			where = append(where, "agent_id=?")
			tblArgs = append(tblArgs, f.AgentID)
		}
		if f.EventType != "" {
			where = append(where, "event_type=?")
			tblArgs = append(tblArgs, f.EventType)
		}

		q := "SELECT id,task_id,project_id,agent_id,event_type,old_status,new_status,description,metadata,timestamp FROM " + tbl
		if len(where) > 0 {
			q += " WHERE " + strings.Join(where, " AND ")
		}
		parts = append(parts, q)
		args = append(args, tblArgs...)
	}

	query := strings.Join(parts, " UNION ALL ") +
		fmt.Sprintf(" ORDER BY timestamp ASC LIMIT %d", limit)

	rows, err := d.LogDB.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*TaskLog
	for rows.Next() {
		var l TaskLog
		var ts string
		if err := rows.Scan(
			&l.ID, &l.TaskID, &l.ProjectID, &l.AgentID,
			&l.EventType, &l.OldStatus, &l.NewStatus,
			&l.Description, &l.Metadata, &ts,
		); err != nil {
			return nil, err
		}
		l.Timestamp = parseTime(ts)
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

// --- helpers that auto-record events in DB-layer mutations ---

func (d *Database) logTaskEvent(ctx context.Context, taskID, projectID, agentID, eventType, oldStatus, newStatus, desc string) {
	if d.LogDB == nil {
		return
	}
	_ = d.CreateTaskLog(ctx, &TaskLog{
		TaskID:      taskID,
		ProjectID:   projectID,
		AgentID:     agentID,
		EventType:   eventType,
		OldStatus:   oldStatus,
		NewStatus:   newStatus,
		Description: desc,
	})
}
