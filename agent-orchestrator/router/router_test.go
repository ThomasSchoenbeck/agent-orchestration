package router_test

import (
	"context"
	"path/filepath"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

func testConfig() *config.Config {
	return &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434"},
		},
		ContextRules: map[string]config.ContextRule{
			"worker": {
				Include: []string{"code", "diff", "test_results"},
				Exclude: []string{"docs"},
			},
			"reviewer": {
				Include: []string{"diff", "test_results"},
			},
		},
	}
}

func testRegistry(t *testing.T, cfg *config.Config) *llm.Registry {
	t.Helper()
	reg := llm.NewRegistry()
	_ = reg.Register("ollama",
		llm.NewOllamaProvider("ollama", "http://localhost:11434", "qwen2.5-coder:7b"))
	// Provider declares the roles it serves (routing resolves via this preference).
	reg.SetRoles("ollama", "qwen2.5-coder:7b", []string{"worker", "orchestrator", "reviewer"})
	return reg
}

// --- Router tests ---

func TestRouteByRole_Worker(t *testing.T) {
	cfg := testConfig()
	reg := testRegistry(t, cfg)
	r := router.New(cfg, reg)

	result, err := r.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	if result.Role != "worker" {
		t.Errorf("expected role worker, got %s", result.Role)
	}
	if result.Model != "qwen2.5-coder:7b" {
		t.Errorf("expected model qwen2.5-coder:7b, got %s", result.Model)
	}
	if result.Provider == nil {
		t.Error("expected non-nil provider")
	}
}

func TestRouteByRole_Unknown(t *testing.T) {
	cfg := testConfig()
	reg := testRegistry(t, cfg)
	r := router.New(cfg, reg)

	_, err := r.RouteByRole("nonexistent-role")
	if err == nil {
		t.Fatal("expected error for unknown role, got nil")
	}
}

func TestRouteByRole_ProviderRolePreferenceFallback(t *testing.T) {
	// Config has no mapping for "reviewer", but the registry has a provider
	// that declares it via role preferences.
	cfg := &config.Config{
		Providers: []config.ProviderConfig{},
	}
	reg := llm.NewRegistry()
	p := llm.NewOllamaProvider("local", "http://localhost:11434", "llama3")
	reg.Set("local", p)
	reg.SetRoles("local", "llama3", []string{"reviewer", "worker"})

	r := router.New(cfg, reg)
	result, err := r.RouteByRole("reviewer")
	if err != nil {
		t.Fatalf("expected fallback to provider role preference, got: %v", err)
	}
	if result.Provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if result.Model != "llama3" {
		t.Errorf("expected model llama3, got %s", result.Model)
	}
	if result.Role != "reviewer" {
		t.Errorf("expected role reviewer, got %s", result.Role)
	}
}

// TestRouteByRole_RoleDefWithoutProviderFallsBackToProviderModels reproduces
// the reported bug: a provider declares all roles across multiple models, but
// the matching role definition has no ProviderID assigned. Previously the
// enabled-but-unbound role definition short-circuited RouteByRole with an
// error, so the agent never reached the provider-declared-role fallback and
// reported "no provider available". The agent must still find a match.
func TestRouteByRole_RoleDefWithoutProviderFallsBackToProviderModels(t *testing.T) {
	d := openRouterDB(t)
	ctx := context.Background()

	// Provider declaring all roles at the provider level (Phase 5, T5.1 removed
	// per-model roles) with several models; its default model serves the role.
	models := []db.ProviderModel{
		{Name: "gemma-a"},
		{Name: "gemma-b"},
		{Name: "gemma-c"},
	}
	prov := &db.Provider{
		Name:      "llama.cpp",
		Type:      "openai_compatible",
		BaseURL:   "http://localhost:7777/v1",
		ModelName: "gemma-a",
		Enabled:   true,
		Roles:     []string{"orchestrator", "reviewer", "worker"},
		Models:    models,
	}
	if err := d.CreateProvider(ctx, prov); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	// Enabled role definition with NO ProviderID — this is what regressed.
	if err := d.CreateRoleDefinition(ctx, &db.RoleDefinition{
		Name: "orchestrator", Label: "Orchestrator", Enabled: true,
	}); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	cfg := &config.Config{Providers: []config.ProviderConfig{}}
	reg := llm.NewRegistry()
	reg.Set("llama.cpp", llm.NewOpenAIProvider("llama.cpp", "http://localhost:7777/v1", "", ""))
	reg.SetRoles("llama.cpp", "gemma-a", prov.Roles) // mirror startup provider-role registration

	rtr := router.New(cfg, reg)
	if err := rtr.LoadFromDB(d); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}

	res, err := rtr.RouteByRole("orchestrator")
	if err != nil {
		t.Fatalf("RouteByRole(orchestrator) should fall back to provider-declared role, got: %v", err)
	}
	if res.Provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if res.Model == "" {
		t.Error("expected a non-empty model from the provider's default")
	}
	if res.Role != "orchestrator" {
		t.Errorf("role = %q, want orchestrator", res.Role)
	}
}

