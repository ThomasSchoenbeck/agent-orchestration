package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const taskTypeSelectSQL = `SELECT id, key, label, branch_template, is_default, sort_order, created_at, updated_at FROM task_types`

func scanTaskType(row interface{ Scan(...interface{}) error }) (*TaskType, error) {
	var tt TaskType
	var isDefault int
	var createdAt, updatedAt string
	if err := row.Scan(&tt.ID, &tt.Key, &tt.Label, &tt.BranchTemplate,
		&isDefault, &tt.SortOrder, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	tt.IsDefault = isDefault != 0
	tt.CreatedAt = parseTime(createdAt)
	tt.UpdatedAt = parseTime(updatedAt)
	return &tt, nil
}

// CreateTaskType inserts a new task type. When IsDefault is set it becomes the
// sole default (all others are cleared) within the same transaction.
func (d *Database) CreateTaskType(ctx context.Context, tt *TaskType) error {
	if tt.ID == "" {
		tt.ID = newID()
	}
	now := time.Now().UTC()
	tt.CreatedAt = now
	tt.UpdatedAt = now

	return d.withImmediateTx(ctx, func(tx *sql.Tx) error {
		if tt.IsDefault {
			if _, err := tx.ExecContext(ctx, "UPDATE task_types SET is_default=0"); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO task_types (id, key, label, branch_template, is_default, sort_order, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			tt.ID, tt.Key, tt.Label, tt.BranchTemplate, boolToInt(tt.IsDefault), tt.SortOrder,
			tt.CreatedAt, tt.UpdatedAt,
		)
		return err
	})
}

// GetTaskType retrieves a task type by id.
func (d *Database) GetTaskType(ctx context.Context, id string) (*TaskType, error) {
	row := d.db.QueryRowContext(ctx, taskTypeSelectSQL+` WHERE id=?`, id)
	tt, err := scanTaskType(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task type %q not found", id)
	}
	return tt, err
}

// GetTaskTypeByKey retrieves a task type by its unique key.
func (d *Database) GetTaskTypeByKey(ctx context.Context, key string) (*TaskType, error) {
	row := d.db.QueryRowContext(ctx, taskTypeSelectSQL+` WHERE key=?`, key)
	tt, err := scanTaskType(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task type with key %q not found", key)
	}
	return tt, err
}

// GetDefaultTaskType returns the default task type, or sql.ErrNoRows wrapped when
// none is marked default.
func (d *Database) GetDefaultTaskType(ctx context.Context) (*TaskType, error) {
	row := d.db.QueryRowContext(ctx, taskTypeSelectSQL+` WHERE is_default=1 ORDER BY sort_order, key LIMIT 1`)
	tt, err := scanTaskType(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no default task type")
	}
	return tt, err
}

// ListTaskTypes returns all task types ordered by sort_order then key.
func (d *Database) ListTaskTypes(ctx context.Context) ([]*TaskType, error) {
	rows, err := d.db.QueryContext(ctx, taskTypeSelectSQL+` ORDER BY sort_order, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TaskType
	for rows.Next() {
		tt, serr := scanTaskType(rows)
		if serr != nil {
			return nil, serr
		}
		out = append(out, tt)
	}
	return out, rows.Err()
}

// UpdateTaskType updates a task type's mutable fields. When IsDefault is set it
// becomes the sole default.
func (d *Database) UpdateTaskType(ctx context.Context, tt *TaskType) error {
	tt.UpdatedAt = time.Now().UTC()
	return d.withImmediateTx(ctx, func(tx *sql.Tx) error {
		if tt.IsDefault {
			if _, err := tx.ExecContext(ctx, "UPDATE task_types SET is_default=0 WHERE id<>?", tt.ID); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE task_types SET key=?, label=?, branch_template=?, is_default=?, sort_order=?, updated_at=?
			 WHERE id=?`,
			tt.Key, tt.Label, tt.BranchTemplate, boolToInt(tt.IsDefault), tt.SortOrder, tt.UpdatedAt, tt.ID,
		)
		return err
	})
}

// DeleteTaskType removes a task type by id.
func (d *Database) DeleteTaskType(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM task_types WHERE id=?", id)
	return err
}

// CountTasksUsingType returns how many tasks reference the given task type id.
func (d *Database) CountTasksUsingType(ctx context.Context, id string) (int, error) {
	var n int
	err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE task_type_id=?", id).Scan(&n)
	return n, err
}

// CountTaskTypes returns the number of task types defined.
func (d *Database) CountTaskTypes(ctx context.Context) (int, error) {
	var n int
	err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM task_types").Scan(&n)
	return n, err
}

// SeedTaskTypes inserts the given task types that don't already exist (matched by
// key). Returns the number actually inserted. Idempotent.
func (d *Database) SeedTaskTypes(ctx context.Context, types []*TaskType) (int, error) {
	seeded := 0
	for _, tt := range types {
		var count int
		if err := d.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM task_types WHERE key=?", tt.Key,
		).Scan(&count); err != nil {
			return seeded, fmt.Errorf("checking task type %q: %w", tt.Key, err)
		}
		if count > 0 {
			continue
		}
		if err := d.CreateTaskType(ctx, tt); err != nil {
			return seeded, fmt.Errorf("seeding task type %q: %w", tt.Key, err)
		}
		seeded++
	}
	return seeded, nil
}

// DefaultTaskTypes returns the built-in task types seeded on a fresh DB when no
// config-defined task types exist. "normal" is the default.
func DefaultTaskTypes() []*TaskType {
	return []*TaskType{
		{Key: "normal", Label: "Normal", BranchTemplate: "feature/{slug}", IsDefault: true, SortOrder: 0},
		{Key: "bug", Label: "Bug", BranchTemplate: "bug/{slug}", SortOrder: 1},
		{Key: "hotfix", Label: "Hotfix", BranchTemplate: "hotfix/{slug}", SortOrder: 2},
		{Key: "release", Label: "Release", BranchTemplate: "release/{slug}", SortOrder: 3},
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
