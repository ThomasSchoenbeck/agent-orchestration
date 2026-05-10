package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/api"
	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
	"agent-orchestrator/tools"
)

// mockLLMProvider is a minimal LLMProvider that returns a fixed response.
type mockLLMProvider struct {
	name     string
	response llm.ChatResponse
}

func (m *mockLLMProvider) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return m.response, nil
}
func (m *mockLLMProvider) Embed(_ context.Context, _ llm.EmbedRequest) (llm.EmbedResponse, error) {
	return llm.EmbedResponse{}, nil
}
func (m *mockLLMProvider) Rerank(_ context.Context, _ llm.RerankRequest) (llm.RerankResponse, error) {
	return llm.RerankResponse{}, nil
}
func (m *mockLLMProvider) Name() string  { return m.name }
func (m *mockLLMProvider) Close() error  { return nil }

// buildExecutorConfig returns a minimal config wiring "worker" → "mock-model" → "mock".
func buildExecutorConfig() *config.Config {
	return &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "mock", Type: "ollama", BaseURL: "http://localhost:11434"},
		},
		Models: []config.ModelConfig{
			{Name: "mock-model", Provider: "mock", Model: "mock-v1", Roles: []string{"worker"}},
		},
		Roles:   map[string]string{"worker": "mock-model"},
		Routing: map[string]string{"implement": "worker"},
		Prompts: map[string]string{
			"implement": "Implement: {description}",
		},
		Server:   config.ServerConfig{Port: 8080},
		Database: config.DatabaseConfig{Path: ":memory:"},
		Agents: config.AgentConfig{
			HeartbeatIntervalSec: 30,
			TaskPollIntervalSec:  5,
			TaskTimeoutSec:       300,
		},
	}
}

func TestExecutor_SubmitsResult(t *testing.T) {
	var submitted atomic.Int32

	// Mock server that accepts result submissions.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(api.RegisterAgentResponse{AgentID: "exec-agent-1"})
	})
	mux.HandleFunc("/api/agents/exec-agent-1/tasks/result", func(w http.ResponseWriter, r *http.Request) {
		submitted.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/tasks/task-001/result", func(w http.ResponseWriter, r *http.Request) {
		submitted.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Build a registry with the mock provider.
	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	provider := &mockLLMProvider{
		name: "mock",
		response: llm.ChatResponse{
			Content:    "Task completed successfully.",
			StopReason: "end_turn",
			TokensUsed: 100,
		},
	}
	_ = reg.Register("mock", provider)

	rtr := router.New(cfg, reg)
	toolReg := tools.NewRegistry()

	// Create agent with executor wired.
	a := agent.NewAgent("exec-test", []string{"worker"}, srv.URL, cfg)
	a.WithExecutor(rtr, toolReg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start agent to get an ID registered.
	if err := a.Start(ctx); err != nil {
		t.Fatalf("agent Start: %v", err)
	}
	defer a.Stop()

	// Directly invoke the server client to submit a result, simulating
	// what the executor does internally.
	client := agent.NewServerClient(srv.URL)
	task := &db.Task{
		ID:      "task-001",
		Type:    "implement",
		Role:    "worker",
		Status:  "in_progress",
		Payload: map[string]interface{}{"description": "Write a hello world program"},
	}

	// Submit a result directly.
	err := client.SubmitTaskResult(ctx, task.ID,
		map[string]interface{}{"output": "done"},
		"completed",
		&api.TaskMetrics{TokensUsed: 100, DurationMs: 50},
	)
	if err != nil {
		t.Fatalf("SubmitTaskResult: %v", err)
	}
	if submitted.Load() < 1 {
		t.Error("expected at least one result submission")
	}
}

func TestExecutor_RouterFallback(t *testing.T) {
	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	provider := &mockLLMProvider{
		name: "mock",
		response: llm.ChatResponse{
			Content:    "Fallback executed.",
			StopReason: "end_turn",
			TokensUsed: 50,
		},
	}
	_ = reg.Register("mock", provider)

	rtr := router.New(cfg, reg)

	// RouteByRole("worker") should succeed.
	result, err := rtr.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	if result.Provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if result.Role != "worker" {
		t.Errorf("expected role worker, got %s", result.Role)
	}

	// RouteByTaskType("implement") should map to "worker".
	result2, err := rtr.RouteByTaskType("implement")
	if err != nil {
		t.Fatalf("RouteByTaskType: %v", err)
	}
	if result2.Role != "worker" {
		t.Errorf("expected role worker, got %s", result2.Role)
	}
}

func TestExecutor_ToolRegistry_ListEmpty(t *testing.T) {
	reg := tools.NewRegistry()
	defs := reg.List()
	if len(defs) != 0 {
		t.Errorf("expected empty list for new registry, got %d", len(defs))
	}
}

func TestExecutor_ToolRegistry_ListAfterRegister(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(tools.Definition{
		Name:        "dummy",
		Description: "a dummy tool",
		Parameters:  map[string]tools.Param{"x": {Type: "string", Description: "arg"}},
		Required:    []string{"x"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return args["x"], nil
		},
	})
	defs := reg.List()
	if len(defs) != 1 {
		t.Errorf("expected 1 tool, got %d", len(defs))
	}
	if defs[0].Name != "dummy" {
		t.Errorf("expected tool name 'dummy', got %s", defs[0].Name)
	}
}

func TestExecutor_ExecuteJSON(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(tools.Definition{
		Name: "echo",
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"echo": args["msg"]}, nil
		},
	})

	result, err := reg.ExecuteJSON(context.Background(), "echo", map[string]interface{}{"msg": "hello"})
	if err != nil {
		t.Fatalf("ExecuteJSON: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty JSON result")
	}
}
