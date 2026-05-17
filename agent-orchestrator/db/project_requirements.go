package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ListRequirements returns all requirements for a project ordered by position.
func (d *Database) ListRequirements(ctx context.Context, projectID string) ([]*ProjectRequirement, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, project_id, title, body, status, position, created_at, updated_at
		 FROM project_requirements WHERE project_id=? ORDER BY position ASC, created_at ASC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequirements(rows)
}

// GetRequirement returns a single requirement by ID.
func (d *Database) GetRequirement(ctx context.Context, id string) (*ProjectRequirement, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT id, project_id, title, body, status, position, created_at, updated_at
		 FROM project_requirements WHERE id=?`, id,
	)
	r, err := scanRequirement(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("requirement %q not found", id)
	}
	return r, err
}

// CreateRequirement inserts a new requirement.
func (d *Database) CreateRequirement(ctx context.Context, r *ProjectRequirement) error {
	if r.ID == "" {
		r.ID = newID()
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	if r.Status == "" {
		r.Status = "proposed"
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO project_requirements (id, project_id, title, body, status, position, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.Title, r.Body, r.Status, r.Position, r.CreatedAt, r.UpdatedAt,
	)
	return err
}

// UpdateRequirement updates mutable fields of a requirement.
func (d *Database) UpdateRequirement(ctx context.Context, r *ProjectRequirement) error {
	r.UpdatedAt = time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE project_requirements SET title=?, body=?, status=?, position=?, updated_at=? WHERE id=?`,
		r.Title, r.Body, r.Status, r.Position, r.UpdatedAt, r.ID,
	)
	return err
}

// DeleteRequirement removes a requirement and its task links.
func (d *Database) DeleteRequirement(ctx context.Context, id string) error {
	return d.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM task_project_links WHERE kind='requirement' AND target_id=?`, id,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM project_requirements WHERE id=?`, id)
		return err
	})
}

// CountLinkedTasksForRequirements returns a map of requirement_id → task count for all
// requirements in a project. Used by the UI to show "n linked tasks" badges.
func (d *Database) CountLinkedTasksForRequirements(ctx context.Context, projectID string) (map[string]int, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT tpl.target_id, COUNT(*) FROM task_project_links tpl
		 JOIN project_requirements r ON r.id = tpl.target_id
		 WHERE tpl.kind='requirement' AND r.project_id=?
		 GROUP BY tpl.target_id`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int{}
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		m[id] = count
	}
	return m, rows.Err()
}

// --- scanners ---

func scanRequirement(row *sql.Row) (*ProjectRequirement, error) {
	var r ProjectRequirement
	var createdAt, updatedAt string
	err := row.Scan(&r.ID, &r.ProjectID, &r.Title, &r.Body, &r.Status, &r.Position, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	r.CreatedAt = parseTime(createdAt)
	r.UpdatedAt = parseTime(updatedAt)
	return &r, nil
}

func scanRequirements(rows *sql.Rows) ([]*ProjectRequirement, error) {
	var list []*ProjectRequirement
	for rows.Next() {
		var r ProjectRequirement
		var createdAt, updatedAt string
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Title, &r.Body, &r.Status, &r.Position, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		r.CreatedAt = parseTime(createdAt)
		r.UpdatedAt = parseTime(updatedAt)
		list = append(list, &r)
	}
	return list, rows.Err()
}