// TestRouteByRole_AcceptsRoleID verifies Phase 2 id-tolerance in the router:
// RouteByRole resolves a role by its definition id, not only by name.
func TestRouteByRole_AcceptsRoleID(t *testing.T) {
	d := openRouterDB(t)
	ctx := context.Background()

	prov := &db.Provider{
		Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		ModelName: "gemma3:4b", Enabled: true,
		Models:    []db.ProviderModel{{Name: "gemma3:4b"}},
	}
	if err := d.CreateProvider(ctx, prov); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	role := &db.RoleDefinition{Name: "worker", Label: "Worker", ProviderID: prov.ID, Enabled: true}
	if err := d.CreateRoleDefinition(ctx, role); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	cfg := &config.Config{Providers: []config.ProviderConfig{}}
	reg := llm.NewRegistry()
	reg.Set("ollama", llm.NewOllamaProvider("ollama", "http://localhost:11434", "gemma3:4b"))
	rtr := router.New(cfg, reg)
	if err := rtr.LoadFromDB(d); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}

	res, err := rtr.RouteByRole(role.ID) // resolve by id, not name
	if err != nil {
		t.Fatalf("RouteByRole(id): %v", err)
	}
	if res.Role != "worker" {
		t.Errorf("role = %q, want worker", res.Role)
	}
	if res.Provider == nil {
		t.Error("expected non-nil provider")
	}
}

// TestRoleName_ResolvesRefToName verifies that RoleName maps a role id (and a
// role name) to the human-readable role name, and falls back to the ref itself
// when the role is unknown. This backs B3: agent logs show role names, not ids.
func TestRoleName_ResolvesRefToName(t *testing.T) {
	d := openRouterDB(t)
	ctx := context.Background()

	prov := &db.Provider{
		Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		ModelName: "gemma3:4b", Enabled: true,
		Models:    []db.ProviderModel{{Name: "gemma3:4b"}},
	}
	if err := d.CreateProvider(ctx, prov); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	role := &db.RoleDefinition{Name: "worker", Label: "Worker", ProviderID: prov.ID, Enabled: true}
	if err := d.CreateRoleDefinition(ctx, role); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	cfg := &config.Config{Providers: []config.ProviderConfig{}}
	reg := llm.NewRegistry()
	reg.Set("ollama", llm.NewOllamaProvider("ollama", "http://localhost:11434", "gemma3:4b"))
	rtr := router.New(cfg, reg)
	if err := rtr.LoadFromDB(d); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}

	if got := rtr.RoleName(role.ID); got != "worker" {
		t.Errorf("RoleName(id) = %q, want worker", got)
	}
	if got := rtr.RoleName("worker"); got != "worker" {
		t.Errorf("RoleName(name) = %q, want worker", got)
	}
	if got := rtr.RoleName("nonexistent"); got != "nonexistent" {
		t.Errorf("RoleName(unknown) = %q, want nonexistent (fallback)", got)
	}
}

// --- ContextBuilder tests ---

func TestBuildContext_FiltersByInclude(t *testing.T) {
	cfg := testConfig()
	r := router.New(cfg, llm.NewRegistry())

	entries := []router.ContextEntry{
		{Type: "code", Content: "func main() {}"},
		{Type: "docs", Content: "API documentation"},
		{Type: "diff", Content: "+added line"},
	}
	ctx := r.BuildContext("worker", entries)

	if !contains(ctx, "func main()") {
		t.Error("expected code in context")
	}
	if contains(ctx, "API documentation") {
		t.Error("docs should be excluded for worker role")
	}
	if !contains(ctx, "+added line") {
		t.Error("expected diff in context")
	}
}

func TestBuildContext_EmptyEntries(t *testing.T) {
	cfg := testConfig()
	r := router.New(cfg, llm.NewRegistry())

	ctx := r.BuildContext("worker", nil)
	if ctx != "" {
		t.Errorf("expected empty context for nil entries, got: %s", ctx)
	}
}

