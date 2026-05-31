package db

import "fmt"

// migrate creates all tables and runs incremental column migrations.
func (d *Database) migrate() error {
	if err := d.migrateSchema(); err != nil {
		return err
	}
	if err := d.applyColumnMigrations(); err != nil {
		return err
	}
	return d.migrateTaskStates()
}

// migrateSchema creates all base tables and indexes (idempotent).
func (d *Database) migrateSchema() error {
	schema := `
-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    repo_path   TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'planned',
    config      TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Tasks / Work Packages table
CREATE TABLE IF NOT EXISTS tasks (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL,
    type              TEXT NOT NULL,
    role              TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'planned',
    priority          INTEGER NOT NULL DEFAULT 0,
    assigned_agent_id TEXT,
    payload           TEXT NOT NULL DEFAULT '{}',
    result            TEXT,
    attempts          INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at        DATETIME,
    completed_at      DATETIME,
    FOREIGN KEY (project_id)        REFERENCES projects(id),
    FOREIGN KEY (assigned_agent_id) REFERENCES agents(id)
);

-- Agents table
CREATE TABLE IF NOT EXISTS agents (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    roles           TEXT NOT NULL DEFAULT '[]',
    status          TEXT NOT NULL DEFAULT 'offline',
    current_task_id TEXT,
    capabilities    TEXT NOT NULL DEFAULT '{}',
    registered_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_heartbeat  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Providers / LLM Clients table
CREATE TABLE IF NOT EXISTS providers (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    type         TEXT NOT NULL,
    base_url     TEXT NOT NULL DEFAULT '',
    model_name   TEXT NOT NULL DEFAULT '',
    api_key      TEXT NOT NULL DEFAULT '',
    capabilities TEXT NOT NULL DEFAULT '[]',
    config       TEXT NOT NULL DEFAULT '{}',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Context Store table
CREATE TABLE IF NOT EXISTS context_store (
    id         TEXT PRIMARY KEY,
    project_id TEXT,
    task_id    TEXT,
    type       TEXT NOT NULL,
    content    TEXT NOT NULL,
    embedding  BLOB,
    metadata   TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id),
    FOREIGN KEY (task_id)    REFERENCES tasks(id)
);

-- Logs / Monitoring table
CREATE TABLE IF NOT EXISTS logs (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT,
    task_id    TEXT,
    project_id TEXT,
    level      TEXT NOT NULL DEFAULT 'info',
    message    TEXT NOT NULL,
    metadata   TEXT NOT NULL DEFAULT '{}',
    timestamp  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id)   REFERENCES agents(id),
    FOREIGN KEY (task_id)    REFERENCES tasks(id),
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

-- Execution Metrics table
CREATE TABLE IF NOT EXISTS metrics (
    id             TEXT PRIMARY KEY,
    task_id        TEXT,
    agent_id       TEXT,
    model          TEXT NOT NULL DEFAULT '',
    tokens_used    INTEGER NOT NULL DEFAULT 0,
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    cost           REAL NOT NULL DEFAULT 0,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    success        INTEGER NOT NULL DEFAULT 0,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id)  REFERENCES tasks(id),
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);

-- Phase 4: add input/output token columns to existing metrics tables (safe if columns already exist).
-- SQLite doesn't support IF NOT EXISTS on ALTER TABLE so we use a separate migration approach.
CREATE TABLE IF NOT EXISTS _schema_migrations (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Agent Role Definitions table
CREATE TABLE IF NOT EXISTS agent_role_definitions (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    label           TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    provider_id     TEXT,
    model_override  TEXT NOT NULL DEFAULT '',
    system_prompt   TEXT NOT NULL DEFAULT '',
    context_include TEXT NOT NULL DEFAULT '[]',
    context_exclude TEXT NOT NULL DEFAULT '[]',
    task_types      TEXT NOT NULL DEFAULT '[]',
    temperature     REAL NOT NULL DEFAULT 0.7,
    max_tokens      INTEGER NOT NULL DEFAULT 4096,
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Conversations table
CREATE TABLE IF NOT EXISTS conversations (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL DEFAULT '',
    provider_id TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Messages table
CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    tokens_used     INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id)
);

-- Project requirements
CREATE TABLE IF NOT EXISTS project_requirements (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    title      TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'proposed',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

-- Project features
CREATE TABLE IF NOT EXISTS project_features (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    title      TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'planned',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

-- Task comments
CREATE TABLE IF NOT EXISTS task_comments (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL,
    review_id   TEXT,
    author_type TEXT NOT NULL DEFAULT 'user',
    author_role TEXT NOT NULL DEFAULT '',
    author_id   TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
CREATE INDEX IF NOT EXISTS idx_task_comments_task ON task_comments(task_id, created_at ASC);

-- Task checklist items (grouped by iteration label)
CREATE TABLE IF NOT EXISTS task_checklist_items (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL,
    group_label TEXT NOT NULL DEFAULT '',
    position    INTEGER NOT NULL DEFAULT 0,
    label       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'pending',
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
CREATE INDEX IF NOT EXISTS idx_checklist_task ON task_checklist_items(task_id, group_label, position);

-- Checklist templates (reusable item sets)
CREATE TABLE IF NOT EXISTS checklist_templates (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    items_json TEXT NOT NULL DEFAULT '[]',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Task dependencies (soft warning only — no scheduler blocking)
CREATE TABLE IF NOT EXISTS task_dependencies (
    task_id        TEXT NOT NULL,
    depends_on_id  TEXT NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (task_id, depends_on_id),
    FOREIGN KEY (task_id)       REFERENCES tasks(id),
    FOREIGN KEY (depends_on_id) REFERENCES tasks(id)
);
CREATE INDEX IF NOT EXISTS idx_task_deps_task ON task_dependencies(task_id);
CREATE INDEX IF NOT EXISTS idx_task_deps_dep  ON task_dependencies(depends_on_id);

-- Task <-> requirement/feature links
CREATE TABLE IF NOT EXISTS task_project_links (
    id         TEXT PRIMARY KEY,
    task_id    TEXT NOT NULL,
    kind       TEXT NOT NULL,
    target_id  TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (task_id, kind, target_id),
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);

-- Task reviews (formal evaluation that drives a state transition)
CREATE TABLE IF NOT EXISTS task_reviews (
    id              TEXT PRIMARY KEY,
    task_id         TEXT NOT NULL,
    author_type     TEXT NOT NULL DEFAULT 'user',   -- user | agent
    author_role     TEXT NOT NULL DEFAULT '',        -- reviewer | '' for users
    author_id       TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL,                  -- APPROVED | REVISION_REQUESTED
    body            TEXT NOT NULL DEFAULT '',        -- markdown
    branch_head_sha TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
CREATE INDEX IF NOT EXISTS idx_task_reviews_task ON task_reviews(task_id, created_at DESC);

-- State migration audit log (one-shot migration from old vocabulary)
CREATE TABLE IF NOT EXISTS state_migration_log (
    id         INTEGER PRIMARY KEY,
    task_id    TEXT NOT NULL,
    old_state  TEXT NOT NULL,
    new_state  TEXT NOT NULL,
    migrated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Persisted state transition history
CREATE TABLE IF NOT EXISTS task_state_transitions (
    id             TEXT PRIMARY KEY,
    task_id        TEXT NOT NULL,
    from_state     TEXT NOT NULL,
    to_state       TEXT NOT NULL,
    actor_agent_id TEXT NOT NULL DEFAULT '',
    reason         TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
CREATE INDEX IF NOT EXISTS idx_state_transitions_task ON task_state_transitions(task_id, created_at ASC);

-- Merge queue: persisted file-path locks for crash recovery
CREATE TABLE IF NOT EXISTS merge_locks (
    id         TEXT PRIMARY KEY,
    task_id    TEXT NOT NULL UNIQUE,
    paths_json TEXT NOT NULL DEFAULT '[]',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_tasks_project_status ON tasks(project_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_agent_status   ON tasks(assigned_agent_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_role           ON tasks(role);
CREATE INDEX IF NOT EXISTS idx_tasks_priority       ON tasks(priority DESC, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_agents_status        ON agents(status);
CREATE INDEX IF NOT EXISTS idx_logs_agent           ON logs(agent_id);
CREATE INDEX IF NOT EXISTS idx_logs_task            ON logs(task_id);
CREATE INDEX IF NOT EXISTS idx_logs_project         ON logs(project_id);
CREATE INDEX IF NOT EXISTS idx_context_project      ON context_store(project_id);
CREATE INDEX IF NOT EXISTS idx_metrics_task         ON metrics(task_id);
CREATE INDEX IF NOT EXISTS idx_metrics_agent        ON metrics(agent_id);
CREATE INDEX IF NOT EXISTS idx_conversations_updated ON conversations(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id, created_at ASC);

-- Platform settings table (key/value store for UI-configurable settings)
CREATE TABLE IF NOT EXISTS platform_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_platform_settings_key ON platform_settings(key);
CREATE INDEX IF NOT EXISTS idx_requirements_project  ON project_requirements(project_id, position);
CREATE INDEX IF NOT EXISTS idx_features_project      ON project_features(project_id, position);
CREATE INDEX IF NOT EXISTS idx_task_links_task        ON task_project_links(task_id);
CREATE INDEX IF NOT EXISTS idx_task_links_target      ON task_project_links(kind, target_id);
`

	_, err := d.db.Exec(schema)
	return err
}

