// Package server implements the HTTP API and serves the embedded Svelte UI.
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/logging"
	"agent-orchestrator/router"
	"agent-orchestrator/storage"
	"agent-orchestrator/workflow"
)

const (
	defaultPortPoolStart = 18000
	defaultPortPoolSize  = 100
)

// AgentPollStatus tracks the last poll activity for an agent.
type AgentPollStatus struct {
	LastPolledAt    time.Time  `json:"last_polled_at"`
	Roles           []string   `json:"roles"`
	LastTaskFoundID string     `json:"last_task_found_id"`
	LastTaskFoundAt *time.Time `json:"last_task_found_at"`
}

// Server holds all dependencies and the HTTP mux.
type Server struct {
	cfg        *config.Config
	db         *db.Database
	llmReg     *llm.Registry
	router     *router.Router
	log        *logging.Logger
	httpSrv    *http.Server
	mux        *http.ServeMux
	pollMu     sync.RWMutex
	pollStatus map[string]*AgentPollStatus
	storage    *storage.Paths
	portPool   *workflow.PortPool
	agentSup   *workflow.AgentSupervisor // Feature 8: server-managed co-located agents

	// Cached debug-mode flag — re-read from DB at most once per 30 s.
	debugMu        sync.RWMutex
	debugModeValue bool
	debugModeFetAt time.Time
}

// ServeHTTP implements http.Handler — delegates to the internal mux.
// This is used in tests and allows embedding in other HTTP servers.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// New creates a fully wired Server ready to be started.
func New(cfg *config.Config, database *db.Database, llmReg *llm.Registry) *Server {
	rtr := router.New(cfg, llmReg)
	if err := rtr.LoadFromDB(database); err != nil {
		log.Printf("server: router.LoadFromDB: %v", err)
	}
	stor := storage.New(cfg.Storage.Root, cfg.Storage.ReposDir, cfg.Storage.WorktreesDir)

	poolStart := cfg.Agents.PortPoolStart
	if poolStart == 0 {
		poolStart = defaultPortPoolStart
	}
	poolSize := cfg.Agents.PortPoolSize
	if poolSize == 0 {
		poolSize = defaultPortPoolSize
	}

	s := &Server{
		cfg:        cfg,
		db:         database,
		llmReg:     llmReg,
		router:     rtr,
		log:        logging.New(database, "", "", ""),
		mux:        http.NewServeMux(),
		pollStatus: make(map[string]*AgentPollStatus),
		storage:    stor,
		portPool:   workflow.NewPortPool(poolStart, poolSize),
	}
	serverURL := fmt.Sprintf("http://localhost:%d", cfg.Server.Port)
	s.agentSup = workflow.NewAgentSupervisor(database, serverURL, cfg.Agents.MaxManagedAgents)
	s.ensureStorageDirs()
	s.registerHandlers()
	s.mux.Handle("/git/", s.newGitHTTPHandler())
	s.registerStaticHandler() // must come after API routes so "/" is the catch-all
	return s
}

// SetAgentLauncher overrides the managed-agent launcher (used in tests to avoid
// spawning real OS processes). It also zeroes the supervisor's timings for
// deterministic, fast tests.
func (s *Server) SetAgentLauncher(l workflow.Launcher) {
	s.agentSup.SetLauncher(l)
	s.agentSup.SetTimings(0, 0)
}

// ensureStorageDirs creates the repos and worktrees subdirectories under the
// storage root on startup (idempotent).
func (s *Server) ensureStorageDirs() {
	reposDir, worktreesDir := s.storage.Dirs()
	for _, sub := range []string{reposDir, worktreesDir} {
		dir := filepath.Join(s.cfg.Storage.Root, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("server: ensureStorageDirs %q: %v", dir, err)
		}
	}
}

// releaseStaleExecutionTasks resets any tasks left in an execution state
// (DEVELOPING, REVIEWING, MERGING) from a previous server run back to their
// queueable state. Safe to call on every startup because no agents are
// registered yet at that point.
func (s *Server) releaseStaleExecutionTasks(ctx context.Context) {
	n, err := s.db.RequeueTimedOutTasks(ctx, 0)
	if err != nil {
		log.Printf("server: releaseStaleExecutionTasks: %v", err)
		return
	}
	if n > 0 {
		log.Printf("server: released %d stale execution-state task(s) back to queue on startup", n)
	}
}