func TestBuildContext_NoRule(t *testing.T) {
	cfg := testConfig()
	r := router.New(cfg, llm.NewRegistry())

	entries := []router.ContextEntry{
		{Type: "anything", Content: "some data"},
	}
	ctx := r.BuildContext("orchestrator", entries) // no rule for orchestrator
	if !contains(ctx, "some data") {
		t.Error("expected all entries when no rule defined")
	}
}

// TestContextInjection_ScopeOnlyForIncludedRoles verifies that the synthetic
// scope entries reach a planner (no exclusion) but not a worker (scope types in
// context_exclude).
func TestContextInjection_ScopeOnlyForIncludedRoles(t *testing.T) {
	cfg := testConfig()
	r := router.New(cfg, llm.NewRegistry())

	base := []router.ContextEntry{{Type: "summary", Content: "architecture notes"}}
	entries := append(base, router.ScopeEntries("REQ: must auth", "FEAT: login page")...)

	planner := &db.RoleDefinition{Name: "planner"} // no include/exclude → sees everything
	worker := &db.RoleDefinition{
		Name:           "worker",
		ContextExclude: []string{router.ScopeTypeRequirements, router.ScopeTypeFeatures},
	}

	plannerCtx := r.BuildContextForRole(planner, entries)
	if !contains(plannerCtx, "must auth") || !contains(plannerCtx, "login page") {
		t.Errorf("planner context should contain scope; got:\n%s", plannerCtx)
	}

	workerCtx := r.BuildContextForRole(worker, entries)
	if contains(workerCtx, "must auth") || contains(workerCtx, "login page") {
		t.Errorf("worker context must not contain scope; got:\n%s", workerCtx)
	}
	if !contains(workerCtx, "architecture notes") {
		t.Error("worker should still receive non-scope context")
	}
}

// --- ResolveAgentPersona tests (Feature 6) ---

func TestResolveAgentPersona_MergesPromptAndContext(t *testing.T) {
	role := &db.RoleDefinition{
		Name:           "worker",
		SystemPrompt:   "You are a worker.",
		ContextInclude: []string{"src/**"},
	}
	skills := []*db.SkillDefinition{
		{Name: "backend", PromptFragment: "Backend focus.", ContextInclude: []string{"server/**"}},
		{Name: "go", PromptFragment: "Go idioms.", ContextInclude: []string{"*.go"}},
	}
	p := router.ResolveAgentPersona(role, skills)

	if !contains(p.SystemPrompt, "You are a worker.") ||
		!contains(p.SystemPrompt, "Backend focus.") ||
		!contains(p.SystemPrompt, "Go idioms.") {
		t.Errorf("system prompt missing parts:\n%s", p.SystemPrompt)
	}
	for _, want := range []string{"src/**", "server/**", "*.go"} {
		found := false
		for _, c := range p.ContextInclude {
			if c == want {
				found = true
			}
		}
		if !found {
			t.Errorf("context include missing %q (got %v)", want, p.ContextInclude)
		}
	}
}

func TestResolveAgentPersona_ToolsUnion(t *testing.T) {
	role := &db.RoleDefinition{Name: "worker", AllowedTools: []string{"read_file", "write_file"}}
	skills := []*db.SkillDefinition{
		{Name: "infra", AllowedTools: []string{"run_command", "read_file"}}, // read_file dup
	}
	p := router.ResolveAgentPersona(role, skills)

	want := map[string]bool{"read_file": false, "write_file": false, "run_command": false}
	for _, tool := range p.AllowedTools {
		if _, ok := want[tool]; ok {
			want[tool] = true
		}
	}
	for tool, seen := range want {
		if !seen {
			t.Errorf("tool %q missing from union %v", tool, p.AllowedTools)
		}
	}
	// No duplicates.
	counts := map[string]int{}
	for _, tool := range p.AllowedTools {
		counts[tool]++
	}
	if counts["read_file"] != 1 {
		t.Errorf("read_file duplicated in tool union: %v", p.AllowedTools)
	}
}

