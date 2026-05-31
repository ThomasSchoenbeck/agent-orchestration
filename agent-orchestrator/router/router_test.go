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
		Models: []config.ModelConfig{
			{Name: "qwen-coder", Provider: "ollama", Model: "qwen2.5-coder:7b", Roles: []string{"worker"}},
			{Name: "o4-mini", Provider: "ollama", Model: "o4-mini", Roles: []string{"orchestrator", "reviewer"}},
		},
		Roles: map[string]string{
			"worker":       "qwen-coder",
			"orchestrator": "o4-mini",
			"reviewer":     "o4-mini",
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
		Models:    []config.ModelConfig{},
		Roles:     map[string]string{},
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
		Models:    []config.ModelConfig{},
		Roles:     map[string]string{},
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
		{Name: "gemma3:4b", Roles: []string{"worker"}, TextToolCalls: true, FoldSystemIntoUser: true, SystemPrefix: "<|think|>"},
		{Name: "qwen2.5:14b", Roles: []string{"reviewer"}}, // no text_tool_calls
	}
	prov := &db.Provider{Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		ModelName: "gemma3:4b", Enabled: true, Models: models}
	_ = d.CreateProvider(ctx, prov)

	for _, roleDef := range []struct{ name, model string }{
		{"worker", "gemma3:4b"},
		{"reviewer", "qwen2.5:14b"},
	} {
		_ = d.CreateRoleDefinition(ctx, &db.RoleDefinition{
			Name: roleDef.name, Label: roleDef.name,
			ProviderID: prov.ID, Enabled: true,
		})
	}

	cfg := &config.Config{Providers: []config.ProviderConfig{}, Models: []config.ModelConfig{},
		Roles: map[string]string{}}
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
		{Name: "gemma3:4b", Roles: []string{"worker"},
			ToolAllowlist: []string{"write_file", "read_file"}},
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

	cfg := &config.Config{Providers: []config.ProviderConfig{}, Models: []config.ModelConfig{},
		Roles: map[string]string{}}
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
		{Name: "gemma3:4b", Roles: []string{"worker"}},
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

	cfg := &config.Config{Providers: []config.ProviderConfig{}, Models: []config.ModelConfig{},
		Roles: map[string]string{}}
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
