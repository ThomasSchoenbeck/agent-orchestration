package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"agent-orchestrator/db"
)

func TestNewRetentionJob_Defaults(t *testing.T) {
	d := openTestDB(t)
	if j := NewRetentionJob(d, 0); j.intervalMin != 60 {
		t.Errorf("default interval = %d, want 60", j.intervalMin)
	}
	if j := NewRetentionJob(d, 30); j.intervalMin != 30 {
		t.Errorf("interval = %d, want 30", j.intervalMin)
	}
}

func TestSettingInt(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if got := settingInt(d, ctx, "missing.key", 7); got != 7 {
		t.Errorf("missing setting → %d, want default 7", got)
	}
	if err := d.SetSetting(ctx, "ret.k", "12", ""); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if got := settingInt(d, ctx, "ret.k", 7); got != 12 {
		t.Errorf("valid setting → %d, want 12", got)
	}
	if err := d.SetSetting(ctx, "ret.bad", "notanint", ""); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if got := settingInt(d, ctx, "ret.bad", 7); got != 7 {
		t.Errorf("non-int setting → %d, want default 7", got)
	}
}

func TestMaxRetentionDays(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	ets := []string{"a_evt", "b_evt"}

	if got := maxRetentionDays(d, ctx, ets, "pre.", 14); got != 14 {
		t.Errorf("no overrides → %d, want 14", got)
	}
	if err := d.SetSetting(ctx, "pre.b_evt_days", "30", ""); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if got := maxRetentionDays(d, ctx, ets, "pre.", 14); got != 30 {
		t.Errorf("with override → %d, want 30", got)
	}
}

func TestRetentionRunOnce_NilLogDB(t *testing.T) {
	d := openTestDB(t)
	// LogDB is nil → runOnce returns early without panicking.
	NewRetentionJob(d, 60).runOnce(context.Background())
}

func TestRetentionRunOnce_WithLogDB(t *testing.T) {
	d := openTestDB(t)
	logDB, err := db.OpenLogDB(filepath.Join(t.TempDir(), "logs.db"))
	if err != nil {
		t.Fatalf("OpenLogDB: %v", err)
	}
	t.Cleanup(func() { _ = logDB.Close() })
	d.LogDB = logDB

	ctx := context.Background()
	NewRetentionJob(d, 60).runOnce(ctx)

	logs, err := d.ListLogs(ctx, db.LogFilters{Limit: 20})
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	found := false
	for _, e := range logs {
		if strings.Contains(e.Message, "retention cleanup") {
			found = true
		}
	}
	if !found {
		t.Error("expected a retention-cleanup summary log entry")
	}
}
