// Package router implements config-driven routing of tasks to LLM providers.
// It resolves: task_type → role → model name → provider config.
package router

import (
	"fmt"

	"agent-orchestrator/config"
	"agent-orchestrator/llm"
)

// Router resolves which LLM provider and model to use for a given role or
// task type, and assembles the system prompt + context for that invocation.
type Router struct {
	cfg      *config.Config
	registry *llm.Registry
	prompter *Prompter
	ctxBuild *ContextBuilder
}

// New creates a Router from the loaded config and provider registry.
func New(cfg *config.Config, registry *llm.Registry) *Router {
	return &Router{
		cfg:      cfg,
		registry: registry,
		prompter: NewPrompter(cfg),
		ctxBuild: NewContextBuilder(cfg),
	}
}

// RouteResult carries everything needed to execute a chat call.
type RouteResult struct {
	Provider     llm.LLMProvider
	Model        string        // resolved model identifier sent to the provider
	Role         string        // resolved role
	SystemPrompt string        // filled system prompt (may be empty)
}

// RouteByRole resolves a role directly to a provider + model.
func (r *Router) RouteByRole(role string) (*RouteResult, error) {
	modelName, ok := r.cfg.Roles[role]
	if !ok {
		return nil, fmt.Errorf("no model mapped to role %q in config", role)
	}
	return r.resolveModel(role, modelName)
}

// RouteByTaskType resolves a task type → role → provider + model.
func (r *Router) RouteByTaskType(taskType string) (*RouteResult, error) {
	role, ok := r.cfg.Routing[taskType]
	if !ok {
		// Fall back: treat task type as role directly.
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
	if role, ok := r.cfg.Routing[taskType]; ok {
		return role
	}
	return taskType
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
