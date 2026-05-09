package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateAgent inserts a new agent record.
func (d *Database) CreateAgent(ctx context.Context, a *Agent) error {
	if a.ID == "" {
		a.ID = newID()
	}
	now := time.Now().UTC()
	a.RegisteredAt = now
	a.LastHeartbeat = now
	if a.Status == "" {
		a.Status = "online"
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO agents (id, name, roles, status, capabilities, registered_at, last_heartbeat)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, marshalJSONArray(a.Roles), a.Status,
		marshalJSON(a.Capabilities), a.RegisteredAt, a.LastHeartbeat,
	)
	return err
}

// GetAgent retrieves an agent by ID.
func (d *Database) GetAgent(ctx context.Context, id string) (*Agent, error) {
	row := d.db.QueryRowContext(ctx, agentSelectSQL+` WHERE id=?`, id)
	a, err := scanAgent(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent %q not found", id)
	}
	return a, err
}

// GetAgentByName retrieves an agent by name.
func (d *Database) GetAgentByName(ctx context.Context, name string) (*Agent, error) {
	row := d.db.QueryRowContext(ctx, agentSelectSQL+` WHERE name=?`, name)
	a, err := scanAgent(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent %q not found", name)
	}
	return a, err
}

// ListAgents returns all agents.
func (d *Database) ListAgents(ctx context.Context) ([]*Agent, error) {
	rows, err := d.db.QueryContext(ctx, agentSelectSQL+` ORDER BY registered_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgents(rows)
}

// UpdateAgent updates mutable agent fields.
func (d *Database) UpdateAgent(ctx context.Context, a *Agent) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE agents SET roles=?, status=?, current_task_id=?, capabilities=?, last_heartbeat=?
		 WHERE id=?`,
		marshalJSONArray(a.Roles), a.Status, nullableStr(a.CurrentTaskID),
		marshalJSON(a.Capabilities), a.LastHeartbeat, a.ID,
	)
	return err
}

// UpdateHeartbeat refreshes the agent's last_heartbeat and sets status to idle if previously offline.
func (d *Database) UpdateHeartbeat(ctx context.Context, agentID string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE agents SET last_heartbeat=CURRENT_TIMESTAMP,
		 status=CASE WHEN status='offline' THEN 'idle' ELSE status END
		 WHERE id=?`, agentID,
	)
	return err
}

// MarkOfflineAgents marks agents whose last heartbeat is older than timeoutSec seconds.
func (d *Database) MarkOfflineAgents(ctx context.Context, timeoutSec int) (int64, error) {
	res, err := d.db.ExecContext(ctx,
		`UPDATE agents SET status='offline'
		 WHERE status != 'offline'
		 AND CAST((julianday('now') - julianday(last_heartbeat)) * 86400 AS INTEGER) > ?`,
		timeoutSec,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteAgent removes an agent.
func (d *Database) DeleteAgent(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM agents WHERE id=?`, id)
	return err
}

// --- SQL and scan helpers ---

const agentSelectSQL = `SELECT id, name, roles, status,
    COALESCE(current_task_id,''), capabilities, registered_at, last_heartbeat
    FROM agents`

func scanAgent(row *sql.Row) (*Agent, error) {
	var a Agent
	var rolesJSON, capsJSON, registeredAt, lastHeartbeat string
	err := row.Scan(&a.ID, &a.Name, &rolesJSON, &a.Status,
		&a.CurrentTaskID, &capsJSON, &registeredAt, &lastHeartbeat)
	if err != nil {
		return nil, err
	}
	a.Roles = unmarshalJSONStringSlice(rolesJSON)
	a.Capabilities = unmarshalJSONMap(capsJSON)
	a.RegisteredAt = parseTime(registeredAt)
	a.LastHeartbeat = parseTime(lastHeartbeat)
	return &a, nil
}

func scanAgents(rows *sql.Rows) ([]*Agent, error) {
	var agents []*Agent
	for rows.Next() {
		var a Agent
		var rolesJSON, capsJSON, registeredAt, lastHeartbeat string
		if err := rows.Scan(&a.ID, &a.Name, &rolesJSON, &a.Status,
			&a.CurrentTaskID, &capsJSON, &registeredAt, &lastHeartbeat); err != nil {
			return nil, err
		}
		a.Roles = unmarshalJSONStringSlice(rolesJSON)
		a.Capabilities = unmarshalJSONMap(capsJSON)
		a.RegisteredAt = parseTime(registeredAt)
		a.LastHeartbeat = parseTime(lastHeartbeat)
		agents = append(agents, &a)
	}
	return agents, rows.Err()
}
