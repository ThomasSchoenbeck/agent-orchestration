package db

import (
	"context"
	"encoding/json"
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
	AgentName   string    `json:"agent_name,omitempty"` // resolved from metadata on read
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

// DeleteTaskLogs deletes task log rows across all partitions.
// If before is non-zero, only rows with timestamp < before are deleted.
// If before is zero, all rows in all partitions are deleted.
// Returns the total number of rows deleted.
func (d *Database) DeleteTaskLogs(ctx context.Context, before time.Time) (int64, error) {
	if d.LogDB == nil {
		return 0, nil
	}
	rows, err := d.LogDB.db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name LIKE ?",
		taskLogPrefix+"_%")
	if err != nil {
		return 0, fmt.Errorf("delete task logs: list partitions: %w", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return 0, err
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var total int64
	for _, tbl := range tables {
		var res interface{ RowsAffected() (int64, error) }
		var execErr error
		if before.IsZero() {
			res, execErr = d.LogDB.db.ExecContext(ctx, "DELETE FROM "+tbl)
		} else {
			res, execErr = d.LogDB.db.ExecContext(ctx, "DELETE FROM "+tbl+" WHERE timestamp < ?", before.UTC())
		}
		if execErr != nil {
			return total, fmt.Errorf("delete task logs from %s: %w", tbl, execErr)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// DeleteTaskLogsByTask deletes all task_log rows for a specific task across all partitions.
func (d *Database) DeleteTaskLogsByTask(ctx context.Context, taskID string) (int64, error) {
	if d.LogDB == nil {
		return 0, nil
	}
	rows, err := d.LogDB.db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name LIKE ?",
		taskLogPrefix+"_%")
	if err != nil {
		return 0, fmt.Errorf("delete task logs by task: list partitions: %w", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return 0, err
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var total int64
	for _, tbl := range tables {
		res, execErr := d.LogDB.db.ExecContext(ctx, "DELETE FROM "+tbl+" WHERE task_id=?", taskID)
		if execErr != nil {
			return total, fmt.Errorf("delete task logs by task from %s: %w", tbl, execErr)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
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
		l.AgentName = agentNameFromMetadata(l.Metadata)
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

// --- helpers that auto-record events in DB-layer mutations ---

func (d *Database) logTaskEvent(ctx context.Context, taskID, projectID, agentID, eventType, oldStatus, newStatus, desc string) {
	if d.LogDB == nil {
		return
	}
	// Resolve the agent's display name and embed identity + branch in metadata so
	// the event log and status timeline can render the actor without extra queries.
	// Identity is stored in the existing metadata JSON column rather than a new
	// column, so partitioned task_logs tables need no schema migration.
	meta := map[string]interface{}{}
	var agentName string
	if agentID != "" {
		agentName = d.agentDisplayName(ctx, agentID)
		meta["agent_id"] = agentID
		if agentName != "" {
			meta["agent_name"] = agentName
		}
	}
	if taskID != "" {
		meta["branch"] = "task/" + taskID
	}
	metaJSON := "{}"
	if len(meta) > 0 {
		if b, err := json.Marshal(meta); err == nil {
			metaJSON = string(b)
		}
	}
	_ = d.CreateTaskLog(ctx, &TaskLog{
		TaskID:      taskID,
		ProjectID:   projectID,
		AgentID:     agentID,
		AgentName:   agentName,
		EventType:   eventType,
		OldStatus:   oldStatus,
		NewStatus:   newStatus,
		Description: desc,
		Metadata:    metaJSON,
	})
}

// agentDisplayName resolves an agent's name by ID, returning "" if the ID is
// empty or does not correspond to a known agent (e.g. a user author ID).
func (d *Database) agentDisplayName(ctx context.Context, agentID string) string {
	if agentID == "" {
		return ""
	}
	a, err := d.GetAgent(ctx, agentID)
	if err != nil || a == nil {
		return ""
	}
	return a.Name
}

// agentNameFromMetadata extracts the agent_name value stored in a task log's
// metadata JSON, returning "" when absent or unparseable.
func agentNameFromMetadata(metadata string) string {
	if metadata == "" || metadata == "{}" {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &m); err != nil {
		return ""
	}
	if v, ok := m["agent_name"].(string); ok {
		return v
	}
	return ""
}
