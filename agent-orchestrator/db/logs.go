package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CreateLog inserts a log entry.
func (d *Database) CreateLog(ctx context.Context, l *LogEntry) error {
	if l.ID == "" {
		l.ID = newID()
	}
	if l.Timestamp.IsZero() {
		l.Timestamp = time.Now().UTC()
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO logs (id, agent_id, task_id, project_id, level, message, metadata, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, nullableStr(l.AgentID), nullableStr(l.TaskID), nullableStr(l.ProjectID),
		l.Level, l.Message, marshalJSON(l.Metadata), l.Timestamp,
	)
	return err
}

// ListLogs returns log entries matching the given filters.
func (d *Database) ListLogs(ctx context.Context, f LogFilters) ([]*LogEntry, error) {
	query := `SELECT id, COALESCE(agent_id,''), COALESCE(task_id,''), COALESCE(project_id,''),
	    level, message, metadata, timestamp FROM logs`
	var args []interface{}
	var where []string

	if f.AgentID != "" {
		where = append(where, "agent_id=?")
		args = append(args, f.AgentID)
	}
	if f.TaskID != "" {
		where = append(where, "task_id=?")
		args = append(args, f.TaskID)
	}
	if f.ProjectID != "" {
		where = append(where, "project_id=?")
		args = append(args, f.ProjectID)
	}
	if f.Level != "" {
		where = append(where, "level=?")
		args = append(args, f.Level)
	}
	if f.SystemOnly {
		where = append(where, "agent_id IS NULL")
		where = append(where, "task_id IS NULL")
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY timestamp DESC"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*LogEntry
	for rows.Next() {
		var l LogEntry
		var metaJSON, ts string
		if err := rows.Scan(&l.ID, &l.AgentID, &l.TaskID, &l.ProjectID,
			&l.Level, &l.Message, &metaJSON, &ts); err != nil {
			return nil, err
		}
		l.Metadata = unmarshalJSONMap(metaJSON)
		l.Timestamp = parseTime(ts)
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

// CreateMetric inserts an execution metric record.
func (d *Database) CreateMetric(ctx context.Context, m *Metric) error {
	if m.ID == "" {
		m.ID = newID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	// tokens_used = input + output for backward compatibility.
	if m.TokensUsed == 0 && (m.InputTokens > 0 || m.OutputTokens > 0) {
		m.TokensUsed = m.InputTokens + m.OutputTokens
	}
	success := 0
	if m.Success {
		success = 1
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO metrics
		 (id, task_id, agent_id, model, tokens_used, input_tokens, output_tokens, cost, duration_ms, success, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, nullableStr(m.TaskID), nullableStr(m.AgentID), m.Model,
		m.TokensUsed, m.InputTokens, m.OutputTokens,
		m.Cost, m.DurationMs, success, m.CreatedAt,
	)
	return err
}

// DeleteLogsByTask removes all log entries for a specific task.
func (d *Database) DeleteLogsByTask(ctx context.Context, taskID string) (int64, error) {
	res, err := d.db.ExecContext(ctx, `DELETE FROM logs WHERE task_id=?`, taskID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteOldLogs removes system log entries older than cutoff.
func (d *Database) DeleteOldLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM logs WHERE timestamp < ?`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
