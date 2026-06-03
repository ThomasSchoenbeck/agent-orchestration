package workflow

import (
	"context"
	"sync"
	"testing"
	"time"

	"agent-orchestrator/db"
)

// --- fakes ---

type fakeProc struct {
	pid  int
	done chan struct{}
}

func (p *fakeProc) PID() int               { return p.pid }
func (p *fakeProc) Done() <-chan struct{}  { return p.done }
func (p *fakeProc) Kill() error            { p.exit(); return nil }
func (p *fakeProc) exit() {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
}

type fakeLauncher struct {
	mu       sync.Mutex
	procs    []*fakeProc
	autoExit bool
	nextPID  int
}

func (l *fakeLauncher) Launch(_ []string) (ManagedProcess, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextPID++
	p := &fakeProc{pid: l.nextPID, done: make(chan struct{})}
	if l.autoExit {
		close(p.done)
	}
	l.procs = append(l.procs, p)
	return p, nil
}

func (l *fakeLauncher) count() int { l.mu.Lock(); defer l.mu.Unlock(); return len(l.procs) }
func (l *fakeLauncher) at(i int) *fakeProc {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.procs[i]
}

// --- helpers (methods on AgentSupervisor are fine in a _test.go in-package) ---

func (s *AgentSupervisor) instanceCount(tid string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.instances[tid])
}
func (s *AgentSupervisor) slotFailed(tid string, slot int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst := s.instances[tid][slot]
	return inst != nil && inst.failed
}

func newTestSupervisor(d *db.Database, fl *fakeLauncher, maxManaged int) *AgentSupervisor {
	s := NewAgentSupervisor(d, "http://localhost:0", maxManaged)
	s.launcher = fl
	s.relaunchBackoff = 0
	s.graceTimeout = 0
	s.maxRestarts = 2
	return s
}

