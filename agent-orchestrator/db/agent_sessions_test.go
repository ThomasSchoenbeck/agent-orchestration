package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestAgentSession_RoundTripByTask(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.CreateAgentSession(ctx, &db.AgentSession{
		TaskID: "task-1", AgentID: "agent-1",
		Summary: "Did X and Y.", Messages: `[{"role":"system","content":"s"}]`, Round: 4,
	}); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}
	if err := d.CreateAgentSession(ctx, &db.AgentSession{
		TaskID: "task-1", AgentID: "agent-1", Summary: "Then Z.", Round: 9,
	}); err != nil {
		t.Fatalf("CreateAgentSession 2: %v", err)
	}
	// A different task's checkpoint must not leak in.
	if err := d.CreateAgentSession(ctx, &db.AgentSession{TaskID: "task-2", Summary: "other"}); err != nil {
		t.Fatalf("CreateAgentSession other: %v", err)
	}

	got, err := d.ListAgentSessionsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListAgentSessionsByTask: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions for task-1, got %d", len(got))
	}
	if got[0].Summary != "Did X and Y." || got[0].Round != 4 {
		t.Errorf("first session = %+v", got[0])
	}
	if got[0].Messages == "" {
		t.Error("messages should round-trip")
	}
}
