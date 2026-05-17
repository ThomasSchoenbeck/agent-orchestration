package db

import (
	"context"
	"database/sql"
	"time"
)

// ChecklistTemplate is a named reusable list of item labels.
type ChecklistTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ItemsJSON string    `json:"items_json"` // JSON array of strings
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListChecklistTemplates returns all templates ordered by name.
func (d *Database) ListChecklistTemplates(ctx context.Context) ([]*ChecklistTemplate, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, name, items_json, created_at, updated_at
		 FROM checklist_templates ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ChecklistTemplate
	for rows.Next() {
		t := &ChecklistTemplate{}
		if err := rows.Scan(&t.ID, &t.Name, &t.ItemsJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if out == nil {
		out = []*ChecklistTemplate{}
	}
	return out, rows.Err()
}

// GetChecklistTemplate fetches a single template by ID.
func (d *Database) GetChecklistTemplate(ctx context.Context, id string) (*ChecklistTemplate, error) {
	t := &ChecklistTemplate{}
	err := d.db.QueryRowContext(ctx,
		`SELECT id, name, items_json, created_at, updated_at
		 FROM checklist_templates WHERE id=?`, id,
	).Scan(&t.ID, &t.Name, &t.ItemsJSON, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, err
	}
	return t, err
}

// CreateChecklistTemplate inserts a new template.
func (d *Database) CreateChecklistTemplate(ctx context.Context, t *ChecklistTemplate) error {
	if t.ID == "" {
		t.ID = newID()
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.ItemsJSON == "" {
		t.ItemsJSON = "[]"
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO checklist_templates(id, name, items_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.ItemsJSON, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

// UpdateChecklistTemplate replaces the name and items for an existing template.
func (d *Database) UpdateChecklistTemplate(ctx context.Context, t *ChecklistTemplate) error {
	t.UpdatedAt = time.Now().UTC()
	res, err := d.db.ExecContext(ctx,
		`UPDATE checklist_templates SET name=?, items_json=?, updated_at=? WHERE id=?`,
		t.Name, t.ItemsJSON, t.UpdatedAt, t.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteChecklistTemplate removes a template by ID.
func (d *Database) DeleteChecklistTemplate(ctx context.Context, id string) error {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM checklist_templates WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
