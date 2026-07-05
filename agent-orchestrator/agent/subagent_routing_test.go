package agent

import (
	"context"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

// A subagent carrying its own provider>model priority list routes through it,
// ignoring the spawning session's route (Phase 5, T5.5 subagent side).
func TestRunSubagent_RoutesViaOwnModels(t *testing.T) {
	var readCalls int
	toolReg := subagentTestRegistry(t, &readCalls)

	subProv := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "sub summary", StopReason: "end_turn", InputTokens: 5, OutputTokens: 5},
	}}
	llmReg := llm.NewRegistry()
	if err := llmReg.Register("subprov", subProv); err != nil {
		t.Fatalf("register: %v", err)
	}
	rtr := router.New(&config.Config{}, llmReg)
	if err := rtr.LoadFromData(
		[]*db.Provider{{
			ID: "sp", Name: "subprov", Enabled: true,
			Models: []db.ProviderModel{{Name: "subm"}},
			Config: map[string]interface{}{},
		}},
		nil,
	); err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}
	e := NewExecutor(rtr, toolReg, nil, "a1")

	// The spawning route points at a DIFFERENT provider that must NOT be used.
	spawnProv := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "spawn (should not be used)", StopReason: "end_turn"},
	}}
	spawnRoute := router.RouteResult{Provider: spawnProv, Model: "spawn-m"}

	skill := &db.SubagentSkill{
		Name: "code_subtask", Enabled: true,
		ToolAllowlist: []string{"read_file"}, MaxRounds: 3,
		Models: []db.ModelRef{{Provider: "subprov", Model: "subm"}},
	}

	summary, _, err := e.runSubagent(context.Background(), e.log.ForTask("t1"), spawnRoute, "/work/t1", skill, "go")
	if err != nil {
		t.Fatalf("runSubagent: %v", err)
	}
	if summary != "sub summary" {
		t.Errorf("summary = %q, want the subagent-routed provider's response", summary)
	}
	if spawnProv.calls != 0 {
		t.Errorf("spawning provider must not be used, got %d calls", spawnProv.calls)
	}
	if subProv.calls != 1 {
		t.Errorf("subagent-routed provider should be used once, got %d", subProv.calls)
	}
}

// With no priority list, a subagent reuses the spawning session's route (current
// behaviour preserved).
func TestRunSubagent_NoModelsReusesSpawnRoute(t *testing.T) {
	var readCalls int
	toolReg := subagentTestRegistry(t, &readCalls)

	spawnProv := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "done", StopReason: "end_turn"},
	}}
	// Router present but the skill has no Models → must fall through to spawnRoute.
	rtr := router.New(&config.Config{}, llm.NewRegistry())
	e := NewExecutor(rtr, toolReg, nil, "a1")

	spawnRoute := router.RouteResult{Provider: spawnProv, Model: "spawn-m"}
	skill := &db.SubagentSkill{Name: "investigate_codebase", Enabled: true,
		ToolAllowlist: []string{"read_file"}, MaxRounds: 3}

	summary, _, err := e.runSubagent(context.Background(), e.log.ForTask("t1"), spawnRoute, "/work/t1", skill, "go")
	if err != nil {
		t.Fatalf("runSubagent: %v", err)
	}
	if summary != "done" || spawnProv.calls != 1 {
		t.Errorf("expected spawn route used once, got summary=%q calls=%d", summary, spawnProv.calls)
	}
}
