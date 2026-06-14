package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

func TestShouldCheckpoint(t *testing.T) {
	if shouldCheckpoint(100, 0) {
		t.Error("unknown window (max=0) must not checkpoint")
	}
	if shouldCheckpoint(700, 1000) {
		t.Error("70% usage should be below the 80% threshold")
	}
	if !shouldCheckpoint(850, 1000) {
		t.Error("85% usage should cross the threshold")
	}
}

func TestCompactMessages_PreservesSystem(t *testing.T) {
	msgs := compactMessages("SYS", "did work", false)
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[0].Content != "SYS" {
		t.Fatalf("expected [system, user], got %+v", msgs)
	}
	if msgs[1].Role != "user" || !strings.Contains(msgs[1].Content, "did work") {
		t.Errorf("user message must carry the summary: %+v", msgs[1])
	}
}

func TestCompactMessages_Folded(t *testing.T) {
	msgs := compactMessages("SYS", "did work", true)
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Fatalf("folded mode must produce a single user message, got %+v", msgs)
	}
	if !strings.Contains(msgs[0].Content, "SYS") || !strings.Contains(msgs[0].Content, "did work") {
		t.Errorf("folded message must include system + summary: %q", msgs[0].Content)
	}
}

func TestCheckpointSummarize_ReturnsSummaryAndStats(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "Summary of progress.", StopReason: "end_turn", InputTokens: 100, OutputTokens: 25},
	}}
	e := NewExecutor(nil, nil, nil, "a1")
	route := router.RouteResult{Provider: prov, Model: "m"}
	msgs := []llm.Message{{Role: "system", Content: "s"}, {Role: "user", Content: "do work"}}

	summary, stats, err := e.checkpointSummarize(context.Background(), route, msgs)
	if err != nil {
		t.Fatalf("checkpointSummarize: %v", err)
	}
	if summary != "Summary of progress." {
		t.Errorf("summary = %q", summary)
	}
	if stats.inputTokens != 100 || stats.outputTokens != 25 {
		t.Errorf("stats not accumulated: %+v", stats)
	}
}

func TestDoCheckpoint_PersistsAndCompacts(t *testing.T) {
	var created atomic.Int32
	var capturedRound int
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/agent-sessions", func(w http.ResponseWriter, r *http.Request) {
		created.Add(1)
		var s db.AgentSession
		_ = json.NewDecoder(r.Body).Decode(&s)
		capturedRound = s.Round
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(&s)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	prov := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "Checkpoint summary.", StopReason: "end_turn", InputTokens: 40, OutputTokens: 10},
	}}
	e := NewExecutor(nil, nil, NewServerClient(srv.URL), "a1")
	route := router.RouteResult{Provider: prov, Model: "m"}
	task := &db.Task{ID: "t1"}
	pre := []llm.Message{
		{Role: "system", Content: "SYS"},
		{Role: "user", Content: "do"},
		{Role: "assistant", Content: "working"},
	}
	stats := execStats{}

	out := e.doCheckpoint(context.Background(), e.log.ForTask("t1"), route, task, pre, "SYS", 6, &stats, "context_pressure")

	if created.Load() != 1 {
		t.Errorf("expected one persisted session, got %d", created.Load())
	}
	if capturedRound != 6 {
		t.Errorf("persisted round = %d, want 6", capturedRound)
	}
	if len(out) != 2 || out[0].Role != "system" {
		t.Fatalf("expected compacted [system, user], got %+v", out)
	}
	if !strings.Contains(out[1].Content, "Checkpoint summary.") {
		t.Errorf("compacted history must seed the summary: %+v", out[1])
	}
	if stats.inputTokens != 40 || stats.outputTokens != 10 {
		t.Errorf("summarization stats not folded into task stats: %+v", stats)
	}
}

func TestDoCheckpoint_SummarizeErrorKeepsMessages(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "", StopReason: "end_turn"}, // empty summary → error
	}}
	e := NewExecutor(nil, nil, nil, "a1")
	route := router.RouteResult{Provider: prov, Model: "m"}
	pre := []llm.Message{{Role: "system", Content: "SYS"}, {Role: "user", Content: "do"}}

	out := e.doCheckpoint(context.Background(), e.log.ForTask("t1"), route, &db.Task{ID: "t1"}, pre, "SYS", 1, &execStats{}, "context_pressure")
	if len(out) != len(pre) {
		t.Errorf("on summarize error, messages must be unchanged; got %d want %d", len(out), len(pre))
	}
}
