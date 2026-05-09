package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"agent-orchestrator/config"
)

// Agent represents a running agent process.
type Agent struct {
	id        string
	name      string
	roles     []string
	serverURL string
	cfg       *config.Config
	client    *ServerClient
	done      chan struct{}
}

// NewAgent creates a new agent (not yet registered).
func NewAgent(name string, roles []string, serverURL string, cfg *config.Config) *Agent {
	return &Agent{
		name:      name,
		roles:     roles,
		serverURL: serverURL,
		cfg:       cfg,
		client:    NewServerClient(serverURL),
		done:      make(chan struct{}),
	}
}

// ID returns the agent's server-assigned ID (empty before Start).
func (a *Agent) ID() string { return a.id }

// Start registers with the server and begins the heartbeat + polling loops.
// It returns after the loops are started; call Stop to terminate them.
func (a *Agent) Start(ctx context.Context) error {
	caps := map[string]interface{}{
		"go_version": "1.22",
	}
	id, err := a.client.Register(ctx, a.name, a.roles, caps)
	if err != nil {
		return fmt.Errorf("agent %q registration failed: %w", a.name, err)
	}
	a.id = id
	log.Printf("agent %q registered (id=%s)", a.name, a.id)

	go a.heartbeatLoop(ctx)
	go a.pollLoop(ctx)
	return nil
}

// Stop signals the agent to stop and deregisters from the server.
func (a *Agent) Stop() {
	select {
	case <-a.done:
	default:
		close(a.done)
	}
}

// heartbeatLoop sends periodic heartbeats to keep the agent marked online.
func (a *Agent) heartbeatLoop(ctx context.Context) {
	interval := time.Duration(a.cfg.Agents.HeartbeatIntervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.done:
			return
		case <-ticker.C:
			if err := a.client.Heartbeat(ctx, a.id); err != nil {
				log.Printf("agent %q heartbeat error: %v", a.name, err)
			}
		}
	}
}

// pollLoop polls for tasks and executes them.
func (a *Agent) pollLoop(ctx context.Context) {
	interval := time.Duration(a.cfg.Agents.TaskPollIntervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.done:
			return
		case <-ticker.C:
			task, err := a.client.GetNextTask(ctx, a.id, a.roles)
			if err != nil {
				log.Printf("agent %q poll error: %v", a.name, err)
				continue
			}
			if task == nil {
				continue // nothing to do
			}

			// Claim and execute asynchronously so polling continues.
			go func() {
				claimed, err := a.client.ClaimTask(ctx, task.ID, a.id)
				if err != nil {
					log.Printf("agent %q could not claim task %s: %v", a.name, task.ID, err)
					return
				}
				a.executeTask(ctx, claimed)
			}()
		}
	}
}

// executeTask is the stub execution entry-point. Phase 2 will wire in the
// real LLM + tool execution here.
func (a *Agent) executeTask(ctx context.Context, task interface{}) {
	log.Printf("agent %q: task claimed (full execution wired in Phase 2)", a.name)
	// Phase 2: call LLM, run tools, submit result.
}
