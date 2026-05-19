package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
// Set chatErr to simulate LLM failures.
type mockLLMProvider struct {
	name     string
	response llm.ChatResponse
	chatErr  error
}

func (m *mockLLMProvider) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	if m.chatErr != nil {
		return llm.ChatResponse{}, m.chatErr
	}
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
		Status:  db.TaskStatusDeveloping,
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

// TestRouter_LoadFromDB_ProviderResolution verifies that LoadFromDB wires the
// correct provider name to a role, so RouteByRole returns that provider.
func TestRouter_LoadFromDB_ProviderResolution(t *testing.T) {
	// Build an in-memory DB with a provider + role definition.
	dbPath := filepath.Join(t.TempDir(), "router_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	// Create provider.
	prov := &db.Provider{
		Name:      "mock",
		Type:      "ollama",
		BaseURL:   "http://localhost:11434",
		ModelName: "mock-v1",
		Enabled:   true,
	}
	if err := database.CreateProvider(ctx, prov); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	// Create role definition pointing at that provider.
	role := &db.RoleDefinition{
		Name:          "worker",
		Label:         "Worker",
		ProviderID:    prov.ID,
		ModelOverride: "mock-v1",
		TaskTypes:     []string{"implement"},
		Enabled:       true,
	}
	if err := database.CreateRoleDefinition(ctx, role); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	// Build registry and register the mock provider.
	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	mockProv := &mockLLMProvider{name: "mock"}
	_ = reg.Register("mock", mockProv)

	rtr := router.New(cfg, reg)
	if err := rtr.LoadFromDB(database); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}

	// RouteByRole("worker") must resolve to the mock provider.
	result, err := rtr.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	if result.Provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if result.Provider.Name() != "mock" {
		t.Errorf("expected provider name mock, got %s", result.Provider.Name())
	}
}

// TestRouter_LoadFromDB_TaskTypeMapping verifies that task type → role mapping
// loaded from the DB is honoured by RouteByTaskType.
func TestRouter_LoadFromDB_TaskTypeMapping(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "router_tt_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	prov := &db.Provider{Name: "mock", Type: "ollama", BaseURL: "http://x", ModelName: "m", Enabled: true}
	_ = database.CreateProvider(ctx, prov)

	role := &db.RoleDefinition{
		Name:       "reviewer",
		Label:      "Reviewer",
		ProviderID: prov.ID,
		TaskTypes:  []string{"review", "audit"},
		Enabled:    true,
	}
	_ = database.CreateRoleDefinition(ctx, role)

	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	_ = reg.Register("mock", &mockLLMProvider{name: "mock"})

	rtr := router.New(cfg, reg)
	_ = rtr.LoadFromDB(database)

	result, err := rtr.RouteByTaskType("review")
	if err != nil {
		t.Fatalf("RouteByTaskType(review): %v", err)
	}
	if result.Role != "reviewer" {
		t.Errorf("expected role reviewer, got %s", result.Role)
	}

	result2, err := rtr.RouteByTaskType("audit")
	if err != nil {
		t.Fatalf("RouteByTaskType(audit): %v", err)
	}
	if result2.Role != "reviewer" {
		t.Errorf("expected role reviewer for audit, got %s", result2.Role)
	}
}

// TestRouter_LoadFromDB_DisabledRoleSkipped ensures disabled roles are not
// added to the routing cache.
func TestRouter_LoadFromDB_DisabledRoleSkipped(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "disabled_role_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	prov := &db.Provider{Name: "mock", Type: "ollama", BaseURL: "http://x", ModelName: "m", Enabled: true}
	_ = database.CreateProvider(ctx, prov)
	role := &db.RoleDefinition{
		Name:       "disabled-role",
		Label:      "Disabled",
		ProviderID: prov.ID,
		TaskTypes:  []string{"special"},
		Enabled:    false, // disabled!
	}
	_ = database.CreateRoleDefinition(ctx, role)

	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	_ = reg.Register("mock", &mockLLMProvider{name: "mock"})

	rtr := router.New(cfg, reg)
	_ = rtr.LoadFromDB(database)

	_, err = rtr.RouteByRole("disabled-role")
	if err == nil {
		t.Error("expected error routing to disabled role, got nil")
	}
}

