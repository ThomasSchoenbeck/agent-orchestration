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
	"path/filepath"
	"strings"
	"syscall"

	"agent-orchestrator/agent"
	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
	"agent-orchestrator/server"
	"agent-orchestrator/tools"
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

	// Open the separate log database (data/logs.db).
	logDBPath := filepath.Join(filepath.Dir(cfg.Database.Path), "logs.db")
	logDB, err := db.OpenLogDB(logDBPath)
	if err != nil {
		return fmt.Errorf("open log database: %w", err)
	}
	defer func() { _ = logDB.Close() }()
	database.LogDB = logDB

	// #55b — seed providers from config on first run
	startCtx := context.Background()
	if count, _ := database.CountProviders(startCtx); count == 0 && len(cfg.Providers) > 0 {
		var toSeed []*db.Provider
		for _, pcfg := range cfg.Providers {
			toSeed = append(toSeed, &db.Provider{
				Name:      pcfg.Name,
				Type:      pcfg.Type,
				BaseURL:   pcfg.BaseURL,
				APIKey:    pcfg.APIKey,
				ModelName: pcfg.Model,
				Enabled:   true,
			})
		}
		if n, serr := database.SeedProviders(startCtx, toSeed); serr != nil {
			log.Printf("warning: seed providers: %v", serr)
		} else if n > 0 {
			log.Printf("seeded %d providers from config into database", n)
		}
	}

	// #55c — init LLM registry from DB; fall back to config if DB is empty
	llmReg := llm.NewRegistry()
	if dbProviders, perr := database.ListProviders(startCtx); perr == nil && len(dbProviders) > 0 {
		for _, p := range dbProviders {
			if !p.Enabled {
				continue
			}
			prov, perr2 := llm.NewFromSpec(p.Name, p.Type, p.BaseURL, p.APIKey, p.ModelName, p.Config)
			if perr2 != nil {
				log.Printf("warning: init provider %q: %v", p.Name, perr2)
				continue
			}
			llmReg.Set(p.Name, prov)
		}
		log.Printf("loaded %d LLM provider(s) from database", len(dbProviders))
	} else {
		log.Printf("no DB providers found — loading from config")
		if err := llmReg.InitFromConfig(cfg); err != nil {
			return fmt.Errorf("init LLM providers: %w", err)
		}
	}
	defer llmReg.CloseAll()

	// Seed role definitions from config on first run.
	if count, _ := database.CountRoleDefinitions(startCtx); count == 0 && len(cfg.Roles) > 0 {
		dbProvs, _ := database.ListProviders(startCtx)
		provByName := make(map[string]*db.Provider, len(dbProvs))
		for _, p := range dbProvs {
			provByName[p.Name] = p
		}
		// Inverse routing: role → task types
		roleTaskTypes := make(map[string][]string)
		for tt, role := range cfg.Routing {
			roleTaskTypes[role] = append(roleTaskTypes[role], tt)
		}
		var rolesToSeed []*db.RoleDefinition
		for roleName, modelName := range cfg.Roles {
			label := strings.ToUpper(roleName[:1]) + roleName[1:]
			rd := &db.RoleDefinition{
				Name:        roleName,
				Label:       label,
				TaskTypes:   roleTaskTypes[roleName],
				Enabled:     true,
				Temperature: 0.7,
				MaxTokens:   4096,
			}
			if model, merr := cfg.ModelByName(modelName); merr == nil {
				if prov, ok := provByName[model.Provider]; ok {
					rd.ProviderID = prov.ID
					rd.ModelOverride = model.Model
				}
			}
			rolesToSeed = append(rolesToSeed, rd)
		}
		if n, serr := database.SeedRoleDefinitions(startCtx, rolesToSeed); serr != nil {
			log.Printf("warning: seed role definitions: %v", serr)
		} else if n > 0 {
			log.Printf("seeded %d role definition(s) from config", n)
		}
	}

	// Seed default log retention settings (only if keys don't already exist).
	if err := database.SeedDefaultRetentionSettings(startCtx); err != nil {
		log.Printf("warning: seed retention settings: %v", err)
	}
	if err := database.SeedRetentionFromConfig(
		startCtx,
		cfg.LogRetention.AgentDefaultDays,
		cfg.LogRetention.TaskDefaultDays,
		cfg.LogRetention.SystemDefaultDays,
		cfg.LogRetention.Overrides,
	); err != nil {
		log.Printf("warning: seed retention config: %v", err)
	}

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

	// If a full config is provided, wire up the LLM router and tool registry
	// so the agent can actually execute tasks.
	if *configPath != "" {
		database, err := db.Open(cfg.Database.Path)
		if err != nil {
			log.Printf("agent: could not open database (%v) — running without tool execution", err)
		} else {
			defer func() { _ = database.Close() }()
			llmReg := llm.NewRegistry()
			if err := llmReg.InitFromConfig(cfg); err != nil {
				log.Printf("agent: could not init LLM providers (%v) — running without LLM execution", err)
			} else {
				defer llmReg.CloseAll()
				rtr := router.New(cfg, llmReg)
				toolReg := tools.NewRegistry()
				_ = tools.RegisterCodeTools(toolReg)
				_ = tools.RegisterTaskTools(toolReg, database)
				_ = tools.RegisterPlanTools(toolReg, database)
				_ = tools.RegisterContextTools(toolReg, database)
				a.WithExecutor(rtr, toolReg)
				log.Printf("agent: executor wired (LLM providers: %d)", len(llmReg.List()))
			}
		}
	}

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
