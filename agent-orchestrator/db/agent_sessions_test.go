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
	// Back-compat: callers that don't set the session-tree fields get defaults.
	if got[0].Kind != "main" || got[0].Status != "done" {
		t.Errorf("expected default kind=main status=done, got kind=%q status=%q", got[0].Kind, got[0].Status)
	}
	if got[0].ParentID != "" || got[0].Cost != 0 {
		t.Errorf("expected empty parent and zero cost by default, got %+v", got[0])
	}
}

func TestAgentSession_SessionTreeFieldsRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	parent := &db.AgentSession{TaskID: "task-tree", AgentID: "a1", Kind: "main", Status: "running", Title: "main loop"}
	if err := d.CreateAgentSession(ctx, parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := d.CreateAgentSession(ctx, &db.AgentSession{
		TaskID: "task-tree", AgentID: "a1", Kind: "work", ParentID: parent.ID,
		Status: "done", Title: "code_subtask", Cost: 0.0125, Summary: "changed foo.go",
	}); err != nil {
		t.Fatalf("create child: %v", err)
	}

	got, err := d.ListAgentSessionsByTask(ctx, "task-tree")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got))
	}
	if got[0].Kind != "main" || got[0].Status != "running" || got[0].Title != "main loop" {
		t.Errorf("parent tree fields = %+v", got[0])
	}
	child := got[1]
	if child.Kind != "work" || child.ParentID != parent.ID || child.Cost != 0.0125 {
		t.Errorf("child tree fields = %+v", child)
	}
}
