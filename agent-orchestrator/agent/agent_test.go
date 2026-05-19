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

// taskOnceServer builds a mock server that serves the given task from
// tasks/next exactly once, then returns nil. It also handles register,
// heartbeat, and claim endpoints.
// Returns the server, a pointer to the tasks/next call counter, and a pointer
// to the claim call counter.
func taskOnceServer(t *testing.T, task *db.Task) (*httptest.Server, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	const agentID = "agent-poll-test"
	var nextCalls, claimCalls atomic.Int32

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
	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/claim") {
			claimCalls.Add(1)
			_ = json.NewEncoder(w).Encode(task)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &nextCalls, &claimCalls
}

// TestAgent_PollLoop_PicksUpTask verifies that when tasks/next returns a task,
// the agent proceeds to call the claim endpoint exactly once.
func TestAgent_PollLoop_PicksUpTask(t *testing.T) {
	task := &db.Task{
		ID:     "task-poll-001",
		Type:   "implement",
		Role:   "worker",
		Status: db.TaskStatusBacklog,
	}
	srv, _, claimCalls := taskOnceServer(t, task)

	cfg := testConfig()
	cfg.Agents.TaskPollIntervalSec = 1

	a := agent.NewAgent("poll-test", []string{"worker"}, srv.URL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	// Wait for the first claim.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for claim (got %d)", claimCalls.Load())
		case <-time.After(100 * time.Millisecond):
			if claimCalls.Load() >= 1 {
				goto claimed
			}
		}
	}
claimed:
	// After the task is claimed, tasks/next returns nil. Wait two poll
	// intervals to confirm the agent does NOT claim a second time.
	time.Sleep(2500 * time.Millisecond)
	if claimCalls.Load() > 1 {
		t.Errorf("expected exactly 1 claim, got %d", claimCalls.Load())
	}
}

// TestAgent_PollLoop_SkipsWrongRoleTask verifies that an agent with role
// "worker" never claims a task whose role requirement is "reviewer".
// The mock simulates the server's role filter: tasks/next only returns a task
// when the ?roles= param includes the task's required role.
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
				_ = json.NewEncoder(w).Encode(reviewerTask)
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
