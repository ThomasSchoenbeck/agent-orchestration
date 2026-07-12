package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
	"agent-orchestrator/tools"
)

func TestBuildUserMessage_IncludesProjectID(t *testing.T) {
	e := &Executor{}
	task := &db.Task{
		ID:           "task-1",
		ProjectID:    "proj-9",
		Role:         "orchestrator",
		WorktreePath: "/work/task-1", // set so buildUserMessage doesn't touch e.log
		Payload:      map[string]interface{}{"title": "Re-sync project scope"},
	}
	msg := e.buildUserMessage(task)
	if !strings.Contains(msg, "Project ID: proj-9") {
		t.Errorf("user message must include the project id; got:\n%s", msg)
	}
	if !strings.Contains(msg, "Re-sync project scope") {
		t.Errorf("user message must include the title; got:\n%s", msg)
	}
}

// scriptedProvider returns a fixed sequence of responses, one per Chat call.
type scriptedProvider struct {
	responses []llm.ChatResponse
	calls     int
}

func (s *scriptedProvider) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	if s.calls >= len(s.responses) {
		return llm.ChatResponse{Content: "(no more scripted responses)", StopReason: "end_turn"}, nil
	}
	r := s.responses[s.calls]
	s.calls++
	return r, nil
}
func (s *scriptedProvider) Embed(_ context.Context, _ llm.EmbedRequest) (llm.EmbedResponse, error) {
	return llm.EmbedResponse{}, nil
}
func (s *scriptedProvider) Rerank(_ context.Context, _ llm.RerankRequest) (llm.RerankResponse, error) {
	return llm.RerankResponse{}, nil
}
func (s *scriptedProvider) Name() string { return "scripted" }
func (s *scriptedProvider) Close() error { return nil }

// subagentTestRegistry returns a registry with a fake read_file tool (recording
// call count) plus the real run_subagent tool registered.
func subagentTestRegistry(t *testing.T, readCalls *int) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	err := reg.Register(tools.Definition{
		Name:       "read_file",
		Parameters: map[string]tools.Param{"file_path": {Type: "string"}},
		Required:   []string{"file_path"},
		Handler: func(_ context.Context, _ map[string]interface{}) (interface{}, error) {
			*readCalls++
			return map[string]string{"content": "package main"}, nil
		},
	})
	if err != nil {
		t.Fatalf("register read_file: %v", err)
	}
	if err := tools.RegisterSubagentTool(reg); err != nil {
		t.Fatalf("register run_subagent: %v", err)
	}
	return reg
}

func TestRunSubagent_NestedLoopReturnsSummary(t *testing.T) {
	var readCalls int
	reg := subagentTestRegistry(t, &readCalls)
	prov := &scriptedProvider{responses: []llm.ChatResponse{
		// Round 0: subagent reads a file.
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "read_file", Arguments: map[string]interface{}{"file_path": "main.go"}}},
			StopReason: "tool_use", InputTokens: 50, OutputTokens: 10, TokensUsed: 60},
		// Round 1: subagent summarizes (no tool calls → done).
		{Content: "Findings: main.go is the entrypoint.", StopReason: "end_turn", InputTokens: 30, OutputTokens: 20, TokensUsed: 50},
	}}
	e := NewExecutor(nil, reg, nil, "agent-1")
	route := router.RouteResult{Provider: prov, Model: "m", Role: "worker"}
	skill := &db.SubagentSkill{
		Name: "investigate_codebase", Enabled: true,
		ToolAllowlist: []string{"read_file"}, MaxRounds: 5,
		PromptTemplate: "Investigate: {{instructions}}. Then summarize.",
	}

	summary, stats, err := e.runSubagent(context.Background(), e.log.ForTask("t1"), route, "/work/t1", skill, "find the entrypoint")
	if err != nil {
		t.Fatalf("runSubagent: %v", err)
	}
	if summary != "Findings: main.go is the entrypoint." {
		t.Errorf("summary = %q", summary)
	}
	if readCalls != 1 {
		t.Errorf("expected read_file called once, got %d", readCalls)
	}
	if stats.inputTokens != 80 || stats.outputTokens != 30 {
		t.Errorf("subagent stats: in=%d out=%d, want in=80 out=30", stats.inputTokens, stats.outputTokens)
	}
}

