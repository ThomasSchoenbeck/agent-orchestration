package db

// migrate creates all tables and indexes if they do not already exist.
func (d *Database) migrate() error {
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
    id          TEXT PRIMARY KEY,
    task_id     TEXT,
    agent_id    TEXT,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    cost        REAL NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    success     INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id)  REFERENCES tasks(id),
    FOREIGN KEY (agent_id) REFERENCES agents(id)
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
`

	_, err := d.db.Exec(schema)
	return err
}
