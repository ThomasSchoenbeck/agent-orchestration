// agent-orchestrator is an autonomous AI development platform packaged as a
// single Go binary with three operating modes: server, agent, and agents.
//
// Build:
//
//	go build -o agent-orchestrator .
//
// Run as server:
//
//	./agent-orchestrator server --config config.yaml
//
// Run as a single agent:
//
//	./agent-orchestrator agent --name worker-1 --roles worker --server http://localhost:8080
//
// Run multiple agents defined in config.yaml:
//
//	./agent-orchestrator agents --config config.yaml
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"
	"strings"
	"sync"
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
	case "agents":
		err = runAgents(args)
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

	// Open the separate log database. Use the configured path when set;
	// otherwise fall back to logs.db next to the main database.
	logDBPath := cfg.LogsDB.Path
	if logDBPath == "" {
		logDBPath = filepath.Join(filepath.Dir(cfg.Database.Path), "logs.db")
	}
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
			if len(p.Roles) > 0 {
				llmReg.SetRoles(p.Name, p.ModelName, p.Roles)
			}
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

	// Seed default platform settings (debug_mode, autorefresh_ms).
	if err := database.SeedDefaultPlatformSettings(startCtx); err != nil {
		log.Printf("warning: seed platform settings: %v", err)
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
	workdir := fs.String("workdir", "", "local workspace root for agent code checkouts (default: .agent-work)")
	mode := fs.String("mode", "colocated", `agent mode: "colocated" (server provisions worktrees) or "remote" (agent clones via git)`)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *rolesStr == "" {
		return fmt.Errorf("--roles is required")
	}

	a, cleanup, cfg, err := buildAgent(*name, parseRoles(*rolesStr), *serverURL, *configPath, *workdir, *mode)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := a.StartWithReconnect(ctx, reconnectCfg(cfg)); err != nil {
		return err
	}
	log.Printf("agent %q running (roles: %v) → %s  [Ctrl-C to stop]", *name, a.Roles(), *serverURL)

	<-ctx.Done()
	log.Println("agent: shutting down…")
	a.Deregister(context.Background())
	a.Stop()
	return nil
}

// buildAgent constructs and optionally wires a fully configured Agent.
// configPath is optional — when empty, a minimal default config is used and
// the LLM executor is not wired up (polling-only mode).
// mode should be "colocated" or "remote"; defaults to "colocated" when empty.
// The returned cleanup func must be called when the agent is done.
func buildAgent(name string, roles []string, serverURL, configPath, workdir, mode string) (*agent.Agent, func(), *config.Config, error) {
	var cfg *config.Config
	if configPath != "" {
		var err error
		cfg, err = config.Load(configPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("load config: %w", err)
		}
	} else {
		cfg = defaultAgentConfig()
	}

	// Workdir: explicit arg wins; fall back to config key; fall back to default.
	wd := workdir
	if wd == "" {
		wd = cfg.Agents.Workdir
	}

	a := agent.NewAgent(name, roles, serverURL, cfg)
	if mode == "" {
		mode = "colocated"
	}
	a.WithMode(mode)
	if wd != "" {
		a.WithWorkdir(wd)
	}

	cleanup := func() {}

	// Wire LLM executor only when a full config is provided.
	if configPath != "" {
		database, err := db.Open(cfg.Database.Path)
		if err != nil {
			log.Printf("agent %q: could not open database (%v) — running without tool execution", name, err)
		} else {
			llmReg := llm.NewRegistry()
			providersOK := true

			// Prefer DB providers so that role preferences and live updates are respected.
			// Fall back to config-file providers if the DB has none yet.
			startCtx := context.Background()
			dbProviders, dbErr := database.ListProviders(startCtx)
			if dbErr == nil && len(dbProviders) > 0 {
				for _, p := range dbProviders {
					if !p.Enabled {
						continue
					}
					prov, perr := llm.NewFromSpec(p.Name, p.Type, p.BaseURL, p.APIKey, p.ModelName, p.Config)
					if perr != nil {
						log.Printf("agent %q: init provider %q: %v", name, p.Name, perr)
						continue
					}
					llmReg.Set(p.Name, prov)
					if len(p.Roles) > 0 {
						llmReg.SetRoles(p.Name, p.ModelName, p.Roles)
					}
				}
			} else {
				if initErr := llmReg.InitFromConfig(cfg); initErr != nil {
					log.Printf("agent %q: could not init LLM providers (%v) — running without LLM execution", name, initErr)
					_ = database.Close()
					providersOK = false
				}
			}

			if providersOK {
				rtr := router.New(cfg, llmReg)
				if rtrErr := rtr.LoadFromDB(database); rtrErr != nil {
					log.Printf("agent %q: could not load role definitions from DB (%v) — using config roles only", name, rtrErr)
				}
				toolReg := tools.NewRegistry()
				_ = tools.RegisterCodeTools(toolReg)
				_ = tools.RegisterTaskTools(toolReg, database)
				_ = tools.RegisterPlanTools(toolReg, database)
				_ = tools.RegisterContextTools(toolReg, database)
				_ = tools.RegisterCommentTools(toolReg, database)
				a.WithExecutor(rtr, toolReg)
				log.Printf("agent %q: executor wired (LLM providers: %d)", name, len(llmReg.List()))
				cleanup = func() {
					llmReg.CloseAll()
					_ = database.Close()
				}
			}
		}
	}

	return a, cleanup, cfg, nil
}

