package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateSkillDefinition inserts a new skill definition.
func (d *Database) CreateSkillDefinition(ctx context.Context, s *SkillDefinition) error {
	if s.ID == "" {
		s.ID = newID()
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now
	if s.ContextInclude == nil {
		s.ContextInclude = []string{}
	}
	if s.ContextExclude == nil {
		s.ContextExclude = []string{}
	}
	if s.AllowedTools == nil {
		s.AllowedTools = []string{}
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO skill_definitions
		 (id, name, label, description, prompt_fragment, context_include, context_exclude,
		  allowed_tools, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Label, s.Description, s.PromptFragment,
		marshalJSONArray(s.ContextInclude), marshalJSONArray(s.ContextExclude),
		marshalJSONArray(s.AllowedTools), s.Enabled, s.CreatedAt, s.UpdatedAt,
	)
	return err
}

// GetSkillDefinition retrieves a skill definition by ID.
func (d *Database) GetSkillDefinition(ctx context.Context, id string) (*SkillDefinition, error) {
	row := d.db.QueryRowContext(ctx, skillDefSelectSQL+` WHERE id=?`, id)
	s, err := scanSkillDef(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("skill definition %q not found", id)
	}
	return s, err
}

// GetSkillDefinitionByName retrieves a skill definition by name.
func (d *Database) GetSkillDefinitionByName(ctx context.Context, name string) (*SkillDefinition, error) {
	row := d.db.QueryRowContext(ctx, skillDefSelectSQL+` WHERE name=?`, name)
	s, err := scanSkillDef(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("skill definition %q not found", name)
	}
	return s, err
}

// ListSkillDefinitions returns all skill definitions ordered by name.
func (d *Database) ListSkillDefinitions(ctx context.Context) ([]*SkillDefinition, error) {
	rows, err := d.db.QueryContext(ctx, skillDefSelectSQL+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSkillDefs(rows)
}

// SkillsByNames returns the enabled skill definitions whose names are in the
// given list, preserving the input order. Unknown/disabled names are skipped.
func (d *Database) SkillsByNames(ctx context.Context, names []string) ([]*SkillDefinition, error) {
	if len(names) == 0 {
		return nil, nil
	}
	all, err := d.ListSkillDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*SkillDefinition, len(all))
	for _, s := range all {
		if s.Enabled {
			byName[s.Name] = s
		}
	}
	var out []*SkillDefinition
	for _, n := range names {
		if s, ok := byName[n]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// UpdateSkillDefinition replaces all mutable fields of a skill definition.
func (d *Database) UpdateSkillDefinition(ctx context.Context, s *SkillDefinition) error {
	s.UpdatedAt = time.Now().UTC()
	if s.ContextInclude == nil {
		s.ContextInclude = []string{}
	}
	if s.ContextExclude == nil {
		s.ContextExclude = []string{}
	}
	if s.AllowedTools == nil {
		s.AllowedTools = []string{}
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE skill_definitions SET
		 name=?, label=?, description=?, prompt_fragment=?,
		 context_include=?, context_exclude=?, allowed_tools=?, enabled=?, updated_at=?
		 WHERE id=?`,
		s.Name, s.Label, s.Description, s.PromptFragment,
		marshalJSONArray(s.ContextInclude), marshalJSONArray(s.ContextExclude),
		marshalJSONArray(s.AllowedTools), s.Enabled, s.UpdatedAt, s.ID,
	)
	return err
}

// DeleteSkillDefinition removes a skill definition.
func (d *Database) DeleteSkillDefinition(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM skill_definitions WHERE id=?`, id)
	return err
}

// SeedSkillDefinitions inserts skill definitions whose name does not already exist.
func (d *Database) SeedSkillDefinitions(ctx context.Context, skills []*SkillDefinition) (int, error) {
	seeded := 0
	for _, s := range skills {
		var count int
		if err := d.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM skill_definitions WHERE name=?`, s.Name,
		).Scan(&count); err != nil {
			return seeded, fmt.Errorf("checking skill %q: %w", s.Name, err)
		}
		if count > 0 {
			continue
		}
		if err := d.CreateSkillDefinition(ctx, s); err != nil {
			return seeded, fmt.Errorf("seeding skill %q: %w", s.Name, err)
		}
		seeded++
	}
	return seeded, nil
}

// DefaultSkillDefinitions returns the built-in starter skill set.
func DefaultSkillDefinitions() []*SkillDefinition {
	return []*SkillDefinition{
		{
			Name: "backend", Label: "Backend", Enabled: true,
			Description:    "Server-side, APIs, data, business logic.",
			PromptFragment: "You specialize in backend development: server-side logic, APIs, data modeling, and persistence. Favor correctness, clear error handling, and tests.",
			ContextInclude: []string{"server/**", "db/**", "api/**"},
		},
		{
			Name: "frontend", Label: "Frontend", Enabled: true,
			Description:    "UI, components, client-side state.",
			PromptFragment: "You specialize in frontend development: UI components, layout, accessibility, and client-side state. Keep components simple and match the existing design system.",
			ContextInclude: []string{"ui/**"},
		},
		{
			Name: "infra", Label: "Infrastructure", Enabled: true,
			Description:    "Build, CI/CD, deployment, configuration.",
			PromptFragment: "You specialize in infrastructure: build pipelines, CI/CD, deployment, and configuration. Prefer reproducible, minimal, well-documented changes.",
			ContextInclude: []string{"deploy/**", "config/**", ".github/**"},
		},
	}
}

// ── SQL / scan helpers ────────────────────────────────────────────────────────

const skillDefSelectSQL = `SELECT id, name, label, description, prompt_fragment,
    context_include, context_exclude, allowed_tools, enabled, created_at, updated_at
    FROM skill_definitions`

func scanSkillDef(row *sql.Row) (*SkillDefinition, error) {
	var s SkillDefinition
	var ciJSON, ceJSON, atJSON, createdAt, updatedAt string
	var enabled int
	err := row.Scan(
		&s.ID, &s.Name, &s.Label, &s.Description, &s.PromptFragment,
		&ciJSON, &ceJSON, &atJSON, &enabled, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.Enabled = enabled != 0
	s.ContextInclude = unmarshalJSONStringSlice(ciJSON)
	s.ContextExclude = unmarshalJSONStringSlice(ceJSON)
	s.AllowedTools = unmarshalJSONStringSlice(atJSON)
	s.CreatedAt = parseTime(createdAt)
	s.UpdatedAt = parseTime(updatedAt)
	return &s, nil
}

func scanSkillDefs(rows *sql.Rows) ([]*SkillDefinition, error) {
	var defs []*SkillDefinition
	for rows.Next() {
		var s SkillDefinition
		var ciJSON, ceJSON, atJSON, createdAt, updatedAt string
		var enabled int
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Label, &s.Description, &s.PromptFragment,
			&ciJSON, &ceJSON, &atJSON, &enabled, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		s.Enabled = enabled != 0
		s.ContextInclude = unmarshalJSONStringSlice(ciJSON)
		s.ContextExclude = unmarshalJSONStringSlice(ceJSON)
		s.AllowedTools = unmarshalJSONStringSlice(atJSON)
		s.CreatedAt = parseTime(createdAt)
		s.UpdatedAt = parseTime(updatedAt)
		defs = append(defs, &s)
	}
	return defs, rows.Err()
}