func TestResolveAgentPersona_SkillsAddNoCapabilities(t *testing.T) {
	// Persona has no capability surface at all — capabilities come only from
	// roles and are resolved separately. This test documents that invariant:
	// merging skills must not introduce a capabilities field on the persona.
	role := &db.RoleDefinition{Name: "reviewer", Capabilities: []string{"handles_review"}}
	skills := []*db.SkillDefinition{{Name: "frontend", PromptFragment: "UI."}}
	p := router.ResolveAgentPersona(role, skills)

	// The persona carries prompt/context/tools but never capabilities; the role's
	// capabilities are untouched and remain the sole authority source.
	if len(role.Capabilities) != 1 || role.Capabilities[0] != "handles_review" {
		t.Errorf("role capabilities should be unchanged, got %v", role.Capabilities)
	}
	if p.SystemPrompt == "" {
		t.Error("expected a composed prompt")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ── helpers for DB-backed router tests ───────────────────────────────────────

func openRouterDB(t *testing.T) *db.Database {
	t.Helper()
	path := filepath.Join(t.TempDir(), "router_test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// setupProviderWithModels creates a provider + role definition in the DB.
// models is the per-model config for the provider.
// Returns a router loaded from that DB state.
func setupProviderWithModels(
	t *testing.T,
	models []db.ProviderModel,
	roleName string,
) *router.Router {
	t.Helper()
	d := openRouterDB(t)
	ctx := context.Background()

	prov := &db.Provider{
		Name:      "ollama",
		Type:      "ollama",
		BaseURL:   "http://localhost:11434",
		ModelName: "gemma3:4b",
		Enabled:   true,
		Models:    models,
	}
	if err := d.CreateProvider(ctx, prov); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	role := &db.RoleDefinition{
		Name:       roleName,
		Label:      roleName,
		ProviderID: prov.ID,
		Enabled:    true,
	}
	if err := d.CreateRoleDefinition(ctx, role); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	cfg := &config.Config{
		Providers: []config.ProviderConfig{},
	}
	reg := llm.NewRegistry()
	reg.Set("ollama", llm.NewOllamaProvider("ollama", "http://localhost:11434", "gemma3:4b"))

	rtr := router.New(cfg, reg)
	if err := rtr.LoadFromDB(d); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}
	return rtr
}

// TestRouterModelLevelTextToolCalls verifies that text_tool_calls set on a
// model entry is returned by RouteByRole for that model's role, while another
// role on the same provider that has no model-level override gets the
// provider's default (false).
func TestRouterModelLevelTextToolCalls(t *testing.T) {
	d := openRouterDB(t)
	ctx := context.Background()

	models := []db.ProviderModel{
		{Name: "gemma3:4b", TextToolCalls: true, FoldSystemIntoUser: true, SystemPrefix: "<|think|>"},
		{Name: "qwen2.5:14b"}, // no text_tool_calls
	}
	prov := &db.Provider{Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		ModelName: "gemma3:4b", Enabled: true, Models: models}
	_ = d.CreateProvider(ctx, prov)

	// Each role targets its model via ModelOverride (T5.1 removed per-model roles).
	for _, roleDef := range []struct{ name, model string }{
		{"worker", "gemma3:4b"},
		{"reviewer", "qwen2.5:14b"},
	} {
		_ = d.CreateRoleDefinition(ctx, &db.RoleDefinition{
			Name: roleDef.name, Label: roleDef.name,
			ProviderID: prov.ID, ModelOverride: roleDef.model, Enabled: true,
		})
	}

	cfg := &config.Config{Providers: []config.ProviderConfig{}}
	reg := llm.NewRegistry()
	reg.Set("ollama", llm.NewOllamaProvider("ollama", "http://localhost:11434", "gemma3:4b"))
	rtr := router.New(cfg, reg)
	_ = rtr.LoadFromDB(d)

	// Worker role → gemma3:4b model with text_tool_calls = true.
	worker, err := rtr.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole(worker): %v", err)
	}
	if worker.Model != "gemma3:4b" {
		t.Errorf("worker model = %q, want gemma3:4b", worker.Model)
	}
	if !worker.TextToolCalls {
		t.Error("worker TextToolCalls should be true (model-level)")
	}
	if !worker.FoldSystemIntoUser {
		t.Error("worker FoldSystemIntoUser should be true (model-level)")
	}
	if worker.SystemPrefix != "<|think|>" {
		t.Errorf("worker SystemPrefix = %q, want <|think|>", worker.SystemPrefix)
	}

	// Reviewer role → qwen2.5:14b with no model-level flags → defaults to false.
	reviewer, err := rtr.RouteByRole("reviewer")
	if err != nil {
		t.Fatalf("RouteByRole(reviewer): %v", err)
	}
	if reviewer.Model != "qwen2.5:14b" {
		t.Errorf("reviewer model = %q, want qwen2.5:14b", reviewer.Model)
	}
	if reviewer.TextToolCalls {
		t.Error("reviewer TextToolCalls should be false (no model-level override)")
	}
}

// TestRouterModelLevelToolAllowlist verifies that a model-level ToolAllowlist
// overrides the provider-level tool_allowlist but loses to a role-level
// AllowedTools when one is set.
func TestRouterModelLevelToolAllowlist(t *testing.T) {
	d := openRouterDB(t)
	ctx := context.Background()

	models := []db.ProviderModel{
		{Name: "gemma3:4b", ToolAllowlist: []string{"write_file", "read_file"}},
	}
	prov := &db.Provider{
		Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		ModelName: "gemma3:4b", Enabled: true,
		Config: map[string]interface{}{"tool_allowlist": []interface{}{"list_files"}}, // provider default
		Models: models,
	}
	_ = d.CreateProvider(ctx, prov)

	// Worker role: no AllowedTools → model-level should apply.
	_ = d.CreateRoleDefinition(ctx, &db.RoleDefinition{
		Name: "worker", Label: "Worker", ProviderID: prov.ID,
		Enabled: true,
	})
	// Orchestrator role: has AllowedTools → role-level should win.
	_ = d.CreateRoleDefinition(ctx, &db.RoleDefinition{
		Name: "orchestrator", Label: "Orchestrator", ProviderID: prov.ID,
		Enabled: true,
		AllowedTools: []string{"list_tasks", "create_work_package"},
	})

	cfg := &config.Config{Providers: []config.ProviderConfig{}}
	reg := llm.NewRegistry()
	reg.Set("ollama", llm.NewOllamaProvider("ollama", "http://localhost:11434", "gemma3:4b"))
	rtr := router.New(cfg, reg)
	_ = rtr.LoadFromDB(d)

	// Worker: model-level wins over provider-level.
	worker, _ := rtr.RouteByRole("worker")
	if len(worker.ToolAllowlist) != 2 || worker.ToolAllowlist[0] != "write_file" {
		t.Errorf("worker ToolAllowlist = %v, want [write_file read_file]", worker.ToolAllowlist)
	}

	// Orchestrator: role-level wins over model-level (no model entry for orchestrator).
	orch, _ := rtr.RouteByRole("orchestrator")
	if len(orch.ToolAllowlist) != 2 || orch.ToolAllowlist[0] != "list_tasks" {
		t.Errorf("orchestrator ToolAllowlist = %v, want [list_tasks create_work_package]", orch.ToolAllowlist)
	}
}

// TestRouterProviderLevelFallback verifies that when a model is matched but
// has no behavioral fields set, the provider-level Config values are used.
func TestRouterProviderLevelFallback(t *testing.T) {
	d := openRouterDB(t)
	ctx := context.Background()

	// Model has no behavioral fields — provider level should apply.
	models := []db.ProviderModel{
		{Name: "gemma3:4b"},
	}
	prov := &db.Provider{
		Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		ModelName: "gemma3:4b", Enabled: true,
		Config: map[string]interface{}{
			"text_tool_calls":       true,
			"fold_system_into_user": true,
			"system_prefix":         "<think>",
			"tool_allowlist":        []interface{}{"read_file", "list_files"},
		},
		Models: models,
	}
	_ = d.CreateProvider(ctx, prov)
	_ = d.CreateRoleDefinition(ctx, &db.RoleDefinition{
		Name: "worker", Label: "Worker", ProviderID: prov.ID,
		Enabled: true,
	})

	cfg := &config.Config{Providers: []config.ProviderConfig{}}
	reg := llm.NewRegistry()
	reg.Set("ollama", llm.NewOllamaProvider("ollama", "http://localhost:11434", "gemma3:4b"))
	rtr := router.New(cfg, reg)
	_ = rtr.LoadFromDB(d)

	result, err := rtr.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	if !result.TextToolCalls {
		t.Error("TextToolCalls should be true from provider-level config")
	}
	if !result.FoldSystemIntoUser {
		t.Error("FoldSystemIntoUser should be true from provider-level config")
	}
	if result.SystemPrefix != "<think>" {
		t.Errorf("SystemPrefix = %q, want <think>", result.SystemPrefix)
	}
	if len(result.ToolAllowlist) != 2 {
		t.Errorf("ToolAllowlist = %v, want [read_file list_files]", result.ToolAllowlist)
	}
}

// TestRouteByRole_ExposesCapabilities verifies the route carries the role's
// capabilities so the executor can tell planner (creates_tasks) tasks apart.
func TestRouteByRole_ExposesCapabilities(t *testing.T) {
	d := openRouterDB(t)
	ctx := context.Background()
	prov := &db.Provider{
		Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		ModelName: "gemma3:4b", Enabled: true,
		Models: []db.ProviderModel{{Name: "gemma3:4b"}},
	}
	if err := d.CreateProvider(ctx, prov); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if err := d.CreateRoleDefinition(ctx, &db.RoleDefinition{
		Name: "orchestrator", Label: "Orchestrator", ProviderID: prov.ID, Enabled: true,
		Capabilities: []string{"creates_tasks", "handles_merge"},
	}); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	cfg := &config.Config{Providers: []config.ProviderConfig{}}
	reg := llm.NewRegistry()
	reg.Set("ollama", llm.NewOllamaProvider("ollama", "http://localhost:11434", "gemma3:4b"))
	rtr := router.New(cfg, reg)
	if err := rtr.LoadFromDB(d); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}

	res, err := rtr.RouteByRole("orchestrator")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	has := false
	for _, c := range res.Capabilities {
		if c == "creates_tasks" {
			has = true
		}
	}
	if !has {
		t.Errorf("route capabilities = %v, want creates_tasks", res.Capabilities)
	}
}

// TestLoadFromData_MatchesLoadFromDB verifies that LoadFromData (the agent's
// no-DB path, fed providers/roles fetched over HTTP) resolves roles identically
// to LoadFromDB for the same provider/role state.
func TestLoadFromData_MatchesLoadFromDB(t *testing.T) {
	d := openRouterDB(t)
	ctx := context.Background()

	models := []db.ProviderModel{
		{Name: "gemma3:4b", ToolAllowlist: []string{"write_file", "read_file"}},
		{Name: "qwen2.5:14b"},
	}
	prov := &db.Provider{
		Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		ModelName: "gemma3:4b", Enabled: true, Models: models,
	}
	if err := d.CreateProvider(ctx, prov); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	for _, name := range []string{"worker", "reviewer"} {
		if err := d.CreateRoleDefinition(ctx, &db.RoleDefinition{
			Name: name, Label: name, ProviderID: prov.ID, Enabled: true,
		}); err != nil {
			t.Fatalf("CreateRoleDefinition(%s): %v", name, err)
		}
	}

	newRtr := func() *router.Router {
		cfg := &config.Config{Providers: []config.ProviderConfig{}}
		reg := llm.NewRegistry()
		reg.Set("ollama", llm.NewOllamaProvider("ollama", "http://localhost:11434", "gemma3:4b"))
		return router.New(cfg, reg)
	}

	// Router A: loaded straight from the DB.
	rA := newRtr()
	if err := rA.LoadFromDB(d); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}

	// Router B: loaded from in-memory data fetched from the DB (mirrors the agent
	// fetching providers/roles over HTTP and calling LoadFromData).
	provs, err := d.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	roles, err := d.ListRoleDefinitions(ctx)
	if err != nil {
		t.Fatalf("ListRoleDefinitions: %v", err)
	}
	rB := newRtr()
	if err := rB.LoadFromData(provs, roles); err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}

	for _, role := range []string{"worker", "reviewer"} {
		ra, err := rA.RouteByRole(role)
		if err != nil {
			t.Fatalf("A RouteByRole(%s): %v", role, err)
		}
		rb, err := rB.RouteByRole(role)
		if err != nil {
			t.Fatalf("B RouteByRole(%s): %v", role, err)
		}
		if ra.Provider.Name() != rb.Provider.Name() {
			t.Errorf("%s provider: A=%s B=%s", role, ra.Provider.Name(), rb.Provider.Name())
		}
		if ra.Model != rb.Model {
			t.Errorf("%s model: A=%s B=%s", role, ra.Model, rb.Model)
		}
		if ra.Role != rb.Role {
			t.Errorf("%s role: A=%s B=%s", role, ra.Role, rb.Role)
		}
		if len(ra.ToolAllowlist) != len(rb.ToolAllowlist) {
			t.Errorf("%s allowlist len: A=%v B=%v", role, ra.ToolAllowlist, rb.ToolAllowlist)
			continue
		}
		for i := range ra.ToolAllowlist {
			if ra.ToolAllowlist[i] != rb.ToolAllowlist[i] {
				t.Errorf("%s allowlist[%d]: A=%s B=%s", role, i, ra.ToolAllowlist[i], rb.ToolAllowlist[i])
			}
		}
	}
}
