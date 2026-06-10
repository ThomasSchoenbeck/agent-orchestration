// Package router implements config-driven routing of tasks to LLM providers.
// It resolves: role → model name → provider config.
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
	ctxBuild *ContextBuilder

	mu          sync.RWMutex
	rolesByName map[string]*cachedRole
	rolesByID   map[string]*cachedRole // Phase 2: resolve a role ref by id too
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
		cfg:         cfg,
		registry:    registry,
		ctxBuild:    NewContextBuilder(cfg),
		rolesByName: make(map[string]*cachedRole),
		rolesByID:   make(map[string]*cachedRole),
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
	Capabilities       []string           // role capabilities (e.g. creates_tasks, handles_merge)
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
	roles, err := database.ListRoleDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("router.LoadFromDB: list roles: %w", err)
	}
	return r.LoadFromData(providers, roles)
}

// LoadFromData populates the in-memory role cache from already-fetched providers
// and role definitions, without touching the database. Used by agents that
// receive this data over HTTP. Disabled roles are skipped.
func (r *Router) LoadFromData(providers []*db.Provider, roles []*db.RoleDefinition) error {
	provByID := make(map[string]*db.Provider, len(providers))
	for _, p := range providers {
		provByID[p.ID] = p
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.rolesByName = make(map[string]*cachedRole)
	r.rolesByID = make(map[string]*cachedRole)

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
		if role.ID != "" {
			r.rolesByID[role.ID] = cr
		}
	}

	// Register model-level role entries in the registry for each provider.
	for _, prov := range providers {
		if len(prov.Models) > 0 {
			r.registry.SetModelRoles(prov.Name, prov.Models)
		}
	}

	log.Printf("router: loaded %d role definition(s)", len(roles))
	for name, cr := range r.rolesByName {
		log.Printf("router: role %q → provider=%q model=%q",
			name, cr.providerName, cr.def.ModelOverride)
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

// ReloadFromData refreshes the in-memory cache from already-fetched providers
// and role definitions (the agent's HTTP reload path). Errors are logged, not
// returned, mirroring ReloadFromDB.
func (r *Router) ReloadFromData(providers []*db.Provider, roles []*db.RoleDefinition) {
	if err := r.LoadFromData(providers, roles); err != nil {
		log.Printf("router: ReloadFromData: %v", err)
	}
}

// RouteByRole resolves a role directly to a provider + model.
// DB-backed definitions take precedence over config.
func (r *Router) RouteByRole(role string) (*RouteResult, error) {
	r.mu.RLock()
	cr, ok := r.rolesByName[role]
	if !ok {
		cr, ok = r.rolesByID[role] // Phase 2: accept a role ref by id too
	}
	r.mu.RUnlock()

	if ok {
		if res, err := r.routeFromCache(cr); err == nil {
			return res, nil
		}
		// The DB role definition exists but could not resolve a provider
		// (e.g. no ProviderID assigned to the role). Fall through to the
		// fallbacks below — a provider may still declare support for this
		// role via its roles/model list.
	}

	// Provider role-preference fallback: find any registered provider that
	// declares support for this role via its provider-level or per-model roles.
	if prov, model, err := r.registry.GetForRole(role); err == nil {
		res := &RouteResult{
			Provider: prov,
			Model:    model,
			Role:     role,
		}
		// Preserve the role definition's persona and tool allowlist when one
		// exists, even though it carried no provider binding.
		if ok && cr.def != nil {
			res.SystemPrompt = cr.def.SystemPrompt
			res.Capabilities = cr.def.Capabilities
			if len(cr.def.AllowedTools) > 0 {
				res.ToolAllowlist = cr.def.AllowedTools
			}
		}
		return res, nil
	}

	return nil, fmt.Errorf("no provider available for role %q (no DB definition, config mapping, or provider preference)", role)
}

// RoleName resolves a role ref (id or name) to its human-readable role name,
// falling back to the ref itself when no definition matches. Used for log output
// so agents report role names instead of opaque role ids.
func (r *Router) RoleName(ref string) string {
	if ref == "" {
		return ref
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if cr, ok := r.rolesByID[ref]; ok && cr.def != nil && cr.def.Name != "" {
		return cr.def.Name
	}
	if cr, ok := r.rolesByName[ref]; ok && cr.def != nil && cr.def.Name != "" {
		return cr.def.Name
	}
	return ref
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
		Capabilities:       def.Capabilities,
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

