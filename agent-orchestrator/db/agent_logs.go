package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Debug-mode event type constants emitted when platform.debug_mode=true.
const (
	EventAgentHeartbeat  = "agent_heartbeat"
	EventAgentPollQuery  = "agent_poll_query"
	EventAgentPollNoTask = "agent_poll_no_task"
)

// Task event type constants.
const (
	EventTaskDependencyWarning = "task_dependency_warning"
	EventTaskDependencyAdded   = "task_dependency_added"
	EventTaskDependencyRemoved = "task_dependency_removed"
	EventTaskChecklistChanged  = "task_checklist_changed"
	EventTaskCommentAdded      = "task_comment_added"
	EventTaskLinkAdded         = "task_link_added"
	EventTaskLinkRemoved       = "task_link_removed"

	// W7.2: lifecycle event types for the agentic dev cycle.
	EventTaskSubmittedForReview = "task_submitted_for_review"
	EventTaskReviewPosted       = "task_review_posted"
	EventTaskRevisionStarted    = "task_revision_started"
	EventTaskMergeStarted       = "task_merge_started"
	EventTaskMergeCompleted     = "task_merge_completed"
	EventTaskMergeFailed        = "task_merge_failed"
	EventTaskPushedUpstream     = "task_pushed_upstream"
	EventTaskUpstreamSyncFailed = "task_upstream_sync_failed"
)

// AgentLog is one structured agent activity event row.
type AgentLog struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	AgentName   string    `json:"agent_name,omitempty"`
	EventType   string    `json:"event_type"`
	TaskID      string    `json:"task_id,omitempty"`
	ExecutionID string    `json:"execution_id,omitempty"`
	Description string    `json:"description,omitempty"`
	Metadata    string    `json:"metadata"`
	Timestamp   time.Time `json:"timestamp"`
}

// AgentLogFilters narrows ListAgentLogs results.
type AgentLogFilters struct {
	AgentID     string
	EventType   string
	TaskID      string
	ExecutionID string
	Search      string // substring match on description
	Since       time.Time
	Until       time.Time
	Limit       int
}

// CreateAgentLog writes one event to today's agent_logs_* partition.
// Returns immediately if LogDB is nil.
func (d *Database) CreateAgentLog(ctx context.Context, l *AgentLog) error {
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
	tbl, err := ensureAgentLogPartition(ctx, d.LogDB.db, l.Timestamp)
	if err != nil {
		return err
	}
	_, err = d.LogDB.db.ExecContext(ctx,
		"INSERT INTO "+tbl+
			" (id, agent_id, agent_name, event_type, task_id, execution_id, description, metadata, timestamp)"+
			" VALUES (?,?,?,?,?,?,?,?,?)",
		l.ID, l.AgentID, l.AgentName, l.EventType,
		l.TaskID, l.ExecutionID, l.Description,
		l.Metadata, l.Timestamp.UTC(),
	)
	return err
}

// DeleteAgentLogs deletes agent log rows across all partitions.
// If before is non-zero, only rows with timestamp < before are deleted.
// If before is zero, all rows in all partitions are deleted.
// Returns the total number of rows deleted.
func (d *Database) DeleteAgentLogs(ctx context.Context, before time.Time) (int64, error) {
	if d.LogDB == nil {
		return 0, nil
	}
	rows, err := d.LogDB.db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name LIKE ?",
		agentLogPrefix+"_%")
	if err != nil {
		return 0, fmt.Errorf("delete agent logs: list partitions: %w", err)
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
			return total, fmt.Errorf("delete agent logs from %s: %w", tbl, execErr)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// ListAgentLogs queries across partition tables overlapping [since, until].
func (d *Database) ListAgentLogs(ctx context.Context, f AgentLogFilters) ([]*AgentLog, error) {
	if d.LogDB == nil {
		return nil, nil
	}
	since := f.Since
	until := f.Until
	if since.IsZero() {
		since = time.Now().UTC().Add(-7 * 24 * time.Hour)
	}
	if until.IsZero() {
		until = time.Now().UTC()
	}
	limit := f.Limit
	if limit == 0 {
		limit = 500
	}

	candidates := partitionTablesForRange(agentLogPrefix, since, until)
	tables, err := existingTables(ctx, d.LogDB.db, candidates)
	if err != nil {
		return nil, fmt.Errorf("list agent log partitions: %w", err)
	}
	if len(tables) == 0 {
		return nil, nil
	}

	var parts []string
	var args []interface{}
	for _, tbl := range tables {
		var where []string
		var tblArgs []interface{}

		where = append(where, "timestamp BETWEEN ? AND ?")
		tblArgs = append(tblArgs, since.UTC(), until.UTC())
		if f.AgentID != "" {
			where = append(where, "agent_id=?")
			tblArgs = append(tblArgs, f.AgentID)
		}
		if f.EventType != "" {
			where = append(where, "event_type=?")
			tblArgs = append(tblArgs, f.EventType)
		}
		if f.TaskID != "" {
			where = append(where, "task_id=?")
			tblArgs = append(tblArgs, f.TaskID)
		}
		if f.ExecutionID != "" {
			where = append(where, "execution_id=?")
			tblArgs = append(tblArgs, f.ExecutionID)
		}
		if f.Search != "" {
			where = append(where, "description LIKE ?")
			tblArgs = append(tblArgs, "%"+f.Search+"%")
		}

		q := "SELECT id,agent_id,agent_name,event_type,task_id,execution_id,description,metadata,timestamp FROM " + tbl
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

	var logs []*AgentLog
	for rows.Next() {
		var l AgentLog
		var ts string
		if err := rows.Scan(
			&l.ID, &l.AgentID, &l.AgentName, &l.EventType,
			&l.TaskID, &l.ExecutionID, &l.Description, &l.Metadata, &ts,
		); err != nil {
			return nil, err
		}
		l.Timestamp = parseTime(ts)
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}