func makeTemplate(t *testing.T, d *db.Database, name string, replicas int, enabled, autostart bool) *db.AgentTemplate {
	t.Helper()
	tpl := &db.AgentTemplate{Name: name, Roles: []string{"worker"}, Replicas: replicas, Enabled: enabled, Autostart: autostart}
	if err := d.CreateAgentTemplate(context.Background(), tpl); err != nil {
		t.Fatalf("CreateAgentTemplate: %v", err)
	}
	return tpl
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		if cond() {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for: %s", msg)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// --- tests ---

func TestSupervisor_StartsNInstances(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	fl := &fakeLauncher{}
	s := newTestSupervisor(d, fl, 0)
	tpl := makeTemplate(t, d, "bw", 3, true, false)

	if err := s.StartTemplate(ctx, tpl.ID); err != nil {
		t.Fatalf("StartTemplate: %v", err)
	}
	if fl.count() != 3 {
		t.Fatalf("launched %d processes, want 3", fl.count())
	}
	rows, _ := d.ListAgentsByTemplate(ctx, tpl.ID)
	if len(rows) != 3 {
		t.Errorf("expected 3 instance rows, got %d", len(rows))
	}
}

func TestSupervisor_ScaleUp(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	fl := &fakeLauncher{}
	s := newTestSupervisor(d, fl, 0)
	tpl := makeTemplate(t, d, "bw", 2, true, false)

	_ = s.StartTemplate(ctx, tpl.ID)
	if fl.count() != 2 {
		t.Fatalf("initial launches = %d, want 2", fl.count())
	}
	if err := s.ScaleTemplate(ctx, tpl.ID, 4); err != nil {
		t.Fatalf("ScaleTemplate: %v", err)
	}
	if fl.count() != 4 {
		t.Errorf("after scale up launches = %d, want 4", fl.count())
	}
}

func TestSupervisor_ScaleDown(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	fl := &fakeLauncher{}
	s := newTestSupervisor(d, fl, 0)
	tpl := makeTemplate(t, d, "bw", 4, true, false)

	_ = s.StartTemplate(ctx, tpl.ID)
	if err := s.ScaleTemplate(ctx, tpl.ID, 2); err != nil {
		t.Fatalf("ScaleTemplate: %v", err)
	}
	waitFor(t, func() bool { return s.instanceCount(tpl.ID) == 2 }, "scale down to 2 running")
	if fl.count() != 4 {
		t.Errorf("scale down must not launch new procs; count = %d, want 4", fl.count())
	}
}

func TestSupervisor_StopTemplate(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	fl := &fakeLauncher{}
	s := newTestSupervisor(d, fl, 0)
	tpl := makeTemplate(t, d, "bw", 2, true, false)

	_ = s.StartTemplate(ctx, tpl.ID)
	s.StopTemplate(ctx, tpl.ID)
	waitFor(t, func() bool { return s.instanceCount(tpl.ID) == 0 }, "all instances stopped")

	rows, _ := d.ListAgentsByTemplate(ctx, tpl.ID)
	for _, a := range rows {
		if a.Status != "offline" {
			t.Errorf("instance %q status = %q, want offline", a.Name, a.Status)
		}
		if a.DesiredState != "stop" {
			t.Errorf("instance %q desired_state = %q, want stop", a.Name, a.DesiredState)
		}
	}
}

func TestSupervisor_RelaunchOnCrash(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	fl := &fakeLauncher{}
	s := newTestSupervisor(d, fl, 0)
	tpl := makeTemplate(t, d, "bw", 1, true, false)

	_ = s.StartTemplate(ctx, tpl.ID)
	waitFor(t, func() bool { return fl.count() == 1 }, "initial launch")

	// Simulate a crash of the running instance.
	fl.at(0).exit()

	waitFor(t, func() bool { return fl.count() == 2 }, "relaunch after crash")
}

func TestSupervisor_CrashLoopMarksFailed(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	fl := &fakeLauncher{autoExit: true} // every spawned process exits immediately
	s := newTestSupervisor(d, fl, 0)    // maxRestarts = 2
	tpl := makeTemplate(t, d, "bw", 1, true, false)

	_ = s.StartTemplate(ctx, tpl.ID)

	// Initial + maxRestarts relaunches = 3 launches, then the slot is failed.
	waitFor(t, func() bool { return s.slotFailed(tpl.ID, 1) }, "slot marked failed")
	if fl.count() != 3 {
		t.Errorf("crash-loop launches = %d, want 3 (1 + maxRestarts)", fl.count())
	}

	// Ensure it does not keep launching (no hot loop).
	time.Sleep(150 * time.Millisecond)
	if fl.count() != 3 {
		t.Errorf("supervisor hot-looped: launches = %d", fl.count())
	}
}

func TestSupervisor_AutostartOnBoot(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	fl := &fakeLauncher{}
	s := newTestSupervisor(d, fl, 0)

	makeTemplate(t, d, "auto", 2, true, true)   // enabled + autostart
	makeTemplate(t, d, "manual", 3, true, false) // enabled, no autostart

	s.Boot(ctx)

	if fl.count() != 2 {
		t.Errorf("autostart launched %d, want 2 (manual template must not autostart)", fl.count())
	}
}

func TestSupervisor_RespectsMaxManagedCap(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	fl := &fakeLauncher{}
	s := newTestSupervisor(d, fl, 2) // cap of 2
	tpl := makeTemplate(t, d, "bw", 2, true, false)

	_ = s.StartTemplate(ctx, tpl.ID)
	if fl.count() != 2 {
		t.Fatalf("initial launches = %d, want 2", fl.count())
	}
	// Scaling beyond the cap is rejected and launches nothing more.
	if err := s.ScaleTemplate(ctx, tpl.ID, 4); err == nil {
		t.Error("expected error scaling beyond max_managed_agents")
	}
	if fl.count() != 2 {
		t.Errorf("cap breach launched extra procs; count = %d, want 2", fl.count())
	}
}

func TestSupervisor_IgnoresRemoteAgents(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	fl := &fakeLauncher{}
	s := newTestSupervisor(d, fl, 0)

	// A remote agent, registered externally (no template).
	remote := &db.Agent{Name: "remote-1", Roles: []string{"worker"}, Status: "online"}
	if err := d.CreateAgent(ctx, remote); err != nil {
		t.Fatalf("CreateAgent remote: %v", err)
	}

	tpl := makeTemplate(t, d, "bw", 1, true, false)
	_ = s.StartTemplate(ctx, tpl.ID)
	s.StopTemplate(ctx, tpl.ID)
	waitFor(t, func() bool { return s.instanceCount(tpl.ID) == 0 }, "managed instance stopped")

	got, _ := d.GetAgentByName(ctx, "remote-1")
	if got.Status != "online" {
		t.Errorf("remote agent status = %q, want online (must not be touched)", got.Status)
	}
}