// fullExecMockServer builds a mock HTTP server suitable for end-to-end agent
// tests that exercise the poll → claim → LLM → submit-result path.
// The task is served from tasks/next on the first call only.
// Returns the server and pointers to: nextCalls, claimCalls, resultCalls,
// and the last-received result status.
func fullExecMockServer(t *testing.T, task *db.Task) (
	srv *httptest.Server,
	nextCalls, claimCalls, resultCalls *atomic.Int32,
	resultStatus *atomic.Value,
) {
	t.Helper()
	const agentID = "full-exec-agent"
	nextCalls = new(atomic.Int32)
	claimCalls = new(atomic.Int32)
	resultCalls = new(atomic.Int32)
	resultStatus = new(atomic.Value)
	resultStatus.Store("")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(api.RegisterAgentResponse{AgentID: agentID})
	})
	mux.HandleFunc("/api/agents/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case strings.Contains(r.URL.Path, "/tasks/next"):
			if nextCalls.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(task)
			} else {
				_ = json.NewEncoder(w).Encode(nil)
			}
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/claim", func(w http.ResponseWriter, r *http.Request) {
		claimCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(task)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/result", func(w http.ResponseWriter, r *http.Request) {
		var req api.SubmitTaskResultRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resultStatus.Store(req.Status)
		resultCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return
}

// TestAgent_FullTaskExecution exercises the complete poll → claim → LLM →
// submit-result path and asserts the result endpoint is called with
// status = "completed".
func TestAgent_FullTaskExecution(t *testing.T) {
	task := &db.Task{
		ID:      "full-exec-task",
		Type:    "implement",
		Role:    "worker",
		Status:  db.TaskStatusBacklog,
		Payload: map[string]interface{}{"description": "Write hello world"},
	}
	srv, _, _, resultCalls, resultStatus := fullExecMockServer(t, task)

	cfg := buildExecutorConfig()
	cfg.Agents.TaskPollIntervalSec = 1

	reg := llm.NewRegistry()
	_ = reg.Register("mock", &mockLLMProvider{
		name: "mock",
		response: llm.ChatResponse{
			Content:    "Task completed.",
			StopReason: "end_turn",
			TokensUsed: 50,
		},
	})

	rtr := router.New(cfg, reg)
	toolReg := tools.NewRegistry()

	a := agent.NewAgent("full-exec", []string{"worker"}, srv.URL, cfg)
	a.WithExecutor(rtr, toolReg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	deadline := time.After(8 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for result submission (got %d)", resultCalls.Load())
		case <-time.After(100 * time.Millisecond):
			if resultCalls.Load() >= 1 {
				if got := resultStatus.Load().(string); got != "completed" {
					t.Errorf("result status = %q, want %q", got, "completed")
				}
				return
			}
		}
	}
}

// TestAgent_LLMFailure_MarksTaskFailed verifies that an LLM error causes the
// result endpoint to be called with status = "failed", and that the agent
// continues polling afterwards.
func TestAgent_LLMFailure_MarksTaskFailed(t *testing.T) {
	task := &db.Task{
		ID:      "fail-exec-task",
		Type:    "implement",
		Role:    "worker",
		Status:  db.TaskStatusBacklog,
		Payload: map[string]interface{}{"description": "Fail me"},
	}
	srv, nextCalls, _, resultCalls, resultStatus := fullExecMockServer(t, task)

	cfg := buildExecutorConfig()
	cfg.Agents.TaskPollIntervalSec = 1

	reg := llm.NewRegistry()
	_ = reg.Register("mock", &mockLLMProvider{
		name:    "mock",
		chatErr: errors.New("simulated LLM error"),
	})

	rtr := router.New(cfg, reg)
	toolReg := tools.NewRegistry()

	a := agent.NewAgent("fail-exec", []string{"worker"}, srv.URL, cfg)
	a.WithExecutor(rtr, toolReg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	// Wait for the failed result to be submitted.
	deadline := time.After(8 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for result submission (got %d)", resultCalls.Load())
		case <-time.After(100 * time.Millisecond):
			if resultCalls.Load() >= 1 {
				if got := resultStatus.Load().(string); got != "failed" {
					t.Errorf("result status = %q, want %q", got, "failed")
				}
				goto resultReceived
			}
		}
	}
resultReceived:
	// After the failure, the agent should continue polling. The mock now
	// returns nil from tasks/next. Verify nextCalls keeps increasing.
	callsBefore := nextCalls.Load()
	time.Sleep(2500 * time.Millisecond)
	if nextCalls.Load() <= callsBefore {
		t.Errorf("agent stopped polling after LLM failure (calls before=%d after=%d)",
			callsBefore, nextCalls.Load())
	}
}
