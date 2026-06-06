package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agent-orchestrator/api"
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
	skills    []string // specializations this agent provides (Feature 6)
	mode      string   // colocated | remote
	serverURL string
	cfg       *config.Config
	client    *ServerClient
	executor  *Executor
	done      chan struct{}
	workdir   string // local root for agent code checkouts (remote mode)
	alog      *AgentLogger
	reloadFn  func() // optional: reload providers/router from DB on each heartbeat
}

// NewAgent creates a new agent (not yet registered). rtr and toolReg are
// optional; pass nil to run without task execution (useful in tests).
func NewAgent(name string, roles []string, serverURL string, cfg *config.Config) *Agent {
	c := NewServerClient(serverURL)
	return &Agent{
		name:      name,
		roles:     roles,
		mode:      "remote",
		serverURL: serverURL,
		cfg:       cfg,
		client:    c,
		done:      make(chan struct{}),
		alog:      newLogger("", c), // agentID set in Start after registration
	}
}

// Client returns the agent's HTTP server client, which implements
// tools.ToolBackend. Used to wire the DB-free tools.
func (a *Agent) Client() *ServerClient {
	return a.client
}

// WithClient replaces the agent's server client (e.g. with one configured for
// TLS + bearer auth). Must be called before Start. The interim logger is
// rebuilt; Start re-creates it again with the registered agent ID.
func (a *Agent) WithClient(c *ServerClient) *Agent {
	a.client = c
	a.alog = newLogger("", c)
	return a
}

// WithMode sets the agent's operating mode ("colocated" or "remote").
func (a *Agent) WithMode(mode string) *Agent {
	a.mode = mode
	return a
}

// WithSkills sets the specialization tags this agent provides (Feature 6). They
// are sent on registration and used to compose the agent's persona.
func (a *Agent) WithSkills(skills []string) *Agent {
	a.skills = skills
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

// WithReload registers a function that refreshes the agent's LLM provider
// registry and router from the database. It is invoked on each heartbeat tick
// so the agent picks up providers/roles added or changed after startup.
func (a *Agent) WithReload(fn func()) *Agent {
	a.reloadFn = fn
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

	id, err := a.client.Register(ctx, a.name, a.roles, a.skills, a.mode, caps)
	if err != nil {
		return fmt.Errorf("agent %q: registration failed: %w", a.name, err)
	}
	a.id = id
	a.alog = newLogger(id, a.client) // re-create with real ID
	a.alog.Info("agent %q registered (id=%s mode=%s)", a.name, a.id, a.mode)

	// Wire the agent ID into the executor now that we have it.
	if a.executor != nil {
		a.executor.client  = a.client
		a.executor.agentID = a.id
		a.executor.log     = newLogger(id, a.client)
		a.executor.skillNames = a.skills
	}

	go a.heartbeatLoop(ctx)
	go a.pollLoop(ctx)
	return nil
}

// Stop signals the agent to stop its internal loops.
// Call Deregister before Stop to notify the server of a clean shutdown.
func (a *Agent) Stop() {
	select {
	case <-a.done:
	default:
		close(a.done)
	}
}

// Deregister notifies the server that this agent is going offline gracefully.
// This prevents stale "online" records from persisting until the next
// heartbeat timeout cycle. Safe to call if the agent was never registered.
func (a *Agent) Deregister(ctx context.Context) {
	if a.id == "" {
		return
	}
	if err := a.client.SetOffline(ctx, a.id); err != nil {
		a.alog.Warn("deregister failed: %v", err)
	}
}

// reconnect re-registers the agent with the server without launching new
// goroutines. Used by heartbeatLoop to recover after consecutive failures.
func (a *Agent) reconnect(ctx context.Context) error {
	caps := map[string]interface{}{"go_version": "1.22"}
	id, err := a.client.Register(ctx, a.name, a.roles, a.skills, a.mode, caps)
	if err != nil {
		return err
	}
	a.id = id
	if a.executor != nil {
		a.executor.agentID = id
	}
	return nil
}

// heartbeatLoop sends periodic heartbeats to keep the agent marked online.
// After 3 consecutive failures it attempts to re-register so the server
// considers the agent online again (e.g. after a server restart).
func (a *Agent) heartbeatLoop(ctx context.Context) {
	interval := time.Duration(a.cfg.Agents.HeartbeatIntervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	consecutiveFailures := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.done:
			return
		case <-ticker.C:
			if a.reloadFn != nil {
				a.reloadFn()
			}
			ctrl, err := a.client.Heartbeat(ctx, a.id)
			if err != nil {
				consecutiveFailures++
				a.alog.Warn("heartbeat error (%d consecutive): %v", consecutiveFailures, err)
				if consecutiveFailures >= 3 {
					a.alog.Warn("attempting re-registration after %d consecutive heartbeat failures", consecutiveFailures)
					if rerr := a.reconnect(ctx); rerr != nil {
						a.alog.Error("re-registration failed: %v", rerr)
					} else {
						a.alog.Info("re-registered successfully (id=%s)", a.id)
						consecutiveFailures = 0
					}
				}
				continue
			}
			consecutiveFailures = 0
			// Feature 7: honor desired_state and recompose persona on live changes.
			if a.applyControl(ctrl) {
				a.alog.Info("received stop signal — going offline")
				_ = a.client.SetOffline(context.Background(), a.id)
				a.Stop()
				return
			}
		}
	}
}

