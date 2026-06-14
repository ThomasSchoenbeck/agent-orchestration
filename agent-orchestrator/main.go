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
	"crypto/tls"
	"crypto/x509"
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
	insecure := fs.Bool("insecure", false, "serve plain HTTP instead of HTTPS (dev/local)")
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
	if *insecure {
		cfg.Server.Insecure = true
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
			// Map per-model config (roles + pricing + behavioral flags) and a
			// coarse provider-level role list (the union of all model roles).
			var models []db.ProviderModel
			roleSet := map[string]bool{}
			for _, m := range pcfg.Models {
				models = append(models, db.ProviderModel{
					Name:               m.Name,
					Roles:              m.Roles,
					InputPerMillion:    m.InputPerMillion,
					OutputPerMillion:   m.OutputPerMillion,
					ContextWindow:      m.ContextWindow,
					TextToolCalls:      m.TextToolCalls,
					FoldSystemIntoUser: m.FoldSystemIntoUser,
					SystemPrefix:       m.SystemPrefix,
					ToolAllowlist:      m.ToolAllowlist,
				})
				for _, r := range m.Roles {
					roleSet[r] = true
				}
			}
			roles := make([]string, 0, len(roleSet))
			for r := range roleSet {
				roles = append(roles, r)
			}
			toSeed = append(toSeed, &db.Provider{
				Name:      pcfg.Name,
				Type:      pcfg.Type,
				BaseURL:   pcfg.BaseURL,
				APIKey:    pcfg.APIKey,
				ModelName: pcfg.DefaultModel(),
				Enabled:   true,
				Roles:     roles,
				Models:    models,
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

	// Seed rich role definitions from config (preferred) on first run.
	if count, _ := database.CountRoleDefinitions(startCtx); count == 0 && len(cfg.RoleDefinitions) > 0 {
		dbProvs, _ := database.ListProviders(startCtx)
		provByName := make(map[string]*db.Provider, len(dbProvs))
		for _, p := range dbProvs {
			provByName[p.Name] = p
		}
		var rolesToSeed []*db.RoleDefinition
		for _, rc := range cfg.RoleDefinitions {
			label := rc.Label
			if label == "" && rc.Name != "" {
				label = strings.ToUpper(rc.Name[:1]) + rc.Name[1:]
			}
			temp := rc.Temperature
			if temp == 0 {
				temp = 0.7
			}
			maxTok := rc.MaxTokens
			if maxTok == 0 {
				maxTok = 4096
			}
			rd := &db.RoleDefinition{
				Name:           rc.Name,
				Label:          label,
				Description:    rc.Description,
				Capabilities:   rc.Capabilities,
				AllowedTools:   rc.AllowedTools,
				ContextInclude: rc.ContextInclude,
				ContextExclude: rc.ContextExclude,
				SystemPrompt:   rc.SystemPrompt,
				ModelOverride:  rc.ModelOverride,
				Temperature:    temp,
				MaxTokens:      maxTok,
				ResyncPrompt:   rc.ResyncPrompt,
				Enabled:        true,
			}
			if prov, ok := provByName[rc.Provider]; ok {
				rd.ProviderID = prov.ID
			}
			rolesToSeed = append(rolesToSeed, rd)
		}
		if n, serr := database.SeedRoleDefinitions(startCtx, rolesToSeed); serr != nil {
			log.Printf("warning: seed role definitions: %v", serr)
		} else if n > 0 {
			log.Printf("seeded %d role definition(s) from config", n)
		}
	}

	// Seed the built-in seven-role taxonomy on a fresh DB that has no roles yet
	// (re-read the count so this runs only when config seeding did not populate
	// roles above). Existing/config-driven role sets are left untouched.
	if count, _ := database.CountRoleDefinitions(startCtx); count == 0 {
		if n, serr := database.SeedRoleDefinitions(startCtx, db.DefaultRoleDefinitions()); serr != nil {
			log.Printf("warning: seed default role definitions: %v", serr)
		} else if n > 0 {
			log.Printf("seeded %d default role definition(s)", n)
		}
	}

	// Seed task types from config (preferred) on first run, else the built-in set.
	if count, _ := database.CountTaskTypes(startCtx); count == 0 && len(cfg.TaskTypes) > 0 {
		var toSeed []*db.TaskType
		for i, tc := range cfg.TaskTypes {
			toSeed = append(toSeed, &db.TaskType{
				Key:            tc.Key,
				Label:          tc.Label,
				BranchTemplate: tc.BranchTemplate,
				IsDefault:      tc.Default,
				SortOrder:      i,
			})
		}
		if n, serr := database.SeedTaskTypes(startCtx, toSeed); serr != nil {
			log.Printf("warning: seed task types: %v", serr)
		} else if n > 0 {
			log.Printf("seeded %d task type(s) from config", n)
		}
	}
	if count, _ := database.CountTaskTypes(startCtx); count == 0 {
		if n, serr := database.SeedTaskTypes(startCtx, db.DefaultTaskTypes()); serr != nil {
			log.Printf("warning: seed default task types: %v", serr)
		} else if n > 0 {
			log.Printf("seeded %d default task type(s)", n)
		}
	}

	// Seed persona skills (Feature 6): from config when provided, else the built-in
	// starter set (idempotent by name).
	skillsToSeed, skillSrc := db.DefaultSkillDefinitions(), "default"
	if len(cfg.Skills) > 0 {
		skillsToSeed, skillSrc = configSkillDefinitions(cfg.Skills), "config"
	}
	if n, serr := database.SeedSkillDefinitions(startCtx, skillsToSeed); serr != nil {
		log.Printf("warning: seed skill definitions: %v", serr)
	} else if n > 0 {
		log.Printf("seeded %d skill definition(s) from %s", n, skillSrc)
	}

	// Seed subagent skills: from config when provided, else the built-in set
	// (idempotent by name).
	subToSeed, subSrc := db.DefaultSubagentSkills(), "default"
	if len(cfg.SubagentSkills) > 0 {
		subToSeed, subSrc = configSubagentSkills(cfg.SubagentSkills), "config"
	}
	if n, serr := database.SeedSubagentSkills(startCtx, subToSeed); serr != nil {
		log.Printf("warning: seed subagent skills: %v", serr)
	} else if n > 0 {
		log.Printf("seeded %d subagent skill(s) from %s", n, subSrc)
	}

	// Seed agent templates from config on first run (idempotent by name).
	if count, _ := database.CountAgentTemplates(startCtx); count == 0 && len(cfg.AgentTemplates) > 0 {
		var tplsToSeed []*db.AgentTemplate
		for _, tc := range cfg.AgentTemplates {
			replicas := tc.Replicas
			if replicas == 0 {
				replicas = 1
			}
			tplsToSeed = append(tplsToSeed, &db.AgentTemplate{
				Name:      tc.Name,
				Roles:     tc.Roles,
				Skills:    tc.Skills,
				Replicas:  replicas,
				Autostart: tc.Autostart,
				Enabled:   true,
			})
		}
		if n, serr := database.SeedAgentTemplates(startCtx, tplsToSeed); serr != nil {
			log.Printf("warning: seed agent templates: %v", serr)
		} else if n > 0 {
			log.Printf("seeded %d agent template(s) from config", n)
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
	skillsStr := fs.String("skills", "", "comma-separated skills, e.g. backend,go (optional)")
	serverURL := fs.String("server", "http://localhost:8080", "orchestrator server URL")
	workdir := fs.String("workdir", "", "local workspace root for agent code checkouts (default: .agent-work)")
	mode := fs.String("mode", "colocated", `agent mode: "colocated" (server provisions worktrees) or "remote" (agent clones via git)`)
	serverOverride := fs.Bool("server-override", false, "fetch providers/roles from the server even when --config is set (in-memory only)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *rolesStr == "" {
		return fmt.Errorf("--roles is required")
	}

	a, cleanup, cfg, err := buildAgent(*name, parseRoles(*rolesStr), parseRoles(*skillsStr), *serverURL, *configPath, *workdir, *mode, *serverOverride)
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
func buildAgent(name string, roles, skills []string, serverURL, configPath, workdir, mode string, serverOverride bool) (*agent.Agent, func(), *config.Config, error) {
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
	if len(skills) > 0 {
		a.WithSkills(skills)
	}
	if wd != "" {
		a.WithWorkdir(wd)
	}

	// Resolve connection settings uniformly for all agents: env first (the
	// supervisor injects these into co-located children), then config. The agent
	// never opens the database — providers/roles come over HTTP.
	token := os.Getenv("AGENT_AUTH_TOKEN")
	if token == "" {
		token = cfg.Agents.APIKey
	}
	if envURL := os.Getenv("AGENT_SERVER_URL"); envURL != "" {
		serverURL = envURL
	}
	caPath := os.Getenv("AGENT_SERVER_CA")
	if caPath == "" {
		caPath = cfg.Agents.ServerCA
	}
	insecureTLS := os.Getenv("AGENT_TLS_INSECURE") == "true" || cfg.Agents.TLSInsecure
	tlsCfg, err := agentTLSConfig(caPath, insecureTLS)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("agent %q: TLS config: %w", name, err)
	}
	client := agent.NewServerClientWithAuth(serverURL, token, tlsCfg)
	a.WithClient(client)
	// Make git clone/push over HTTPS trust the same cert as the API client.
	agent.InstallGitTLS(tlsCfg)

	llmReg := llm.NewRegistry()
	rtr := router.New(cfg, llmReg)

	// syncFromServer (re)builds the registry + router from the server's providers
	// (with keys) and role definitions, over HTTP. Used at startup and on reload.
	syncFromServer := func() error {
		provs, perr := client.ListProvidersWithKeys(context.Background())
		if perr != nil {
			return perr
		}
		roleDefs, perr := client.ListRoleDefinitions(context.Background())
		if perr != nil {
			return perr
		}
		seen := make(map[string]bool, len(provs))
		for _, p := range provs {
			if !p.Enabled {
				continue
			}
			prov, e := llm.NewFromSpec(p.Name, p.Type, p.BaseURL, p.APIKey, p.ModelName, p.Config)
			if e != nil {
				log.Printf("agent %q: init provider %q: %v", name, p.Name, e)
				continue
			}
			llmReg.Set(p.Name, prov)
			if len(p.Roles) > 0 {
				llmReg.SetRoles(p.Name, p.ModelName, p.Roles)
			}
			seen[p.Name] = true
		}
		for _, existing := range llmReg.List() {
			if !seen[existing] {
				llmReg.Remove(existing)
			}
		}
		// Wrap providers with retry/circuit-breaker/failover using the
		// UI-configurable resilience settings (best effort: defaults on error).
		if settings, serr := client.ListSettings(context.Background()); serr == nil {
			llmReg.WrapResilience(agent.ResilienceFromSettings(settings))
		} else {
			log.Printf("agent %q: could not load resilience settings (%v); using defaults", name, serr)
			llmReg.WrapResilience(agent.ResilienceFromSettings(nil))
		}
		return rtr.LoadFromData(provs, roleDefs)
	}

	useServer := configPath == "" || serverOverride
	if useServer {
		if err := syncFromServer(); err != nil {
			// A 401 with no token means the server requires an agent API key that
			// this agent doesn't have — abort with a clear message rather than
			// silently running unauthenticated.
			if token == "" && strings.Contains(err.Error(), "401") {
				return nil, nil, nil, fmt.Errorf("agent %q: server requires an agent API key — set AGENT_AUTH_TOKEN or agents.api_key: %w", name, err)
			}
			// Other errors (server starting, transient): wire the executor anyway
			// and let the reload recover; the poll loop pauses until a provider is
			// available.
			log.Printf("agent %q: initial provider sync failed (%v) — will retry on reload", name, err)
		}
		a.WithReload(func() {
			if err := syncFromServer(); err != nil {
				log.Printf("agent %q: provider reload failed: %v", name, err)
			}
		})
	} else {
		// Config-driven providers/roles (no server override). Role routing relies
		// on the provider role preferences declared in config.
		if initErr := llmReg.InitFromConfig(cfg); initErr != nil {
			return nil, nil, nil, fmt.Errorf("agent %q: init LLM providers from config: %w", name, initErr)
		}
		cfgProvs := configProvidersToDB(cfg)
		for _, p := range cfgProvs {
			if len(p.Roles) > 0 {
				llmReg.SetRoles(p.Name, p.ModelName, p.Roles)
			}
		}
		_ = rtr.LoadFromData(cfgProvs, nil)
	}

	// Tools reach the server over HTTP via the agent's client (tools.ToolBackend);
	// they never touch the database.
	backend := a.Client()
	toolReg := tools.NewRegistry()
	_ = tools.RegisterCodeTools(toolReg)
	_ = tools.RegisterTaskTools(toolReg, backend)
	_ = tools.RegisterPlanTools(toolReg, backend)
	_ = tools.RegisterContextTools(toolReg, backend)
	_ = tools.RegisterCommentTools(toolReg, backend)
	_ = tools.RegisterSubagentTool(toolReg)
	_ = tools.RegisterSessionTool(toolReg)
	a.WithExecutor(rtr, toolReg)

	log.Printf("agent %q: executor wired (LLM providers: %d)", name, len(llmReg.List()))
	for _, role := range roles {
		if route, rerr := rtr.RouteByRole(role); rerr != nil {
			log.Printf("agent %q: WARNING role %q has no route — tasks for this role will fail until a provider is available (%v)", name, role, rerr)
		} else {
			log.Printf("agent %q: role %q → provider=%q model=%q", name, role, route.Provider.Name(), route.Model)
		}
	}

	cleanup := func() { llmReg.CloseAll() }
	return a, cleanup, cfg, nil
}

// agentTLSConfig builds the TLS client config for the agent. insecure skips
// verification (dev only); caPath pins a CA certificate (for self-signed servers
// reached by remote agents). When neither is set, system roots are used.
func agentTLSConfig(caPath string, insecure bool) (*tls.Config, error) {
	if insecure {
		return &tls.Config{InsecureSkipVerify: true}, nil
	}
	if caPath == "" {
		return nil, nil
	}
	// Layer the server CA on top of system roots so external HTTPS (e.g. upstream
	// git remotes) still verifies while the self-signed server cert is trusted.
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA %q: %w", caPath, err)
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA %q: no certificates found", caPath)
	}
	return &tls.Config{RootCAs: pool}, nil
}

// configProvidersToDB converts config-file providers into db.Provider values so
// the router can register their model-level role preferences (config-driven
// agents route via provider role preferences, with no DB-backed role defs).
func configProvidersToDB(cfg *config.Config) []*db.Provider {
	out := make([]*db.Provider, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		var models []db.ProviderModel
		roleSet := map[string]bool{}
		for _, m := range p.Models {
			models = append(models, db.ProviderModel{Name: m.Name, Roles: m.Roles})
			for _, r := range m.Roles {
				roleSet[r] = true
			}
		}
		roles := make([]string, 0, len(roleSet))
		for r := range roleSet {
			roles = append(roles, r)
		}
		out = append(out, &db.Provider{
			Name:      p.Name,
			Type:      p.Type,
			BaseURL:   p.BaseURL,
			ModelName: p.DefaultModel(),
			Roles:     roles,
			Models:    models,
			Enabled:   true,
		})
	}
	return out
}

// configSkillDefinitions converts config skill entries into db.SkillDefinition
// seed values (mirrors the role/task-type config→db conversion).
func configSkillDefinitions(skills []config.SkillConfig) []*db.SkillDefinition {
	out := make([]*db.SkillDefinition, 0, len(skills))
	for _, s := range skills {
		out = append(out, &db.SkillDefinition{
			Name:           s.Name,
			Label:          s.Label,
			Description:    s.Description,
			PromptFragment: s.PromptFragment,
			ContextInclude: s.ContextInclude,
			ContextExclude: s.ContextExclude,
			AllowedTools:   s.AllowedTools,
			Enabled:        true,
		})
	}
	return out
}

// configSubagentSkills converts config subagent-skill entries into
// db.SubagentSkill seed values.
func configSubagentSkills(skills []config.SubagentSkillConfig) []*db.SubagentSkill {
	out := make([]*db.SubagentSkill, 0, len(skills))
	for _, s := range skills {
		out = append(out, &db.SubagentSkill{
			Name:           s.Name,
			Label:          s.Label,
			Description:    s.Description,
			PromptTemplate: s.PromptTemplate,
			ToolAllowlist:  s.ToolAllowlist,
			ContextInclude: s.ContextInclude,
			ContextExclude: s.ContextExclude,
			MaxRounds:      s.MaxRounds,
			MaxTokens:      s.MaxTokens,
			Enabled:        true,
		})
	}
	return out
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
		a, cleanup, _, err := buildAgent(def.Name, def.Roles, def.Skills, serverURL, agentConfig, workdir, defMode, false)
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
