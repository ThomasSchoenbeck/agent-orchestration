package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CreateContextEntry saves a context entry.
func (d *Database) CreateContextEntry(ctx context.Context, e *ContextEntry) error {
	if e.ID == "" {
		e.ID = newID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO context_store (id, project_id, task_id, type, content, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, nullableStr(e.ProjectID), nullableStr(e.TaskID),
		e.Type, e.Content, marshalJSON(e.Metadata), e.CreatedAt,
	)
	return err
}

// QueryContext retrieves context entries for a project, optionally filtered.
func (d *Database) QueryContext(ctx context.Context, projectID, query string, limit int) ([]*ContextEntry, error) {
	sqlQuery := `SELECT id, COALESCE(project_id,''), COALESCE(task_id,''),
	    type, content, metadata, created_at
	    FROM context_store`
	var args []interface{}
	var where []string

	if projectID != "" {
		where = append(where, "project_id=?")
		args = append(args, projectID)
	}
	if query != "" {
		where = append(where, "content LIKE ?")
		args = append(args, "%"+query+"%")
	}
	if len(where) > 0 {
		sqlQuery += " WHERE " + strings.Join(where, " AND ")
	}
	sqlQuery += " ORDER BY created_at DESC"
	if limit > 0 {
		sqlQuery += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := d.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*ContextEntry
	for rows.Next() {
		var e ContextEntry
		var metaJSON, createdAt string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.TaskID,
			&e.Type, &e.Content, &metaJSON, &createdAt); err != nil {
			return nil, err
		}
		e.Metadata = unmarshalJSONMap(metaJSON)
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}
