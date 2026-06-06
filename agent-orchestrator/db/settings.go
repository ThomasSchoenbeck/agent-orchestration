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

// SettingKeyResyncPrompt is the platform-settings key holding the task
// description used for project scope re-sync.
const SettingKeyResyncPrompt = "orchestrator.resync_prompt"

// DefaultResyncPrompt is the built-in re-sync task description used when neither
// the config file nor the DB provides one.
const DefaultResyncPrompt = "Read the project description, then reconcile the project scope against it:\n" +
	"1. Use sync_scope (or bootstrap_project if the project has no requirements/features yet) " +
	"to create or update the requirements and features that the description implies.\n" +
	"2. Create work packages (create_work_package / plan_project) for the work needed to " +
	"complete the project, assigning each an appropriate role so other agents can pick them up."

// SeedResyncPrompt seeds the orchestrator re-sync prompt. A non-empty
// configPrompt (from the config file) takes precedence over the built-in
// default on first run. Existing DB values are preserved (DB wins after first
// seed), matching the retention-settings behaviour.
func (d *Database) SeedResyncPrompt(ctx context.Context, configPrompt string) error {
	value := configPrompt
	if value == "" {
		value = DefaultResyncPrompt
	}
	return d.SeedSetting(ctx, SettingKeyResyncPrompt, value,
		"Task description handed to the orchestrator on project scope re-sync")
}

// SeedDefaultPlatformSettings inserts the default platform settings
// if they don't already exist. Called at server startup.
func (d *Database) SeedDefaultPlatformSettings(ctx context.Context) error {
	defaults := []struct{ key, value, description string }{
		{"platform.debug_mode", "false", "Emit verbose debug events (agent_heartbeat, agent_poll_query, agent_poll_no_task)"},
		{"platform.charts.autorefresh_ms", "5000", "Chart/log auto-refresh interval in milliseconds"},
	}
	for _, s := range defaults {
		if err := d.SeedSetting(ctx, s.key, s.value, s.description); err != nil {
			return fmt.Errorf("seed platform setting %q: %w", s.key, err)
		}
	}
	return nil
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
