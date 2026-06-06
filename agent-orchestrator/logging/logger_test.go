package logging_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/logging"
)

func TestLogger_NilDBDoesNotPanic(t *testing.T) {
	// Long ids exercise the min()-based truncation in the console prefix.
	l := logging.New(nil, "agent-1234567890", "task-abcdef12", "proj-1")
	l = l.WithTask("task-xyz").WithProject("proj-2")

	ctx := context.Background()
	l.Debug(ctx, "debug")
	l.Info(ctx, "info", map[string]interface{}{"k": "v"})
	l.Warn(ctx, "warn")
	l.Error(ctx, "err", map[string]interface{}{"a": 1}, map[string]interface{}{"b": 2})
}

func TestLogger_PersistsToDB(t *testing.T) {
	d, _ := openMetricsDB(t)
	l := logging.New(d, "agent-1", "task-1", "proj-1")

	l.Info(context.Background(), "hello world", map[string]interface{}{"x": "y"})

	logs, err := d.ListLogs(context.Background(), db.LogFilters{Limit: 10})
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	found := false
	for _, e := range logs {
		if e.Message == "hello world" && e.Level == logging.LevelInfo {
			found = true
		}
	}
	if !found {
		t.Errorf("persisted log entry not found; got %d entries", len(logs))
	}
}
