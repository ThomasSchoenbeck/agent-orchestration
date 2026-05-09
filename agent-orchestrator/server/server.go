// Package server implements the HTTP API and serves the embedded Svelte UI.
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// Server holds all dependencies and the HTTP mux.
type Server struct {
	cfg     *config.Config
	db      *db.Database
	llmReg  *llm.Registry
	httpSrv *http.Server
	mux     *http.ServeMux
}

// ServeHTTP implements http.Handler — delegates to the internal mux.
// This is used in tests and allows embedding in other HTTP servers.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// New creates a fully wired Server ready to be started.
func New(cfg *config.Config, database *db.Database, llmReg *llm.Registry) *Server {
	s := &Server{
		cfg:    cfg,
		db:     database,
		llmReg: llmReg,
		mux:    http.NewServeMux(),
	}
	s.registerHandlers()
	return s
}

// Start begins listening on the configured address. It blocks until the
// server stops or the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
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

	log.Printf("server: listening on http://%s", addr)
	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
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
			n, err := s.db.MarkOfflineAgents(ctx, s.cfg.Agents.HeartbeatIntervalSec*2)
			if err != nil {
				log.Printf("maintenance: MarkOfflineAgents: %v", err)
			} else if n > 0 {
				log.Printf("maintenance: marked %d agents offline", n)
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
