package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/api"
	"agent-orchestrator/db"
	"agent-orchestrator/git"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
	"agent-orchestrator/tools"
)

// cleanupMockServer serves the given task once (with repo_url + branch so the
// agent clones a real workspace) and counts terminal submissions. doneCalls is
// incremented on /submit-for-review and /result, which mark the end of a task.
func cleanupMockServer(t *testing.T, task *db.Task, repoURL, branch string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	const agentID = "cleanup-agent"
	var nextCalls atomic.Int32
	var doneCalls atomic.Int32

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
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"task":     task,
					"repo_url": repoURL,
					"branch":   branch,
				})
			} else {
				_ = json.NewEncoder(w).Encode(nil)
			}
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost &&
			(strings.HasSuffix(r.URL.Path, "/submit-for-review") || strings.HasSuffix(r.URL.Path, "/result")) {
			doneCalls.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &doneCalls
}

// runCleanupAgent wires an agent with a real git remote and a mock LLM, runs it
// until the task finishes, and returns the local workspace path that should have
// been removed.
func runCleanupAgent(t *testing.T, slug string, llmErr error) string {
	t.Helper()

	// Real bare repo + git HTTP server so the clone succeeds.
	repoPath := filepath.Join(t.TempDir(), "srv.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "main", "seed.txt", []byte("seed"), "init", "u", "u@x"); err != nil {
		t.Fatalf("CommitFile seed: %v", err)
	}
	repoURL := startRepoServer(t, slug, repoPath)

	taskID := "cleanup-" + slug
	branch := "task/" + taskID
	task := &db.Task{ID: taskID, Role: "worker", Status: db.TaskStatusBacklog}

	srv, doneCalls := cleanupMockServer(t, task, repoURL, branch)

	workdir := t.TempDir()
	localPath := filepath.Join(workdir, taskID)

	cfg := testConfig()
	cfg.Agents.TaskPollIntervalSec = 1

	reg := llm.NewRegistry()
	_ = reg.Register("mock", &mockLLMProvider{
		name:     "mock",
		response: llm.ChatResponse{Content: "done", StopReason: "end_turn"},
		chatErr:  llmErr,
	})
	rtr := router.New(buildExecutorConfig(), reg)

	a := agent.NewAgent("cleanup-test", []string{"worker"}, srv.URL, cfg)
	a.WithWorkdir(workdir)
	a.WithExecutor(rtr, tools.NewRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop()

	// Wait for the task to reach a terminal submission.
	deadline := time.After(8 * time.Second)
	for doneCalls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for task to finish")
		case <-time.After(100 * time.Millisecond):
		}
	}
	return localPath
}

// assertRemoved polls until path no longer exists or the deadline passes.
func assertRemoved(t *testing.T, path string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		select {
		case <-deadline:
			t.Errorf("workspace %q was not removed after task finished", path)
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestAgentCleanup_RemovesWorkdir verifies the local workspace is deleted after
// a task completes successfully.
func TestAgentCleanup_RemovesWorkdir(t *testing.T) {
	localPath := runCleanupAgent(t, "cleanup-ok", nil)
	assertRemoved(t, localPath)
}

// TestAgentCleanup_OnFailure verifies the local workspace is deleted even when
// the task fails (LLM error).
func TestAgentCleanup_OnFailure(t *testing.T) {
	localPath := runCleanupAgent(t, "cleanup-fail", context.DeadlineExceeded)
	assertRemoved(t, localPath)
}
