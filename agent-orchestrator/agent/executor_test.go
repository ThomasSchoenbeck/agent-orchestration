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

// buildExecutorConfig returns a minimal config. Role routing is wired on the
// registry via SetRoles (see mockWorkerRegistry) rather than legacy config maps.
func buildExecutorConfig() *config.Config {
	return &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "mock", Type: "ollama", BaseURL: "http://localhost:11434"},
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
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
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
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
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
		Enabled:       true,
	}
	if err := database.CreateRoleDefinition(ctx, role); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}

	// Build registry and register the mock provider.
	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
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
		Enabled:    false, // disabled!
	}
	_ = database.CreateRoleDefinition(ctx, role)

	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
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
// reviewCalls, and the last-received result status.
func fullExecMockServer(t *testing.T, task *db.Task) (
	srv *httptest.Server,
	nextCalls, claimCalls, resultCalls, reviewCalls *atomic.Int32,
	resultStatus *atomic.Value,
) {
	t.Helper()
	const agentID = "full-exec-agent"
	nextCalls = new(atomic.Int32)
	claimCalls = new(atomic.Int32)
	resultCalls = new(atomic.Int32)
	reviewCalls = new(atomic.Int32)
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
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"task": task})
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"task": task})
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/result", func(w http.ResponseWriter, r *http.Request) {
		var req api.SubmitTaskResultRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resultStatus.Store(req.Status)
		resultCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/submit-for-review", func(w http.ResponseWriter, r *http.Request) {
		reviewCalls.Add(1)
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
		Role:    "worker",
		Status:  db.TaskStatusBacklog,
		Payload: map[string]interface{}{"description": "Write hello world"},
	}
	srv, _, _, _, reviewCalls, _ := fullExecMockServer(t, task)

	cfg := buildExecutorConfig()
	cfg.Agents.TaskPollIntervalSec = 1

	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
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

	// Successful non-reviewer tasks must submit for review, not complete directly.
	deadline := time.After(8 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for submit-for-review (got %d)", reviewCalls.Load())
		case <-time.After(100 * time.Millisecond):
			if reviewCalls.Load() >= 1 {
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
		Role:    "worker",
		Status:  db.TaskStatusBacklog,
		Payload: map[string]interface{}{"description": "Fail me"},
	}
	srv, nextCalls, _, resultCalls, _, resultStatus := fullExecMockServer(t, task)

	cfg := buildExecutorConfig()
	cfg.Agents.TaskPollIntervalSec = 1

	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
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
				if got := resultStatus.Load().(string); got != db.TaskStatusFailed {
					t.Errorf("result status = %q, want %q", got, db.TaskStatusFailed)
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

// TestExecutor_PostsCompletionComment verifies that after a successful task
// the executor calls POST /api/tasks/{id}/comments with the agent as author.
func TestExecutor_PostsCompletionComment(t *testing.T) {
	const agentID = "comment-agent"
	var commentCalls atomic.Int32
	var commentBody atomic.Value
	commentBody.Store("")

	task := &db.Task{
		ID:      "comment-task",
		Role:    "worker",
		Status:  db.TaskStatusDeveloping,
		Payload: map[string]interface{}{"description": "Do something"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks/"+task.ID+"/result", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/submit-for-review", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			commentCalls.Add(1)
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if b, ok := body["body"].(string); ok {
				commentBody.Store(b)
			}
		}
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
	_ = reg.Register("mock", &mockLLMProvider{
		name: "mock",
		response: llm.ChatResponse{
			Content:    "Task done successfully.",
			StopReason: "end_turn",
			TokensUsed: 42,
		},
	})
	rtr := router.New(cfg, reg)
	client := agent.NewServerClient(srv.URL)
	exec := agent.NewExecutor(rtr, tools.NewRegistry(), client, agentID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exec.Run(ctx, task)

	if commentCalls.Load() < 1 {
		t.Error("expected at least one POST to /comments, got none")
	}
	body := commentBody.Load().(string)
	if body == "" {
		t.Error("expected non-empty comment body")
	}
	// Successful non-reviewer tasks now go to AWAITING_REVIEW.
	if !strings.Contains(strings.ToUpper(body), "AWAITING_REVIEW") {
		t.Errorf("comment body should mention AWAITING_REVIEW, got: %q", body)
	}
}

// TestClaimTask_RepoURLAndBranchPropagated verifies that repo_url and branch
// from the claim response are applied to the returned task so executeTask can
// clone the repo before running the executor.
func TestClaimTask_RepoURLAndBranchPropagated(t *testing.T) {
	const repoURL = "http://localhost:8080/git/my-project.git"
	const branch = "task/abc123"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks/claim-task/claim", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"task": map[string]interface{}{
				"id":     "claim-task",
				"type":   "implement",
				"role":   "worker",
				"status": db.TaskStatusDeveloping,
			},
			"repo_url": repoURL,
			"branch":   branch,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := agent.NewServerClient(srv.URL)
	ctx := context.Background()
	task, err := client.ClaimTask(ctx, "claim-task", "agent-1")
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	if task.RepoURL != repoURL {
		t.Errorf("RepoURL = %q, want %q", task.RepoURL, repoURL)
	}
	if task.Branch != branch {
		t.Errorf("Branch = %q, want %q", task.Branch, branch)
	}
}

// TestExecutor_SubmitsForReviewOnSuccess verifies that a successful non-reviewer
// task calls submit-for-review (not /result) so it enters AWAITING_REVIEW.
func TestExecutor_SubmitsForReviewOnSuccess(t *testing.T) {
	var reviewCalled atomic.Bool

	task := &db.Task{
		ID:      "status-task",
		Role:    "worker",
		Status:  db.TaskStatusDeveloping,
		Payload: map[string]interface{}{"description": "Verify review submission"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks/"+task.ID+"/submit-for-review", func(w http.ResponseWriter, _ *http.Request) {
		reviewCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/result", func(w http.ResponseWriter, _ *http.Request) {
		// Should not be called for a successful non-reviewer task.
		t.Error("unexpected call to /result for successful non-reviewer task")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
	_ = reg.Register("mock", &mockLLMProvider{
		name: "mock",
		response: llm.ChatResponse{
			Content:    "Done.",
			StopReason: "end_turn",
			TokensUsed: 10,
		},
	})
	rtr := router.New(cfg, reg)
	client := agent.NewServerClient(srv.URL)
	exec := agent.NewExecutor(rtr, tools.NewRegistry(), client, "status-agent")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exec.Run(ctx, task)

	if !reviewCalled.Load() {
		t.Error("expected submit-for-review to be called, but it was not")
	}
}

// TestExecutor_SubmitsReviewOnReviewingTask verifies that a task claimed for
// review (status REVIEWING, review_role set) posts a verdict to /reviews rather
// than calling submit-for-review again (Bug 9 B). Routing resolves the review
// role, not the task's original implementation role.
func TestExecutor_SubmitsReviewOnReviewingTask(t *testing.T) {
	var reviewPosted atomic.Bool

	task := &db.Task{
		ID:         "review-task",
		Role:       "worker",
		ReviewRole: "reviewer",
		Status:     db.TaskStatusReviewing,
		Payload:    map[string]interface{}{"description": "Review the change"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks/"+task.ID+"/reviews", func(w http.ResponseWriter, _ *http.Request) {
		reviewPosted.Store(true)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/submit-for-review", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected call to submit-for-review for a review task")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/result", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := buildExecutorConfig()

	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
	_ = reg.Register("mock", &mockLLMProvider{
		name: "mock",
		response: llm.ChatResponse{
			Content:    `{"review_status":"approved","review_body":"LGTM"}`,
			StopReason: "end_turn",
			TokensUsed: 10,
		},
	})
	// Wire both worker and reviewer to the mock provider so the review role resolves.
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
	rtr := router.New(cfg, reg)
	client := agent.NewServerClient(srv.URL)
	exec := agent.NewExecutor(rtr, tools.NewRegistry(), client, "review-agent")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exec.Run(ctx, task)

	if !reviewPosted.Load() {
		t.Error("expected a review verdict to be posted to /reviews, but it was not")
	}
}

// TestExecutor_UppercaseStatusOnFailure verifies that an LLM error results in
// status=FAILED (not lowercase "failed").
func TestExecutor_UppercaseStatusOnFailure(t *testing.T) {
	var submittedStatus atomic.Value
	submittedStatus.Store("")

	task := &db.Task{
		ID:      "fail-status-task",
		Role:    "worker",
		Status:  db.TaskStatusDeveloping,
		Payload: map[string]interface{}{"description": "Fail me"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks/"+task.ID+"/result", func(w http.ResponseWriter, r *http.Request) {
		var req api.SubmitTaskResultRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		submittedStatus.Store(req.Status)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
	_ = reg.Register("mock", &mockLLMProvider{
		name:    "mock",
		chatErr: errors.New("simulated LLM error"),
	})
	rtr := router.New(cfg, reg)
	client := agent.NewServerClient(srv.URL)
	exec := agent.NewExecutor(rtr, tools.NewRegistry(), client, "fail-status-agent")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exec.Run(ctx, task)

	got := submittedStatus.Load().(string)
	if got != db.TaskStatusFailed {
		t.Errorf("submitted status = %q, want %q (FAILED)", got, db.TaskStatusFailed)
	}
}

// TestExecutor_PushFailureSetsTaskFailed verifies that when CommitAndPush fails
// (e.g. the worktree path doesn't exist), the task is submitted as FAILED and
// not forwarded to submit-for-review.
func TestExecutor_PushFailureSetsTaskFailed(t *testing.T) {
	var submittedStatus atomic.Value
	var reviewCalled atomic.Bool
	submittedStatus.Store("")

	task := &db.Task{
		ID:           "push-fail-task",
		Role:         "worker",
		Status:       db.TaskStatusDeveloping,
		WorktreePath: "/nonexistent/worktree/path", // will cause CommitAndPush to fail
		Payload:      map[string]interface{}{"description": "Push will fail"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks/"+task.ID+"/result", func(w http.ResponseWriter, r *http.Request) {
		var req api.SubmitTaskResultRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		submittedStatus.Store(req.Status)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/submit-for-review", func(w http.ResponseWriter, _ *http.Request) {
		reviewCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
	_ = reg.Register("mock", &mockLLMProvider{
		name: "mock",
		response: llm.ChatResponse{
			Content:    "Done.",
			StopReason: "end_turn",
			TokensUsed: 10,
		},
	})
	rtr := router.New(cfg, reg)
	client := agent.NewServerClient(srv.URL)
	exec := agent.NewExecutor(rtr, tools.NewRegistry(), client, "push-fail-agent")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exec.Run(ctx, task)

	if got := submittedStatus.Load().(string); got != db.TaskStatusFailed {
		t.Errorf("submitted status = %q, want %q", got, db.TaskStatusFailed)
	}
	if reviewCalled.Load() {
		t.Error("submit-for-review must not be called when push fails")
	}
}

// TestExecutor_InjectsRepoPath verifies that the executor injects task.WorktreePath
// as repo_path into tool call arguments when the LLM omits it.
func TestExecutor_InjectsRepoPath(t *testing.T) {
	const agentID = "inject-agent"
	const worktreePath = "/tmp/test-worktree"

	// Record the arguments received by the tool.
	var capturedArgs atomic.Value
	capturedArgs.Store(map[string]interface{}{})

	task := &db.Task{
		ID:          "inject-task",
		Role:        "worker",
		Status:      db.TaskStatusDeveloping,
		WorktreePath: worktreePath,
		Payload:     map[string]interface{}{"description": "Test injection"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks/"+task.ID+"/result", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
	_ = reg.Register("mock", &mockLLMProvider{
		name: "mock",
		response: llm.ChatResponse{
			// First call returns a tool call with no repo_path.
			// Second call (after tool result) ends the loop.
			Content:    "",
			StopReason: "tool_calls",
			TokensUsed: 10,
			ToolCalls: []llm.ToolCall{
				{ID: "tc1", Name: "spy_tool", Arguments: map[string]interface{}{
					"file_path": "hello.txt",
					// repo_path intentionally omitted
				}},
			},
		},
	})

	rtr := router.New(cfg, reg)
	toolReg := tools.NewRegistry()

	// Register a spy tool that captures args.
	_ = toolReg.Register(tools.Definition{
		Name:    "spy_tool",
		Handler: func(_ context.Context, args map[string]interface{}) (interface{}, error) {
			capturedArgs.Store(args)
			return map[string]interface{}{"ok": true}, nil
		},
	})

	client := agent.NewServerClient(srv.URL)
	exec := agent.NewExecutor(rtr, toolReg, client, agentID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exec.Run(ctx, task)

	args := capturedArgs.Load().(map[string]interface{})
	got, ok := args["repo_path"]
	if !ok {
		t.Fatal("expected repo_path to be injected into tool arguments, but it was missing")
	}
	if got != worktreePath {
		t.Errorf("repo_path = %q, want %q", got, worktreePath)
	}
}

// TestExecutor_LogsErrorToServer verifies that when the LLM fails, the executor
// ships an error-level entry to POST /api/logs.
func TestExecutor_LogsErrorToServer(t *testing.T) {
	const agentID = "log-agent"
	var logCalls atomic.Int32
	var logLevel atomic.Value
	var sawErrorLevel atomic.Bool
	logLevel.Store("")

	task := &db.Task{
		ID:      "log-fail-task",
		Role:    "worker",
		Status:  db.TaskStatusDeveloping,
		Payload: map[string]interface{}{"description": "Fail me"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks/"+task.ID+"/result", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tasks/"+task.ID+"/comments", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			logCalls.Add(1)
			var entry map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&entry)
			if lvl, ok := entry["level"].(string); ok {
				logLevel.Store(lvl)
				if lvl == "error" {
					sawErrorLevel.Store(true)
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := buildExecutorConfig()
	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"worker", "reviewer"})
	_ = reg.Register("mock", &mockLLMProvider{
		name:    "mock",
		chatErr: errors.New("simulated LLM error"),
	})
	rtr := router.New(cfg, reg)
	client := agent.NewServerClient(srv.URL)
	exec := agent.NewExecutor(rtr, tools.NewRegistry(), client, agentID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exec.Run(ctx, task)

	if logCalls.Load() < 1 {
		t.Error("expected at least one POST to /api/logs, got none")
	}
	if !sawErrorLevel.Load() {
		t.Errorf("expected at least one log entry with level %q, last level was %q", "error", logLevel.Load().(string))
	}
}
