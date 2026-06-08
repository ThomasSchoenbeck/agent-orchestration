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
	if m.Source == "" {
		m.Source = "agent"
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO metrics
		 (id, task_id, agent_id, model, tokens_used, input_tokens, output_tokens, cost, duration_ms, success, created_at,
		  source, provider_id, agent_role, conversation_id, project_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, nullableStr(m.TaskID), nullableStr(m.AgentID), m.Model,
		m.TokensUsed, m.InputTokens, m.OutputTokens,
		m.Cost, m.DurationMs, success, m.CreatedAt,
		m.Source, m.ProviderID, m.AgentRole, m.ConversationID, m.ProjectID,
	)
	return err
}

// CostBucket is one row of a cost breakdown grouped by some dimension.
type CostBucket struct {
	Key          string  `json:"key"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Cost         float64 `json:"cost"`
	Count        int     `json:"count"`
}

// costBreakdownColumns whitelists the group-by dimensions to a safe SQL column
// expression (prevents injection via the query param).
var costBreakdownColumns = map[string]string{
	"source":     "source",
	"agent_role": "agent_role",
	"agent_id":   "agent_id",
	"task":       "task_id",
	"chat":       "conversation_id",
	"provider":   "provider_id",
	"model":      "model",
	"project":    "project_id",
	// substr(...,1,10) takes the leading "YYYY-MM-DD"; the modernc/sqlite driver
	// stores time.Time in Go's String() layout, which SQLite's date() cannot parse.
	"day": "substr(created_at, 1, 10)",
}

// CostFilter narrows a cost breakdown to a subset of the metrics ledger.
// Zero-value fields are ignored, so the filter is fully optional and
// combinable. AgentRole is the "task type" dimension in the current model
// (tasks.type was retired in Feature 3; the role is recorded on each metric).
type CostFilter struct {
	From       *time.Time // inclusive lower bound on created_at
	To         *time.Time // exclusive upper bound on created_at
	Model      string
	AgentRole  string
	Source     string
	ProviderID string
}

// where builds the SQL WHERE fragment (with leading " WHERE ") and bound args
// for the non-empty filter fields. Values are always bound, never interpolated.
func (f CostFilter) where() (string, []interface{}) {
	var conds []string
	var args []interface{}
	if f.From != nil {
		conds = append(conds, "created_at >= ?")
		args = append(args, f.From.UTC())
	}
	if f.To != nil {
		conds = append(conds, "created_at < ?")
		args = append(args, f.To.UTC())
	}
	if f.Model != "" {
		conds = append(conds, "model = ?")
		args = append(args, f.Model)
	}
	if f.AgentRole != "" {
		conds = append(conds, "agent_role = ?")
		args = append(args, f.AgentRole)
	}
	if f.Source != "" {
		conds = append(conds, "source = ?")
		args = append(args, f.Source)
	}
	if f.ProviderID != "" {
		conds = append(conds, "provider_id = ?")
		args = append(args, f.ProviderID)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// CostBreakdown aggregates token usage and cost from the metrics ledger grouped
// by the given dimension (defaults to "source" for an unknown value) and
// narrowed by the optional filter.
func (d *Database) CostBreakdown(ctx context.Context, groupBy string, f CostFilter) ([]*CostBucket, error) {
	col, ok := costBreakdownColumns[groupBy]
	if !ok {
		col = "source"
	}
	whereSQL, args := f.where()
	rows, err := d.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT COALESCE(%s, '') AS k,
		        COALESCE(SUM(input_tokens), 0),
		        COALESCE(SUM(output_tokens), 0),
		        COALESCE(SUM(cost), 0),
		        COUNT(*)
		 FROM metrics%s
		 GROUP BY k
		 ORDER BY SUM(cost) DESC`, col, whereSQL), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CostBucket
	for rows.Next() {
		var b CostBucket
		if err := rows.Scan(&b.Key, &b.InputTokens, &b.OutputTokens, &b.Cost, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, rows.Err()
}

// CostFilterOptions enumerates the distinct values available for each filter
// dimension plus the min/max metric dates, so the UI can populate its controls.
type CostFilterOptions struct {
	Models     []string `json:"models"`
	AgentRoles []string `json:"agent_roles"`
	Sources    []string `json:"sources"`
	Providers  []string `json:"providers"`
	MinDate    string   `json:"min_date"`
	MaxDate    string   `json:"max_date"`
}

// CostFilterOptions returns the selectable filter values from the metrics ledger.
func (d *Database) CostFilterOptions(ctx context.Context) (*CostFilterOptions, error) {
	opts := &CostFilterOptions{
		Models: []string{}, AgentRoles: []string{}, Sources: []string{}, Providers: []string{},
	}
	cols := map[string]*[]string{
		"model":       &opts.Models,
		"agent_role":  &opts.AgentRoles,
		"source":      &opts.Sources,
		"provider_id": &opts.Providers,
	}
	for col, dst := range cols {
		rows, err := d.db.QueryContext(ctx, fmt.Sprintf(
			`SELECT DISTINCT %s FROM metrics WHERE %s <> '' AND %s IS NOT NULL ORDER BY %s`,
			col, col, col, col))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				return nil, err
			}
			*dst = append(*dst, v)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	row := d.db.QueryRowContext(ctx,
		`SELECT COALESCE(MIN(substr(created_at, 1, 10)), ''), COALESCE(MAX(substr(created_at, 1, 10)), '') FROM metrics`)
	if err := row.Scan(&opts.MinDate, &opts.MaxDate); err != nil {
		return nil, err
	}
	return opts, nil
}

// TaskCostSummary is the response payload for GET /api/tasks/{id}/cost.
type TaskCostSummary struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Rounds       int     `json:"rounds"`
}

// GetTaskCost aggregates token usage and cost from the metrics table for a task.
func (d *Database) GetTaskCost(ctx context.Context, taskID string) (*TaskCostSummary, error) {
	row := d.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(tokens_used), 0),
			COALESCE(SUM(cost), 0.0),
			COUNT(*)
		FROM metrics
		WHERE task_id = ?`, taskID)
	var s TaskCostSummary
	if err := row.Scan(&s.InputTokens, &s.OutputTokens, &s.TotalTokens, &s.CostUSD, &s.Rounds); err != nil {
		return nil, fmt.Errorf("GetTaskCost %q: %w", taskID, err)
	}
	return &s, nil
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