func TestRunSubagent_RejectsToolOutsideAllowlist(t *testing.T) {
	var readCalls int
	reg := subagentTestRegistry(t, &readCalls)
	// Subagent tries to spawn another subagent — must be blocked (no nesting),
	// then summarizes.
	prov := &scriptedProvider{responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "run_subagent", Arguments: map[string]interface{}{"skill": "x", "instructions": "y"}}},
			StopReason: "tool_use", InputTokens: 10, OutputTokens: 5},
		{Content: "Done.", StopReason: "end_turn", InputTokens: 10, OutputTokens: 5},
	}}
	e := NewExecutor(nil, reg, nil, "agent-1")
	route := router.RouteResult{Provider: prov, Model: "m", Role: "worker"}
	skill := &db.SubagentSkill{Name: "investigate_codebase", Enabled: true, ToolAllowlist: []string{"read_file"}, MaxRounds: 5}

	summary, _, err := e.runSubagent(context.Background(), e.log.ForTask("t1"), route, "/work/t1", skill, "do it")
	if err != nil {
		t.Fatalf("runSubagent: %v", err)
	}
	if summary != "Done." {
		t.Errorf("summary = %q", summary)
	}
	// The second response (summary) confirms the loop continued after rejecting
	// the disallowed call rather than crashing.
}

func TestRunSubagent_CodingSubagentWritesButCannotNest(t *testing.T) {
	var writeCalls int
	reg := tools.NewRegistry()
	if err := reg.Register(tools.Definition{
		Name:       "write_file",
		Parameters: map[string]tools.Param{"file_path": {Type: "string"}, "content": {Type: "string"}},
		Required:   []string{"file_path", "content"},
		Handler: func(_ context.Context, _ map[string]interface{}) (interface{}, error) {
			writeCalls++
			return map[string]string{"status": "written"}, nil
		},
	}); err != nil {
		t.Fatalf("register write_file: %v", err)
	}
	if err := tools.RegisterSubagentTool(reg); err != nil {
		t.Fatalf("register run_subagent: %v", err)
	}

	prov := &scriptedProvider{responses: []llm.ChatResponse{
		// Round 0: legitimate write.
		{ToolCalls: []llm.ToolCall{{ID: "w", Name: "write_file", Arguments: map[string]interface{}{"file_path": "a.go", "content": "x"}}}, StopReason: "tool_use", InputTokens: 5, OutputTokens: 5},
		// Round 1: attempt to nest — must be rejected.
		{ToolCalls: []llm.ToolCall{{ID: "n", Name: "run_subagent", Arguments: map[string]interface{}{"skill": "x", "instructions": "y"}}}, StopReason: "tool_use", InputTokens: 5, OutputTokens: 5},
		// Round 2: summary.
		{Content: "Changed a.go.", StopReason: "end_turn", InputTokens: 5, OutputTokens: 5},
	}}
	e := NewExecutor(nil, reg, nil, "agent-1")
	route := router.RouteResult{Provider: prov, Model: "m", Role: "worker"}
	skill := &db.SubagentSkill{
		Name: "code_subtask", Enabled: true,
		ToolAllowlist: []string{"read_file", "write_file", "apply_diff", "list_files", "run_tests"},
		MaxRounds:     6,
	}

	summary, _, err := e.runSubagent(context.Background(), e.log.ForTask("t1"), route, "/work/t1", skill, "edit a.go")
	if err != nil {
		t.Fatalf("runSubagent: %v", err)
	}
	if writeCalls != 1 {
		t.Errorf("expected write_file called once, got %d", writeCalls)
	}
	if summary != "Changed a.go." {
		t.Errorf("summary = %q", summary)
	}
	// run_subagent is not in the allowlist, so the nesting attempt was rejected
	// and the loop continued to a summary (proven by reaching round 2).
}

func TestRunSubagent_MaxRoundsBounded(t *testing.T) {
	var readCalls int
	reg := subagentTestRegistry(t, &readCalls)
	// Model keeps calling tools forever; loop must stop at MaxRounds.
	loop := llm.ChatResponse{
		Content:    "still working",
		ToolCalls:  []llm.ToolCall{{ID: "c", Name: "read_file", Arguments: map[string]interface{}{"file_path": "a.go"}}},
		StopReason: "tool_use", InputTokens: 5, OutputTokens: 5,
	}
	prov := &scriptedProvider{responses: []llm.ChatResponse{loop, loop, loop, loop, loop}}
	e := NewExecutor(nil, reg, nil, "agent-1")
	route := router.RouteResult{Provider: prov, Model: "m", Role: "worker"}
	skill := &db.SubagentSkill{Name: "x", Enabled: true, ToolAllowlist: []string{"read_file"}, MaxRounds: 2}

	summary, _, err := e.runSubagent(context.Background(), e.log.ForTask("t1"), route, "/work/t1", skill, "go")
	if err != nil {
		t.Fatalf("runSubagent: %v", err)
	}
	if prov.calls != 2 {
		t.Errorf("expected exactly MaxRounds=2 LLM calls, got %d", prov.calls)
	}
	if summary != "still working" {
		t.Errorf("expected last assistant content as summary, got %q", summary)
	}
}

