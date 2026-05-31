package db

import (
	"context"
	"database/sql"
	"time"
)

// TaskComment is a single comment on a task.
type TaskComment struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"task_id"`
	ReviewID   string    `json:"review_id,omitempty"`
	AuthorType string    `json:"author_type"` // user | agent
	AuthorRole string    `json:"author_role,omitempty"`
	AuthorID   string    `json:"author_id,omitempty"`
	AuthorName string    `json:"author_name,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListComments returns comments for a task, ordered oldest-first.
// If reviewID is non-empty, only returns comments for that review thread.
func (d *Database) ListComments(ctx context.Context, taskID, reviewID string) ([]*TaskComment, error) {
	query := `SELECT id, task_id, COALESCE(review_id,''), author_type,
	                 author_role, author_id, COALESCE(author_name,''), body, created_at
	          FROM task_comments WHERE task_id=?`
	args := []interface{}{taskID}
	if reviewID != "" {
		query += ` AND review_id=?`
		args = append(args, reviewID)
	} else {
		query += ` AND review_id IS NULL`
	}
	query += ` ORDER BY created_at ASC`

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TaskComment
	for rows.Next() {
		c := &TaskComment{}
		if err := rows.Scan(&c.ID, &c.TaskID, &c.ReviewID, &c.AuthorType,
			&c.AuthorRole, &c.AuthorID, &c.AuthorName, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if out == nil {
		out = []*TaskComment{}
	}
	return out, rows.Err()
}

// CreateComment inserts a new comment.
func (d *Database) CreateComment(ctx context.Context, c *TaskComment) error {
	if c.ID == "" {
		c.ID = newID()
	}
	c.CreatedAt = time.Now().UTC()
	// Resolve and store the author's display name at write time so the comments
	// panel can label the commenter without a join on read.
	if c.AuthorName == "" && c.AuthorType == "agent" {
		c.AuthorName = d.agentDisplayName(ctx, c.AuthorID)
	}
	var reviewID interface{}
	if c.ReviewID != "" {
		reviewID = c.ReviewID
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO task_comments(id, task_id, review_id, author_type, author_role, author_id, author_name, body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.TaskID, reviewID, c.AuthorType, c.AuthorRole, c.AuthorID, c.AuthorName, c.Body, c.CreatedAt,
	)
	if err != nil {
		return err
	}
	preview := c.Body
	if len(preview) > 60 {
		preview = preview[:60] + "…"
	}
	d.logTaskEvent(ctx, c.TaskID, "", c.AuthorID, EventTaskCommentAdded, "", "",
		c.AuthorType+" comment: "+preview)
	return nil
}

// DeleteComment removes a comment by ID and task_id.
func (d *Database) DeleteComment(ctx context.Context, taskID, commentID string) error {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM task_comments WHERE id=? AND task_id=?`, commentID, taskID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
