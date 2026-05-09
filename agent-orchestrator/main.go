// agent-orchestrator is an autonomous AI development platform packaged as a
// single Go binary with two operating modes: server and agent.
//
// Build:
//
//	go build -o agent-orchestrator .
//
// Run as server:
//
//	./agent-orchestrator server --config config.yaml
//
// Run as agent:
//
//	./agent-orchestrator agent --name worker-1 --roles worker --server http://localhost:8080
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"agent-orchestrator/agent"
	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/server"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "server":
		err = runServer(args)
	case "agent":
		err = runAgent(args)
	case "version":
		fmt.Println("agent-orchestrator v0.1.0-phase1")
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("error: %v", err)
	}
}

// -------------------------------------------------------------------------
// server subcommand
// -------------------------------------------------------------------------

func runServer(args []string) error {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to config YAML")
	portOverride := fs.Int("port", 0, "override server port from config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if *portOverride != 0 {
		cfg.Server.Port = *portOverride
	}

	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = database.Close() }()

	llmReg := llm.NewRegistry()
	if err := llmReg.InitFromConfig(cfg); err != nil {
		return fmt.Errorf("init LLM providers: %w", err)
	}
	defer llmReg.CloseAll()

	srv := server.New(cfg, database, llmReg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Start(ctx)
}

// -------------------------------------------------------------------------
// agent subcommand
// -------------------------------------------------------------------------

func runAgent(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	configPath := fs.String("config", "", "optional path to config YAML (defaults applied if omitted)")
	name := fs.String("name", "", "agent name (required)")
	rolesStr := fs.String("roles", "", "comma-separated roles, e.g. worker,reviewer (required)")
	serverURL := fs.String("server", "http://localhost:8080", "orchestrator server URL")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *rolesStr == "" {
		return fmt.Errorf("--roles is required")
	}

	roles := parseRoles(*rolesStr)

	// Config is optional for agents; fall back to defaults.
	var cfg *config.Config
	if *configPath != "" {
		var err error
		cfg, err = config.Load(*configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
	} else {
		cfg = defaultAgentConfig()
	}

	a := agent.NewAgent(*name, roles, *serverURL, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := a.Start(ctx); err != nil {
		return err
	}
	log.Printf("agent %q running (roles: %v) → %s  [Ctrl-C to stop]", *name, roles, *serverURL)

	<-ctx.Done()
	log.Println("agent: shutting down…")
	a.Stop()
	return nil
}

// -------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------

func parseRoles(s string) []string {
	var roles []string
	for _, r := range strings.Split(s, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			roles = append(roles, r)
		}
	}
	return roles
}

func defaultAgentConfig() *config.Config {
	return &config.Config{
		Server:   config.ServerConfig{Port: config.DefaultServerPort, Host: config.DefaultServerHost},
		Database: config.DatabaseConfig{Type: config.DefaultDBType, Path: config.DefaultDBPath},
		Agents: config.AgentConfig{
			HeartbeatIntervalSec: config.DefaultHeartbeatIntervalSec,
			TaskPollIntervalSec:  config.DefaultTaskPollIntervalSec,
			TaskTimeoutSec:       config.DefaultTaskTimeoutSec,
		},
	}
}

func printUsage() {
	fmt.Print(`agent-orchestrator - Autonomous AI development platform

Usage:
  agent-orchestrator server [flags]
  agent-orchestrator agent  [flags]
  agent-orchestrator version

Server flags:
  --config string   Path to config YAML (default "config.yaml")
  --port   int      Override server port

Agent flags:
  --name   string   Agent name (required)
  --roles  string   Comma-separated roles: worker,reviewer,orchestrator (required)
  --server string   Server URL (default "http://localhost:8080")
  --config string   Optional config YAML path

Examples:
  agent-orchestrator server --config config.yaml --port 8080
  agent-orchestrator agent --name worker-1 --roles worker --server http://localhost:8080
  agent-orchestrator agent --name orch-1   --roles orchestrator,reviewer
`)
}
