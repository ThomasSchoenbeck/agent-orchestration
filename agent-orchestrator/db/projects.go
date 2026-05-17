package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const projectSelectSQL = `
	SELECT id, name, description, repo_path, git_url, slug, remote_url, remote_credentials_ref,
	       coding_rules, status, config, server_repo_initialised_at, created_at, updated_at
	FROM projects`

// ListProjects returns all projects ordered by created_at desc.
func (d *Database) ListProjects(ctx context.Context) ([]*Project, error) {
	rows, err := d.db.QueryContext(ctx, projectSelectSQL+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjects(rows)
}

// GetProject retrieves a project by ID.
func (d *Database) GetProject(ctx context.Context, id string) (*Project, error) {
	row := d.db.QueryRowContext(ctx, projectSelectSQL+` WHERE id = ?`, id)
	p, err := scanProject(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project %q not found", id)
	}
	return p, err
}

// GetProjectBySlug retrieves a project by its URL slug.
func (d *Database) GetProjectBySlug(ctx context.Context, slug string) (*Project, error) {
	row := d.db.QueryRowContext(ctx, projectSelectSQL+` WHERE slug = ?`, slug)
	p, err := scanProject(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project with slug %q not found", slug)
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

	var initialisedAt interface{}
	if p.ServerRepoInitialisedAt != nil {
		initialisedAt = p.ServerRepoInitialisedAt.UTC().Format("2006-01-02 15:04:05")
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO projects
		   (id, name, description, repo_path, git_url, slug, remote_url, remote_credentials_ref,
		    coding_rules, status, config, server_repo_initialised_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, p.RepoPath, p.GitURL,
		p.Slug, p.RemoteURL, p.RemoteCredentialsRef, p.CodingRules,
		p.Status, marshalJSON(p.Config), initialisedAt,
		p.CreatedAt, p.UpdatedAt,
	)
	return err
}

// UpdateProject updates mutable fields of a project.
func (d *Database) UpdateProject(ctx context.Context, p *Project) error {
	p.UpdatedAt = time.Now().UTC()

	var initialisedAt interface{}
	if p.ServerRepoInitialisedAt != nil {
		initialisedAt = p.ServerRepoInitialisedAt.UTC().Format("2006-01-02 15:04:05")
	}

	_, err := d.db.ExecContext(ctx,
		`UPDATE projects
		 SET name=?, description=?, repo_path=?, git_url=?, slug=?, remote_url=?,
		     remote_credentials_ref=?, coding_rules=?, status=?, config=?,
		     server_repo_initialised_at=?, updated_at=?
		 WHERE id=?`,
		p.Name, p.Description, p.RepoPath, p.GitURL,
		p.Slug, p.RemoteURL, p.RemoteCredentialsRef, p.CodingRules,
		p.Status, marshalJSON(p.Config), initialisedAt,
		p.UpdatedAt, p.ID,
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
	var initialisedAt sql.NullString
	err := row.Scan(
		&p.ID, &p.Name, &p.Description, &p.RepoPath, &p.GitURL,
		&p.Slug, &p.RemoteURL, &p.RemoteCredentialsRef, &p.CodingRules,
		&p.Status, &configJSON, &initialisedAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Config = unmarshalJSONMap(configJSON)
	p.CreatedAt = parseTime(createdAt)
	p.UpdatedAt = parseTime(updatedAt)
	if initialisedAt.Valid && initialisedAt.String != "" {
		t := parseTime(initialisedAt.String)
		p.ServerRepoInitialisedAt = &t
	}
	return &p, nil
}

func scanProjects(rows *sql.Rows) ([]*Project, error) {
	var projects []*Project
	for rows.Next() {
		var p Project
		var configJSON, createdAt, updatedAt string
		var initialisedAt sql.NullString
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.RepoPath, &p.GitURL,
			&p.Slug, &p.RemoteURL, &p.RemoteCredentialsRef, &p.CodingRules,
			&p.Status, &configJSON, &initialisedAt, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		p.Config = unmarshalJSONMap(configJSON)
		p.CreatedAt = parseTime(createdAt)
		p.UpdatedAt = parseTime(updatedAt)
		if initialisedAt.Valid && initialisedAt.String != "" {
			t := parseTime(initialisedAt.String)
			p.ServerRepoInitialisedAt = &t
		}
		projects = append(projects, &p)
	}
	return projects, rows.Err()
}
