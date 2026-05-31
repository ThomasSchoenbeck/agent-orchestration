// Package router implements config-driven routing of tasks to LLM providers.
// It resolves: task_type → role → model name → provider config.
// When role definitions are loaded from the database they take precedence over
// the static config, allowing live updates without a server restart.
package router

import (
	"context"
	"fmt"
	"log"
	"sync"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// Router resolves which LLM provider and model to use for a given role or
// task type, and assembles the system prompt + context for that invocation.
type Router struct {
	cfg      *config.Config
	registry *llm.Registry
	prompter *Prompter
	ctxBuild *ContextBuilder

	mu              sync.RWMutex
	rolesByName     map[string]*cachedRole
	rolesByTaskType map[string]*cachedRole
}

// cachedRole holds a DB-backed role definition and the resolved provider name.
type cachedRole struct {
	def                  *db.RoleDefinition
	providerName         string
	defaultModel         string             // provider's default model (fallback when ModelOverride is "")
	providerModels       []db.ProviderModel // per-model role and pricing config from the provider
	// Provider-level behavioral defaults (from prov.Config).
	textToolCalls        bool
	foldSystemIntoUser   bool
	systemPrefix         string
	providerToolAllowlist []string
	// Role-level allowlist (from RoleDefinition.AllowedTools); highest priority.
	roleToolAllowlist    []string
}

// New creates a Router from the loaded config and provider registry.
func New(cfg *config.Config, registry *llm.Registry) *Router {
	return &Router{
		cfg:             cfg,
		registry:        registry,
		prompter:        NewPrompter(cfg),
		ctxBuild:        NewContextBuilder(cfg),
		rolesByName:     make(map[string]*cachedRole),
		rolesByTaskType: make(map[string]*cachedRole),
	}
}

// RouteResult carries everything needed to execute a chat call.
type RouteResult struct {
	Provider           llm.LLMProvider
	Model              string           // resolved model identifier sent to the provider
	Role               string           // resolved role
	SystemPrompt       string           // filled system prompt (may be empty)
	TextToolCalls      bool
	FoldSystemIntoUser bool
	SystemPrefix       string
	ToolAllowlist      []string
	ProviderModels     []db.ProviderModel // for cost calculation; may be empty
}

// LoadFromDB populates the in-memory role cache from the database.
// Disabled roles are skipped. If the table is empty the router falls back to
// config-driven behaviour automatically.
func (r *Router) LoadFromDB(database *db.Database) error {
	ctx := context.Background()

	providers, err := database.ListProviders(ctx)
	if err != nil {
		return fmt.Errorf("router.LoadFromDB: list providers: %w", err)
	}
	provByID := make(map[string]*db.Provider, len(providers))
	for _, p := range providers {
		provByID[p.ID] = p
	}

	roles, err := database.ListRoleDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("router.LoadFromDB: list roles: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.rolesByName = make(map[string]*cachedRole)
	r.rolesByTaskType = make(map[string]*cachedRole)

	for _, role := range roles {
		if !role.Enabled {
			continue
		}
		cr := &cachedRole{def: role}
		if prov, ok := provByID[role.ProviderID]; ok {
			cr.providerName = prov.Name
			cr.defaultModel = prov.ModelName
			cr.providerModels = prov.Models
			if v, ok := prov.Config["text_tool_calls"]; ok {
				cr.textToolCalls, _ = v.(bool)
			}
			if v, ok := prov.Config["fold_system_into_user"]; ok {
				cr.foldSystemIntoUser, _ = v.(bool)
			}
			if v, ok := prov.Config["system_prefix"]; ok {
				cr.systemPrefix, _ = v.(string)
			}
			// Provider-level allowlist stored separately so model-level can sit
			// between it and the role-level override in priority.
			if v, ok := prov.Config["tool_allowlist"]; ok {
				if raw, ok := v.([]interface{}); ok {
					for _, item := range raw {
						if s, ok := item.(string); ok {
							cr.providerToolAllowlist = append(cr.providerToolAllowlist, s)
						}
					}
				}
			}
		}
		// Role-level allowlist (highest priority — stored separately).
		if len(role.AllowedTools) > 0 {
			cr.roleToolAllowlist = role.AllowedTools
		}
		r.rolesByName[role.Name] = cr
		for _, tt := range role.TaskTypes {
			r.rolesByTaskType[tt] = cr
		}
	}

	// Register model-level role entries in the registry for each provider.
	for _, prov := range providers {
		if len(prov.Models) > 0 {
			r.registry.SetModelRoles(prov.Name, prov.Models)
		}
	}

	log.Printf("router: loaded %d role definition(s) from database", len(roles))
	for name, cr := range r.rolesByName {
		log.Printf("router: role %q → provider=%q model=%q task_types=%v",
			name, cr.providerName, cr.def.ModelOverride, cr.def.TaskTypes)
	}
	return nil
}

// ReloadFromDB is identical to LoadFromDB and can be called after any role
// create/update/delete to refresh the in-memory cache.
func (r *Router) ReloadFromDB(database *db.Database) {
	if err := r.LoadFromDB(database); err != nil {
		log.Printf("router: ReloadFromDB: %v", err)
	}
}

// RouteByRole resolves a role directly to a provider + model.
// DB-backed definitions take precedence over config.
func (r *Router) RouteByRole(role string) (*RouteResult, error) {
	r.mu.RLock()
	cr, ok := r.rolesByName[role]
	r.mu.RUnlock()

	if ok {
		return r.routeFromCache(cr)
	}

	// Config fallback.
	modelName, ok := r.cfg.Roles[role]
	if ok {
		return r.resolveModel(role, modelName)
	}

	// Provider role-preference fallback: find any registered provider that
	// declares support for this role via its roles list.
	if prov, model, err := r.registry.GetForRole(role); err == nil {
		return &RouteResult{
			Provider: prov,
			Model:    model,
			Role:     role,
		}, nil
	}

	return nil, fmt.Errorf("no provider available for role %q (no DB definition, config mapping, or provider preference)", role)
}

// RouteByTaskType resolves a task type → role → provider + model.
// DB-backed definitions take precedence over config.
func (r *Router) RouteByTaskType(taskType string) (*RouteResult, error) {
	r.mu.RLock()
	cr, ok := r.rolesByTaskType[taskType]
	r.mu.RUnlock()

	if ok {
		return r.routeFromCache(cr)
	}

	// Config fallback.
	role, ok := r.cfg.Routing[taskType]
	if !ok {
		// Treat task type as role directly.
		role = taskType
	}
	return r.RouteByRole(role)
}

// BuildPrompt fills in a prompt template for the given task type and variables.
func (r *Router) BuildPrompt(taskType string, vars map[string]interface{}) string {
	return r.prompter.Render(taskType, vars)
}

// BuildContext assembles a context string for the given role (config-backed fallback).
// Deprecated: use BuildContextForRole instead for DB-backed rules.
func (r *Router) BuildContext(role string, entries []ContextEntry) string {
	return r.ctxBuild.Build(role, entries)
}

// BuildContextForRole assembles a context string using a DB-backed role definition's
// include/exclude rules.
func (r *Router) BuildContextForRole(role *db.RoleDefinition, entries []ContextEntry) string {
	return r.ctxBuild.BuildWithRules(entries, role.ContextInclude, role.ContextExclude)
}

// RoleForTaskType returns the role string for a task type (without fully routing).
func (r *Router) RoleForTaskType(taskType string) string {
	r.mu.RLock()
	if cr, ok := r.rolesByTaskType[taskType]; ok {
		r.mu.RUnlock()
		return cr.def.Name
	}
	r.mu.RUnlock()

	if role, ok := r.cfg.Routing[taskType]; ok {
		return role
	}
	return taskType
}

// routeFromCache builds a RouteResult from a cached DB-backed role definition.
func (r *Router) routeFromCache(cr *cachedRole) (*RouteResult, error) {
	def := cr.def
	if cr.providerName == "" {
		return nil, fmt.Errorf("role %q has no provider configured", def.Name)
	}
	prov, err := r.registry.Get(cr.providerName)
	if err != nil {
		return nil, fmt.Errorf("role %q: provider %q not in registry (is the provider enabled?): %w",
			def.Name, cr.providerName, err)
	}

	// Resolution order:
	// 1. Explicit ModelOverride on the role definition (highest priority).
	// 2. First model in the provider's model list whose Roles contain this role.
	// 3. Provider's top-level default model.
	model := def.ModelOverride
	if model == "" {
		model = modelForRole(cr.providerModels, def.Name)
	}
	if model == "" {
		model = cr.defaultModel
	}

	// Resolve behavioral settings with priority: role > model > provider.
	textToolCalls := cr.textToolCalls
	foldSystemIntoUser := cr.foldSystemIntoUser
	systemPrefix := cr.systemPrefix
	toolAllowlist := cr.providerToolAllowlist

	// Model-level: override provider defaults for the matched model.
	for _, m := range cr.providerModels {
		if m.Name == model {
			if m.TextToolCalls {
				textToolCalls = true
			}
			if m.FoldSystemIntoUser {
				foldSystemIntoUser = true
			}
			if m.SystemPrefix != "" {
				systemPrefix = m.SystemPrefix
			}
			if len(m.ToolAllowlist) > 0 {
				toolAllowlist = m.ToolAllowlist
			}
			break
		}
	}

	// Role-level allowlist wins over everything.
	if len(cr.roleToolAllowlist) > 0 {
		toolAllowlist = cr.roleToolAllowlist
	}

	return &RouteResult{
		Provider:           prov,
		Model:              model,
		Role:               def.Name,
		SystemPrompt:       def.SystemPrompt,
		TextToolCalls:      textToolCalls,
		FoldSystemIntoUser: foldSystemIntoUser,
		SystemPrefix:       systemPrefix,
		ToolAllowlist:      toolAllowlist,
		ProviderModels:     cr.providerModels,
	}, nil
}

// modelForRole returns the name of the first model in models whose Roles
// list contains role, or "" if none match.
func modelForRole(models []db.ProviderModel, role string) string {
	for _, m := range models {
		for _, r := range m.Roles {
			if r == role {
				return m.Name
			}
		}
	}
	return ""
}

// resolveModel looks up a model by name and returns its provider.
func (r *Router) resolveModel(role, modelName string) (*RouteResult, error) {
	model, err := r.cfg.ModelByName(modelName)
	if err != nil {
		return nil, fmt.Errorf("role %q: %w", role, err)
	}
	provider, err := r.registry.Get(model.Provider)
	if err != nil {
		return nil, fmt.Errorf("role %q model %q: provider %q not in registry: %w",
			role, modelName, model.Provider, err)
	}
	return &RouteResult{
		Provider: provider,
		Model:    model.Model,
		Role:     role,
	}, nil
}
