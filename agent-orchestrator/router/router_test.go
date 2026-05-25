package router_test

import (
	"testing"

	"agent-orchestrator/config"
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
		Routing: map[string]string{
			"implement": "worker",
			"plan":      "orchestrator",
			"review":    "reviewer",
		},
		Prompts: map[string]string{
			"implement": "Implement: {task_description}\nRepo: {repo_path}\nContext: {context}",
			"review":    "Review: {diff}",
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
		Routing:   map[string]string{},
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

func TestRouteByTaskType_Implement(t *testing.T) {
	cfg := testConfig()
	reg := testRegistry(t, cfg)
	r := router.New(cfg, reg)

	result, err := r.RouteByTaskType("implement")
	if err != nil {
		t.Fatalf("RouteByTaskType: %v", err)
	}
	if result.Role != "worker" {
		t.Errorf("expected role worker, got %s", result.Role)
	}
}

func TestRouteByTaskType_FallbackToRole(t *testing.T) {
	cfg := testConfig()
	reg := testRegistry(t, cfg)
	r := router.New(cfg, reg)

	// "worker" is not in routing map but IS a valid role
	result, err := r.RouteByTaskType("worker")
	if err != nil {
		t.Fatalf("RouteByTaskType fallback: %v", err)
	}
	if result.Role != "worker" {
		t.Errorf("expected role worker, got %s", result.Role)
	}
}

func TestRoleForTaskType(t *testing.T) {
	cfg := testConfig()
	r := router.New(cfg, llm.NewRegistry())

	if role := r.RoleForTaskType("implement"); role != "worker" {
		t.Errorf("expected worker, got %s", role)
	}
	if role := r.RoleForTaskType("unknown-type"); role != "unknown-type" {
		t.Errorf("expected passthrough, got %s", role)
	}
}

// --- Prompter tests ---

func TestBuildPrompt_WithVars(t *testing.T) {
	cfg := testConfig()
	r := router.New(cfg, llm.NewRegistry())

	prompt := r.BuildPrompt("implement", map[string]interface{}{
		"task_description": "Add login endpoint",
		"repo_path":        "/tmp/myapp",
		"context":          "Uses JWT auth",
	})
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !contains(prompt, "Add login endpoint") {
		t.Error("prompt missing task_description")
	}
	if !contains(prompt, "/tmp/myapp") {
		t.Error("prompt missing repo_path")
	}
}

func TestBuildPrompt_Fallback(t *testing.T) {
	cfg := testConfig()
	r := router.New(cfg, llm.NewRegistry())

	prompt := r.BuildPrompt("unknown-type", map[string]interface{}{
		"payload": "do something",
	})
	if !contains(prompt, "do something") {
		t.Errorf("fallback prompt should include payload, got: %s", prompt)
	}
}

func TestBuildPrompt_MissingVar(t *testing.T) {
	cfg := testConfig()
	r := router.New(cfg, llm.NewRegistry())

	// {context} is not provided — should remain in output unreplaced
	prompt := r.BuildPrompt("implement", map[string]interface{}{
		"task_description": "x",
		"repo_path":        "y",
	})
	if !contains(prompt, "{context}") {
		t.Errorf("expected unfilled {context} placeholder, got: %s", prompt)
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
