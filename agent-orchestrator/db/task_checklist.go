package db

import (
	"context"
	"database/sql"
	"time"
)

// ChecklistItem is one item in a task's progress checklist.
type ChecklistItem struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"task_id"`
	GroupLabel string    `json:"group_label"`
	Position   int       `json:"position"`
	Label      string    `json:"label"`
	Status     string    `json:"status"` // pending|in_progress|passed|failed|skipped
	UpdatedAt  time.Time `json:"updated_at"`
}

// ListChecklistItems returns all checklist items for a task, ordered by group then position.
func (d *Database) ListChecklistItems(ctx context.Context, taskID string) ([]*ChecklistItem, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, task_id, group_label, position, label, status, updated_at
		 FROM task_checklist_items
		 WHERE task_id = ?
		 ORDER BY group_label, position ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ChecklistItem
	for rows.Next() {
		item := &ChecklistItem{}
		if err := rows.Scan(&item.ID, &item.TaskID, &item.GroupLabel, &item.Position,
			&item.Label, &item.Status, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if out == nil {
		out = []*ChecklistItem{}
	}
	return out, rows.Err()
}

// CreateChecklistItem inserts a new checklist item.
func (d *Database) CreateChecklistItem(ctx context.Context, item *ChecklistItem) error {
	if item.ID == "" {
		item.ID = newID()
	}
	if item.Status == "" {
		item.Status = "pending"
	}
	item.UpdatedAt = time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO task_checklist_items(id, task_id, group_label, position, label, status, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.TaskID, item.GroupLabel, item.Position,
		item.Label, item.Status, item.UpdatedAt,
	)
	return err
}

// UpdateChecklistItem patches label, status, position, or group_label.
func (d *Database) UpdateChecklistItem(ctx context.Context, item *ChecklistItem) error {
	item.UpdatedAt = time.Now().UTC()
	res, err := d.db.ExecContext(ctx,
		`UPDATE task_checklist_items
		 SET group_label=?, position=?, label=?, status=?, updated_at=?
		 WHERE id=? AND task_id=?`,
		item.GroupLabel, item.Position, item.Label, item.Status, item.UpdatedAt,
		item.ID, item.TaskID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	d.logTaskEvent(ctx, item.TaskID, "", "", EventTaskChecklistChanged, "", "",
		"checklist item \""+item.Label+"\": "+item.Status)
	return nil
}

// DeleteChecklistItem removes a single item.
func (d *Database) DeleteChecklistItem(ctx context.Context, taskID, itemID string) error {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM task_checklist_items WHERE id=? AND task_id=?`, itemID, taskID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CloneChecklistIteration duplicates all items with the latest group_label into a new
// group with reset status ("pending"). Returns the new group label.
// If the task has no items, this is a no-op and returns an empty string.
func (d *Database) CloneChecklistIteration(ctx context.Context, taskID string) (string, error) {
	// Find the latest group label.
	var latestGroup string
	err := d.db.QueryRowContext(ctx,
		`SELECT group_label FROM task_checklist_items
		 WHERE task_id=?
		 ORDER BY group_label DESC LIMIT 1`, taskID,
	).Scan(&latestGroup)
	if err == sql.ErrNoRows {
		return "", nil // nothing to clone
	}
	if err != nil {
		return "", err
	}

	// Determine next iteration number.
	newGroup := nextIterationLabel(latestGroup)

	// Fetch items from the latest group. Materialize the full result set and
	// close the cursor BEFORE inserting: the insert below acquires a connection,
	// and holding an open rows cursor while doing so deadlocks a single-connection
	// pool (the cursor never releases its connection until closed).
	rows, err := d.db.QueryContext(ctx,
		`SELECT position, label FROM task_checklist_items
		 WHERE task_id=? AND group_label=? ORDER BY position ASC`, taskID, latestGroup)
	if err != nil {
		return "", err
	}
	type clonedItem struct {
		pos   int
		label string
	}
	var sources []clonedItem
	for rows.Next() {
		var it clonedItem
		if err := rows.Scan(&it.pos, &it.label); err != nil {
			rows.Close()
			return "", err
		}
		sources = append(sources, it)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	now := time.Now().UTC()
	for _, it := range sources {
		_, err := d.db.ExecContext(ctx,
			`INSERT INTO task_checklist_items(id, task_id, group_label, position, label, status, updated_at)
			 VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
			newID(), taskID, newGroup, it.pos, it.label, now,
		)
		if err != nil {
			return "", err
		}
	}
	return newGroup, nil
}

// nextIterationLabel derives the next iteration label from the current one.
// "attempt 1" → "attempt 2", "" → "attempt 1", "attempt 9" → "attempt 10", etc.
func nextIterationLabel(current string) string {
	if current == "" {
		return "attempt 1"
	}
	// Try to parse trailing integer.
	i := len(current) - 1
	for i >= 0 && current[i] >= '0' && current[i] <= '9' {
		i--
	}
	if i < len(current)-1 {
		prefix := current[:i+1]
		var n int
		for _, ch := range current[i+1:] {
			n = n*10 + int(ch-'0')
		}
		return prefix + itoa(n+1)
	}
	return current + " 2"
}

// itoa converts an int to its decimal string representation.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
