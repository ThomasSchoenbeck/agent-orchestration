package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"agent-orchestrator/db"
	"agent-orchestrator/workflow"
)

// itFakeProc / itFakeLauncher are a non-spawning launcher for the managed-agent
// E2E test (the real launcher would exec the orchestrator binary).
type itFakeProc struct {
	done chan struct{}
}

func (p *itFakeProc) PID() int              { return 1 }
func (p *itFakeProc) Done() <-chan struct{} { return p.done }
func (p *itFakeProc) Kill() error {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	return nil
}

type itFakeLauncher struct {
	mu    sync.Mutex
	procs []*itFakeProc
}

func (l *itFakeLauncher) Launch(_ []string) (workflow.ManagedProcess, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := &itFakeProc{done: make(chan struct{})}
	l.procs = append(l.procs, p)
	return p, nil
}
func (l *itFakeLauncher) count() int { l.mu.Lock(); defer l.mu.Unlock(); return len(l.procs) }

// TestManagedAgent_E2E drives the managed-agent lifecycle through the public API:
// define a template → start → instances are created → scale to 0 → instances stop.
// (A real task-claiming child process is out of scope for an in-binary test;
// process spawning is verified via the injected fake launcher.)
func TestManagedAgent_E2E(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	fl := &itFakeLauncher{}
	srv.Srv.SetAgentLauncher(fl)

	// Define a template via the API.
	var tpl db.AgentTemplate
	if st := apiJSON(t, "POST", srv.BaseURL, "/api/agent-templates",
		map[string]interface{}{"name": "mw", "roles": []string{"worker"}, "replicas": 2}, &tpl); st != 201 {
		t.Fatalf("create template: status %d", st)
	}

	// Start it → two instances spawn and register as mw-1 / mw-2.
	apiJSON(t, "POST", srv.BaseURL, "/api/agent-templates/"+tpl.ID+"/start", nil, nil)

	instances, _ := srv.DB.ListAgentsByTemplate(context.Background(), tpl.ID)
	if len(instances) != 2 {
		t.Fatalf("expected 2 instance rows, got %d", len(instances))
	}
	if fl.count() != 2 {
		t.Fatalf("expected 2 launched processes, got %d", fl.count())
	}

	// Scale to 0 → instances stop and go offline.
	apiJSON(t, "POST", srv.BaseURL, "/api/agent-templates/"+tpl.ID+"/scale",
		map[string]interface{}{"replicas": 0}, nil)

	deadline := time.After(3 * time.Second)
	for {
		offline := 0
		rows, _ := srv.DB.ListAgentsByTemplate(context.Background(), tpl.ID)
		for _, a := range rows {
			if a.Status == "offline" {
				offline++
			}
		}
		if offline == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("instances did not go offline after scale to 0")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
