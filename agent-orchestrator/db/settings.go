package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Setting represents one platform configuration entry.
type Setting struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"` // JSON-encoded value
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GetSetting retrieves a single setting by key.
// Returns an error if the key does not exist.
func (d *Database) GetSetting(ctx context.Context, key string) (*Setting, error) {
	var s Setting
	var updatedAt string
	err := d.db.QueryRowContext(ctx,
		`SELECT key, value, description, updated_at FROM platform_settings WHERE key=?`, key,
	).Scan(&s.Key, &s.Value, &s.Description, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("setting %q not found", key)
	}
	if err != nil {
		return nil, err
	}
	s.UpdatedAt = parseTime(updatedAt)
	return &s, nil
}

// SetSetting creates or updates a setting.
func (d *Database) SetSetting(ctx context.Context, key, value, description string) error {
	now := time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO platform_settings (key, value, description, updated_at)
         VALUES (?, ?, ?, ?)
         ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, description, now,
	)
	return err
}

// SeedSetting inserts a setting only if the key does not already exist.
// DB values set via the UI are never overwritten by config seeding.
func (d *Database) SeedSetting(ctx context.Context, key, value, description string) error {
	now := time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO platform_settings (key, value, description, updated_at)
         VALUES (?, ?, ?, ?)`,
		key, value, description, now,
	)
	return err
}

// ListSettings returns all settings ordered by key.
func (d *Database) ListSettings(ctx context.Context) ([]*Setting, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT key, value, description, updated_at FROM platform_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []*Setting
	for rows.Next() {
		var s Setting
		var updatedAt string
		if err := rows.Scan(&s.Key, &s.Value, &s.Description, &updatedAt); err != nil {
			return nil, err
		}
		s.UpdatedAt = parseTime(updatedAt)
		settings = append(settings, &s)
	}
	return settings, rows.Err()
}

// SeedDefaultRetentionSettings inserts the default log retention settings
// if they don't already exist. Called at server startup.
func (d *Database) SeedDefaultRetentionSettings(ctx context.Context) error {
	defaults := []struct{ key, value, description string }{
		{"log.retention.agent.default_days", "14", "Default retention for agent log events (days)"},
		{"log.retention.task.default_days", "30", "Default retention for task log events (days)"},
		{"log.retention.system.default_days", "7", "Default retention for system/API/MCP logs (days)"},
	}
	for _, d2 := range defaults {
		if err := d.SeedSetting(ctx, d2.key, d2.value, d2.description); err != nil {
			return fmt.Errorf("seed setting %q: %w", d2.key, err)
		}
	}
	return nil
}

// SeedRetentionFromConfig seeds per-type retention overrides from the config
// into platform_settings. Called at startup; existing DB values are preserved.
func (d *Database) SeedRetentionFromConfig(ctx context.Context, agentDefault, taskDefault, systemDefault int, overrides map[string]int) error {
	type kv struct{ key, value, desc string }
	seeds := []kv{
		{"log.retention.agent.default_days", fmt.Sprintf("%d", agentDefault), "Default retention for agent log events (days)"},
		{"log.retention.task.default_days", fmt.Sprintf("%d", taskDefault), "Default retention for task log events (days)"},
		{"log.retention.system.default_days", fmt.Sprintf("%d", systemDefault), "Default retention for system/API/MCP logs (days)"},
	}
	for k, v := range overrides {
		key := "log.retention.agent." + k + "_days"
		seeds = append(seeds, kv{key, fmt.Sprintf("%d", v), "Per-type retention override (days)"})
	}
	for _, s := range seeds {
		if err := d.SeedSetting(ctx, s.key, s.value, s.desc); err != nil {
			return fmt.Errorf("seed %q: %w", s.key, err)
		}
	}
	return nil
}
