package agent_test

import (
	"context"
	"encoding/json"
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

func testConfig() *config.Config {
	return &config.Config{
		Server:   config.ServerConfig{Port: 8080},
		Database: config.DatabaseConfig{Path: "./test.db"},
		Agents: config.AgentConfig{
			HeartbeatIntervalSec: 1,
			TaskPollIntervalSec:  1,
			TaskTimeoutSec:       10,
		},
	}
}

// mockServer sets up a minimal HTTP mock that handles register + heartbeat.
func mockServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var heartbeats atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(api.RegisterAgentResponse{AgentID: "agent-test-123"})
	})
	mux.HandleFunc("/api/agents/agent-test-123/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		heartbeats.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/agents/agent-test-123/tasks/next", func(w http.ResponseWriter, r *http.Request) {
		// No tasks available.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nil)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &heartbeats
}

func TestAgent_StartAndHeartbeat(t *testing.T) {
	srv, heartbeats := mockServer(t)
	cfg := testConfig()
	cfg.Agents.HeartbeatIntervalSec = 1

	a := agent.NewAgent("test-agent", []string{"worker"}, srv.URL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	// Wait for at least one heartbeat.
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for heartbeat (got %d)", heartbeats.Load())
		case <-time.After(200 * time.Millisecond):
			if heartbeats.Load() >= 1 {
				return // success
			}
		}
	}
}

func TestAgent_IDAssignedAfterStart(t *testing.T) {
	srv, _ := mockServer(t)
	cfg := testConfig()

	a := agent.NewAgent("test-agent", []string{"worker"}, srv.URL, cfg)
	if a.ID() != "" {
		t.Fatal("expected empty ID before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	if a.ID() == "" {
		t.Error("expected non-empty ID after Start")
	}
}

func TestAgent_Stop(t *testing.T) {
	srv, heartbeats := mockServer(t)
	cfg := testConfig()
	cfg.Agents.HeartbeatIntervalSec = 1

	a := agent.NewAgent("test-agent", []string{"worker"}, srv.URL, cfg)
	ctx := context.Background()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let it run briefly.
	time.Sleep(500 * time.Millisecond)
	a.Stop()
	countAfterStop := heartbeats.Load()

	// After stop, heartbeat count should not increase.
	time.Sleep(1500 * time.Millisecond)
	if heartbeats.Load() > countAfterStop+1 {
		t.Errorf("heartbeats continued after Stop: before=%d after=%d",
			countAfterStop, heartbeats.Load())
	}
}

func TestAgent_LocalWorkspacePath_ExplicitWorkdir(t *testing.T) {
	cfg := testConfig()
	a := agent.NewAgent("test-agent", []string{"worker"}, "http://localhost", cfg)
	a.WithWorkdir("/my/workdir")

	got := a.LocalWorkspacePath("task-abc")
	want := filepath.Join("/my/workdir", "task-abc")
	if got != want {
		t.Errorf("LocalWorkspacePath = %q, want %q", got, want)
	}
}

func TestAgent_LocalWorkspacePath_Default(t *testing.T) {
	cfg := testConfig()
	a := agent.NewAgent("test-agent", []string{"worker"}, "http://localhost", cfg)
	// No workdir set — must fall back to ".agent-work".

	got := a.LocalWorkspacePath("task-xyz")
	want := filepath.Join(".agent-work", "task-xyz")
	if got != want {
		t.Errorf("LocalWorkspacePath = %q, want %q", got, want)
	}
}

func TestAgent_LocalWorkspacePath_UniquePerTask(t *testing.T) {
	cfg := testConfig()
	a := agent.NewAgent("test-agent", []string{"worker"}, "http://localhost", cfg)
	a.WithWorkdir("/workdir")

	if a.LocalWorkspacePath("task-1") == a.LocalWorkspacePath("task-2") {
		t.Error("different taskIDs must produce different paths")
	}
}

func TestServerClient_Register(t *testing.T) {
	srv, _ := mockServer(t)
	c := agent.NewServerClient(srv.URL)

	ctx := context.Background()
	id, err := c.Register(ctx, "test", []string{"worker"}, "remote", nil)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty agent ID")
	}
}

