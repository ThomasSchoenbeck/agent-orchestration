package tools_test

import (
	"context"
	"errors"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/tools"
)

type fakeProgressBackend struct {
	sessions []*db.AgentSession
	err      error
}

func (f *fakeProgressBackend) ListAgentSessions(_ context.Context, _ string) ([]*db.AgentSession, error) {
	return f.sessions, f.err
}

func TestGetTaskProgress_ReturnsCheckpoints(t *testing.T) {
	be := &fakeProgressBackend{sessions: []*db.AgentSession{
		{Round: 3, Kind: "main", Status: "done", Summary: "did part one"},
		{Round: 7, Kind: "main", Status: "done", Summary: "did part two"},
	}}
	reg := tools.NewRegistry()
	if err := tools.RegisterProgressTool(reg, be); err != nil {
		t.Fatalf("RegisterProgressTool: %v", err)
	}

	res, err := reg.Execute(context.Background(), "get_task_progress", map[string]interface{}{"task_id": "t1"})
	if err != nil {
		t.Fatalf("get_task_progress: %v", err)
	}
	m := res.(map[string]interface{})
	if m["count"].(int) != 2 {
		t.Errorf("count = %v, want 2", m["count"])
	}
	cps := m["checkpoints"].([]map[string]interface{})
	if cps[0]["summary"] != "did part one" || cps[1]["round"] != 7 {
		t.Errorf("checkpoints = %+v", cps)
	}
}

func TestGetTaskProgress_BackendErrorPropagates(t *testing.T) {
	be := &fakeProgressBackend{err: errors.New("db down")}
	reg := tools.NewRegistry()
	_ = tools.RegisterProgressTool(reg, be)
	if _, err := reg.Execute(context.Background(), "get_task_progress", map[string]interface{}{"task_id": "t1"}); err == nil {
		t.Fatal("expected backend error to propagate")
	}
}

func TestGetTaskProgress_NilBackendErrors(t *testing.T) {
	reg := tools.NewRegistry()
	if err := tools.RegisterProgressTool(reg, nil); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := reg.Execute(context.Background(), "get_task_progress", map[string]interface{}{"task_id": "t1"}); err == nil {
		t.Fatal("expected error with nil backend")
	}
}
