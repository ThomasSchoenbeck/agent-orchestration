package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"agent-orchestrator/db"
)

// openTestLogDB opens a temporary LogDatabase and registers cleanup.
func openTestLogDB(t *testing.T) *db.LogDatabase {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs.db")
	ld, err := db.OpenLogDB(path)
	if err != nil {
		t.Fatalf("OpenLogDB: %v", err)
	}
	t.Cleanup(func() { _ = ld.Close() })
	return ld
}

// openTestDBWithLogs opens a main DB + a log DB and wires them together.
func openTestDBWithLogs(t *testing.T) *db.Database {
	t.Helper()
	d := openTestDB(t)
	d.LogDB = openTestLogDB(t)
	return d
}

// ── AgentLog ──────────────────────────────────────────────────────────────────

func TestCreateAgentLog_NilLogDB(t *testing.T) {
	d := openTestDB(t) // LogDB is nil
	ctx := context.Background()
	err := d.CreateAgentLog(ctx, &db.AgentLog{
		AgentID:   "a1",
		EventType: "agent_registered",
	})
	// Must not error when LogDB is nil.
	if err != nil {
		t.Errorf("expected nil error when LogDB is nil, got: %v", err)
	}
}

func TestCreateAndListAgentLogs(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()

	now := time.Now().UTC()
	logs := []*db.AgentLog{
		{AgentID: "a1", AgentName: "worker-1", EventType: "agent_registered", Timestamp: now},
		{AgentID: "a1", AgentName: "worker-1", EventType: "agent_claim_success", Timestamp: now.Add(time.Second)},
		{AgentID: "a2", AgentName: "worker-2", EventType: "agent_registered", Timestamp: now.Add(2 * time.Second)},
	}
	for _, l := range logs {
		if err := d.CreateAgentLog(ctx, l); err != nil {
			t.Fatalf("CreateAgentLog: %v", err)
		}
		if l.ID == "" {
			t.Error("expected non-empty ID after create")
		}
	}

	got, err := d.ListAgentLogs(ctx, db.AgentLogFilters{Since: now.Add(-time.Minute), Until: now.Add(time.Minute)})
	if err != nil {
		t.Fatalf("ListAgentLogs: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 agent logs, got %d", len(got))
	}
}

func TestListAgentLogs_FilterAgentID(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_ = d.CreateAgentLog(ctx, &db.AgentLog{AgentID: "a1", EventType: "agent_registered", Timestamp: now})
	_ = d.CreateAgentLog(ctx, &db.AgentLog{AgentID: "a2", EventType: "agent_registered", Timestamp: now})

	got, err := d.ListAgentLogs(ctx, db.AgentLogFilters{
		AgentID: "a1",
		Since:   now.Add(-time.Minute),
		Until:   now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ListAgentLogs: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 log for agent a1, got %d", len(got))
	}
	if got[0].AgentID != "a1" {
		t.Errorf("expected agent_id a1, got %s", got[0].AgentID)
	}
}

func TestListAgentLogs_FilterEventType(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_ = d.CreateAgentLog(ctx, &db.AgentLog{AgentID: "a1", EventType: "agent_registered", Timestamp: now})
	_ = d.CreateAgentLog(ctx, &db.AgentLog{AgentID: "a1", EventType: "agent_claim_success", Timestamp: now.Add(time.Second)})
	_ = d.CreateAgentLog(ctx, &db.AgentLog{AgentID: "a1", EventType: "agent_registered", Timestamp: now.Add(2 * time.Second)})

	got, err := d.ListAgentLogs(ctx, db.AgentLogFilters{
		EventType: "agent_registered",
		Since:     now.Add(-time.Minute),
		Until:     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ListAgentLogs: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 agent_registered logs, got %d", len(got))
	}
}

func TestListAgentLogs_NilLogDB(t *testing.T) {
	d := openTestDB(t) // no LogDB
	ctx := context.Background()
	got, err := d.ListAgentLogs(ctx, db.AgentLogFilters{})
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil slice, got %v", got)
	}
}

// ── TaskLog ───────────────────────────────────────────────────────────────────

func TestCreateTaskLog_NilLogDB(t *testing.T) {
	d := openTestDB(t) // LogDB is nil
	ctx := context.Background()
	err := d.CreateTaskLog(ctx, &db.TaskLog{
		TaskID:    "t1",
		EventType: "task_created",
	})
	if err != nil {
		t.Errorf("expected nil error when LogDB is nil, got: %v", err)
	}
}

func TestCreateAndListTaskLogs(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()
	now := time.Now().UTC()

	entries := []*db.TaskLog{
		{TaskID: "t1", EventType: "task_created", NewStatus: "planned", Timestamp: now},
		{TaskID: "t1", EventType: "task_claimed", OldStatus: "planned", NewStatus: "in_progress", Timestamp: now.Add(time.Second)},
		{TaskID: "t2", EventType: "task_created", NewStatus: "planned", Timestamp: now.Add(2 * time.Second)},
	}
	for _, l := range entries {
		if err := d.CreateTaskLog(ctx, l); err != nil {
			t.Fatalf("CreateTaskLog: %v", err)
		}
		if l.ID == "" {
			t.Error("expected non-empty ID after create")
		}
	}

	got, err := d.ListTaskLogs(ctx, db.TaskLogFilters{
		Since: now.Add(-time.Minute),
		Until: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ListTaskLogs: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 task logs, got %d", len(got))
	}
}

func TestListTaskLogs_FilterTaskID(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_ = d.CreateTaskLog(ctx, &db.TaskLog{TaskID: "t1", EventType: "task_created", Timestamp: now})
	_ = d.CreateTaskLog(ctx, &db.TaskLog{TaskID: "t2", EventType: "task_created", Timestamp: now})

	got, err := d.ListTaskLogs(ctx, db.TaskLogFilters{
		TaskID: "t1",
		Since:  now.Add(-time.Minute),
		Until:  now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ListTaskLogs: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 log for task t1, got %d", len(got))
	}
	if got[0].TaskID != "t1" {
		t.Errorf("expected task_id t1, got %s", got[0].TaskID)
	}
}

func TestListTaskLogs_StatusTransition(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()
	now := time.Now().UTC()

	l := &db.TaskLog{
		TaskID:      "t1",
		EventType:   "task_claimed",
		OldStatus:   "planned",
		NewStatus:   "in_progress",
		Description: "agent claimed the task",
		Timestamp:   now,
	}
	_ = d.CreateTaskLog(ctx, l)

	got, err := d.ListTaskLogs(ctx, db.TaskLogFilters{
		Since: now.Add(-time.Minute),
		Until: now.Add(time.Minute),
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("unexpected result: %v / len=%d", err, len(got))
	}
	if got[0].OldStatus != "planned" || got[0].NewStatus != "in_progress" {
		t.Errorf("status transition: got %s → %s", got[0].OldStatus, got[0].NewStatus)
	}
}

func TestListTaskLogs_NilLogDB(t *testing.T) {
	d := openTestDB(t) // no LogDB
	ctx := context.Background()
	got, err := d.ListTaskLogs(ctx, db.TaskLogFilters{})
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil slice, got %v", got)
	}
}
