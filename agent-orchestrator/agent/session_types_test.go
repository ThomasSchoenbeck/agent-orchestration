package agent

import (
	"encoding/json"
	"testing"

	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

func TestNewSession_RootDefaults(t *testing.T) {
	route := router.RouteResult{Model: "gpt-x"}
	s := newSession(SessionKindMain, "task-1", "agent-1", route)

	if s.ID == "" {
		t.Error("expected a generated ID")
	}
	if s.Kind != SessionKindMain || s.TaskID != "task-1" || s.AgentID != "agent-1" {
		t.Errorf("unexpected session fields: %+v", s)
	}
	if s.ParentID != "" {
		t.Errorf("root session must have no parent, got %q", s.ParentID)
	}
	if s.Status != SessionStatusRunning {
		t.Errorf("expected running, got %q", s.Status)
	}
	if s.Stats.model != "gpt-x" {
		t.Errorf("expected stats.model seeded from route, got %q", s.Stats.model)
	}
}

func TestNewChildSession_LinksToParent(t *testing.T) {
	parent := newSession(SessionKindMain, "task-1", "agent-1", router.RouteResult{Model: "big"})
	child := newChildSession(parent, SessionKindWork, router.RouteResult{Model: "small"})

	if child.ParentID != parent.ID {
		t.Errorf("child.ParentID = %q, want %q", child.ParentID, parent.ID)
	}
	if child.TaskID != parent.TaskID || child.AgentID != parent.AgentID {
		t.Errorf("child should inherit task/agent: %+v", child)
	}
	if child.ID == parent.ID {
		t.Error("child must have its own ID")
	}
	if child.Kind != SessionKindWork {
		t.Errorf("child kind = %q", child.Kind)
	}
	if child.Route.Model != "small" {
		t.Errorf("child should keep its own route, got model %q", child.Route.Model)
	}
}

func TestSession_StatusTransitions(t *testing.T) {
	s := newSession(SessionKindMain, "t", "a", router.RouteResult{})
	if s.isTerminal() {
		t.Fatal("new session must not be terminal")
	}
	s.markDone()
	if !s.isTerminal() || s.Status != SessionStatusDone {
		t.Fatalf("expected done, got %q", s.Status)
	}
	// Terminal is sticky: a later transition is a no-op.
	s.markFailed()
	if s.Status != SessionStatusDone {
		t.Errorf("terminal state must be sticky, got %q", s.Status)
	}
}

func TestSession_ToAgentSession(t *testing.T) {
	s := newChildSession(
		newSession(SessionKindMain, "task-9", "agent-9", router.RouteResult{}),
		SessionKindWork, router.RouteResult{Model: "m"},
	)
	s.Title = "code_subtask"
	s.Messages = []llm.Message{{Role: "user", Content: "hi"}}
	s.Stats.cost = 0.5
	s.markDone()

	row := s.toAgentSession("did the thing", 3)
	if row.ID != s.ID || row.ParentID != s.ParentID || row.TaskID != "task-9" {
		t.Errorf("identity fields not carried: %+v", row)
	}
	if row.Kind != "work" || row.Status != "done" || row.Title != "code_subtask" {
		t.Errorf("tree fields not carried: %+v", row)
	}
	if row.Summary != "did the thing" || row.Round != 3 || row.Cost != 0.5 {
		t.Errorf("summary/round/cost not carried: %+v", row)
	}
	var msgs []llm.Message
	if err := json.Unmarshal([]byte(row.Messages), &msgs); err != nil {
		t.Fatalf("messages should be valid JSON: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "hi" {
		t.Errorf("messages did not round-trip: %v", msgs)
	}
}
