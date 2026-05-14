package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// partitionName returns the table name for the given prefix and date (UTC).
func partitionName(prefix string, t time.Time) string {
	return fmt.Sprintf("%s_%s", prefix, t.UTC().Format("2006_01_02"))
}

// partitionTablesForRange returns all table names for [since, until] (inclusive,
// one per calendar day in UTC). The caller should filter out tables that don't
// actually exist before querying.
func partitionTablesForRange(prefix string, since, until time.Time) []string {
	since = since.UTC().Truncate(24 * time.Hour)
	until = until.UTC()
	var tables []string
	for d := since; !d.After(until); d = d.Add(24 * time.Hour) {
		tables = append(tables, partitionName(prefix, d))
	}
	return tables
}

// existingTables returns only those table names (from the given list) that
// actually exist in the database, querying sqlite_master.
func existingTables(ctx context.Context, sqlDB *sql.DB, candidates []string) ([]string, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(candidates))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(candidates))
	for i, c := range candidates {
		args[i] = c
	}
	rows, err := sqlDB.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	return result, rows.Err()
}

// ----- Agent log partitions -----

const agentLogPrefix = "agent_logs"

const agentLogSchema = `
CREATE TABLE IF NOT EXISTS %s (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL,
    agent_name   TEXT NOT NULL DEFAULT '',
    event_type   TEXT NOT NULL,
    task_id      TEXT NOT NULL DEFAULT '',
    execution_id TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    metadata     TEXT NOT NULL DEFAULT '{}',
    timestamp    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_%s_agent      ON %s(agent_id);
CREATE INDEX IF NOT EXISTS idx_%s_type       ON %s(event_type);
CREATE INDEX IF NOT EXISTS idx_%s_exec       ON %s(execution_id);
CREATE INDEX IF NOT EXISTS idx_%s_ts         ON %s(timestamp);
`

// ensureAgentLogPartition creates the daily agent log table if it doesn't exist.
// CREATE TABLE IF NOT EXISTS is idempotent, so no in-process cache is needed.
func ensureAgentLogPartition(ctx context.Context, sqlDB *sql.DB, t time.Time) (string, error) {
	name := partitionName(agentLogPrefix, t)
	ddl := fmt.Sprintf(agentLogSchema,
		name, name, name, name, name, name, name, name, name)
	if _, err := sqlDB.ExecContext(ctx, ddl); err != nil {
		return "", fmt.Errorf("create agent log partition %s: %w", name, err)
	}
	return name, nil
}

// ----- Task log partitions -----

const taskLogPrefix = "task_logs"

const taskLogSchema = `
CREATE TABLE IF NOT EXISTS %s (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL,
    project_id  TEXT NOT NULL DEFAULT '',
    agent_id    TEXT NOT NULL DEFAULT '',
    event_type  TEXT NOT NULL,
    old_status  TEXT NOT NULL DEFAULT '',
    new_status  TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    metadata    TEXT NOT NULL DEFAULT '{}',
    timestamp   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_%s_task_id    ON %s(task_id);
CREATE INDEX IF NOT EXISTS idx_%s_event_type ON %s(event_type);
CREATE INDEX IF NOT EXISTS idx_%s_ts         ON %s(timestamp);
`

// ensureTaskLogPartition creates the daily task log table if it doesn't exist.
func ensureTaskLogPartition(ctx context.Context, sqlDB *sql.DB, t time.Time) (string, error) {
	name := partitionName(taskLogPrefix, t)
	ddl := fmt.Sprintf(taskLogSchema,
		name, name, name, name, name, name, name)
	if _, err := sqlDB.ExecContext(ctx, ddl); err != nil {
		return "", fmt.Errorf("create task log partition %s: %w", name, err)
	}
	return name, nil
}

// ----- Retention cleanup (two-pass) -----

// DropOldAgentLogPartitions drops all agent_logs_* tables older than maxAgeDays.
func DropOldAgentLogPartitions(ctx context.Context, sqlDB *sql.DB, maxAgeDays int) (int, error) {
	return dropOldPartitions(ctx, sqlDB, agentLogPrefix, maxAgeDays)
}

// DropOldTaskLogPartitions drops all task_logs_* tables older than maxAgeDays.
func DropOldTaskLogPartitions(ctx context.Context, sqlDB *sql.DB, maxAgeDays int) (int, error) {
	return dropOldPartitions(ctx, sqlDB, taskLogPrefix, maxAgeDays)
}

func dropOldPartitions(ctx context.Context, sqlDB *sql.DB, prefix string, maxAgeDays int) (int, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)
	rows, err := sqlDB.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name LIKE ?",
		prefix+"_%")
	if err != nil {
		return 0, err
	}
	var toDropNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return 0, err
		}
		// Parse date suffix: prefix_YYYY_MM_DD
		suffix := strings.TrimPrefix(name, prefix+"_")
		t, err := time.Parse("2006_01_02", suffix)
		if err != nil {
			continue // not a date partition, skip
		}
		if t.Before(cutoff) {
			toDropNames = append(toDropNames, name)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	dropped := 0
	for _, name := range toDropNames {
		if _, err := sqlDB.ExecContext(ctx, "DROP TABLE IF EXISTS "+name); err != nil {
			return dropped, fmt.Errorf("drop %s: %w", name, err)
		}
		dropped++
	}
	return dropped, nil
}

// DeleteShortRetentionAgentRows deletes rows for short-retention event types
// from all live agent_logs_* partitions.
func DeleteShortRetentionAgentRows(ctx context.Context, sqlDB *sql.DB, eventType string, cutoff time.Time) (int64, error) {
	return deleteShortRetentionRows(ctx, sqlDB, agentLogPrefix, eventType, cutoff)
}

// DeleteShortRetentionTaskRows deletes rows for short-retention event types
// from all live task_logs_* partitions.
func DeleteShortRetentionTaskRows(ctx context.Context, sqlDB *sql.DB, eventType string, cutoff time.Time) (int64, error) {
	return deleteShortRetentionRows(ctx, sqlDB, taskLogPrefix, eventType, cutoff)
}

func deleteShortRetentionRows(ctx context.Context, sqlDB *sql.DB, prefix, eventType string, cutoff time.Time) (int64, error) {
	rows, err := sqlDB.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name LIKE ?",
		prefix+"_%")
	if err != nil {
		return 0, err
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

	var total int64
	for _, tbl := range tables {
		res, err := sqlDB.ExecContext(ctx,
			"DELETE FROM "+tbl+" WHERE event_type=? AND timestamp<?",
			eventType, cutoff.UTC())
		if err != nil {
			return total, fmt.Errorf("delete from %s: %w", tbl, err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}
