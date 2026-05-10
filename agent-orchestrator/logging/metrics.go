package logging

import (
	"context"
	"database/sql"
	"fmt"

	"agent-orchestrator/db"
)

// MetricsSummary is the aggregated view returned by /api/metrics.
type MetricsSummary struct {
	TotalTokens    int                `json:"total_tokens"`
	TotalCost      float64            `json:"total_cost"`
	TotalTasks     int                `json:"total_tasks"`
	SuccessRate    float64            `json:"success_rate"`
	AvgDurationMs  float64            `json:"avg_duration_ms"`
	ByAgent        []AgentMetrics     `json:"by_agent,omitempty"`
	ByProject      []ProjectMetrics   `json:"by_project,omitempty"`
}

// AgentMetrics holds per-agent aggregates.
type AgentMetrics struct {
	AgentID    string  `json:"agent_id"`
	Tasks      int     `json:"tasks"`
	Tokens     int     `json:"tokens"`
	Cost       float64 `json:"cost"`
	SuccessRate float64 `json:"success_rate"`
}

// ProjectMetrics holds per-project aggregates.
type ProjectMetrics struct {
	ProjectID string  `json:"project_id"`
	Tasks     int     `json:"tasks"`
	Tokens    int     `json:"tokens"`
	Cost      float64 `json:"cost"`
}

// Collector provides metric aggregation queries over the database.
type Collector struct {
	sqlDB interface {
		QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
		QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	}
}

// NewCollector creates a Collector. It accepts the raw *db.Database through
// a helper to keep the logging package decoupled.
func NewCollector(database *db.Database) *Collector {
	return &Collector{sqlDB: database.RawDB()}
}

// Summary returns aggregated metrics across all tasks and agents.
func (c *Collector) Summary(ctx context.Context) (*MetricsSummary, error) {
	s := &MetricsSummary{}

	// Overall totals from metrics table.
	row := c.sqlDB.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(tokens_used), 0),
			COALESCE(SUM(cost), 0.0),
			COUNT(*),
			CASE WHEN COUNT(*) > 0
				THEN CAST(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END) AS REAL) / COUNT(*)
				ELSE 0.0 END,
			COALESCE(AVG(duration_ms), 0.0)
		FROM metrics`)
	if err := row.Scan(&s.TotalTokens, &s.TotalCost, &s.TotalTasks,
		&s.SuccessRate, &s.AvgDurationMs); err != nil {
		return nil, fmt.Errorf("summary query: %w", err)
	}

	// Per-agent breakdown.
	rows, err := c.sqlDB.QueryContext(ctx, `
		SELECT
			COALESCE(agent_id,''),
			COUNT(*),
			COALESCE(SUM(tokens_used), 0),
			COALESCE(SUM(cost), 0.0),
			CASE WHEN COUNT(*) > 0
				THEN CAST(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END) AS REAL) / COUNT(*)
				ELSE 0.0 END
		FROM metrics
		WHERE agent_id IS NOT NULL AND agent_id != ''
		GROUP BY agent_id
		ORDER BY SUM(tokens_used) DESC
		LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("per-agent query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var am AgentMetrics
		if err := rows.Scan(&am.AgentID, &am.Tasks, &am.Tokens, &am.Cost, &am.SuccessRate); err != nil {
			return nil, err
		}
		s.ByAgent = append(s.ByAgent, am)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Per-project breakdown: join metrics → tasks → projects.
	prows, err := c.sqlDB.QueryContext(ctx, `
		SELECT
			COALESCE(t.project_id,''),
			COUNT(*),
			COALESCE(SUM(m.tokens_used), 0),
			COALESCE(SUM(m.cost), 0.0)
		FROM metrics m
		LEFT JOIN tasks t ON t.id = m.task_id
		WHERE t.project_id IS NOT NULL AND t.project_id != ''
		GROUP BY t.project_id
		ORDER BY SUM(m.tokens_used) DESC
		LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("per-project query: %w", err)
	}
	defer prows.Close()
	for prows.Next() {
		var pm ProjectMetrics
		if err := prows.Scan(&pm.ProjectID, &pm.Tasks, &pm.Tokens, &pm.Cost); err != nil {
			return nil, err
		}
		s.ByProject = append(s.ByProject, pm)
	}
	return s, prows.Err()
}

// TokenMetrics returns input/output token counts, aggregated by project and agent.
func (c *Collector) TokenMetrics(ctx context.Context) (*TokenMetrics, error) {
	t := &TokenMetrics{}

	// Overall totals.
	row := c.sqlDB.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(tokens_used), 0)
		FROM metrics`)
	if err := row.Scan(&t.TotalInputTokens, &t.TotalOutputTokens, &t.TotalTokens); err != nil {
		return nil, fmt.Errorf("token totals query: %w", err)
	}

	// Per-project.
	prows, err := c.sqlDB.QueryContext(ctx, `
		SELECT
			COALESCE(ta.project_id,'') AS project_id,
			COALESCE(SUM(m.input_tokens),0),
			COALESCE(SUM(m.output_tokens),0),
			COALESCE(SUM(m.tokens_used),0)
		FROM metrics m
		LEFT JOIN tasks ta ON ta.id = m.task_id
		WHERE ta.project_id IS NOT NULL AND ta.project_id != ''
		GROUP BY ta.project_id
		ORDER BY SUM(m.tokens_used) DESC
		LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("token by-project query: %w", err)
	}
	defer prows.Close()
	for prows.Next() {
		var pm ProjectTokenMetrics
		if err := prows.Scan(&pm.ProjectID, &pm.InputTokens, &pm.OutputTokens, &pm.TotalTokens); err != nil {
			return nil, err
		}
		t.ByProject = append(t.ByProject, pm)
	}
	if err := prows.Err(); err != nil {
		return nil, err
	}

	// Per-agent.
	arows, err := c.sqlDB.QueryContext(ctx, `
		SELECT
			COALESCE(agent_id,''),
			COALESCE(SUM(input_tokens),0),
			COALESCE(SUM(output_tokens),0),
			COALESCE(SUM(tokens_used),0)
		FROM metrics
		WHERE agent_id IS NOT NULL AND agent_id != ''
		GROUP BY agent_id
		ORDER BY SUM(tokens_used) DESC
		LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("token by-agent query: %w", err)
	}
	defer arows.Close()
	for arows.Next() {
		var am AgentTokenMetrics
		if err := arows.Scan(&am.AgentID, &am.InputTokens, &am.OutputTokens, &am.TotalTokens); err != nil {
			return nil, err
		}
		t.ByAgent = append(t.ByAgent, am)
	}
	return t, arows.Err()
}

