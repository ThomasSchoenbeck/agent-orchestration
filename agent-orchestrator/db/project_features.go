package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ListFeatures returns all features for a project ordered by position.
func (d *Database) ListFeatures(ctx context.Context, projectID string) ([]*ProjectFeature, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, project_id, title, body, status, position, created_at, updated_at
		 FROM project_features WHERE project_id=? ORDER BY position ASC, created_at ASC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeatures(rows)
}

// GetFeature returns a single feature by ID.
func (d *Database) GetFeature(ctx context.Context, id string) (*ProjectFeature, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT id, project_id, title, body, status, position, created_at, updated_at
		 FROM project_features WHERE id=?`, id,
	)
	f, err := scanFeature(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("feature %q not found", id)
	}
	return f, err
}

// CreateFeature inserts a new feature.
func (d *Database) CreateFeature(ctx context.Context, f *ProjectFeature) error {
	if f.ID == "" {
		f.ID = newID()
	}
	now := time.Now().UTC()
	f.CreatedAt = now
	f.UpdatedAt = now
	if f.Status == "" {
		f.Status = "planned"
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO project_features (id, project_id, title, body, status, position, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.ProjectID, f.Title, f.Body, f.Status, f.Position, f.CreatedAt, f.UpdatedAt,
	)
	return err
}

// UpdateFeature updates mutable fields of a feature.
func (d *Database) UpdateFeature(ctx context.Context, f *ProjectFeature) error {
	f.UpdatedAt = time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE project_features SET title=?, body=?, status=?, position=?, updated_at=? WHERE id=?`,
		f.Title, f.Body, f.Status, f.Position, f.UpdatedAt, f.ID,
	)
	return err
}

// DeleteFeature removes a feature and its task links.
func (d *Database) DeleteFeature(ctx context.Context, id string) error {
	return d.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM task_project_links WHERE kind='feature' AND target_id=?`, id,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM project_features WHERE id=?`, id)
		return err
	})
}

// CountLinkedTasksForFeatures returns a map of feature_id → task count for all
// features in a project.
func (d *Database) CountLinkedTasksForFeatures(ctx context.Context, projectID string) (map[string]int, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT tpl.target_id, COUNT(*) FROM task_project_links tpl
		 JOIN project_features f ON f.id = tpl.target_id
		 WHERE tpl.kind='feature' AND f.project_id=?
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

func scanFeature(row *sql.Row) (*ProjectFeature, error) {
	var f ProjectFeature
	var createdAt, updatedAt string
	err := row.Scan(&f.ID, &f.ProjectID, &f.Title, &f.Body, &f.Status, &f.Position, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	f.CreatedAt = parseTime(createdAt)
	f.UpdatedAt = parseTime(updatedAt)
	return &f, nil
}

func scanFeatures(rows *sql.Rows) ([]*ProjectFeature, error) {
	var list []*ProjectFeature
	for rows.Next() {
		var f ProjectFeature
		var createdAt, updatedAt string
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Title, &f.Body, &f.Status, &f.Position, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		f.CreatedAt = parseTime(createdAt)
		f.UpdatedAt = parseTime(updatedAt)
		list = append(list, &f)
	}
	return list, rows.Err()
}
