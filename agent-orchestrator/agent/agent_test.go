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
