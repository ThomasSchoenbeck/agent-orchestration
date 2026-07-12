package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

// TestDispatchSubagent_PersistsLinkedChildSession (T3.2) verifies that a subagent
// dispatched from the main loop is persisted as a tracked child AgentSession
// parented to the spawning session, so it appears in the task's session tree.
func TestDispatchSubagent_PersistsLinkedChildSession(t *testing.T) {
	var readCalls int
	reg := subagentTestRegistry(t, &readCalls)

	var mu sync.Mutex
	var captured []*db.AgentSession
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/agent-sessions", func(w http.ResponseWriter, r *http.Request) {
		var s db.AgentSession
		_ = json.NewDecoder(r.Body).Decode(&s)
		mu.Lock()
		captured = append(captured, &s)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(s)
	})
	// Internal (package agent) tests register the already-rewritten /api/agent/…
	// path directly, so no wrapAgentAPI wrapper is needed (that helper lives in the
	// external agent_test package).
	srv := httptest.NewServer(mux)
	defer srv.Close()

	prov := &scriptedProvider{responses: []llm.ChatResponse{
		// round 0: the subagent reads a file (tool call → loop continues).
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "read_file", Arguments: map[string]interface{}{"file_path": "main.go"}}},
			StopReason: "tool_use", InputTokens: 30, OutputTokens: 5},
		// round 1: the subagent summarizes (no tool calls → done).
		{Content: "Findings summary.", StopReason: "end_turn", InputTokens: 10, OutputTokens: 5},
	}}

	e := NewExecutor(nil, reg, NewServerClient(srv.URL), "agent-1")
	e.subagentSkills = []*db.SubagentSkill{{
		Name: "investigate_codebase", Enabled: true,
		ToolAllowlist: []string{"read_file"}, MaxRounds: 5,
		PromptTemplate: "Investigate {{instructions}}.",
	}}
	e.subagentSkillsResolved = true

	route := router.RouteResult{Provider: prov, Model: "m", Role: "worker"}
	sess := newSession(SessionKindMain, "t1", "agent-1", route)
	task := &db.Task{ID: "t1", WorktreePath: "/work/t1"}
	tc := llm.ToolCall{Name: "run_subagent", Arguments: map[string]interface{}{
		"skill": "investigate_codebase", "instructions": "find X",
	}}

	out := e.dispatchSubagent(context.Background(), e.log.ForTask("t1"), route, sess, task, tc, &execStats{})
	if !strings.Contains(out, "Findings summary.") {
		t.Fatalf("dispatch result missing summary: %s", out)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("expected 1 persisted subagent session, got %d", len(captured))
	}
	c := captured[0]
	if c.ParentID != sess.ID {
		t.Errorf("child parent_id = %q, want spawning session %q", c.ParentID, sess.ID)
	}
	if c.Kind != string(SessionKindDiscovery) {
		t.Errorf("child kind = %q, want %q (investigate_codebase → discovery)", c.Kind, SessionKindDiscovery)
	}
	if c.Status != string(SessionStatusDone) {
		t.Errorf("child status = %q, want done", c.Status)
	}
	if c.Title != "investigate_codebase" {
		t.Errorf("child title = %q, want investigate_codebase", c.Title)
	}
	if c.Summary != "Findings summary." {
		t.Errorf("child summary = %q", c.Summary)
	}
	if c.TaskID != "t1" {
		t.Errorf("child task_id = %q, want t1", c.TaskID)
	}
	// The child's own cost/tokens reflect the subagent run (not the parent's).
	if c.Cost < 0 {
		t.Errorf("child cost should be non-negative, got %v", c.Cost)
	}
}

// TestSubagentSessionKind maps skills to session-tree kinds (T3.2).
func TestSubagentSessionKind(t *testing.T) {
	cases := map[string]SessionKind{
		"investigate_codebase": SessionKindDiscovery,
		"task_status":          SessionKindTaskStatus,
		"code_subtask":         SessionKindWork,
		"review_subtask":       SessionKindWork,
		"anything_else":        SessionKindWork,
	}
	for name, want := range cases {
		if got := subagentSessionKind(name); got != want {
			t.Errorf("subagentSessionKind(%q) = %q, want %q", name, got, want)
		}
	}
}
