package agent

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/router"
	"agent-orchestrator/tools"
)

// Agent represents a running agent process.
type Agent struct {
	id        string
	name      string
	roles     []string
	mode      string // colocated | remote
	serverURL string
	cfg       *config.Config
	client    *ServerClient
	executor  *Executor
	done      chan struct{}
	workdir   string // local root for agent code checkouts (remote mode)
}

// NewAgent creates a new agent (not yet registered). rtr and toolReg are
// optional; pass nil to run without task execution (useful in tests).
func NewAgent(name string, roles []string, serverURL string, cfg *config.Config) *Agent {
	return &Agent{
		name:      name,
		roles:     roles,
		mode:      "remote",
		serverURL: serverURL,
		cfg:       cfg,
		client:    NewServerClient(serverURL),
		done:      make(chan struct{}),
	}
}

// WithMode sets the agent's operating mode ("colocated" or "remote").
func (a *Agent) WithMode(mode string) *Agent {
	a.mode = mode
	return a
}

// WithWorkdir sets the local filesystem root under which the agent creates
// per-task checkout directories. Used in remote mode; ignored in colocated mode.
// If never called (or set to ""), LocalWorkspacePath falls back to ".agent-work".
func (a *Agent) WithWorkdir(dir string) *Agent {
	a.workdir = dir
	return a
}

// LocalWorkspacePath returns the path the agent should use as its working
// directory for the given task: {workdir}/{taskID}.
// Defaults to ".agent-work/{taskID}" when no workdir is configured.
func (a *Agent) LocalWorkspacePath(taskID string) string {
	root := a.workdir
	if root == "" {
		root = ".agent-work"
	}
	return filepath.Join(root, taskID)
}

// WithExecutor attaches an LLM router and tool registry so the agent can
// fully execute tasks. Call before Start.
func (a *Agent) WithExecutor(rtr *router.Router, toolReg *tools.Registry) *Agent {
	// executor.agentID will be set in Start once we have a server-assigned ID.
	a.executor = &Executor{
		rtr:   rtr,
		tools: toolReg,
	}
	return a
}

// ID returns the agent's server-assigned ID (empty before Start).
func (a *Agent) ID() string { return a.id }

// Roles returns the agent's configured roles.
func (a *Agent) Roles() []string { return a.roles }

// Start registers with the server and begins the heartbeat + polling loops.
// It makes a single registration attempt and returns an error if it fails.
// For automatic retry on startup, use StartWithReconnect instead.
func (a *Agent) Start(ctx context.Context) error {
	caps := map[string]interface{}{
		"go_version": "1.22",
	}

	id, err := a.client.Register(ctx, a.name, a.roles, a.mode, caps)
	if err != nil {
		return fmt.Errorf("agent %q: registration failed: %w", a.name, err)
	}
	a.id = id
	log.Printf("agent %q registered (id=%s)", a.name, a.id)

	// Wire the agent ID into the executor now that we have it.
	if a.executor != nil {
		a.executor.client = a.client
		a.executor.agentID = a.id
	}

	go a.heartbeatLoop(ctx)
	go a.pollLoop(ctx)
	return nil
}

// Stop signals the agent to stop.
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
				log.Printf("agent %q heartbeat error: %v — retrying in 5s", a.name, err)
				select {
				case <-time.After(5 * time.Second):
					if err2 := a.client.Heartbeat(ctx, a.id); err2 != nil {
						log.Printf("agent %q heartbeat retry failed: %v", a.name, err2)
					}
				case <-ctx.Done():
					return
				case <-a.done:
					return
				}
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
			go func(t *db.Task) {
				claimed, err := a.client.ClaimTask(ctx, t.ID, a.id)
				if err != nil {
					log.Printf("agent %q could not claim task %s: %v", a.name, t.ID, err)
					return
				}
				a.executeTask(ctx, claimed)
			}(task)
		}
	}
}

// executeTask runs the claimed task via the executor, or logs a warning if no
// executor is configured.
func (a *Agent) executeTask(ctx context.Context, task *db.Task) {
	if a.executor == nil || a.executor.rtr == nil {
		log.Printf("agent %q: no executor configured — skipping task %s", a.name, task.ID)
		return
	}
	a.executor.Run(ctx, task)
}