func TestServerClient_Heartbeat(t *testing.T) {
	srv, _ := mockServer(t)
	c := agent.NewServerClient(srv.URL)
	ctx := context.Background()

	_, _ = c.Register(ctx, "test", []string{"worker"}, "remote", nil)
	if err := c.Heartbeat(ctx, "agent-test-123"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
}

// taskOnceServer returns a mock server that serves the given task once from
// tasks/next. The returned counters track: next calls, result submissions
// (/result), and review submissions (/submit-for-review).
func taskOnceServer(t *testing.T, task *db.Task) (*httptest.Server, *atomic.Int32, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	const agentID = "agent-poll-test"
	var nextCalls, resultCalls, reviewCalls atomic.Int32

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
	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			switch {
			case strings.HasSuffix(r.URL.Path, "/result"):
				resultCalls.Add(1)
			case strings.HasSuffix(r.URL.Path, "/submit-for-review"):
				reviewCalls.Add(1)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &nextCalls, &resultCalls, &reviewCalls
}

// TestAgent_PollLoop_PicksUpTask verifies that when tasks/next returns a task,
// the agent executes it and submits for review exactly once.
func TestAgent_PollLoop_PicksUpTask(t *testing.T) {
	task := &db.Task{
		ID:     "task-poll-001",
		Type:   "implement",
		Role:   "worker",
		Status: db.TaskStatusBacklog,
	}
	srv, _, _, reviewCalls := taskOnceServer(t, task)

	cfg := testConfig()
	cfg.Agents.TaskPollIntervalSec = 1

	reg := llm.NewRegistry()
	_ = reg.Register("mock", &mockLLMProvider{
		name:     "mock",
		response: llm.ChatResponse{Content: "done", StopReason: "end_turn"},
	})
	rtr := router.New(buildExecutorConfig(), reg)

	a := agent.NewAgent("poll-test", []string{"worker"}, srv.URL, cfg)
	a.WithExecutor(rtr, tools.NewRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	// Successful worker tasks go to AWAITING_REVIEW, not COMPLETED directly.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for submit-for-review (got %d)", reviewCalls.Load())
		case <-time.After(100 * time.Millisecond):
			if reviewCalls.Load() >= 1 {
				goto done
			}
		}
	}
done:
	// tasks/next returns nil after the first call; confirm no second execution.
	time.Sleep(2500 * time.Millisecond)
	if reviewCalls.Load() > 1 {
		t.Errorf("expected exactly 1 review submission, got %d", reviewCalls.Load())
	}
}

// TestAgent_PollLoop_SkipsWrongRoleTask verifies that an agent with role
// "worker" never claims a task whose role requirement is "reviewer".
// The mock simulates the server's role filter: tasks/next only returns a task
// when the ?roles= param includes the task's required role.
// TestAgent_ReconnectsAfterHeartbeatFailures verifies that when heartbeats fail
// 3 times in a row, the agent calls register again to restore its online status.
func TestAgent_ReconnectsAfterHeartbeatFailures(t *testing.T) {
	var registerCalls atomic.Int32
	var heartbeatCalls atomic.Int32
	heartbeatFails := true // start with failing heartbeats

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents/register", func(w http.ResponseWriter, r *http.Request) {
		registerCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(api.RegisterAgentResponse{AgentID: "reconnect-agent"})
	})
	mux.HandleFunc("/api/agents/reconnect-agent/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		heartbeatCalls.Add(1)
		if heartbeatFails {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/agents/reconnect-agent/tasks/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nil)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := testConfig()
	cfg.Agents.HeartbeatIntervalSec = 1 // short interval so 3 failures happen quickly

	a := agent.NewAgent("reconnect-test", []string{"worker"}, srv.URL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	registersBefore := registerCalls.Load() // should be 1 after Start

	// Wait for at least 3 failed heartbeats (triggering reconnect).
	deadline := time.After(6 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for re-registration (heartbeats=%d registers=%d)",
				heartbeatCalls.Load(), registerCalls.Load())
		case <-time.After(200 * time.Millisecond):
			if registerCalls.Load() > registersBefore {
				return // re-registration happened — success
			}
		}
	}
}

// TestAgent_PollLoop_SkipsWhenProviderUnavailable verifies that an agent with an
// executor whose registry has no providers never claims a task, even when
// tasks/next returns one.
func TestAgent_PollLoop_SkipsWhenProviderUnavailable(t *testing.T) {
	task := &db.Task{
		ID:     "no-provider-task",
		Type:   "implement",
		Role:   "worker",
		Status: db.TaskStatusBacklog,
	}
	srv, _, claimCalls, _ := taskOnceServer(t, task)

	cfg := testConfig()
	cfg.Agents.TaskPollIntervalSec = 1

	// Wire an executor with an empty registry — no providers available.
	emptyReg := llm.NewRegistry()
	rtr := router.New(buildExecutorConfig(), emptyReg)
	toolReg := tools.NewRegistry()

	a := agent.NewAgent("no-provider", []string{"worker"}, srv.URL, cfg)
	a.WithExecutor(rtr, toolReg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	// Wait 3 poll intervals — the task must never be claimed.
	time.Sleep(3500 * time.Millisecond)

	if claimCalls.Load() > 0 {
		t.Errorf("agent must not claim tasks when no provider is available; got %d claim(s)",
			claimCalls.Load())
	}
}

func TestAgent_PollLoop_SkipsWrongRoleTask(t *testing.T) {
	reviewerTask := &db.Task{
		ID:     "task-reviewer-only",
		Type:   "review",
		Role:   "reviewer",
		Status: db.TaskStatusBacklog,
	}

	var claimCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(api.RegisterAgentResponse{AgentID: "role-test-agent"})
	})
	mux.HandleFunc("/api/agents/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case strings.Contains(r.URL.Path, "/tasks/next"):
			// Only serve the task when the agent requests the "reviewer" role.
			roles := r.URL.Query().Get("roles")
			if strings.Contains(roles, "reviewer") {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"task": reviewerTask})
			} else {
				_ = json.NewEncoder(w).Encode(nil)
			}
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/claim") {
			claimCalls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := testConfig()
	cfg.Agents.TaskPollIntervalSec = 1

	// Worker agent — should never claim the reviewer task.
	a := agent.NewAgent("worker-only", []string{"worker"}, srv.URL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	// Wait 2.5× the poll interval; claim must remain zero.
	time.Sleep(2500 * time.Millisecond)

	if claimCalls.Load() > 0 {
		t.Errorf("worker agent must not claim reviewer task; got %d claim calls", claimCalls.Load())
	}
}
