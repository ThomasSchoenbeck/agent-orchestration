package server_test

// task_diff_log_test.go verifies that task update operations emit
// the correct log entries (old_status / new_status) via the task log endpoint.

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/server"
)

// newTestServerWithLogs creates a test server with a wired-up LogDatabase so
// that task log entries are persisted and visible via the /logs endpoint.
func newTestServerWithLogs(t *testing.T) (*server.Server, *db.Database) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	logPath := filepath.Join(dir, "logs.db")
	logDB, err := db.OpenLogDB(logPath)
	if err != nil {
		t.Fatalf("open log db: %v", err)
	}
	database.LogDB = logDB
	t.Cleanup(func() {
		_ = logDB.Close()
		_ = database.Close()
	})

	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
		Database: config.DatabaseConfig{Path: path},
		Agents:   config.AgentConfig{HeartbeatIntervalSec: 30, TaskTimeoutSec: 300},
		Storage:  config.StorageConfig{Root: dir},
	}
	reg := llm.NewRegistry()
	srv := server.New(cfg, database, reg)
	return srv, database
}

// helpers: do, createTestProject are in handlers_test.go

func TestTaskUpdateStatus_LogsTransition(t *testing.T) {
	srv, _ := newTestServerWithLogs(t)
	projectID := createTestProject(t, srv)

	// Create a task.
	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID,
		"type":       "implement",
		"role":       "worker",
	})
	if tw.Code != http.StatusCreated {
		t.Fatalf("create task: expected 201, got %d: %s", tw.Code, tw.Body.String())
	}
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	// Update the task status via PUT.
	newStatus := db.TaskStatusDeveloping
	uw := do(t, srv, http.MethodPut, "/api/tasks/"+task.ID, map[string]interface{}{
		"status": newStatus,
	})
	if uw.Code != http.StatusOK {
		t.Fatalf("update task: expected 200, got %d: %s", uw.Code, uw.Body.String())
	}

	// Fetch task logs and verify a task_updated entry with correct states.
	lw := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/logs", nil)
	if lw.Code != http.StatusOK {
		t.Fatalf("list logs: expected 200, got %d: %s", lw.Code, lw.Body.String())
	}
	var logs []*db.TaskLog
	_ = json.Unmarshal(lw.Body.Bytes(), &logs)

	var found bool
	for _, l := range logs {
		if l.EventType == "task_updated" {
			found = true
			if l.OldStatus != db.TaskStatusBacklog {
				t.Errorf("OldStatus = %q, want %q", l.OldStatus, db.TaskStatusBacklog)
			}
			if l.NewStatus != newStatus {
				t.Errorf("NewStatus = %q, want %q", l.NewStatus, newStatus)
			}
		}
	}
	if !found {
		t.Errorf("no task_updated log entry found in %d entries", len(logs))
	}
}

func TestTaskUpdateStatus_NoChange_LogsFieldsUpdated(t *testing.T) {
	srv, _ := newTestServerWithLogs(t)
	projectID := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID,
		"type":       "implement",
		"role":       "worker",
		"priority":   3,
	})
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	// PUT with only priority change — status unchanged.
	uw := do(t, srv, http.MethodPut, "/api/tasks/"+task.ID, map[string]interface{}{
		"priority": 8,
	})
	if uw.Code != http.StatusOK {
		t.Fatalf("update task: expected 200, got %d: %s", uw.Code, uw.Body.String())
	}

	lw := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/logs", nil)
	var logs []*db.TaskLog
	_ = json.Unmarshal(lw.Body.Bytes(), &logs)

	var found bool
	for _, l := range logs {
		if l.EventType == "task_updated" {
			found = true
			// Status unchanged — old and new should both be BACKLOG.
			if l.OldStatus != db.TaskStatusBacklog || l.NewStatus != db.TaskStatusBacklog {
				t.Errorf("expected both statuses BACKLOG, got old=%q new=%q", l.OldStatus, l.NewStatus)
			}
		}
	}
	if !found {
		t.Errorf("no task_updated log entry found")
	}
}
