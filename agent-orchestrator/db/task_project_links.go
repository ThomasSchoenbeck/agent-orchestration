package db

import (
	"context"
	"fmt"
	"time"
)

// ListTaskLinks returns all requirement and feature links for a task.
func (d *Database) ListTaskLinks(ctx context.Context, taskID string) ([]*TaskProjectLink, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, task_id, kind, target_id, created_at FROM task_project_links WHERE task_id=? ORDER BY created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []*TaskProjectLink
	for rows.Next() {
		var l TaskProjectLink
		var createdAt string
		if err := rows.Scan(&l.ID, &l.TaskID, &l.Kind, &l.TargetID, &createdAt); err != nil {
			return nil, err
		}
		l.CreatedAt = parseTime(createdAt)
		links = append(links, &l)
	}
	return links, rows.Err()
}

// AddTaskLink creates a link between a task and a requirement or feature.
// Returns an error (not 500) if the link already exists.
func (d *Database) AddTaskLink(ctx context.Context, taskID, kind, targetID string) (*TaskProjectLink, error) {
	if kind != "requirement" && kind != "feature" {
		return nil, fmt.Errorf("invalid kind %q: must be 'requirement' or 'feature'", kind)
	}
	// Verify task and target exist in the same project.
	task, err := d.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}
	var targetProjectID string
	switch kind {
	case "requirement":
		r, err := d.GetRequirement(ctx, targetID)
		if err != nil {
			return nil, fmt.Errorf("requirement not found: %w", err)
		}
		targetProjectID = r.ProjectID
	case "feature":
		f, err := d.GetFeature(ctx, targetID)
		if err != nil {
			return nil, fmt.Errorf("feature not found: %w", err)
		}
		targetProjectID = f.ProjectID
	}
	if task.ProjectID != targetProjectID {
		return nil, fmt.Errorf("task project %q does not match target project %q", task.ProjectID, targetProjectID)
	}

	l := &TaskProjectLink{
		ID:        newID(),
		TaskID:    taskID,
		Kind:      kind,
		TargetID:  targetID,
		CreatedAt: time.Now().UTC(),
	}
	_, err = d.db.ExecContext(ctx,
		`INSERT INTO task_project_links (id, task_id, kind, target_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		l.ID, l.TaskID, l.Kind, l.TargetID, l.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	d.logTaskEvent(ctx, taskID, "", "", EventTaskLinkAdded, "", "",
		kind+" linked: "+targetID)
	return l, nil
}

// RemoveTaskLink deletes a specific link by task ID + kind + target ID.
func (d *Database) RemoveTaskLink(ctx context.Context, taskID, kind, targetID string) error {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM task_project_links WHERE task_id=? AND kind=? AND target_id=?`,
		taskID, kind, targetID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("link not found")
	}
	d.logTaskEvent(ctx, taskID, "", "", EventTaskLinkRemoved, "", "",
		kind+" unlinked: "+targetID)
	return nil
}