// applyControl reacts to the server's heartbeat control response: it syncs the
// agent's live roles/skills (recomposing the persona on a skill change) and
// reports whether the agent has been told to stop. Only enriched (Feature 7)
// responses — which always carry a desired_state — drive control; minimal
// responses are ignored for backward compatibility.
func (a *Agent) applyControl(ctrl *api.HeartbeatResponse) (stop bool) {
	if ctrl == nil || ctrl.DesiredState == "" {
		return false
	}
	if len(ctrl.Roles) > 0 {
		a.roles = ctrl.Roles
	}
	if !strSlicesEqual(a.skills, ctrl.Skills) {
		a.skills = ctrl.Skills
		if a.executor != nil {
			a.executor.skillNames = ctrl.Skills
			a.executor.skillsResolved = false // force persona recompose on next task
		}
	}
	return ctrl.DesiredState == "stop"
}

func strSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// pollLoop polls for tasks and executes them.
// It skips polling entirely when no model provider is available for any of the
// agent's roles, logging once when the provider disappears and once when it
// comes back, so the log stays quiet during prolonged outages.
func (a *Agent) pollLoop(ctx context.Context) {
	interval := time.Duration(a.cfg.Agents.TaskPollIntervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	providerAvailable := true // optimistic — log only on transitions

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.done:
			return
		case <-ticker.C:
			// Guard: skip task pickup if the executor has no live provider.
			canRun := a.executor != nil && a.executor.CanExecute(a.roles)
			if !canRun {
				if providerAvailable {
					a.alog.Warn("no provider available for roles %v — pausing task pickup", a.roles)
					providerAvailable = false
				}
				continue
			}
			if !providerAvailable {
				a.alog.Info("provider available for roles %v — resuming task pickup", a.roles)
				providerAvailable = true
			}

			task, err := a.client.GetNextTask(ctx, a.id, a.roles)
			if err != nil {
				a.alog.Warn("poll error: %v", err)
				continue
			}
			if task == nil {
				continue // nothing to do
			}

			// GetNextTask already claimed the task and provisioned the worktree
			// server-side; execute directly without a redundant second claim.
			go func(t *db.Task) {
				a.executeTask(ctx, t)
			}(task)
		}
	}
}

// executeTask runs the claimed task via the executor, or logs a warning if no
// executor is configured. A deferred recover prevents a panicking task from
// taking down the whole agent process.
func (a *Agent) executeTask(ctx context.Context, task *db.Task) {
	tlog := a.alog.ForTask(task.ID)
	defer func() {
		if r := recover(); r != nil {
			tlog.ErrorCtx(ctx, "panic executing task: %v", r)
		}
	}()
	if a.executor == nil || a.executor.rtr == nil {
		tlog.Warn("no executor configured — skipping task")
		return
	}

	// Clone the project repo so the executor's file tools have a local workspace.
	// All agents use the server's git HTTP endpoint regardless of topology.
	// Planner/orchestrator tasks touch no files, so skip the clone entirely.
	if task.RepoURL != "" && !a.executor.IsPlannerTask(task) {
		localPath := a.LocalWorkspacePath(task.ID)
		branch := task.Branch
		if branch == "" {
			branch = fmt.Sprintf("task/%s", task.ID)
		}
		if err := CloneOrOpen(task.RepoURL, localPath, branch); err != nil {
			tlog.ErrorCtx(ctx, "clone failed, task cannot proceed: %v", err)
			return
		}
		task.WorktreePath = localPath
		// Guarantee the local workspace is removed when the task finishes —
		// success, failure, review, or merge — including on panic or early return.
		defer os.RemoveAll(localPath)
		tlog.InfoCtx(ctx, "cloned %s branch %s → %s", task.RepoURL, branch, localPath)
	}

	a.executor.Run(ctx, task)
}