// applyColumnMigrations runs named ALTER TABLE migrations exactly once,
// recording each in _schema_migrations so they are never re-applied.
func (d *Database) applyColumnMigrations() error {
	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "add_git_url_to_projects",
			sql:  "ALTER TABLE projects ADD COLUMN git_url TEXT NOT NULL DEFAULT ''",
		},
		{
			name: "add_enabled_to_providers",
			sql:  "ALTER TABLE providers ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1",
		},
		{
			name: "add_input_tokens_to_messages",
			sql:  "ALTER TABLE messages ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0",
		},
		{
			name: "add_output_tokens_to_messages",
			sql:  "ALTER TABLE messages ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0",
		},
		{
			name: "add_duration_ms_to_messages",
			sql:  "ALTER TABLE messages ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0",
		},
		// W1.2: bare-repo tracking
		{
			name: "add_server_repo_initialised_at_to_projects",
			sql:  "ALTER TABLE projects ADD COLUMN server_repo_initialised_at DATETIME",
		},
		// W1.3: upstream remote
		{
			name: "add_remote_url_to_projects",
			sql:  "ALTER TABLE projects ADD COLUMN remote_url TEXT NOT NULL DEFAULT ''",
		},
		{
			name: "add_remote_credentials_ref_to_projects",
			sql:  "ALTER TABLE projects ADD COLUMN remote_credentials_ref TEXT NOT NULL DEFAULT ''",
		},
		// W1.4: git HTTP server / branch tracking
		{
			name: "add_branch_head_sha_to_tasks",
			sql:  "ALTER TABLE tasks ADD COLUMN branch_head_sha TEXT NOT NULL DEFAULT ''",
		},
		{
			name: "add_last_push_at_to_tasks",
			sql:  "ALTER TABLE tasks ADD COLUMN last_push_at DATETIME",
		},
		{
			name: "add_slug_to_projects",
			sql:  "ALTER TABLE projects ADD COLUMN slug TEXT NOT NULL DEFAULT ''",
		},
		// W3.2: worktree path
		{
			name: "add_worktree_path_to_tasks",
			sql:  "ALTER TABLE tasks ADD COLUMN worktree_path TEXT NOT NULL DEFAULT ''",
		},
		// W3.4: port pool
		{
			name: "add_assigned_port_to_tasks",
			sql:  "ALTER TABLE tasks ADD COLUMN assigned_port INTEGER",
		},
		// W3.1: agent mode
		{
			name: "add_mode_to_agents",
			sql:  "ALTER TABLE agents ADD COLUMN mode TEXT NOT NULL DEFAULT 'remote'",
		},
		// W5.2: task_comments.review_id FK index (column already created in schema)
		{
			name: "add_review_id_index_to_task_comments",
			sql:  "CREATE INDEX IF NOT EXISTS idx_task_comments_review ON task_comments(review_id)",
		},
		// W2.3: projects coding rules
		{
			name: "add_coding_rules_to_projects",
			sql:  "ALTER TABLE projects ADD COLUMN coding_rules TEXT NOT NULL DEFAULT ''",
		},
		// provider role preferences
		{
			name: "add_roles_to_providers",
			sql:  "ALTER TABLE providers ADD COLUMN roles TEXT NOT NULL DEFAULT '[]'",
		},
		// role-level tool allowlist for small models
		{
			name: "add_allowed_tools_to_role_definitions",
			sql:  "ALTER TABLE agent_role_definitions ADD COLUMN allowed_tools TEXT NOT NULL DEFAULT '[]'",
		},
		// per-provider model list with role and pricing config
		{
			name: "add_models_to_providers",
			sql:  "ALTER TABLE providers ADD COLUMN models TEXT NOT NULL DEFAULT '[]'",
		},
		// Bug 8: persist comment author display name at write time
		{
			name: "add_author_name_to_task_comments",
			sql:  "ALTER TABLE task_comments ADD COLUMN author_name TEXT NOT NULL DEFAULT ''",
		},
		// Feature 3: capability flags on role definitions
		{
			name: "add_capabilities_to_role_definitions",
			sql:  "ALTER TABLE agent_role_definitions ADD COLUMN capabilities TEXT NOT NULL DEFAULT '[]'",
		},
		// Feature 3: per-task review role (routing key for AWAITING_REVIEW)
		{
			name: "add_review_role_to_tasks",
			sql:  "ALTER TABLE tasks ADD COLUMN review_role TEXT NOT NULL DEFAULT ''",
		},
	}

	for _, m := range migrations {
		var count int
		if err := d.db.QueryRow(
			"SELECT COUNT(*) FROM _schema_migrations WHERE name = ?", m.name,
		).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %q: %w", m.name, err)
		}
		if count > 0 {
			continue
		}
		if _, err := d.db.Exec(m.sql); err != nil {
			return fmt.Errorf("applying migration %q: %w", m.name, err)
		}
		if _, err := d.db.Exec(
			"INSERT INTO _schema_migrations (name) VALUES (?)", m.name,
		); err != nil {
			return fmt.Errorf("recording migration %q: %w", m.name, err)
		}
	}
	return nil
}