// -------------------------------------------------------------------------
// agents subcommand — start multiple agents from the main config file
// -------------------------------------------------------------------------

func runAgents(args []string) error {
	fs := flag.NewFlagSet("agents", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to config YAML")
	serverOverride := fs.String("server", "", "override agents.server_url from config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(cfg.Agents.Definitions) == 0 {
		return fmt.Errorf("config %q defines no agents under agents.definitions", *configPath)
	}

	serverURL := cfg.Agents.ServerURL
	if *serverOverride != "" {
		serverURL = *serverOverride
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	for _, def := range cfg.Agents.Definitions {
		// Per-agent workdir: use explicit value when set; otherwise derive a
		// unique sub-directory from the top-level workdir + agent name so
		// multiple agents never share the same checkout root.
		workdir := def.Workdir
		if workdir == "" {
			root := cfg.Agents.Workdir
			if root == "" {
				root = ".agent-work"
			}
			workdir = filepath.Join(root, def.Name)
		}
		// Per-agent config override; fall back to the main config file.
		agentConfig := def.Config
		if agentConfig == "" {
			agentConfig = *configPath
		}

		defMode := def.Mode
		if defMode == "" {
			defMode = "colocated"
		}
		a, cleanup, _, err := buildAgent(def.Name, def.Roles, serverURL, agentConfig, workdir, defMode)
		if err != nil {
			return fmt.Errorf("build agent %q: %w", def.Name, err)
		}

		if err := a.StartWithReconnect(ctx, reconnectCfg(cfg)); err != nil {
			cleanup()
			return fmt.Errorf("start agent %q: %w", def.Name, err)
		}
		log.Printf("agent %q started (roles: %v)", def.Name, def.Roles)

		wg.Add(1)
		go func(a *agent.Agent, cleanup func(), name string) {
			defer wg.Done()
			defer cleanup()
			<-ctx.Done()
			log.Printf("agent %q: shutting down…", name)
			a.Deregister(context.Background())
			a.Stop()
		}(a, cleanup, def.Name)
	}

	log.Printf("agents: %d agent(s) running — Ctrl-C to stop", len(cfg.Agents.Definitions))
	wg.Wait()
	log.Println("agents: all stopped")
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
			HeartbeatIntervalSec:  config.DefaultHeartbeatIntervalSec,
			TaskPollIntervalSec:   config.DefaultTaskPollIntervalSec,
			TaskTimeoutSec:        config.DefaultTaskTimeoutSec,
			ConnectInitialDelayMs: config.DefaultConnectInitialDelayMs,
			ConnectMaxDelayMs:     config.DefaultConnectMaxDelayMs,
			ConnectMaxRetries:     config.DefaultConnectMaxRetries,
		},
	}
}

// reconnectCfg builds an agent.ReconnectConfig from the loaded Config.
func reconnectCfg(cfg *config.Config) agent.ReconnectConfig {
	return agent.ReconnectConfig{
		InitialDelay: time.Duration(cfg.Agents.ConnectInitialDelayMs) * time.Millisecond,
		MaxDelay:     time.Duration(cfg.Agents.ConnectMaxDelayMs) * time.Millisecond,
		MaxAttempts:  cfg.Agents.ConnectMaxRetries,
	}
}

func printUsage() {
	fmt.Print(`agent-orchestrator - Autonomous AI development platform

Usage:
  agent-orchestrator server [flags]
  agent-orchestrator agent  [flags]
  agent-orchestrator agents [flags]
  agent-orchestrator version

Server flags:
  --config string   Path to config YAML (default "config.yaml")
  --port   int      Override server port

Agent flags:
  --name    string   Agent name (required)
  --roles   string   Comma-separated roles: worker,reviewer,orchestrator (required)
  --server  string   Server URL (default "http://localhost:8080")
  --workdir string   Local workspace root for code checkouts (default: .agent-work)
  --mode    string   Agent mode: colocated (default) or remote
  --config  string   Optional config YAML path

Agents flags (start multiple agents defined in config.yaml):
  --config string   Path to config YAML (default "config.yaml")
  --server string   Override agents.server_url from config

Examples:
  agent-orchestrator server --config config.yaml --port 8080
  agent-orchestrator agent --name worker-1 --roles worker --server http://localhost:8080
  agent-orchestrator agent --name orch-1   --roles orchestrator,reviewer
  agent-orchestrator agents --config config.yaml
`)
}
