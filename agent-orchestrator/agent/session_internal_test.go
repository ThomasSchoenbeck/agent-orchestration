package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestShouldCheckpointNow_HonoursOverride (T7.1): the executor-scoped check uses
// the configured threshold when set, and the 0.80 default otherwise.
func TestShouldCheckpointNow_HonoursOverride(t *testing.T) {
	e := &Executor{} // no override → 0.80 default
	if e.shouldCheckpointNow(700, 1000) {
		t.Error("70% < 0.80 default: should not checkpoint")
	}
	if !e.shouldCheckpointNow(850, 1000) {
		t.Error("85% >= 0.80 default: should checkpoint")
	}

	e.contextThresholdFraction = 0.5
	if !e.shouldCheckpointNow(600, 1000) {
		t.Error("60% >= 0.50 override: should checkpoint")
	}
	if e.shouldCheckpointNow(400, 1000) {
		t.Error("40% < 0.50 override: should not checkpoint")
	}

	e.contextThresholdFraction = 5 // out of range → default 0.80
	if e.shouldCheckpointNow(700, 1000) {
		t.Error("out-of-range override should fall back to the 0.80 default")
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

// sessionSink captures persisted AgentSession rows from the mock server.
type sessionSink struct {
	mu   sync.Mutex
	rows []db.AgentSession
}

func (s *sessionSink) handler(w http.ResponseWriter, r *http.Request) {
	var row db.AgentSession
	_ = json.NewDecoder(r.Body).Decode(&row)
	s.mu.Lock()
	s.rows = append(s.rows, row)
	s.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(&row)
}

func (s *sessionSink) snapshot() []db.AgentSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]db.AgentSession, len(s.rows))
	copy(out, s.rows)
	return out
}

func newSessionSinkServer(t *testing.T) (*sessionSink, string) {
	t.Helper()
	sink := &sessionSink{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/agent-sessions", sink.handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return sink, srv.URL
}

func TestDoCheckpoint_PersistsDoneNodeAndRollsOver(t *testing.T) {
	sink, url := newSessionSinkServer(t)

	prov := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "Checkpoint summary.", StopReason: "end_turn", InputTokens: 40, OutputTokens: 10},
	}}
	e := NewExecutor(nil, nil, NewServerClient(url), "a1")
	route := router.RouteResult{Provider: prov, Model: "m"}
	task := &db.Task{ID: "t1"}
	pre := []llm.Message{
		{Role: "system", Content: "SYS"},
		{Role: "user", Content: "do"},
		{Role: "assistant", Content: "working"},
	}
	sess := newSession(SessionKindMain, "t1", "a1", route)
	sess.Messages = pre
	firstID := sess.ID

	out := e.doCheckpoint(context.Background(), e.log.ForTask("t1"), route, task, sess, pre, "SYS", 6, "context_pressure")

	rows := sink.snapshot()
	if len(rows) != 1 {
		t.Fatalf("expected one persisted session, got %d", len(rows))
	}
	if rows[0].ID != firstID || rows[0].Round != 6 || rows[0].Status != "done" || rows[0].Kind != "main" {
		t.Errorf("persisted node = %+v (want id=%s round=6 status=done kind=main)", rows[0], firstID)
	}
	// The session rolled over to a fresh id linked to the persisted node.
	if sess.ID == firstID || sess.ParentID != firstID {
		t.Errorf("rollover wrong: id=%s parent=%s (want new id, parent=%s)", sess.ID, sess.ParentID, firstID)
	}
	if len(out) != 2 || out[0].Role != "system" || !strings.Contains(out[1].Content, "Checkpoint summary.") {
		t.Fatalf("expected seeded [system,user] carrying the summary, got %+v", out)
	}
	if sess.Stats.inputTokens != 40 || sess.Stats.outputTokens != 10 {
		t.Errorf("summarization stats not folded into session stats: %+v", sess.Stats)
	}
}

func TestDoCheckpoint_LinksSessionsAndSeedsMemory(t *testing.T) {
	sink, url := newSessionSinkServer(t)

	// Worktree with task memory the continuation must carry forward.
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, ".agent_context"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".agent_context", "memory.md"), []byte("REMEMBER: use option A"), 0o644); err != nil {
		t.Fatal(err)
	}

	prov := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "summary one", InputTokens: 10, OutputTokens: 5},
		{Content: "summary two", InputTokens: 10, OutputTokens: 5},
	}}
	e := NewExecutor(nil, nil, NewServerClient(url), "a1")
	route := router.RouteResult{Provider: prov, Model: "m"}
	task := &db.Task{ID: "t1", WorktreePath: wt}
	sess := newSession(SessionKindMain, "t1", "a1", route)
	id1 := sess.ID

	msgs := []llm.Message{{Role: "system", Content: "SYS"}, {Role: "user", Content: "do"}}
	out1 := e.doCheckpoint(context.Background(), e.log.ForTask("t1"), route, task, sess, msgs, "SYS", 3, "context_pressure")
	id2 := sess.ID
	_ = e.doCheckpoint(context.Background(), e.log.ForTask("t1"), route, task, sess, out1, "SYS", 6, "context_pressure")

	rows := sink.snapshot()
	if len(rows) != 2 {
		t.Fatalf("expected 2 persisted sessions across 2 checkpoints, got %d", len(rows))
	}
	if rows[0].ID != id1 || rows[0].ParentID != "" {
		t.Errorf("first session should be the root: %+v", rows[0])
	}
	if rows[1].ID != id2 || rows[1].ParentID != id1 {
		t.Errorf("second session must link to first: got id=%s parent=%s want id=%s parent=%s",
			rows[1].ID, rows[1].ParentID, id2, id1)
	}
	// The continuation seeded from the first checkpoint carries the worktree memory.
	if !strings.Contains(out1[len(out1)-1].Content, "REMEMBER: use option A") {
		t.Errorf("continuation must include task memory, got %+v", out1)
	}
}

func TestDoCheckpoint_SummarizeErrorKeepsMessages(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "", StopReason: "end_turn"}, // empty summary → error
	}}
	e := NewExecutor(nil, nil, nil, "a1")
	route := router.RouteResult{Provider: prov, Model: "m"}
	pre := []llm.Message{{Role: "system", Content: "SYS"}, {Role: "user", Content: "do"}}
	sess := newSession(SessionKindMain, "t1", "a1", route)

	out := e.doCheckpoint(context.Background(), e.log.ForTask("t1"), route, &db.Task{ID: "t1"}, sess, pre, "SYS", 1, "context_pressure")
	if len(out) != len(pre) {
		t.Errorf("on summarize error, messages must be unchanged; got %d want %d", len(out), len(pre))
	}
	// No rollover occurred (still the original session).
	if sess.ParentID != "" {
		t.Errorf("summarize error must not roll over the session, got parent=%s", sess.ParentID)
	}
}
