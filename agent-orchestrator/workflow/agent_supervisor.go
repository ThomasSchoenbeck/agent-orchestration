package workflow

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"agent-orchestrator/db"
)

// ManagedProcess is a handle to a spawned agent process. The production
// implementation wraps *exec.Cmd; tests inject a fake.
type ManagedProcess interface {
	PID() int
	Kill() error
	Done() <-chan struct{} // closed when the process exits
}

// Launcher spawns an agent process with the given CLI args.
type Launcher interface {
	Launch(args []string) (ManagedProcess, error)
}

// execLauncher runs the orchestrator's own binary in `agent` mode (Bug 3: the
// child uses the embedded git server over localhost like any other agent).
type execLauncher struct{}

func (execLauncher) Launch(args []string) (ManagedProcess, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("os.Executable: %w", err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &execProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

type execProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *execProcess) PID() int {
	if p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}
func (p *execProcess) Kill() error {
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}
func (p *execProcess) Done() <-chan struct{} { return p.done }

// instance is one tracked managed agent (one template slot).
type instance struct {
	slot     int
	name     string
	proc     ManagedProcess
	stopping bool
	failed   bool
	failures int
}

// AgentSupervisor launches and manages co-located agent processes spawned from
// AgentTemplates. It only manages processes it spawned; remote agents are never
// touched.
type AgentSupervisor struct {
	db        *db.Database
	launcher  Launcher
	serverURL string
	maxManaged int

	maxRestarts     int
	relaunchBackoff time.Duration
	graceTimeout    time.Duration

	mu        sync.Mutex
	instances map[string]map[int]*instance // templateID -> slot -> instance
}

// NewAgentSupervisor creates a supervisor using the real exec launcher.
// maxManaged caps the total number of managed instances (0 = unlimited).
func NewAgentSupervisor(database *db.Database, serverURL string, maxManaged int) *AgentSupervisor {
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	return &AgentSupervisor{
		db:              database,
		launcher:        execLauncher{},
		serverURL:       serverURL,
		maxManaged:      maxManaged,
		maxRestarts:     5,
		relaunchBackoff: time.Second,
		graceTimeout:    5 * time.Second,
		instances:       map[string]map[int]*instance{},
	}
}

// SetLauncher overrides the process launcher (tests inject a fake).
func (s *AgentSupervisor) SetLauncher(l Launcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.launcher = l
}

// SetTimings overrides the relaunch backoff and stop grace period (tests use 0
// for determinism).
func (s *AgentSupervisor) SetTimings(grace, backoff time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.graceTimeout = grace
	s.relaunchBackoff = backoff
}

// StartTemplate launches up to the template's Replicas instances.
func (s *AgentSupervisor) StartTemplate(ctx context.Context, templateID string) error {
	tpl, err := s.db.GetAgentTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcileLocked(ctx, tpl)
}

// ScaleTemplate sets the desired replica count and reconciles. Scaling beyond
// the max_managed_agents cap is rejected.
func (s *AgentSupervisor) ScaleTemplate(ctx context.Context, templateID string, replicas int) error {
	if replicas < 0 {
		replicas = 0
	}
	tpl, err := s.db.GetAgentTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.maxManaged > 0 {
		newTotal := s.totalManagedLocked() - s.runningCountLocked(templateID) + replicas
		if newTotal > s.maxManaged {
			return fmt.Errorf("scaling to %d would exceed max_managed_agents (%d)", replicas, s.maxManaged)
		}
	}
	if err := s.db.SetTemplateReplicas(ctx, templateID, replicas); err != nil {
		return err
	}
	tpl.Replicas = replicas
	return s.reconcileLocked(ctx, tpl)
}

// StopTemplate gracefully stops all instances of a template.
func (s *AgentSupervisor) StopTemplate(ctx context.Context, templateID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inst := range s.instances[templateID] {
		s.stopSlotLocked(ctx, templateID, inst)
	}
}

// DeleteTemplate stops all instances then removes the template.
func (s *AgentSupervisor) DeleteTemplate(ctx context.Context, templateID string) error {
	s.StopTemplate(ctx, templateID)
	return s.db.DeleteAgentTemplate(ctx, templateID)
}

// Boot launches the desired replicas for every enabled+autostart template.
func (s *AgentSupervisor) Boot(ctx context.Context) {
	tpls, err := s.db.ListAgentTemplates(ctx)
	if err != nil {
		log.Printf("agent_supervisor: Boot ListAgentTemplates: %v", err)
		return
	}
	for _, tpl := range tpls {
		if tpl.Enabled && tpl.Autostart {
			if err := s.StartTemplate(ctx, tpl.ID); err != nil {
				log.Printf("agent_supervisor: autostart template %q: %v", tpl.Name, err)
			}
		}
	}
}

// Shutdown stops every managed instance.
func (s *AgentSupervisor) Shutdown(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tid, slots := range s.instances {
		for _, inst := range slots {
			s.stopSlotLocked(ctx, tid, inst)
		}
	}
}

// --- internals (caller must hold s.mu) ---

func (s *AgentSupervisor) totalManagedLocked() int {
	n := 0
	for _, m := range s.instances {
		for _, inst := range m {
			if !inst.failed {
				n++
			}
		}
	}
	return n
}

func (s *AgentSupervisor) runningCountLocked(templateID string) int {
	n := 0
	for _, inst := range s.instances[templateID] {
		if !inst.failed {
			n++
		}
	}
	return n
}

func (s *AgentSupervisor) reconcileLocked(ctx context.Context, tpl *db.AgentTemplate) error {
	cur := s.instances[tpl.ID]
	if cur == nil {
		cur = map[int]*instance{}
		s.instances[tpl.ID] = cur
	}
	desired := tpl.Replicas
	if !tpl.Enabled {
		desired = 0
	}

	// Scale up: ensure slots 1..desired are running (skip failed slots).
	for slot := 1; slot <= desired; slot++ {
		if inst, ok := cur[slot]; ok && (!inst.stopping || inst.failed) {
			continue
		}
		if s.maxManaged > 0 && s.totalManagedLocked() >= s.maxManaged {
			return fmt.Errorf("max_managed_agents cap (%d) reached", s.maxManaged)
		}
		if err := s.launchSlotLocked(ctx, tpl, slot, 0); err != nil {
			return err
		}
	}
	// Scale down: stop slots above the desired count.
	for slot, inst := range cur {
		if slot > desired && !inst.stopping {
			s.stopSlotLocked(ctx, tpl.ID, inst)
		}
	}
	return nil
}

func (s *AgentSupervisor) launchSlotLocked(ctx context.Context, tpl *db.AgentTemplate, slot, failuresSeed int) error {
	name := fmt.Sprintf("%s-%d", tpl.Name, slot)

	// Ensure a backing agent row exists (mirrored for cross-restart visibility).
	if existing, err := s.db.GetAgentByName(ctx, name); err == nil && existing != nil {
		_ = s.db.SetAgentTemplateID(ctx, existing.ID, tpl.ID)
		_ = s.db.SetAgentDesiredState(ctx, existing.ID, "run")
	} else {
		_ = s.db.CreateAgent(ctx, &db.Agent{
			Name:         name,
			Roles:        tpl.Roles,
			Skills:       tpl.Skills,
			StartRoles:   tpl.Roles,
			StartSkills:  tpl.Skills,
			DesiredState: "run",
			TemplateID:   tpl.ID,
			Mode:         "colocated",
			Status:       "starting",
		})
	}

	args := []string{
		"agent",
		"--name", name,
		"--roles", strings.Join(tpl.Roles, ","),
		"--skills", strings.Join(tpl.Skills, ","),
		"--server", s.serverURL,
		"--mode", "colocated",
	}
	proc, err := s.launcher.Launch(args)
	if err != nil {
		return fmt.Errorf("launch %q: %w", name, err)
	}
	s.instances[tpl.ID][slot] = &instance{slot: slot, name: name, proc: proc, failures: failuresSeed}
	go s.watch(tpl.ID, slot, proc)
	return nil
}

func (s *AgentSupervisor) stopSlotLocked(ctx context.Context, templateID string, inst *instance) {
	if inst.stopping {
		return
	}
	inst.stopping = true
	// Graceful: ask the agent to stop (Feature 7). It finishes its task, goes
	// offline, and exits; if it doesn't within the grace period, hard-kill.
	if a, err := s.db.GetAgentByName(ctx, inst.name); err == nil && a != nil {
		_ = s.db.SetAgentDesiredState(ctx, a.ID, "stop")
	}
	proc := inst.proc
	grace := s.graceTimeout
	go func() {
		select {
		case <-proc.Done():
		case <-time.After(grace):
			_ = proc.Kill()
		}
	}()
}

// watch blocks until the process exits, then reconciles the slot.
func (s *AgentSupervisor) watch(templateID string, slot int, proc ManagedProcess) {
	<-proc.Done()
	s.onExit(templateID, slot, proc)
}

func (s *AgentSupervisor) onExit(templateID string, slot int, proc ManagedProcess) {
	s.mu.Lock()
	cur := s.instances[templateID]
	inst := cur[slot]
	// Ignore exits from a process that has already been replaced.
	if inst == nil || inst.proc != proc {
		s.mu.Unlock()
		return
	}

	if inst.stopping {
		// Mark the row offline before removing the instance so that an observer
		// seeing the instance count reach zero also sees the row as offline.
		s.markRowOffline(context.Background(), inst.name)
		delete(cur, slot)
		s.mu.Unlock()
		return
	}

	// Unexpected exit (crash).
	inst.failures++
	if inst.failures > s.maxRestarts {
		inst.failed = true
		name, failures := inst.name, inst.failures
		s.mu.Unlock()
		s.markRowStatus(context.Background(), name, "failed")
		log.Printf("agent_supervisor: slot %s-%d crash-looped (%d failures) — marked failed", templateID, slot, failures)
		return
	}
	failures := inst.failures
	backoff := s.relaunchBackoff
	s.mu.Unlock()

	go func() {
		if backoff > 0 {
			time.Sleep(backoff)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		cur := s.instances[templateID]
		if cur == nil || cur[slot] == nil || cur[slot].proc != proc || cur[slot].stopping {
			return // slot was replaced or stopped meanwhile
		}
		tpl, err := s.db.GetAgentTemplate(context.Background(), templateID)
		if err != nil || !tpl.Enabled || slot > tpl.Replicas {
			delete(cur, slot)
			return
		}
		if err := s.launchSlotLocked(context.Background(), tpl, slot, failures); err != nil {
			log.Printf("agent_supervisor: relaunch %s-%d: %v", tpl.Name, slot, err)
		}
	}()
}

func (s *AgentSupervisor) markRowOffline(ctx context.Context, name string) {
	if a, err := s.db.GetAgentByName(ctx, name); err == nil && a != nil {
		a.Status = "offline"
		a.DesiredState = "stop"
		_ = s.db.UpdateAgent(ctx, a)
	}
}

func (s *AgentSupervisor) markRowStatus(ctx context.Context, name, status string) {
	if a, err := s.db.GetAgentByName(ctx, name); err == nil && a != nil {
		a.Status = status
		_ = s.db.UpdateAgent(ctx, a)
	}
}
