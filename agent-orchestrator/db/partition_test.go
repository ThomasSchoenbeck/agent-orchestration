package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"agent-orchestrator/db"
)

// TestOpenLogDB_WALMode verifies that the log database is opened with
// WAL journal mode enabled.
func TestOpenLogDB_WALMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal_test.db")
	ld, err := db.OpenLogDB(path)
	if err != nil {
		t.Fatalf("OpenLogDB: %v", err)
	}
	defer ld.Close()

	var mode string
	if err := ld.RawDB().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("expected journal_mode=wal, got %q", mode)
	}
}

// TestOpenLogDB_BusyTimeout verifies that busy_timeout is set (non-zero).
func TestOpenLogDB_BusyTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy_test.db")
	ld, err := db.OpenLogDB(path)
	if err != nil {
		t.Fatalf("OpenLogDB: %v", err)
	}
	defer ld.Close()

	var timeout int
	if err := ld.RawDB().QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if timeout == 0 {
		t.Error("expected non-zero busy_timeout")
	}
}

// TestAgentLogPartition_CreatedOnWrite verifies that writing an agent log
// creates the expected daily partition table.
func TestAgentLogPartition_CreatedOnWrite(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()
	now := time.Now().UTC()

	err := d.CreateAgentLog(ctx, &db.AgentLog{
		AgentID:   "a1",
		EventType: "agent_registered",
		Timestamp: now,
	})
	if err != nil {
		t.Fatalf("CreateAgentLog: %v", err)
	}

	// Check that the partition table exists in sqlite_master.
	tblName := "agent_logs_" + now.Format("2006_01_02")
	var count int
	err = d.LogDB.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", tblName,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected partition table %q to exist, count=%d", tblName, count)
	}
}

// TestTaskLogPartition_CreatedOnWrite verifies that writing a task log
// creates the expected daily partition table.
func TestTaskLogPartition_CreatedOnWrite(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()
	now := time.Now().UTC()

	err := d.CreateTaskLog(ctx, &db.TaskLog{
		TaskID:    "t1",
		EventType: "task_created",
		Timestamp: now,
	})
	if err != nil {
		t.Fatalf("CreateTaskLog: %v", err)
	}

	tblName := "task_logs_" + now.Format("2006_01_02")
	var count int
	err = d.LogDB.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", tblName,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected partition table %q to exist, count=%d", tblName, count)
	}
}

// TestDropOldAgentLogPartitions_Empty verifies the drop function is a no-op
// when no partitions exist.
func TestDropOldAgentLogPartitions_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drop_empty.db")
	ld, err := db.OpenLogDB(path)
	if err != nil {
		t.Fatalf("OpenLogDB: %v", err)
	}
	defer ld.Close()

	dropped, err := db.DropOldAgentLogPartitions(context.Background(), ld.RawDB(), 7)
	if err != nil {
		t.Fatalf("DropOldAgentLogPartitions: %v", err)
	}
	if dropped != 0 {
		t.Errorf("expected 0 dropped, got %d", dropped)
	}
}

// TestDropOldAgentLogPartitions_DropsOld creates a partition dated far in the
// past and verifies it gets dropped.
func TestDropOldAgentLogPartitions_DropsOld(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()

	// Write a log entry dated 30 days ago so the partition exists.
	oldTime := time.Now().UTC().Add(-30 * 24 * time.Hour)
	err := d.CreateAgentLog(ctx, &db.AgentLog{
		AgentID:   "a1",
		EventType: "agent_registered",
		Timestamp: oldTime,
	})
	if err != nil {
		t.Fatalf("CreateAgentLog: %v", err)
	}

	// Drop partitions older than 7 days; the 30-day-old one should go.
	dropped, err := db.DropOldAgentLogPartitions(ctx, d.LogDB.RawDB(), 7)
	if err != nil {
		t.Fatalf("DropOldAgentLogPartitions: %v", err)
	}
	if dropped < 1 {
		t.Errorf("expected at least 1 partition dropped, got %d", dropped)
	}
}

// TestDropOldTaskLogPartitions_DropsOld mirrors the agent log test for task logs.
func TestDropOldTaskLogPartitions_DropsOld(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()

	oldTime := time.Now().UTC().Add(-30 * 24 * time.Hour)
	err := d.CreateTaskLog(ctx, &db.TaskLog{
		TaskID:    "t1",
		EventType: "task_created",
		Timestamp: oldTime,
	})
	if err != nil {
		t.Fatalf("CreateTaskLog: %v", err)
	}

	dropped, err := db.DropOldTaskLogPartitions(ctx, d.LogDB.RawDB(), 7)
	if err != nil {
		t.Fatalf("DropOldTaskLogPartitions: %v", err)
	}
	if dropped < 1 {
		t.Errorf("expected at least 1 partition dropped, got %d", dropped)
	}
}

// TestMultiDayPartitions verifies that logs written on different days land in
// different partition tables and are both retrieved by ListAgentLogs.
func TestMultiDayPartitions(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()

	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)

	_ = d.CreateAgentLog(ctx, &db.AgentLog{AgentID: "a1", EventType: "agent_registered", Timestamp: yesterday})
	_ = d.CreateAgentLog(ctx, &db.AgentLog{AgentID: "a1", EventType: "agent_registered", Timestamp: now})

	// Both days should be covered by a 2-day window.
	got, err := d.ListAgentLogs(ctx, db.AgentLogFilters{
		Since: yesterday.Add(-time.Minute),
		Until: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ListAgentLogs: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 logs across 2 days, got %d", len(got))
	}
}
