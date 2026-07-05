package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/router"
)

func TestReconstructProgress_NilClientReturnsEmpty(t *testing.T) {
	e := NewExecutor(nil, nil, nil, "a1")
	brief, stats := e.reconstructProgress(context.Background(), e.log.ForTask("t1"), &router.RouteResult{}, &db.Task{ID: "t1"})
	if brief != "" || stats.totalTokens != 0 {
		t.Errorf("expected empty brief and zero stats, got %q %+v", brief, stats)
	}
}

func TestReconstructProgress_NoPriorSessionsReturnsEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/agent-sessions", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]*db.AgentSession{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := NewExecutor(nil, nil, NewServerClient(srv.URL), "a1")
	brief, _ := e.reconstructProgress(context.Background(), e.log.ForTask("t1"), &router.RouteResult{}, &db.Task{ID: "t1"})
	if brief != "" {
		t.Errorf("no prior sessions must yield empty brief, got %q", brief)
	}
}

func TestReconstructProgress_FallsBackToLatestSummary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/agent-sessions", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]*db.AgentSession{
			{Round: 2, Summary: "did part one", Status: "done"},
			{Round: 5, Summary: "did part two", Status: "done"},
		})
	})
	// No task_status subagent available → reconstructProgress falls back to the
	// latest checkpoint summary (no LLM call).
	mux.HandleFunc("/api/agent/subagent-skills", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]*db.SubagentSkill{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := NewExecutor(nil, nil, NewServerClient(srv.URL), "a1")
	brief, _ := e.reconstructProgress(context.Background(), e.log.ForTask("t1"), &router.RouteResult{}, &db.Task{ID: "t1"})
	if brief != "did part two" {
		t.Errorf("expected fallback to latest checkpoint summary, got %q", brief)
	}
}

func TestLatestCheckpointSummary_PicksLastNonEmpty(t *testing.T) {
	sessions := []*db.AgentSession{
		{Summary: "first"},
		{Summary: "second"},
		{Summary: "   "}, // whitespace only → skipped
	}
	if got := latestCheckpointSummary(sessions); got != "second" {
		t.Errorf("got %q, want second", got)
	}
	if got := latestCheckpointSummary(nil); got != "" {
		t.Errorf("nil sessions → empty, got %q", got)
	}
}
