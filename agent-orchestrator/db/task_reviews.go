package db

import (
	"context"
	"database/sql"
	"time"
)

// TaskReview is one review posted on a task (by a reviewer agent or the merge
// supervisor in case of a conflict).
type TaskReview struct {
	ID            string    `json:"id"`
	TaskID        string    `json:"task_id"`
	AuthorType    string    `json:"author_type"`   // agent | system
	AuthorRole    string    `json:"author_role"`   // reviewer | merge_supervisor
	AuthorID      string    `json:"author_id,omitempty"`
	Status        string    `json:"status"`        // approved | changes_requested | revision_requested
	Body          string    `json:"body"`          // markdown review body
	BranchHeadSHA string    `json:"branch_head_sha,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

const reviewSelectSQL = `
	SELECT id, task_id, author_type, author_role, COALESCE(author_id,''),
	       status, body, COALESCE(branch_head_sha,''), created_at
	FROM task_reviews`

// CreateTaskReview inserts a review.
func (d *Database) CreateTaskReview(ctx context.Context, r *TaskReview) error {
	if r.ID == "" {
		r.ID = newID()
	}
	r.CreatedAt = time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO task_reviews
		   (id, task_id, author_type, author_role, author_id, status, body, branch_head_sha, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.TaskID, r.AuthorType, r.AuthorRole, r.AuthorID,
		r.Status, r.Body, r.BranchHeadSHA, r.CreatedAt,
	)
	return err
}

// GetTaskReview retrieves a review by ID.
func (d *Database) GetTaskReview(ctx context.Context, id string) (*TaskReview, error) {
	row := d.db.QueryRowContext(ctx, reviewSelectSQL+` WHERE id=?`, id)
	return scanTaskReview(row)
}

// ListTaskReviews returns all reviews for a task, oldest first.
func (d *Database) ListTaskReviews(ctx context.Context, taskID string) ([]*TaskReview, error) {
	rows, err := d.db.QueryContext(ctx, reviewSelectSQL+` WHERE task_id=? ORDER BY created_at ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTaskReviews(rows)
}

// ListTaskComments is a convenience wrapper that returns all comments for a task.
func (d *Database) ListTaskComments(ctx context.Context, taskID string) ([]*TaskComment, error) {
	return d.ListComments(ctx, taskID, "")
}

// --- scan helpers ---

func scanTaskReview(row *sql.Row) (*TaskReview, error) {
	var r TaskReview
	var createdAt string
	err := row.Scan(
		&r.ID, &r.TaskID, &r.AuthorType, &r.AuthorRole, &r.AuthorID,
		&r.Status, &r.Body, &r.BranchHeadSHA, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	r.CreatedAt = parseTime(createdAt)
	return &r, nil
}

func scanTaskReviews(rows *sql.Rows) ([]*TaskReview, error) {
	var out []*TaskReview
	for rows.Next() {
		var r TaskReview
		var createdAt string
		if err := rows.Scan(
			&r.ID, &r.TaskID, &r.AuthorType, &r.AuthorRole, &r.AuthorID,
			&r.Status, &r.Body, &r.BranchHeadSHA, &createdAt,
		); err != nil {
			return nil, err
		}
		r.CreatedAt = parseTime(createdAt)
		out = append(out, &r)
	}
	return out, rows.Err()
}
