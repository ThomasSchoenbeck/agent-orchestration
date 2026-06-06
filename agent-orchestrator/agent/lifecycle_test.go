package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/api"
)

// TestAgentHonorsStop verifies the agent calls SetOffline and stops when a
// heartbeat returns desired_state="stop" (Feature 7).
func TestAgentHonorsStop(t *testing.T) {
	const agentID = "stop-agent"
	var offlineCalls atomic.Int32

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
			_ = json.NewEncoder(w).Encode(api.HeartbeatResponse{DesiredState: "stop"})
		case strings.HasSuffix(r.URL.Path, "/offline"):
			offlineCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "offline"})
		case strings.Contains(r.URL.Path, "/tasks/next"):
			_ = json.NewEncoder(w).Encode(nil)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	srv := httptest.NewServer(wrapAgentAPI(mux))
	t.Cleanup(srv.Close)

	cfg := testConfig()
	cfg.Agents.HeartbeatIntervalSec = 1

	a := agent.NewAgent("stop-test", []string{"worker"}, srv.URL, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	deadline := time.After(6 * time.Second)
	for offlineCalls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("agent did not go offline after receiving stop")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