func TestDispatchSubagent_FoldsStatsAndReturnsSummaryOnly(t *testing.T) {
	var readCalls int
	reg := subagentTestRegistry(t, &readCalls)
	prov := &scriptedProvider{responses: []llm.ChatResponse{
		{Content: "Summary only.", StopReason: "end_turn", InputTokens: 40, OutputTokens: 15},
	}}
	e := NewExecutor(nil, reg, nil, "agent-1")
	// Pre-populate the subagent skill cache (skip the HTTP fetch).
	e.subagentSkills = []*db.SubagentSkill{{
		Name: "investigate_codebase", Enabled: true,
		ToolAllowlist: []string{"read_file"}, MaxRounds: 5,
		PromptTemplate: "Investigate {{instructions}}.",
	}}
	e.subagentSkillsResolved = true

	route := router.RouteResult{Provider: prov, Model: "m", Role: "worker"}
	task := &db.Task{ID: "t1", WorktreePath: "/work/t1"}
	tc := llm.ToolCall{ID: "x", Name: "run_subagent", Arguments: map[string]interface{}{
		"skill": "investigate_codebase", "instructions": "find X",
	}}
	stats := execStats{}

	sess := newSession(SessionKindMain, "t1", "agent-1", route)
	out := e.dispatchSubagent(context.Background(), e.log.ForTask("t1"), route, sess, task, tc, &stats)

	// Token isolation: the task stats include the subagent usage.
	if stats.inputTokens != 40 || stats.outputTokens != 15 {
		t.Errorf("task stats not folded: in=%d out=%d", stats.inputTokens, stats.outputTokens)
	}
	// The tool result is only the summary, never the subagent transcript.
	if !strings.Contains(out, "Summary only.") {
		t.Errorf("dispatch result missing summary: %s", out)
	}
	if strings.Contains(out, "Investigate") {
		t.Errorf("dispatch result leaked the subagent prompt/transcript: %s", out)
	}
}

func TestDispatchSubagent_UnknownSkillReturnsError(t *testing.T) {
	var readCalls int
	reg := subagentTestRegistry(t, &readCalls)
	e := NewExecutor(nil, reg, nil, "agent-1")
	e.subagentSkills = nil
	e.subagentSkillsResolved = true // resolved but empty → lookup fails

	route := router.RouteResult{Provider: &scriptedProvider{}, Model: "m", Role: "worker"}
	task := &db.Task{ID: "t1"}
	tc := llm.ToolCall{Name: "run_subagent", Arguments: map[string]interface{}{"skill": "nope", "instructions": "x"}}
	sess := newSession(SessionKindMain, "t1", "agent-1", route)
	out := e.dispatchSubagent(context.Background(), e.log.ForTask("t1"), route, sess, task, tc, &execStats{})
	if !strings.Contains(out, "error") || !strings.Contains(out, "nope") {
		t.Errorf("expected error for unknown skill, got %s", out)
	}
}

func TestResolveSubagentSkills_CachesEnabled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/subagent-skills", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]*db.SubagentSkill{
			{Name: "investigate_codebase", Enabled: true, ToolAllowlist: []string{"read_file"}},
			{Name: "disabled_one", Enabled: false},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := NewExecutor(nil, nil, NewServerClient(srv.URL), "agent-1")
	got := e.lookupSubagentSkill(context.Background(), "investigate_codebase")
	if got == nil {
		t.Fatal("expected investigate_codebase to resolve")
	}
	if e.lookupSubagentSkill(context.Background(), "disabled_one") != nil {
		t.Error("disabled subagent skill must not be returned")
	}
	if e.lookupSubagentSkill(context.Background(), "nope") != nil {
		t.Error("unknown subagent skill must return nil")
	}
}

func TestResolveAgentSystemPrompt_FetchesAndCaches(t *testing.T) {
	var calls int
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/agents/agent-1", func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(&db.Agent{ID: "agent-1", Name: "w", SystemPrompt: "be careful"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := NewExecutor(nil, nil, NewServerClient(srv.URL), "agent-1")
	if got := e.resolveAgentSystemPrompt(context.Background()); got != "be careful" {
		t.Fatalf("system prompt = %q, want %q", got, "be careful")
	}
	// Second call is served from cache (no extra HTTP request).
	if got := e.resolveAgentSystemPrompt(context.Background()); got != "be careful" {
		t.Errorf("cached system prompt = %q, want %q", got, "be careful")
	}
	if calls != 1 {
		t.Errorf("agent record fetched %d times, want 1 (cached)", calls)
	}
}

