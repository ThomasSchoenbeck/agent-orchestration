package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateAgentTemplate inserts a new agent template.
func (d *Database) CreateAgentTemplate(ctx context.Context, t *AgentTemplate) error {
	if t.ID == "" {
		t.ID = newID()
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Replicas < 0 {
		t.Replicas = 0
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO agent_templates (id, name, roles, skills, replicas, autostart, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, marshalJSONArray(t.Roles), marshalJSONArray(t.Skills),
		t.Replicas, t.Autostart, t.Enabled, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

// GetAgentTemplate retrieves a template by ID.
func (d *Database) GetAgentTemplate(ctx context.Context, id string) (*AgentTemplate, error) {
	row := d.db.QueryRowContext(ctx, agentTemplateSelectSQL+` WHERE id=?`, id)
	t, err := scanAgentTemplate(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent template %q not found", id)
	}
	return t, err
}

// ListAgentTemplates returns all templates ordered by name.
func (d *Database) ListAgentTemplates(ctx context.Context) ([]*AgentTemplate, error) {
	rows, err := d.db.QueryContext(ctx, agentTemplateSelectSQL+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AgentTemplate
	for rows.Next() {
		t, err := scanAgentTemplateRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateAgentTemplate replaces all mutable fields of a template.
func (d *Database) UpdateAgentTemplate(ctx context.Context, t *AgentTemplate) error {
	t.UpdatedAt = time.Now().UTC()
	if t.Replicas < 0 {
		t.Replicas = 0
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE agent_templates SET name=?, roles=?, skills=?, replicas=?, autostart=?, enabled=?, updated_at=?
		 WHERE id=?`,
		t.Name, marshalJSONArray(t.Roles), marshalJSONArray(t.Skills),
		t.Replicas, t.Autostart, t.Enabled, t.UpdatedAt, t.ID,
	)
	return err
}

// SetTemplateReplicas updates only the desired replica count.
func (d *Database) SetTemplateReplicas(ctx context.Context, id string, replicas int) error {
	if replicas < 0 {
		replicas = 0
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE agent_templates SET replicas=?, updated_at=? WHERE id=?`,
		replicas, time.Now().UTC(), id)
	return err
}

// DeleteAgentTemplate removes a template.
func (d *Database) DeleteAgentTemplate(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM agent_templates WHERE id=?`, id)
	return err
}

// CountAgentTemplates returns the number of agent templates.
func (d *Database) CountAgentTemplates(ctx context.Context) (int, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM agent_templates`).Scan(&n)
	return n, err
}

// SeedAgentTemplates inserts templates that don't already exist (idempotent by
// name). Returns the number inserted.
func (d *Database) SeedAgentTemplates(ctx context.Context, templates []*AgentTemplate) (int, error) {
	inserted := 0
	for _, t := range templates {
		var exists int
		if err := d.db.QueryRowContext(ctx,
			`SELECT count(*) FROM agent_templates WHERE name=?`, t.Name).Scan(&exists); err != nil {
			return inserted, err
		}
		if exists > 0 {
			continue
		}
		if err := d.CreateAgentTemplate(ctx, t); err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

// --- SQL / scan helpers ---

const agentTemplateSelectSQL = `SELECT id, name, roles, skills, replicas, autostart, enabled, created_at, updated_at
    FROM agent_templates`

func scanAgentTemplate(row *sql.Row) (*AgentTemplate, error) {
	var t AgentTemplate
	var rolesJSON, skillsJSON, createdAt, updatedAt string
	var autostart, enabled int
	err := row.Scan(&t.ID, &t.Name, &rolesJSON, &skillsJSON, &t.Replicas, &autostart, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.Roles = unmarshalJSONStringSlice(rolesJSON)
	t.Skills = unmarshalJSONStringSlice(skillsJSON)
	t.Autostart = autostart != 0
	t.Enabled = enabled != 0
	t.CreatedAt = parseTime(createdAt)
	t.UpdatedAt = parseTime(updatedAt)
	return &t, nil
}

func scanAgentTemplateRows(rows *sql.Rows) (*AgentTemplate, error) {
	var t AgentTemplate
	var rolesJSON, skillsJSON, createdAt, updatedAt string
	var autostart, enabled int
	err := rows.Scan(&t.ID, &t.Name, &rolesJSON, &skillsJSON, &t.Replicas, &autostart, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.Roles = unmarshalJSONStringSlice(rolesJSON)
	t.Skills = unmarshalJSONStringSlice(skillsJSON)
	t.Autostart = autostart != 0
	t.Enabled = enabled != 0
	t.CreatedAt = parseTime(createdAt)
	t.UpdatedAt = parseTime(updatedAt)
	return &t, nil
}
