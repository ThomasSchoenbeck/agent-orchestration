package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreatePR inserts a new pull request. Defaults Base to "main" and Status to
// "open" when unset.
func (d *Database) CreatePR(ctx context.Context, pr *PullRequest) error {
	if pr.ID == "" {
		pr.ID = newID()
	}
	if pr.Base == "" {
		pr.Base = "main"
	}
	if pr.Status == "" {
		pr.Status = "open"
	}
	now := time.Now().UTC()
	pr.CreatedAt = now
	pr.UpdatedAt = now
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO pull_requests
		 (id, task_id, project_id, branch, base, title, body, status,
		  author_id, author_name, decider_id, decision_body, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pr.ID, pr.TaskID, pr.ProjectID, pr.Branch, pr.Base, pr.Title, pr.Body, pr.Status,
		pr.AuthorID, pr.AuthorName, pr.DeciderID, pr.DecisionBody, pr.CreatedAt, pr.UpdatedAt,
	)
	return err
}

// GetPR returns a single pull request by ID.
func (d *Database) GetPR(ctx context.Context, id string) (*PullRequest, error) {
	row := d.db.QueryRowContext(ctx, prSelectSQL+` WHERE id=?`, id)
	pr, err := scanPR(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("pull request %q not found", id)
	}
	return pr, err
}

// ListPRsForTask returns all pull requests for a task, newest first.
func (d *Database) ListPRsForTask(ctx context.Context, taskID string) ([]*PullRequest, error) {
	rows, err := d.db.QueryContext(ctx, prSelectSQL+` WHERE task_id=? ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*PullRequest
	for rows.Next() {
		pr, err := scanPRRows(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, pr)
	}
	return list, rows.Err()
}

// UpdatePRStatus sets just the status of a pull request.
func (d *Database) UpdatePRStatus(ctx context.Context, id, status string) error {
	res, err := d.db.ExecContext(ctx,
		`UPDATE pull_requests SET status=?, updated_at=? WHERE id=?`,
		status, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("pull request %q not found", id)
	}
	return nil
}

// SetPRDecision records a decision (approval or rejection) on a pull request:
// the final status, who decided, and the decision notes.
func (d *Database) SetPRDecision(ctx context.Context, id, status, deciderID, decisionBody string) error {
	res, err := d.db.ExecContext(ctx,
		`UPDATE pull_requests SET status=?, decider_id=?, decision_body=?, updated_at=? WHERE id=?`,
		status, deciderID, decisionBody, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("pull request %q not found", id)
	}
	return nil
}

// --- SQL fragment and scanners ---

const prSelectSQL = `SELECT id, task_id, project_id, branch, base, title, body, status,
    author_id, author_name, decider_id, decision_body, created_at, updated_at
    FROM pull_requests`

func scanPR(row *sql.Row) (*PullRequest, error) {
	var pr PullRequest
	var createdAt, updatedAt string
	err := row.Scan(
		&pr.ID, &pr.TaskID, &pr.ProjectID, &pr.Branch, &pr.Base, &pr.Title, &pr.Body, &pr.Status,
		&pr.AuthorID, &pr.AuthorName, &pr.DeciderID, &pr.DecisionBody, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	pr.CreatedAt = parseTime(createdAt)
	pr.UpdatedAt = parseTime(updatedAt)
	return &pr, nil
}

func scanPRRows(rows *sql.Rows) (*PullRequest, error) {
	var pr PullRequest
	var createdAt, updatedAt string
	err := rows.Scan(
		&pr.ID, &pr.TaskID, &pr.ProjectID, &pr.Branch, &pr.Base, &pr.Title, &pr.Body, &pr.Status,
		&pr.AuthorID, &pr.AuthorName, &pr.DeciderID, &pr.DecisionBody, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	pr.CreatedAt = parseTime(createdAt)
	pr.UpdatedAt = parseTime(updatedAt)
	return &pr, nil
}
