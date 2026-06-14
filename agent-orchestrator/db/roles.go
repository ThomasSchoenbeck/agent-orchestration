package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateRoleDefinition inserts a new role definition.
func (d *Database) CreateRoleDefinition(ctx context.Context, r *RoleDefinition) error {
	if r.ID == "" {
		r.ID = newID()
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now

	if r.ContextInclude == nil {
		r.ContextInclude = []string{}
	}
	if r.ContextExclude == nil {
		r.ContextExclude = []string{}
	}
	if r.Capabilities == nil {
		r.Capabilities = []string{}
	}
	if r.AllowedTools == nil {
		r.AllowedTools = []string{}
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO agent_role_definitions
		 (id, name, label, description, provider_id, model_override, system_prompt,
		  context_include, context_exclude, capabilities, allowed_tools,
		  temperature, max_tokens, resync_prompt, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Label, r.Description,
		nullableStr(r.ProviderID), r.ModelOverride, r.SystemPrompt,
		marshalJSONArray(r.ContextInclude), marshalJSONArray(r.ContextExclude),
		marshalJSONArray(r.Capabilities), marshalJSONArray(r.AllowedTools),
		r.Temperature, r.MaxTokens, r.ResyncPrompt, r.Enabled,
		r.CreatedAt, r.UpdatedAt,
	)
	return err
}

// GetRoleDefinition retrieves a role definition by ID.
func (d *Database) GetRoleDefinition(ctx context.Context, id string) (*RoleDefinition, error) {
	row := d.db.QueryRowContext(ctx, roleDefSelectSQL+` WHERE id=?`, id)
	rd, err := scanRoleDef(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("role definition %q not found", id)
	}
	return rd, err
}

// GetRoleDefinitionByName retrieves a role definition by name.
func (d *Database) GetRoleDefinitionByName(ctx context.Context, name string) (*RoleDefinition, error) {
	row := d.db.QueryRowContext(ctx, roleDefSelectSQL+` WHERE name=?`, name)
	rd, err := scanRoleDef(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("role definition %q not found", name)
	}
	return rd, err
}

// ListRoleDefinitions returns all role definitions ordered by name.
func (d *Database) ListRoleDefinitions(ctx context.Context) ([]*RoleDefinition, error) {
	rows, err := d.db.QueryContext(ctx, roleDefSelectSQL+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoleDefs(rows)
}

// UpdateRoleDefinition replaces all mutable fields of a role definition.
func (d *Database) UpdateRoleDefinition(ctx context.Context, r *RoleDefinition) error {
	r.UpdatedAt = time.Now().UTC()
	if r.ContextInclude == nil {
		r.ContextInclude = []string{}
	}
	if r.ContextExclude == nil {
		r.ContextExclude = []string{}
	}
	if r.Capabilities == nil {
		r.Capabilities = []string{}
	}
	if r.AllowedTools == nil {
		r.AllowedTools = []string{}
	}

	_, err := d.db.ExecContext(ctx,
		`UPDATE agent_role_definitions SET
		 name=?, label=?, description=?, provider_id=?, model_override=?, system_prompt=?,
		 context_include=?, context_exclude=?, capabilities=?, allowed_tools=?,
		 temperature=?, max_tokens=?, resync_prompt=?, enabled=?, updated_at=?
		 WHERE id=?`,
		r.Name, r.Label, r.Description,
		nullableStr(r.ProviderID), r.ModelOverride, r.SystemPrompt,
		marshalJSONArray(r.ContextInclude), marshalJSONArray(r.ContextExclude),
		marshalJSONArray(r.Capabilities), marshalJSONArray(r.AllowedTools),
		r.Temperature, r.MaxTokens, r.ResyncPrompt, r.Enabled, r.UpdatedAt, r.ID,
	)
	return err
}

// DeleteRoleDefinition removes a role definition.
func (d *Database) DeleteRoleDefinition(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM agent_role_definitions WHERE id=?`, id)
	return err
}

// CountRoleDefinitions returns the total number of role definitions.
func (d *Database) CountRoleDefinitions(ctx context.Context) (int, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_role_definitions`).Scan(&n)
	return n, err
}

// SeedRoleDefinitions inserts role definitions whose name does not already exist.
func (d *Database) SeedRoleDefinitions(ctx context.Context, roles []*RoleDefinition) (int, error) {
	seeded := 0
	for _, r := range roles {
		var count int
		if err := d.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM agent_role_definitions WHERE name=?`, r.Name,
		).Scan(&count); err != nil {
			return seeded, fmt.Errorf("checking role %q: %w", r.Name, err)
		}
		if count > 0 {
			continue
		}
		if err := d.CreateRoleDefinition(ctx, r); err != nil {
			return seeded, fmt.Errorf("seeding role %q: %w", r.Name, err)
		}
		seeded++
	}
	return seeded, nil
}

// ── SQL / scan helpers ────────────────────────────────────────────────────────

const roleDefSelectSQL = `SELECT id, name, label, description, provider_id, model_override,
    system_prompt, context_include, context_exclude, capabilities, allowed_tools,
    temperature, max_tokens, resync_prompt, enabled, created_at, updated_at
    FROM agent_role_definitions`

func scanRoleDef(row *sql.Row) (*RoleDefinition, error) {
	var r RoleDefinition
	var providerID sql.NullString
	var ciJSON, ceJSON, capJSON, atJSON, createdAt, updatedAt string
	var enabled int
	err := row.Scan(
		&r.ID, &r.Name, &r.Label, &r.Description,
		&providerID, &r.ModelOverride, &r.SystemPrompt,
		&ciJSON, &ceJSON, &capJSON, &atJSON,
		&r.Temperature, &r.MaxTokens, &r.ResyncPrompt, &enabled,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	r.ProviderID = providerID.String
	r.Enabled = enabled != 0
	r.ContextInclude = unmarshalJSONStringSlice(ciJSON)
	r.ContextExclude = unmarshalJSONStringSlice(ceJSON)
	r.Capabilities = unmarshalJSONStringSlice(capJSON)
	r.AllowedTools = unmarshalJSONStringSlice(atJSON)
	r.CreatedAt = parseTime(createdAt)
	r.UpdatedAt = parseTime(updatedAt)
	return &r, nil
}

func scanRoleDefs(rows *sql.Rows) ([]*RoleDefinition, error) {
	var defs []*RoleDefinition
	for rows.Next() {
		var r RoleDefinition
		var providerID sql.NullString
		var ciJSON, ceJSON, capJSON, atJSON, createdAt, updatedAt string
		var enabled int
		if err := rows.Scan(
			&r.ID, &r.Name, &r.Label, &r.Description,
			&providerID, &r.ModelOverride, &r.SystemPrompt,
			&ciJSON, &ceJSON, &capJSON, &atJSON,
			&r.Temperature, &r.MaxTokens, &r.ResyncPrompt, &enabled,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		r.ProviderID = providerID.String
		r.Enabled = enabled != 0
		r.ContextInclude = unmarshalJSONStringSlice(ciJSON)
		r.ContextExclude = unmarshalJSONStringSlice(ceJSON)
		r.Capabilities = unmarshalJSONStringSlice(capJSON)
		r.AllowedTools = unmarshalJSONStringSlice(atJSON)
		r.CreatedAt = parseTime(createdAt)
		r.UpdatedAt = parseTime(updatedAt)
		defs = append(defs, &r)
	}
	return defs, rows.Err()
}
