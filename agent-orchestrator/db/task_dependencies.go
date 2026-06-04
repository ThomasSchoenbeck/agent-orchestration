package db

import (
	"context"
	"database/sql"
	"fmt"
)

// TaskDependency represents a single dependency edge: task_id depends on depends_on_id.
type TaskDependency struct {
	TaskID      string `json:"task_id"`
	DependsOnID string `json:"depends_on_id"`
	CreatedAt   string `json:"created_at"`

	// Populated by ListDependencies for display.
	DependsOnTitle  string `json:"depends_on_title,omitempty"`
	DependsOnStatus string `json:"depends_on_status,omitempty"`
}

// ListDependencies returns all dependencies of the given task,
// joined with the depended-on task's title (from payload) and status.
func (d *Database) ListDependencies(ctx context.Context, taskID string) ([]*TaskDependency, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT td.depends_on_id, td.created_at,
		       t.status,
		       COALESCE(json_extract(t.payload, '$.title'), t.id)
		FROM task_dependencies td
		JOIN tasks t ON t.id = td.depends_on_id
		WHERE td.task_id = ?
		ORDER BY td.created_at ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TaskDependency
	for rows.Next() {
		dep := &TaskDependency{TaskID: taskID}
		if err := rows.Scan(&dep.DependsOnID, &dep.CreatedAt, &dep.DependsOnStatus, &dep.DependsOnTitle); err != nil {
			return nil, err
		}
		out = append(out, dep)
	}
	if out == nil {
		out = []*TaskDependency{}
	}
	return out, rows.Err()
}

// AddDependency adds a dependency edge (task depends on dependsOnID).
// Returns an error if the dependency already exists, if either task doesn't exist,
// or if both IDs are identical.
func (d *Database) AddDependency(ctx context.Context, taskID, dependsOnID string) (*TaskDependency, error) {
	if taskID == dependsOnID {
		return nil, fmt.Errorf("a task cannot depend on itself")
	}
	// Verify both tasks exist.
	var n int
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE id IN (?, ?)`, taskID, dependsOnID,
	).Scan(&n); err != nil {
		return nil, err
	}
	if n != 2 {
		return nil, fmt.Errorf("one or both tasks not found")
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO task_dependencies(task_id, depends_on_id) VALUES (?, ?)`,
		taskID, dependsOnID,
	)
	if err != nil {
		return nil, err
	}
	dep := &TaskDependency{TaskID: taskID, DependsOnID: dependsOnID}
	_ = d.db.QueryRowContext(ctx,
		`SELECT td.created_at, t.status,
		        COALESCE(json_extract(t.payload, '$.title'), t.id)
		 FROM task_dependencies td JOIN tasks t ON t.id = td.depends_on_id
		 WHERE td.task_id=? AND td.depends_on_id=?`, taskID, dependsOnID,
	).Scan(&dep.CreatedAt, &dep.DependsOnStatus, &dep.DependsOnTitle)
	d.logTaskEvent(ctx, taskID, "", "", EventTaskDependencyAdded, "", "",
		"dependency added: "+dep.DependsOnTitle)
	return dep, nil
}

// RemoveDependency deletes a dependency edge.
func (d *Database) RemoveDependency(ctx context.Context, taskID, dependsOnID string) error {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM task_dependencies WHERE task_id=? AND depends_on_id=?`,
		taskID, dependsOnID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	d.logTaskEvent(ctx, taskID, "", "", EventTaskDependencyRemoved, "", "",
		"dependency removed: "+dependsOnID)
	return nil
}

// uncompletedDeps returns the IDs of dependencies that are not yet completed.
// Used by ClaimTask to decide whether to emit a warning.
func (d *Database) uncompletedDeps(ctx context.Context, taskID string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT td.depends_on_id
		FROM task_dependencies td
		JOIN tasks t ON t.id = td.depends_on_id
		WHERE td.task_id = ? AND t.status != 'completed'`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