// CostMetrics returns cost aggregates by project and agent.
func (c *Collector) CostMetrics(ctx context.Context) (*CostMetrics, error) {
	cm := &CostMetrics{}

	// Overall total.
	row := c.sqlDB.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost), 0.0) FROM metrics`)
	if err := row.Scan(&cm.TotalCost); err != nil {
		return nil, fmt.Errorf("cost total query: %w", err)
	}

	// Per-project.
	prows, err := c.sqlDB.QueryContext(ctx, `
		SELECT
			COALESCE(ta.project_id,''),
			COALESCE(SUM(m.cost), 0.0),
			COUNT(*)
		FROM metrics m
		LEFT JOIN tasks ta ON ta.id = m.task_id
		WHERE ta.project_id IS NOT NULL AND ta.project_id != ''
		GROUP BY ta.project_id
		ORDER BY SUM(m.cost) DESC
		LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("cost by-project query: %w", err)
	}
	defer prows.Close()
	for prows.Next() {
		var pm ProjectCostMetrics
		if err := prows.Scan(&pm.ProjectID, &pm.Cost, &pm.Tasks); err != nil {
			return nil, err
		}
		cm.ByProject = append(cm.ByProject, pm)
	}
	if err := prows.Err(); err != nil {
		return nil, err
	}

	// Per-agent.
	arows, err := c.sqlDB.QueryContext(ctx, `
		SELECT
			COALESCE(agent_id,''),
			COALESCE(SUM(cost), 0.0),
			COUNT(*)
		FROM metrics
		WHERE agent_id IS NOT NULL AND agent_id != ''
		GROUP BY agent_id
		ORDER BY SUM(cost) DESC
		LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("cost by-agent query: %w", err)
	}
	defer arows.Close()
	for arows.Next() {
		var am AgentCostMetrics
		if err := arows.Scan(&am.AgentID, &am.Cost, &am.Tasks); err != nil {
			return nil, err
		}
		cm.ByAgent = append(cm.ByAgent, am)
	}
	return cm, arows.Err()
}
