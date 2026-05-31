package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// CreateProvider inserts a new provider.
func (d *Database) CreateProvider(ctx context.Context, p *Provider) error {
	if p.ID == "" {
		p.ID = newID()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO providers
		 (id, name, type, base_url, model_name, api_key, enabled, roles, capabilities, config, models, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Type, p.BaseURL, p.ModelName, p.APIKey, p.Enabled,
		marshalJSONArray(p.Roles), marshalJSONArray(p.Capabilities), marshalJSON(p.Config),
		marshalJSONArray(p.Models), p.CreatedAt, p.UpdatedAt,
	)
	return err
}

// GetProvider retrieves a provider by ID.
func (d *Database) GetProvider(ctx context.Context, id string) (*Provider, error) {
	row := d.db.QueryRowContext(ctx, providerSelectSQL+` WHERE id=?`, id)
	p, err := scanProvider(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("provider %q not found", id)
	}
	return p, err
}

// ListProviders returns all providers ordered by name.
func (d *Database) ListProviders(ctx context.Context) ([]*Provider, error) {
	rows, err := d.db.QueryContext(ctx, providerSelectSQL+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProviders(rows)
}

// UpdateProvider replaces all mutable fields of a provider.
func (d *Database) UpdateProvider(ctx context.Context, p *Provider) error {
	p.UpdatedAt = time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE providers SET name=?, type=?, base_url=?, model_name=?, api_key=?,
		 enabled=?, roles=?, capabilities=?, config=?, models=?, updated_at=? WHERE id=?`,
		p.Name, p.Type, p.BaseURL, p.ModelName, p.APIKey, p.Enabled,
		marshalJSONArray(p.Roles), marshalJSONArray(p.Capabilities), marshalJSON(p.Config),
		marshalJSONArray(p.Models), p.UpdatedAt, p.ID,
	)
	return err
}

// DeleteProvider removes a provider.
func (d *Database) DeleteProvider(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM providers WHERE id=?`, id)
	return err
}

// CountProviders returns the total number of providers in the DB.
func (d *Database) CountProviders(ctx context.Context) (int, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM providers`).Scan(&n)
	return n, err
}

// SeedProviders inserts providers whose name does not already exist.
// Returns the number of newly inserted records.
func (d *Database) SeedProviders(ctx context.Context, providers []*Provider) (int, error) {
	seeded := 0
	for _, p := range providers {
		var count int
		if err := d.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM providers WHERE name=?`, p.Name,
		).Scan(&count); err != nil {
			return seeded, fmt.Errorf("checking provider %q: %w", p.Name, err)
		}
		if count > 0 {
			continue
		}
		if err := d.CreateProvider(ctx, p); err != nil {
			return seeded, fmt.Errorf("seeding provider %q: %w", p.Name, err)
		}
		seeded++
	}
	return seeded, nil
}

// ── SQL / scan helpers ────────────────────────────────────────────────────────

const providerSelectSQL = `SELECT id, name, type, base_url, model_name, api_key,
    enabled, roles, capabilities, config, models, created_at, updated_at FROM providers`

func scanProvider(row *sql.Row) (*Provider, error) {
	var p Provider
	var enabled int
	var rolesJSON, capsJSON, cfgJSON, modelsJSON, createdAt, updatedAt string
	err := row.Scan(
		&p.ID, &p.Name, &p.Type, &p.BaseURL, &p.ModelName, &p.APIKey,
		&enabled, &rolesJSON, &capsJSON, &cfgJSON, &modelsJSON, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Enabled = enabled != 0
	p.Roles = unmarshalJSONStringSlice(rolesJSON)
	p.Capabilities = unmarshalJSONStringSlice(capsJSON)
	p.Config = unmarshalJSONMap(cfgJSON)
	p.Models = unmarshalProviderModels(modelsJSON)
	p.CreatedAt = parseTime(createdAt)
	p.UpdatedAt = parseTime(updatedAt)
	return &p, nil
}

func scanProviders(rows *sql.Rows) ([]*Provider, error) {
	var providers []*Provider
	for rows.Next() {
		var p Provider
		var enabled int
		var rolesJSON, capsJSON, cfgJSON, modelsJSON, createdAt, updatedAt string
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Type, &p.BaseURL, &p.ModelName, &p.APIKey,
			&enabled, &rolesJSON, &capsJSON, &cfgJSON, &modelsJSON, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		p.Enabled = enabled != 0
		p.Roles = unmarshalJSONStringSlice(rolesJSON)
		p.Capabilities = unmarshalJSONStringSlice(capsJSON)
		p.Config = unmarshalJSONMap(cfgJSON)
		p.Models = unmarshalProviderModels(modelsJSON)
		p.CreatedAt = parseTime(createdAt)
		p.UpdatedAt = parseTime(updatedAt)
		providers = append(providers, &p)
	}
	return providers, rows.Err()
}

// unmarshalProviderModels decodes the JSON models column into []ProviderModel.
func unmarshalProviderModels(s string) []ProviderModel {
	if s == "" || s == "null" {
		return []ProviderModel{}
	}
	var models []ProviderModel
	if err := json.Unmarshal([]byte(s), &models); err != nil || models == nil {
		return []ProviderModel{}
	}
	return models
}