// migrateTaskStates is a one-shot migration that renames old task status strings
// to the new vocabulary. Each renamed row is recorded in state_migration_log.
// Running this on a fresh database is a no-op.
func (d *Database) migrateTaskStates() error {
	const migName = "rename_task_states_to_new_vocabulary"
	var count int
	if err := d.db.QueryRow(
		"SELECT COUNT(*) FROM _schema_migrations WHERE name = ?", migName,
	).Scan(&count); err != nil {
		return fmt.Errorf("checking migration %q: %w", migName, err)
	}
	if count > 0 {
		return nil // already applied
	}

	// Map old → new state strings.
	renames := [][2]string{
		{"planned", TaskStatusBacklog},
		{"queued", TaskStatusBacklog},
		{"in_progress", TaskStatusDeveloping},
		{"needs_review", TaskStatusAwaitingReview},
		{"approved", TaskStatusAwaitingMerge},
		{"completed", TaskStatusCompleted},
		{"failed", TaskStatusFailed},
	}

	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("migrateTaskStates: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, pair := range renames {
		oldState, newState := pair[0], pair[1]

		rows, err := tx.Query("SELECT id FROM tasks WHERE status = ?", oldState)
		if err != nil {
			return fmt.Errorf("migrateTaskStates: query %q: %w", oldState, err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()

		for _, id := range ids {
			if _, err := tx.Exec("UPDATE tasks SET status = ? WHERE id = ?", newState, id); err != nil {
				return fmt.Errorf("migrateTaskStates: update task %s: %w", id, err)
			}
			if _, err := tx.Exec(
				`INSERT INTO state_migration_log (task_id, old_state, new_state) VALUES (?, ?, ?)`,
				id, oldState, newState,
			); err != nil {
				return fmt.Errorf("migrateTaskStates: log task %s: %w", id, err)
			}
		}
	}

	if _, err := tx.Exec("INSERT INTO _schema_migrations (name) VALUES (?)", migName); err != nil {
		return fmt.Errorf("recording migration %q: %w", migName, err)
	}
	return tx.Commit()
}
