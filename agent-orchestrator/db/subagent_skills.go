package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateSubagentSkill inserts a new subagent skill.
func (d *Database) CreateSubagentSkill(ctx context.Context, s *SubagentSkill) error {
	if s.ID == "" {
		s.ID = newID()
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now
	if s.MaxRounds == 0 {
		s.MaxRounds = defaultSubagentMaxRounds
	}
	if s.ToolAllowlist == nil {
		s.ToolAllowlist = []string{}
	}
	if s.ContextInclude == nil {
		s.ContextInclude = []string{}
	}
	if s.ContextExclude == nil {
		s.ContextExclude = []string{}
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO subagent_skills
		 (id, name, label, description, prompt_template, tool_allowlist,
		  context_include, context_exclude, max_rounds, max_tokens, enabled,
		  created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Label, s.Description, s.PromptTemplate,
		marshalJSONArray(s.ToolAllowlist), marshalJSONArray(s.ContextInclude),
		marshalJSONArray(s.ContextExclude), s.MaxRounds, s.MaxTokens, s.Enabled,
		s.CreatedAt, s.UpdatedAt,
	)
	return err
}

// GetSubagentSkill retrieves a subagent skill by ID.
func (d *Database) GetSubagentSkill(ctx context.Context, id string) (*SubagentSkill, error) {
	row := d.db.QueryRowContext(ctx, subagentSkillSelectSQL+` WHERE id=?`, id)
	s, err := scanSubagentSkill(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("subagent skill %q not found", id)
	}
	return s, err
}

// GetSubagentSkillByName retrieves a subagent skill by name.
func (d *Database) GetSubagentSkillByName(ctx context.Context, name string) (*SubagentSkill, error) {
	row := d.db.QueryRowContext(ctx, subagentSkillSelectSQL+` WHERE name=?`, name)
	s, err := scanSubagentSkill(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("subagent skill %q not found", name)
	}
	return s, err
}

// ListSubagentSkills returns all subagent skills ordered by name.
func (d *Database) ListSubagentSkills(ctx context.Context) ([]*SubagentSkill, error) {
	rows, err := d.db.QueryContext(ctx, subagentSkillSelectSQL+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubagentSkills(rows)
}

// UpdateSubagentSkill replaces all mutable fields of a subagent skill.
func (d *Database) UpdateSubagentSkill(ctx context.Context, s *SubagentSkill) error {
	s.UpdatedAt = time.Now().UTC()
	if s.MaxRounds == 0 {
		s.MaxRounds = defaultSubagentMaxRounds
	}
	if s.ToolAllowlist == nil {
		s.ToolAllowlist = []string{}
	}
	if s.ContextInclude == nil {
		s.ContextInclude = []string{}
	}
	if s.ContextExclude == nil {
		s.ContextExclude = []string{}
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE subagent_skills SET
		 name=?, label=?, description=?, prompt_template=?, tool_allowlist=?,
		 context_include=?, context_exclude=?, max_rounds=?, max_tokens=?,
		 enabled=?, updated_at=?
		 WHERE id=?`,
		s.Name, s.Label, s.Description, s.PromptTemplate,
		marshalJSONArray(s.ToolAllowlist), marshalJSONArray(s.ContextInclude),
		marshalJSONArray(s.ContextExclude), s.MaxRounds, s.MaxTokens, s.Enabled,
		s.UpdatedAt, s.ID,
	)
	return err
}

// DeleteSubagentSkill removes a subagent skill.
func (d *Database) DeleteSubagentSkill(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM subagent_skills WHERE id=?`, id)
	return err
}

// SeedSubagentSkills inserts subagent skills whose name does not already exist.
func (d *Database) SeedSubagentSkills(ctx context.Context, skills []*SubagentSkill) (int, error) {
	seeded := 0
	for _, s := range skills {
		var count int
		if err := d.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM subagent_skills WHERE name=?`, s.Name,
		).Scan(&count); err != nil {
			return seeded, fmt.Errorf("checking subagent skill %q: %w", s.Name, err)
		}
		if count > 0 {
			continue
		}
		if err := d.CreateSubagentSkill(ctx, s); err != nil {
			return seeded, fmt.Errorf("seeding subagent skill %q: %w", s.Name, err)
		}
		seeded++
	}
	return seeded, nil
}

// defaultSubagentMaxRounds bounds a subagent loop when a skill omits max_rounds.
const defaultSubagentMaxRounds = 8

// DefaultSubagentSkills returns the built-in subagent skill set. v1 ships one
// read-only investigator. Its tool_allowlist is restricted to read/search tools
// over the main agent's worktree; the prompt_template ends by instructing the
// subagent to return a concise findings summary.
func DefaultSubagentSkills() []*SubagentSkill {
	return []*SubagentSkill{
		{
			Name:          "investigate_codebase",
			Label:         "Investigate Codebase",
			Enabled:       true,
			Description:   "Read-only exploration of a checked-out codebase; returns a findings summary.",
			ToolAllowlist: []string{"read_file", "list_files", "search_files"},
			MaxRounds:     8,
			PromptTemplate: "You are a focused investigation subagent operating on a checked-out codebase. " +
				"You have read-only tools (read_file, list_files, search_files). Use search_files to locate " +
				"files by name or by content (regex) before reading them. Investigate exactly what is asked, " +
				"reading only the files you need. Do not attempt to modify anything.\n\n" +
				"Investigation request:\n{{instructions}}\n\n" +
				"When you have gathered enough, STOP calling tools and reply with a single concise " +
				"findings summary: the key files, how they fit together, and direct answers to the " +
				"request. This summary is the only thing returned to the main agent, so make it " +
				"self-contained.",
		},
		{
			Name:          "code_subtask",
			Label:         "Code Subtask",
			Enabled:       true,
			Description:   "Read/write coding subtask over the shared worktree; returns a summary of changes made.",
			ToolAllowlist: []string{"read_file", "write_file", "apply_diff", "list_files", "run_tests", "search_files"},
			MaxRounds:     12,
			PromptTemplate: "You are a focused coding subagent operating on the main agent's checked-out " +
				"worktree. You may read and modify files (read_file, write_file, apply_diff, list_files) " +
				"and run tests (run_tests). Make only the changes the task describes; do not commit or " +
				"push — the main agent owns version control.\n\n" +
				"Coding task:\n{{instructions}}\n\n" +
				"When done, STOP calling tools and reply with a concise summary of exactly what you " +
				"changed (files touched and why) and the test outcome. This summary is the only thing " +
				"returned to the main agent, so make it self-contained.",
		},
	}
}

// ── SQL / scan helpers ────────────────────────────────────────────────────────

const subagentSkillSelectSQL = `SELECT id, name, label, description, prompt_template,
    tool_allowlist, context_include, context_exclude, max_rounds, max_tokens,
    enabled, created_at, updated_at
    FROM subagent_skills`

func scanSubagentSkill(row *sql.Row) (*SubagentSkill, error) {
	var s SubagentSkill
	var taJSON, ciJSON, ceJSON, createdAt, updatedAt string
	var enabled int
	err := row.Scan(
		&s.ID, &s.Name, &s.Label, &s.Description, &s.PromptTemplate,
		&taJSON, &ciJSON, &ceJSON, &s.MaxRounds, &s.MaxTokens, &enabled,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.Enabled = enabled != 0
	s.ToolAllowlist = unmarshalJSONStringSlice(taJSON)
	s.ContextInclude = unmarshalJSONStringSlice(ciJSON)
	s.ContextExclude = unmarshalJSONStringSlice(ceJSON)
	s.CreatedAt = parseTime(createdAt)
	s.UpdatedAt = parseTime(updatedAt)
	return &s, nil
}

func scanSubagentSkills(rows *sql.Rows) ([]*SubagentSkill, error) {
	var defs []*SubagentSkill
	for rows.Next() {
		var s SubagentSkill
		var taJSON, ciJSON, ceJSON, createdAt, updatedAt string
		var enabled int
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Label, &s.Description, &s.PromptTemplate,
			&taJSON, &ciJSON, &ceJSON, &s.MaxRounds, &s.MaxTokens, &enabled,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		s.Enabled = enabled != 0
		s.ToolAllowlist = unmarshalJSONStringSlice(taJSON)
		s.ContextInclude = unmarshalJSONStringSlice(ciJSON)
		s.ContextExclude = unmarshalJSONStringSlice(ceJSON)
		s.CreatedAt = parseTime(createdAt)
		s.UpdatedAt = parseTime(updatedAt)
		defs = append(defs, &s)
	}
	return defs, rows.Err()
}