// Start begins listening on the configured address. It blocks until the
// server stops or the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	// Release any tasks stuck in an execution state from the previous run.
	s.releaseStaleExecutionTasks(ctx)

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Background goroutine to shut down the HTTP server when ctx is done.
	go func() {
		<-ctx.Done()
		log.Println("server: shutting down…")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutdownCtx)
	}()

	// Background goroutine to perform periodic maintenance.
	go s.runMaintenance(ctx)

	// Background log retention cleanup.
	go func() {
		intervalMin := 60
		if s.cfg.LogRetention.CleanupIntervalMins > 0 {
			intervalMin = s.cfg.LogRetention.CleanupIntervalMins
		}
		job := workflow.NewRetentionJob(s.db, intervalMin)
		job.Run(ctx)
	}()

	// Feature 2: merging is gated by an explicit approval decision on a pull
	// request (deployer or human via the approve endpoint), not an autonomous
	// background merger. The MergeSupervisor is intentionally not started here;
	// it is retained only for the merge integration regression tests.

	// Feature 4: bounded auto-queue. Replenishes armed/active projects' backlogs
	// when they drain, and self-stops on completion / plan-round ceiling.
	go func() {
		workflow.NewQueueSupervisor(s.db, 0).Run(ctx)
	}()

	// Feature 8: launch autostart managed-agent templates; stop them on shutdown.
	s.agentSup.Boot(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.agentSup.Shutdown(shutdownCtx)
	}()

	log.Printf("server: listening on http://%s", addr)
	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// recordPoll updates the poll status for an agent (called from tasks/next handler).
func (s *Server) recordPoll(agentID string, roles []string, taskFoundID string) {
	now := time.Now().UTC()
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	ps, ok := s.pollStatus[agentID]
	if !ok {
		ps = &AgentPollStatus{}
		s.pollStatus[agentID] = ps
	}
	ps.LastPolledAt = now
	ps.Roles = roles
	if taskFoundID != "" {
		ps.LastTaskFoundID = taskFoundID
		ps.LastTaskFoundAt = &now
	}
}

// getPollStatus returns a copy of the current poll status for an agent.
func (s *Server) getPollStatus(agentID string) *AgentPollStatus {
	s.pollMu.RLock()
	defer s.pollMu.RUnlock()
	ps, ok := s.pollStatus[agentID]
	if !ok {
		return &AgentPollStatus{}
	}
	// Return a copy to avoid races.
	copy := *ps
	return &copy
}

// isDebugMode returns true when platform.debug_mode is set to "true" in the
// platform_settings table. The value is cached for 30 s to avoid per-request
// DB reads on high-frequency paths like heartbeat and poll.
func (s *Server) isDebugMode(ctx context.Context) bool {
	s.debugMu.RLock()
	if time.Since(s.debugModeFetAt) < 30*time.Second {
		v := s.debugModeValue
		s.debugMu.RUnlock()
		return v
	}
	s.debugMu.RUnlock()

	s.debugMu.Lock()
	defer s.debugMu.Unlock()
	// Re-check after acquiring the write lock.
	if time.Since(s.debugModeFetAt) < 30*time.Second {
		return s.debugModeValue
	}
	setting, err := s.db.GetSetting(ctx, "platform.debug_mode")
	s.debugModeValue = err == nil && setting.Value == "true"
	s.debugModeFetAt = time.Now()
	return s.debugModeValue
}

// runMaintenance performs periodic tasks: marking stale agents offline and
// re-queuing timed-out tasks.
func (s *Server) runMaintenance(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := s.db.MarkOfflineAgents(ctx, s.cfg.Agents.HeartbeatIntervalSec*3)
			if err != nil {
				log.Printf("maintenance: MarkOfflineAgents: %v", err)
			} else if n > 0 {
				log.Printf("maintenance: marked %d agents offline", n)
			}

			// Remove agent records that have been offline for more than 10 minutes
			// so stale entries from previous runs don't show up in the UI.
			if n, err := s.db.DeleteStaleOfflineAgents(ctx, 600); err != nil {
				log.Printf("maintenance: DeleteStaleOfflineAgents: %v", err)
			} else if n > 0 {
				log.Printf("maintenance: deleted %d stale offline agent(s)", n)
			}

			n, err = s.db.RequeueTimedOutTasks(ctx, s.cfg.Agents.TaskTimeoutSec)
			if err != nil {
				log.Printf("maintenance: RequeueTimedOutTasks: %v", err)
			} else if n > 0 {
				log.Printf("maintenance: requeued %d timed-out tasks", n)
			}
		}
	}
}
