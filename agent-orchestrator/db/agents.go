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

	if a.Mode == "" {
		a.Mode = "remote"
	}
	// Feature 7 reset rule: start params capture the registration payload and the
	// live values are initialised to them; control defaults to "run".
	if len(a.StartRoles) == 0 {
		a.StartRoles = a.Roles
	}
	if len(a.StartSkills) == 0 {
		a.StartSkills = a.Skills
	}
	if a.DesiredState == "" {
		a.DesiredState = "run"
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO agents (id, name, roles, status, mode, skills, start_roles, start_skills, desired_state, template_id, capabilities, registered_at, last_heartbeat)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, marshalJSONArray(a.Roles), a.Status, a.Mode,
		marshalJSONArray(a.Skills), marshalJSONArray(a.StartRoles), marshalJSONArray(a.StartSkills),
		a.DesiredState, a.TemplateID, marshalJSON(a.Capabilities), a.RegisteredAt, a.LastHeartbeat,
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
	if a.DesiredState == "" {
		a.DesiredState = "run"
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE agents SET roles=?, status=?, mode=?, current_task_id=?, skills=?,
		 start_roles=?, start_skills=?, desired_state=?, capabilities=?, last_heartbeat=?
		 WHERE id=?`,
		marshalJSONArray(a.Roles), a.Status, a.Mode, nullableStr(a.CurrentTaskID),
		marshalJSONArray(a.Skills), marshalJSONArray(a.StartRoles), marshalJSONArray(a.StartSkills),
		a.DesiredState, marshalJSON(a.Capabilities), a.LastHeartbeat, a.ID,
	)
	return err
}

// SetAgentDesiredState sets the control flag ("run" | "stop") for an agent.
func (d *Database) SetAgentDesiredState(ctx context.Context, agentID, state string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE agents SET desired_state=? WHERE id=?`, state, agentID)
	return err
}

// UpdateAgentLiveConfig overrides an agent's live roles/skills at runtime
// without touching its start params (Feature 7).
func (d *Database) UpdateAgentLiveConfig(ctx context.Context, agentID string, roles, skills []string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE agents SET roles=?, skills=? WHERE id=?`,
		marshalJSONArray(roles), marshalJSONArray(skills), agentID)
	return err
}

// ResetAgentToStart sets an agent's live roles/skills back to its start params
// (runtime reset, no restart).
func (d *Database) ResetAgentToStart(ctx context.Context, agentID string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE agents SET roles=start_roles, skills=start_skills WHERE id=?`, agentID)
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

// DeleteStaleOfflineAgents removes agents that have been offline for longer
// than olderThanSec seconds. This prevents stale records from prior runs
// from cluttering the agent list.
func (d *Database) DeleteStaleOfflineAgents(ctx context.Context, olderThanSec int) (int64, error) {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM agents WHERE status='offline'
		 AND CAST((julianday('now') - julianday(last_heartbeat)) * 86400 AS INTEGER) > ?`,
		olderThanSec,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- SQL and scan helpers ---

const agentSelectSQL = `SELECT id, name, roles, status,
    COALESCE(mode,'remote'), COALESCE(current_task_id,''), COALESCE(skills,'[]'),
    COALESCE(start_roles,'[]'), COALESCE(start_skills,'[]'), COALESCE(desired_state,'run'),
    COALESCE(template_id,''), capabilities, registered_at, last_heartbeat
    FROM agents`

func scanAgent(row *sql.Row) (*Agent, error) {
	var a Agent
	var rolesJSON, skillsJSON, startRolesJSON, startSkillsJSON, capsJSON, registeredAt, lastHeartbeat string
	err := row.Scan(&a.ID, &a.Name, &rolesJSON, &a.Status,
		&a.Mode, &a.CurrentTaskID, &skillsJSON,
		&startRolesJSON, &startSkillsJSON, &a.DesiredState, &a.TemplateID,
		&capsJSON, &registeredAt, &lastHeartbeat)
	if err != nil {
		return nil, err
	}
	a.Roles = unmarshalJSONStringSlice(rolesJSON)
	a.Skills = unmarshalJSONStringSlice(skillsJSON)
	a.StartRoles = unmarshalJSONStringSlice(startRolesJSON)
	a.StartSkills = unmarshalJSONStringSlice(startSkillsJSON)
	a.Capabilities = unmarshalJSONMap(capsJSON)
	a.RegisteredAt = parseTime(registeredAt)
	a.LastHeartbeat = parseTime(lastHeartbeat)
	return &a, nil
}

func scanAgents(rows *sql.Rows) ([]*Agent, error) {
	var agents []*Agent
	for rows.Next() {
		var a Agent
		var rolesJSON, skillsJSON, startRolesJSON, startSkillsJSON, capsJSON, registeredAt, lastHeartbeat string
		if err := rows.Scan(&a.ID, &a.Name, &rolesJSON, &a.Status,
			&a.Mode, &a.CurrentTaskID, &skillsJSON,
			&startRolesJSON, &startSkillsJSON, &a.DesiredState, &a.TemplateID,
			&capsJSON, &registeredAt, &lastHeartbeat); err != nil {
			return nil, err
		}
		a.Roles = unmarshalJSONStringSlice(rolesJSON)
		a.Skills = unmarshalJSONStringSlice(skillsJSON)
		a.StartRoles = unmarshalJSONStringSlice(startRolesJSON)
		a.StartSkills = unmarshalJSONStringSlice(startSkillsJSON)
		a.Capabilities = unmarshalJSONMap(capsJSON)
		a.RegisteredAt = parseTime(registeredAt)
		a.LastHeartbeat = parseTime(lastHeartbeat)
		agents = append(agents, &a)
	}
	return agents, rows.Err()
}

// ListAgentsByTemplate returns the agent rows spawned from a template (Feature 8).
func (d *Database) ListAgentsByTemplate(ctx context.Context, templateID string) ([]*Agent, error) {
	rows, err := d.db.QueryContext(ctx, agentSelectSQL+` WHERE template_id=? ORDER BY name`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgents(rows)
}

// SetAgentTemplateID links an existing agent row to a template (used when the
// supervisor adopts a row created by a prior run).
func (d *Database) SetAgentTemplateID(ctx context.Context, agentID, templateID string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE agents SET template_id=? WHERE id=?`, templateID, agentID)
	return err
}
