package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CreateContextEntry saves a context entry (with optional embedding).
func (d *Database) CreateContextEntry(ctx context.Context, e *ContextEntry) error {
	if e.ID == "" {
		e.ID = newID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}

	var embeddingJSON interface{}
	if len(e.Embedding) > 0 {
		b, _ := json.Marshal(e.Embedding)
		embeddingJSON = string(b)
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO context_store (id, project_id, task_id, type, content, embedding, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, nullableStr(e.ProjectID), nullableStr(e.TaskID),
		e.Type, e.Content, embeddingJSON, marshalJSON(e.Metadata), e.CreatedAt,
	)
	return err
}

// UpdateContextEmbedding stores the embedding for an existing context entry.
func (d *Database) UpdateContextEmbedding(ctx context.Context, entryID string, embedding []float32) error {
	b, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	_, err = d.db.ExecContext(ctx,
		`UPDATE context_store SET embedding=? WHERE id=?`, string(b), entryID,
	)
	return err
}

// GetContextEntriesWithEmbeddings returns all context entries for a project that have embeddings.
// Used for semantic similarity search.
func (d *Database) GetContextEntriesWithEmbeddings(ctx context.Context, projectID string) ([]*ContextEntry, error) {
	query := `SELECT id, COALESCE(project_id,''), COALESCE(task_id,''),
	    type, content, COALESCE(embedding,''), metadata, created_at
	    FROM context_store WHERE embedding IS NOT NULL AND embedding != ''`
	var args []interface{}
	if projectID != "" {
		query += " AND project_id=?"
		args = append(args, projectID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*ContextEntry
	for rows.Next() {
		var e ContextEntry
		var embeddingJSON, metaJSON, createdAt string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.TaskID,
			&e.Type, &e.Content, &embeddingJSON, &metaJSON, &createdAt); err != nil {
			return nil, err
		}
		e.Metadata = unmarshalJSONMap(metaJSON)
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		if embeddingJSON != "" && embeddingJSON != "null" {
			_ = json.Unmarshal([]byte(embeddingJSON), &e.Embedding)
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// PruneProjectContext removes older context entries beyond maxItems for a project.
func (d *Database) PruneProjectContext(ctx context.Context, projectID string, maxItems int) error {
	_, err := d.db.ExecContext(ctx,
		`DELETE FROM context_store WHERE project_id=? AND id NOT IN (
		   SELECT id FROM context_store WHERE project_id=?
		   ORDER BY created_at DESC LIMIT ?
		 )`, projectID, projectID, maxItems,
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
