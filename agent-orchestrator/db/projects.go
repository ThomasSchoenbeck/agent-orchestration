package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ListProjects returns all projects ordered by created_at desc.
func (d *Database) ListProjects(ctx context.Context) ([]*Project, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, name, description, repo_path, git_url, status, config, created_at, updated_at
		 FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjects(rows)
}

// GetProject retrieves a project by ID.
func (d *Database) GetProject(ctx context.Context, id string) (*Project, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT id, name, description, repo_path, git_url, status, config, created_at, updated_at
		 FROM projects WHERE id = ?`, id)
	p, err := scanProject(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project %q not found", id)
	}
	return p, err
}

// CreateProject inserts a new project and returns it with its generated ID.
func (d *Database) CreateProject(ctx context.Context, p *Project) error {
	if p.ID == "" {
		p.ID = newID()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = "planned"
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, description, repo_path, git_url, status, config, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, p.RepoPath, p.GitURL, p.Status,
		marshalJSON(p.Config), p.CreatedAt, p.UpdatedAt,
	)
	return err
}

// UpdateProject updates mutable fields of a project.
func (d *Database) UpdateProject(ctx context.Context, p *Project) error {
	p.UpdatedAt = time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE projects SET name=?, description=?, repo_path=?, git_url=?, status=?, config=?, updated_at=?
		 WHERE id=?`,
		p.Name, p.Description, p.RepoPath, p.GitURL, p.Status,
		marshalJSON(p.Config), p.UpdatedAt, p.ID,
	)
	return err
}

// DeleteProject removes a project by ID.
func (d *Database) DeleteProject(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM projects WHERE id=?`, id)
	return err
}

// --- scan helpers ---

func scanProject(row *sql.Row) (*Project, error) {
	var p Project
	var configJSON string
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.RepoPath, &p.GitURL, &p.Status,
		&configJSON, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.Config = unmarshalJSONMap(configJSON)
	p.CreatedAt = parseTime(createdAt)
	p.UpdatedAt = parseTime(updatedAt)
	return &p, nil
}

func scanProjects(rows *sql.Rows) ([]*Project, error) {
	var projects []*Project
	for rows.Next() {
		var p Project
		var configJSON, createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.RepoPath, &p.GitURL, &p.Status,
			&configJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p.Config = unmarshalJSONMap(configJSON)
		p.CreatedAt = parseTime(createdAt)
		p.UpdatedAt = parseTime(updatedAt)
		projects = append(projects, &p)
	}
	return projects, rows.Err()
}
