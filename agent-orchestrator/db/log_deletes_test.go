package db_test

import (
	"context"
	"testing"
	"time"

	"agent-orchestrator/db"
)

// General logs table (no partitioned log DB needed).
func TestDeleteLogsByTaskAndOld(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.CreateLog(ctx, &db.LogEntry{TaskID: "t1", Level: "info", Message: "for t1"}); err != nil {
		t.Fatalf("CreateLog: %v", err)
	}
	n, err := d.DeleteLogsByTask(ctx, "t1")
	if err != nil {
		t.Fatalf("DeleteLogsByTask: %v", err)
	}
	if n < 1 {
		t.Errorf("DeleteLogsByTask removed %d, want >= 1", n)
	}

	if err := d.CreateLog(ctx, &db.LogEntry{Level: "info", Message: "old"}); err != nil {
		t.Fatalf("CreateLog: %v", err)
	}
	if _, err := d.DeleteOldLogs(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Errorf("DeleteOldLogs: %v", err)
	}
}

// Partitioned task/agent log tables (require the log DB).
func TestDeletePartitionedLogs(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()
	future := time.Now().Add(time.Hour)

	if err := d.CreateTaskLog(ctx, &db.TaskLog{TaskID: "t1", ProjectID: "p1", EventType: "claimed", Description: "x"}); err != nil {
		t.Fatalf("CreateTaskLog: %v", err)
	}
	if _, err := d.DeleteTaskLogsByTask(ctx, "t1"); err != nil {
		t.Errorf("DeleteTaskLogsByTask: %v", err)
	}
	if _, err := d.DeleteTaskLogs(ctx, future); err != nil {
		t.Errorf("DeleteTaskLogs: %v", err)
	}

	if err := d.CreateAgentLog(ctx, &db.AgentLog{AgentID: "a1", EventType: "start", Description: "y"}); err != nil {
		t.Fatalf("CreateAgentLog: %v", err)
	}
	if _, err := d.DeleteAgentLogs(ctx, future); err != nil {
		t.Errorf("DeleteAgentLogs: %v", err)
	}
}
