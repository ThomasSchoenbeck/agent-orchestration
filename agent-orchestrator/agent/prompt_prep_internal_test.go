package agent

import (
	"context"
	"strings"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

func TestComposePromptLayers_OrdersAndOmits(t *testing.T) {
	got := composePromptLayers(PromptLayers{
		Role: "role prompt",
		Task: "the task",
		// Agent/Subagent/Provider/Model intentionally empty → omitted.
	})
	if !strings.Contains(got, "[ROLE]") || !strings.Contains(got, "role prompt") {
		t.Errorf("expected ROLE layer present, got:\n%s", got)
	}
	if !strings.Contains(got, "[TASK]") || !strings.Contains(got, "the task") {
		t.Errorf("expected TASK layer present, got:\n%s", got)
	}
	if strings.Contains(got, "[AGENT]") || strings.Contains(got, "[PROVIDER]") || strings.Contains(got, "[MODEL]") {
		t.Errorf("empty layers must be omitted, got:\n%s", got)
	}
	// Stable order: ROLE precedes TASK.
	if strings.Index(got, "[ROLE]") > strings.Index(got, "[TASK]") {
		t.Errorf("layers out of order:\n%s", got)
	}
}

func TestPromptPrepInstructions_RollsForwardWithPrior(t *testing.T) {
	layers := composePromptLayers(PromptLayers{Role: "r"})

	first := promptPrepInstructions(layers, &priorRound{})
	if strings.Contains(first, "last round") {
		t.Errorf("first round must not reference a prior round:\n%s", first)
	}

	rolled := promptPrepInstructions(layers, &priorRound{prompt: "PREV-PROMPT", result: "PREV-RESULT"})
	if !strings.Contains(rolled, "PREV-PROMPT") || !strings.Contains(rolled, "PREV-RESULT") {
		t.Errorf("roll-forward must include prior prompt + result:\n%s", rolled)
	}
}

func TestPreparePrompt_SwapsSystemAndFoldsStats(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "SYNTHESIZED", InputTokens: 3, OutputTokens: 4},
	}}
	e := NewExecutor(nil, nil, nil, "a1")
	e.subagentSkills = []*db.SubagentSkill{{
		Name: "prompt_prep", Enabled: true, MaxRounds: 1,
		PromptTemplate: "Prep. Inputs:\n{{instructions}}",
	}}
	e.subagentSkillsResolved = true

	route := router.RouteResult{Provider: prov, Model: "m"}
	messages := []llm.Message{
		{Role: "system", Content: "BASE"},
		{Role: "user", Content: "hi"},
	}
	stats := execStats{}
	out := e.preparePrompt(context.Background(), e.log.ForTask("t1"), route, "t1", "s1", 0,
		messages, PromptLayers{Role: "BASE", Task: "do X"}, &priorRound{}, &stats)

	if out[0].Content != "SYNTHESIZED" {
		t.Errorf("messages[0] not swapped: %q", out[0].Content)
	}
	if out[1].Content != "hi" {
		t.Errorf("history after messages[0] must be preserved, got %q", out[1].Content)
	}
	if stats.inputTokens != 3 || stats.outputTokens != 4 {
		t.Errorf("synthesis stats not folded: in=%d out=%d", stats.inputTokens, stats.outputTokens)
	}
	if prov.calls != 1 {
		t.Errorf("expected 1 synthesis call, got %d", prov.calls)
	}
}

func TestPreparePrompt_NoopWhenSkillAbsent(t *testing.T) {
	prov := &scriptedProvider{}
	e := NewExecutor(nil, nil, nil, "a1")
	e.subagentSkills = nil
	e.subagentSkillsResolved = true

	route := router.RouteResult{Provider: prov, Model: "m"}
	messages := []llm.Message{{Role: "system", Content: "BASE"}, {Role: "user", Content: "hi"}}
	stats := execStats{}
	out := e.preparePrompt(context.Background(), e.log.ForTask("t1"), route, "t1", "s1", 0,
		messages, PromptLayers{Role: "BASE"}, &priorRound{}, &stats)

	if out[0].Content != "BASE" {
		t.Errorf("expected no-op when prompt_prep absent, got %q", out[0].Content)
	}
	if prov.calls != 0 {
		t.Errorf("expected no synthesis call, got %d", prov.calls)
	}
}

func TestPreparePrompt_NoopInFoldMode(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.ChatResponse{{Content: "X"}}}
	e := NewExecutor(nil, nil, nil, "a1")
	e.subagentSkills = []*db.SubagentSkill{{
		Name: "prompt_prep", Enabled: true, MaxRounds: 1, PromptTemplate: "P {{instructions}}",
	}}
	e.subagentSkillsResolved = true

	route := router.RouteResult{Provider: prov, Model: "m"}
	messages := []llm.Message{{Role: "user", Content: "folded system + user"}}
	stats := execStats{}
	out := e.preparePrompt(context.Background(), e.log.ForTask("t1"), route, "t1", "s1", 0,
		messages, PromptLayers{Role: "BASE"}, &priorRound{}, &stats)

	if out[0].Content != "folded system + user" {
		t.Errorf("expected fold-mode no-op, got %q", out[0].Content)
	}
	if prov.calls != 0 {
		t.Errorf("expected no synthesis call in fold mode, got %d", prov.calls)
	}
}

func TestSubagentLoop_RunsPromptPrepEachRound(t *testing.T) {
	var readCalls int
	reg := subagentTestRegistry(t, &readCalls)
	prov := &scriptedProvider{responses: []llm.ChatResponse{
		// Call 1: prompt_prep synthesis before the subagent's round 0.
		{Content: "SYNTH", InputTokens: 1, OutputTokens: 1},
		// Call 2: the subagent's round 0 — no tool calls → returns the summary.
		{Content: "final summary", StopReason: "end_turn", InputTokens: 2, OutputTokens: 2},
	}}
	e := NewExecutor(nil, reg, nil, "a1")
	e.subagentSkills = []*db.SubagentSkill{{
		Name: "prompt_prep", Enabled: true, MaxRounds: 1, PromptTemplate: "Prep {{instructions}}",
	}}
	e.subagentSkillsResolved = true

	route := router.RouteResult{Provider: prov, Model: "m", Role: "worker"}
	skill := &db.SubagentSkill{
		Name: "investigate_codebase", Enabled: true,
		ToolAllowlist: []string{"read_file"}, MaxRounds: 5,
		PromptTemplate: "Investigate: {{instructions}}.",
	}

	summary, _, err := e.runSubagent(context.Background(), e.log.ForTask("t1"), route, "/work/t1", skill, "go")
	if err != nil {
		t.Fatalf("runSubagent: %v", err)
	}
	if summary != "final summary" {
		t.Errorf("summary = %q", summary)
	}
	// 2 calls proves prompt_prep ran before the subagent's round (1 synth + 1 round).
	if prov.calls != 2 {
		t.Errorf("expected 2 LLM calls (prompt_prep + subagent round), got %d", prov.calls)
	}
}
