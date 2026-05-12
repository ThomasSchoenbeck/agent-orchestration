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
	def          *db.RoleDefinition
	providerName string
	defaultModel string // provider's default model (fallback when ModelOverride is "")
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
	Provider     llm.LLMProvider
	Model        string // resolved model identifier sent to the provider
	Role         string // resolved role
	SystemPrompt string // filled system prompt (may be empty)
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
		}
		r.rolesByName[role.Name] = cr
		for _, tt := range role.TaskTypes {
			r.rolesByTaskType[tt] = cr
		}
	}

	log.Printf("router: loaded %d role definition(s) from database", len(roles))
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
	if !ok {
		return nil, fmt.Errorf("no model mapped to role %q in config", role)
	}
	return r.resolveModel(role, modelName)
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

// BuildContext assembles a context string for the given role.
func (r *Router) BuildContext(role string, entries []ContextEntry) string {
	return r.ctxBuild.Build(role, entries)
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
		return nil, fmt.Errorf("role %q: %w", def.Name, err)
	}
	model := def.ModelOverride
	if model == "" {
		model = cr.defaultModel
	}
	return &RouteResult{
		Provider:     prov,
		Model:        model,
		Role:         def.Name,
		SystemPrompt: def.SystemPrompt,
	}, nil
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