func TestResolveAgentSystemPrompt_ToleratesFetchError(t *testing.T) {
	mux := http.NewServeMux() // no handler → 404
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := NewExecutor(nil, nil, NewServerClient(srv.URL), "missing")
	if got := e.resolveAgentSystemPrompt(context.Background()); got != "" {
		t.Errorf("on fetch error, system prompt = %q, want empty", got)
	}
	// No client → empty, no panic.
	e2 := NewExecutor(nil, nil, nil, "x")
	if got := e2.resolveAgentSystemPrompt(context.Background()); got != "" {
		t.Errorf("nil client system prompt = %q, want empty", got)
	}
}

func TestDefaultToolsForRole_IncludeRunSubagent(t *testing.T) {
	for _, role := range []string{"worker", "reviewer", "orchestrator"} {
		tset := map[string]bool{}
		for _, tool := range defaultToolsForRole(role) {
			tset[tool] = true
		}
		if !tset["run_subagent"] {
			t.Errorf("role %q default tools missing run_subagent", role)
		}
		if !tset["checkpoint_session"] {
			t.Errorf("role %q default tools missing checkpoint_session", role)
		}
	}
}

func TestSubagentPromptFragment_MentionsTool(t *testing.T) {
	if !strings.Contains(subagentPromptFragment, "run_subagent") {
		t.Error("subagent prompt fragment must mention run_subagent")
	}
	if !strings.Contains(subagentPromptFragment, "investigate_codebase") {
		t.Error("subagent prompt fragment should reference the investigate_codebase skill")
	}
}

func TestDefaultToolsForRole_WorkerReviewerOrchestrationOnly(t *testing.T) {
	for _, role := range []string{"worker", "reviewer"} {
		set := map[string]bool{}
		for _, tool := range defaultToolsForRole(role) {
			set[tool] = true
		}
		for _, want := range []string{"run_subagent", "read_memory", "write_memory", "checkpoint_session"} {
			if !set[want] {
				t.Errorf("role %q must expose orchestration tool %q", role, want)
			}
		}
		// Work tools moved to the subagents — must NOT be on the main session.
		for _, forbidden := range []string{"read_file", "write_file", "apply_diff", "run_tests", "list_files"} {
			if set[forbidden] {
				t.Errorf("role %q must not expose work tool %q (it lives on the work subagent)", role, forbidden)
			}
		}
	}
}

func TestDefaultToolsForRole_OrchestratorHasScopeTools(t *testing.T) {
	got := defaultToolsForRole("orchestrator")
	set := make(map[string]bool, len(got))
	for _, name := range got {
		set[name] = true
	}
	// The orchestrator runs project re-sync: it must be able to reconcile scope
	// (requirements/features) and create work packages.
	for _, want := range []string{"bootstrap_project", "sync_scope", "create_work_package"} {
		if !set[want] {
			t.Errorf("orchestrator default tools missing %q; got %v", want, got)
		}
	}
}

func TestIsPlannerTask(t *testing.T) {
	reg := llm.NewRegistry()
	reg.Set("p", llm.NewOllamaProvider("p", "http://localhost:11434", "m"))
	reg.SetRoles("p", "m", []string{"orchestrator", "worker"}) // provider-level role preference
	rtr := router.New(&config.Config{}, reg)
	providers := []*db.Provider{{
		Name: "p", ModelName: "m", Enabled: true,
		Roles:  []string{"orchestrator", "worker"},
		Models: []db.ProviderModel{{Name: "m"}},
	}}
	roles := []*db.RoleDefinition{
		{Name: "orchestrator", Enabled: true, Capabilities: []string{"creates_tasks"}},
		{Name: "worker", Enabled: true},
	}
	if err := rtr.LoadFromData(providers, roles); err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}
	e := &Executor{rtr: rtr}

	if !e.IsPlannerTask(&db.Task{Role: "orchestrator"}) {
		t.Error("orchestrator (creates_tasks) should be a planner task")
	}
	if e.IsPlannerTask(&db.Task{Role: "worker"}) {
		t.Error("worker should not be a planner task")
	}
}

func TestPlanningContext_IncludesDescriptionAndScope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/projects/p1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "p1", "description": "Track expenses with login"})
	})
	mux.HandleFunc("/api/agent/projects/p1/requirements", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "r1", "title": "User login", "status": "accepted"}})
	})
	mux.HandleFunc("/api/agent/projects/p1/features", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "f1", "title": "Reports", "status": "planned"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := &Executor{client: NewServerClient(srv.URL)}
	pc := e.planningContext(context.Background(), "p1")
	for _, want := range []string{"Track expenses with login", "User login", "Reports"} {
		if !strings.Contains(pc, want) {
			t.Errorf("planning context missing %q; got:\n%s", want, pc)
		}
	}
}
