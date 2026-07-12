package agent

import (
	"context"
	"strings"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

// recordingProvider is a scripted provider that also captures each Chat request's
// messages, so a test can assert what the prompt_prep synthesis actually received
// (used to prove roll-forward).
type recordingProvider struct {
	responses []llm.ChatResponse
	reqs      [][]llm.Message
	calls     int
}

func (p *recordingProvider) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	p.reqs = append(p.reqs, req.Messages)
	if p.calls >= len(p.responses) {
		p.calls++
		return llm.ChatResponse{Content: "(no more scripted responses)", StopReason: "end_turn"}, nil
	}
	r := p.responses[p.calls]
	p.calls++
	return r, nil
}
func (p *recordingProvider) Embed(_ context.Context, _ llm.EmbedRequest) (llm.EmbedResponse, error) {
	return llm.EmbedResponse{}, nil
}
func (p *recordingProvider) Rerank(_ context.Context, _ llm.RerankRequest) (llm.RerankResponse, error) {
	return llm.RerankResponse{}, nil
}
func (p *recordingProvider) Name() string { return "recording" }
func (p *recordingProvider) Close() error { return nil }

// TestMultiSession_DelegatesWithPromptPrepEveryRoundAndRollsForward is an
// integration-style test (Phase 8, T8.1) that exercises, against stub providers,
// a full synchronous subagent delegation combining several multi-session
// behaviours in one flow:
//   - the parent delegates a tool-using subtask to a subagent and blocks for its
//     isolated summary (only the summary returns);
//   - prompt_prep synthesizes the system prompt before EVERY round; and
//   - each synthesis rolls forward from the prior round's prompt + result; while
//   - the parent folds the subagent's and prompt_prep's token stats.
//
// Scenario map for the remaining T8.1 items (covered by existing per-feature
// tests against stub providers): context exhaustion → new session
// (session_internal_test / recovery_test, T2.2), model failover
// (router/failover_test, T5.3/T5.5), worktree-deleted catch-up
// (recovery_test, T2.4).
func TestMultiSession_DelegatesWithPromptPrepEveryRoundAndRollsForward(t *testing.T) {
	var readCalls int
	reg := subagentTestRegistry(t, &readCalls)

	prov := &recordingProvider{responses: []llm.ChatResponse{
		// idx0: prompt_prep synthesis before the subagent's round 0.
		{Content: "SYNTH-0", InputTokens: 1, OutputTokens: 1},
		// idx1: subagent round 0 — makes progress AND calls a tool (loop continues).
		{Content: "partial progress",
			ToolCalls:  []llm.ToolCall{{ID: "c1", Name: "read_file", Arguments: map[string]interface{}{"file_path": "main.go"}}},
			StopReason: "tool_use", InputTokens: 10, OutputTokens: 2},
		// idx2: prompt_prep synthesis before round 1 — must roll forward.
		{Content: "SYNTH-1", InputTokens: 1, OutputTokens: 1},
		// idx3: subagent round 1 — no tool calls → returns the final summary.
		{Content: "final summary", StopReason: "end_turn", InputTokens: 5, OutputTokens: 3},
	}}

	e := NewExecutor(nil, reg, nil, "agent-1")
	e.subagentSkills = []*db.SubagentSkill{{
		Name: "prompt_prep", Enabled: true, MaxRounds: 1,
		PromptTemplate: "Prep. Inputs:\n{{instructions}}",
	}}
	e.subagentSkillsResolved = true

	route := router.RouteResult{Provider: prov, Model: "m", Role: "worker"}
	skill := &db.SubagentSkill{
		Name: "investigate_codebase", Enabled: true,
		ToolAllowlist:  []string{"read_file"}, MaxRounds: 5,
		PromptTemplate: "Investigate: {{instructions}}.",
	}

	summary, stats, err := e.runSubagent(context.Background(), e.log.ForTask("t1"), route, "/work/t1", skill, "find the entrypoint")
	if err != nil {
		t.Fatalf("runSubagent: %v", err)
	}

	// Synchronous delegation returns only the subagent's final summary.
	if summary != "final summary" {
		t.Errorf("summary = %q, want %q", summary, "final summary")
	}
	// The delegated tool ran inside the subagent.
	if readCalls != 1 {
		t.Errorf("read_file calls = %d, want 1", readCalls)
	}
	// 4 calls = (prompt_prep + subagent round) × 2 rounds → prompt_prep ran before
	// every round.
	if prov.calls != 4 {
		t.Fatalf("LLM calls = %d, want 4 (prompt_prep + subagent round, x2)", prov.calls)
	}
	// Roll-forward: the round-1 prompt_prep synthesis (recorded call idx 2) must
	// receive the prior round's synthesized prompt and the model's prior result.
	if len(prov.reqs) < 3 || len(prov.reqs[2]) == 0 {
		t.Fatalf("missing recorded request for the round-1 prompt_prep synthesis")
	}
	synth1 := prov.reqs[2][0].Content
	if !strings.Contains(synth1, "partial progress") {
		t.Errorf("round-1 synthesis missing prior result (roll-forward):\n%s", synth1)
	}
	if !strings.Contains(synth1, "SYNTH-0") {
		t.Errorf("round-1 synthesis missing prior prompt (roll-forward):\n%s", synth1)
	}
	// Stats fold across both subagent rounds AND both prompt_prep syntheses:
	// in = 1+10+1+5 = 17, out = 1+2+1+3 = 7.
	if stats.inputTokens != 17 || stats.outputTokens != 7 {
		t.Errorf("folded stats: in=%d out=%d, want in=17 out=7", stats.inputTokens, stats.outputTokens)
	}
}
